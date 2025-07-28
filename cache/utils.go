// Package cache provides utility functions for cache management, TTL handling, and cleanup operations.
package cache

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

/*
=============================================================================
CACHE UTILITIES LOCKING DOCUMENTATION
=============================================================================
*/

// Environment-configurable cache bounds
var (
	MaxTenants           = config.MaxTenants
	MaxMemoryMB          = config.MaxMemoryMB
	MaxSessionsPerTenant = config.MaxSessionsPerTenant
)

type DatabasePoolCleanupFunc func()

// Global reference to database pool cleanup function
var databasePoolCleanup DatabasePoolCleanupFunc

// TTL constants for different cache types
var (
	ContentCacheTTL       = config.ContentCacheTTL
	UserStateTTL          = config.UserStateTTL
	HTMLChunkTTL          = config.HTMLChunkTTL
	AnalyticsBinTTL       = config.AnalyticsBinTTL
	CurrentHourTTL        = config.CurrentHourTTL
	LeadMetricsTTL        = config.LeadMetricsTTL
	DashboardTTL          = config.DashboardTTL
	CleanupInterval       = config.CleanupInterval
	TenantTimeout         = config.TenantTimeout
	SSECleanupInterval    = config.SSECleanupInterval
	DBPoolCleanupInterval = config.DBPoolCleanupInterval
)

// CacheLock provides cache management for cache operations
type CacheLock struct {
	mu        sync.Mutex
	locks     map[string]*sync.Mutex
	lockTimes map[string]time.Time
}

// NewCacheLock creates a new cache lock manager
func NewCacheLock() *CacheLock {
	cl := &CacheLock{
		locks:     make(map[string]*sync.Mutex),
		lockTimes: make(map[string]time.Time),
	}

	// Launch a background goroutine to periodically clean up stale locks.
	go func() {
		ticker := time.NewTicker(10 * time.Minute) // Check for stale locks every 10 minutes
		defer ticker.Stop()

		for range ticker.C {
			cl.mu.Lock()
			now := time.Now().UTC()
			// Define a stale threshold, e.g., a lock held for more than 5 minutes is considered stale.
			staleThreshold := 5 * time.Minute

			for key, lockTime := range cl.lockTimes {
				if now.Sub(lockTime) > staleThreshold {
					log.Printf("WARN: Stale lock detected for key '%s'. Held for over %v. Forcibly removing.", key, staleThreshold)
					delete(cl.locks, key)
					delete(cl.lockTimes, key)
				}
			}
			cl.mu.Unlock()
		}
	}()

	return cl
}

// Lock acquires a lock for the given cache key
func (cl *CacheLock) Lock(key string) {
	cl.mu.Lock()
	if _, exists := cl.locks[key]; !exists {
		cl.locks[key] = &sync.Mutex{}
	}
	lock := cl.locks[key]
	cl.lockTimes[key] = time.Now().UTC()
	cl.mu.Unlock()

	lock.Lock()
}

// Unlock releases a lock for the given cache key
func (cl *CacheLock) Unlock(key string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if lock, exists := cl.locks[key]; exists {
		lock.Unlock()
		delete(cl.locks, key)
		delete(cl.lockTimes, key)
	}
}

// GetLockInfo returns information about current locks
func (cl *CacheLock) GetLockInfo() map[string]time.Duration {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	info := make(map[string]time.Duration)
	now := time.Now().UTC()
	for key, lockTime := range cl.lockTimes {
		info[key] = now.Sub(lockTime)
	}
	return info
}

// IsExpired checks if a cached item has exceeded its TTL
func IsExpired(cachedAt time.Time, ttl time.Duration) bool {
	return time.Since(cachedAt) > ttl
}

// IsAnalyticsBinExpired checks if an analytics bin has expired using context-aware logic.
func IsAnalyticsBinExpired(bin *models.HourlyEpinetBin, hourKey string) bool {
	if bin == nil {
		return true
	}

	now := time.Now().UTC()
	currentHour := utils.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))

	var effectiveTTL time.Duration
	if hourKey == currentHour {
		effectiveTTL = CurrentHourTTL
	} else {
		effectiveTTL = AnalyticsBinTTL
	}

	if bin.TTL > effectiveTTL {
		effectiveTTL = bin.TTL
	}

	return IsExpired(bin.ComputedAt, effectiveTTL)
}

// IsContentCacheExpired checks if content cache has expired
func IsContentCacheExpired(cache *models.TenantContentCache) bool {
	if cache == nil {
		return true
	}
	return IsExpired(cache.LastUpdated, ContentCacheTTL)
}

// IsUserStateCacheExpired checks if user state cache has expired
func IsUserStateCacheExpired(cache *models.TenantUserStateCache) bool {
	if cache == nil {
		return true
	}
	return IsExpired(cache.LastLoaded, UserStateTTL)
}

// SetDatabasePoolCleanup sets the database pool cleanup function
func SetDatabasePoolCleanup(cleanup DatabasePoolCleanupFunc) {
	databasePoolCleanup = cleanup
}

// cleanupDatabasePools performs database connection pool cleanup
func cleanupDatabasePools() {
	if databasePoolCleanup != nil {
		databasePoolCleanup()
	}
}

// StartCleanupRoutine starts background goroutines for cache cleanup.
// It now accepts a context to allow for graceful shutdown.
func StartCleanupRoutine(ctx context.Context, manager *Manager) {
	// Goroutine for general cache entry cleanup
	go func() {
		ticker := time.NewTicker(CleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cleanupExpiredEntries(manager)
			case <-ctx.Done():
				log.Println("Shutting down cache cleanup routine.")
				return
			}
		}
	}()

	// Goroutine for SSE connection cleanup
	go func() {
		sseTicker := time.NewTicker(SSECleanupInterval)
		defer sseTicker.Stop()
		for {
			select {
			case <-sseTicker.C:
				cleanupSSEConnections()
			case <-ctx.Done():
				log.Println("Shutting down SSE cleanup routine.")
				return
			}
		}
	}()

	// Goroutine for database connection pool cleanup
	go func() {
		dbTicker := time.NewTicker(DBPoolCleanupInterval)
		defer dbTicker.Stop()
		for {
			select {
			case <-dbTicker.C:
				cleanupDatabasePools()
			case <-ctx.Done():
				log.Println("Shutting down database pool cleanup routine.")
				return
			}
		}
	}()
}

// cleanupExpiredEntries removes expired cache entries
func cleanupExpiredEntries(manager *Manager) {
	manager.Mu.RLock()
	tenantIDs := make([]string, 0, len(manager.ContentCache))
	for tenantID := range manager.ContentCache {
		tenantIDs = append(tenantIDs, tenantID)
	}
	manager.Mu.RUnlock()

	for _, tenantID := range tenantIDs {
		cleanupTenantExpiredEntries(manager, tenantID)
	}
}

// cleanupExpiredSessions removes expired sessions from user state cache
func cleanupExpiredSessions(manager *Manager, tenantID string) {
	manager.Mu.RLock()
	userCache, exists := manager.UserStateCache[tenantID]
	manager.Mu.RUnlock()

	if !exists || userCache == nil {
		return
	}

	userCache.Mu.Lock()
	defer userCache.Mu.Unlock()

	now := time.Now().UTC()
	for sessionID, session := range userCache.SessionStates {
		if session.IsExpired() || now.Sub(session.LastActivity) > UserStateTTL {
			delete(userCache.SessionStates, sessionID)
		}
	}
}

// cleanupTenantExpiredEntries cleans up expired entries for a specific tenant
func cleanupTenantExpiredEntries(manager *Manager, tenantID string) {
	manager.Mu.RLock()
	htmlCache, htmlExists := manager.HTMLChunkCache[tenantID]
	manager.Mu.RUnlock()
	if htmlExists && htmlCache != nil {
		htmlCache.Mu.Lock()
		for key, chunk := range htmlCache.Chunks {
			if IsExpired(chunk.CachedAt, HTMLChunkTTL) {
				delete(htmlCache.Chunks, key)
			}
		}
		htmlCache.Mu.Unlock()
	}

	cleanupExpiredSessions(manager, tenantID)

	manager.Mu.RLock()
	analyticsCache, analyticsExists := manager.AnalyticsCache[tenantID]
	manager.Mu.RUnlock()
	if analyticsExists && analyticsCache != nil {
		analyticsCache.Mu.Lock()

		for binKey, bin := range analyticsCache.EpinetBins {
			lastColonIndex := strings.LastIndex(binKey, ":")
			if lastColonIndex == -1 {
				continue
			}
			hourKey := binKey[lastColonIndex+1:]

			if IsAnalyticsBinExpired(bin, hourKey) {
				delete(analyticsCache.EpinetBins, binKey)
			}
		}

		for key, bin := range analyticsCache.ContentBins {
			if IsExpired(bin.ComputedAt, bin.TTL) {
				delete(analyticsCache.ContentBins, key)
			}
		}
		for key, bin := range analyticsCache.SiteBins {
			if IsExpired(bin.ComputedAt, bin.TTL) {
				delete(analyticsCache.SiteBins, key)
			}
		}
		if analyticsCache.LeadMetrics != nil && IsExpired(analyticsCache.LeadMetrics.ComputedAt, analyticsCache.LeadMetrics.TTL) {
			analyticsCache.LeadMetrics = nil
		}
		if analyticsCache.DashboardData != nil && IsExpired(analyticsCache.DashboardData.ComputedAt, analyticsCache.DashboardData.TTL) {
			analyticsCache.DashboardData = nil
		}
		analyticsCache.Mu.Unlock()
	}
}

// cleanupSSEConnections performs periodic cleanup of dead SSE connections
func cleanupSSEConnections() {
	tenantIDs := models.Broadcaster.GetActiveTenantIDs()
	totalCleaned := 0
	totalActive := 0
	for _, tenantID := range tenantIDs {
		deadChannels := models.Broadcaster.GetDeadChannelsForTenant(tenantID)
		if len(deadChannels) > 0 {
			models.Broadcaster.CleanupDeadChannels(tenantID, deadChannels)
			totalCleaned += len(deadChannels)
		}
		totalActive += models.Broadcaster.GetActiveConnectionCount(tenantID)
	}
	if totalCleaned > 0 {
		log.Printf("SSE cleanup: removed %d dead connections across all tenants, %d active remaining", totalCleaned, totalActive)
	}
}
