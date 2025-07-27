// Package cache provides utility functions for cache management, TTL handling, and cleanup operations.
package cache

import (
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
	return &CacheLock{
		locks:     make(map[string]*sync.Mutex),
		lockTimes: make(map[string]time.Time),
	}
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
	if lock, exists := cl.locks[key]; exists {
		lock.Unlock()
		delete(cl.lockTimes, key)
	}
	cl.mu.Unlock()
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
// LOCKING: None required (pure computation)
func IsExpired(cachedAt time.Time, ttl time.Duration) bool {
	return time.Since(cachedAt) > ttl
}

// IsAnalyticsBinExpired checks if an analytics bin has expired using context-aware logic.
// It correctly applies a shorter TTL for the current hour.
// LOCKING: None required (pure computation)
func IsAnalyticsBinExpired(bin *models.HourlyEpinetBin, hourKey string) bool {
	if bin == nil {
		return true
	}

	now := time.Now().UTC()
	currentHour := utils.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))

	// Determine the correct TTL based on whether the bin is for the current hour
	var effectiveTTL time.Duration
	if hourKey == currentHour {
		effectiveTTL = CurrentHourTTL
	} else {
		effectiveTTL = AnalyticsBinTTL
	}

	// Use the greater of the bin's own TTL and the dynamically determined TTL.
	// This respects the bin's intended TTL while ensuring the current hour expires quickly.
	if bin.TTL > effectiveTTL {
		effectiveTTL = bin.TTL
	}

	return IsExpired(bin.ComputedAt, effectiveTTL)
}

// IsContentCacheExpired checks if content cache has expired
// LOCKING: None required (pure computation)
func IsContentCacheExpired(cache *models.TenantContentCache) bool {
	if cache == nil {
		return true
	}
	return IsExpired(cache.LastUpdated, ContentCacheTTL)
}

// IsUserStateCacheExpired checks if user state cache has expired
// LOCKING: None required (pure computation)
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
func StartCleanupRoutine(manager *Manager) {
	go func() {
		ticker := time.NewTicker(CleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			cleanupExpiredEntries(manager)
		}
	}()
	go func() {
		sseTicker := time.NewTicker(SSECleanupInterval)
		defer sseTicker.Stop()
		for range sseTicker.C {
			cleanupSSEConnections()
		}
	}()
	go func() {
		dbTicker := time.NewTicker(DBPoolCleanupInterval)
		defer dbTicker.Stop()
		for range dbTicker.C {
			cleanupDatabasePools()
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
	// Clean up HTML chunks
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

		// Clean up epinet bins with corrected, context-aware logic
		for binKey, bin := range analyticsCache.EpinetBins {
			// CORRECT: Parse the hourKey from the binKey to provide context
			lastColonIndex := strings.LastIndex(binKey, ":")
			if lastColonIndex == -1 {
				continue // Skip malformed keys
			}
			hourKey := binKey[lastColonIndex+1:]

			if IsAnalyticsBinExpired(bin, hourKey) {
				delete(analyticsCache.EpinetBins, binKey)
			}
		}

		// Clean up other bins
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
