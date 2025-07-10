// Package cache provides utility functions for cache management, TTL handling, and cleanup operations.
package cache

import (
	"log"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/models"
)

/*
=============================================================================
CACHE UTILITIES LOCKING DOCUMENTATION
=============================================================================

This utilities file contains helper functions for cache management that MUST
follow the locking hierarchy established in manager.go:

LOCKING HIERARCHY:
1. Manager.Mu (highest priority)
2. Individual cache mutexes (lowest priority)

UTILITY FUNCTION GUIDELINES:
- Functions that accept *Manager parameter should NOT acquire Manager.Mu
- They assume the caller has already acquired appropriate locks
- Always document locking requirements in function comments
- Utility functions should be "lock-neutral" - not acquiring manager-level locks

SAFE PATTERNS:
- cleanupTenantExpiredEntries: Only acquires individual cache locks
- cleanupExpiredSessions: Only acquires individual cache locks
- Time/TTL functions: No locking required (pure computation)

UNSAFE PATTERNS (AVOIDED):
- Utility functions calling manager.GetTenantStats() (would cause deadlock)
- Utility functions acquiring Manager.Mu while caller holds it
- Cross-tenant operations that could create lock ordering issues

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

// IsAnalyticsBinExpired checks if an analytics bin has expired
// LOCKING: None required (pure computation)
func IsAnalyticsBinExpired(bin *models.HourlyEpinetBin) bool {
	if bin == nil {
		return true
	}

	// Current hour bins expire faster
	// currentHour := GetCurrentHourKey()

	// Extract hour from bin's implied time (would need hour key context)
	// For now, use the TTL from the bin itself
	return IsExpired(bin.ComputedAt, bin.TTL)
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
// This allows the cache package to trigger database pool cleanup without import cycles
func SetDatabasePoolCleanup(cleanup DatabasePoolCleanupFunc) {
	databasePoolCleanup = cleanup
}

// cleanupDatabasePools performs database connection pool cleanup
// This function calls the database package's cleanup function if available
func cleanupDatabasePools() {
	if databasePoolCleanup != nil {
		databasePoolCleanup()
	}
}

// StartCleanupRoutine starts a background goroutine for cache cleanup
// LOCKING: Creates independent goroutine that acquires manager locks safely
func StartCleanupRoutine(manager *Manager) {
	go func() {
		ticker := time.NewTicker(CleanupInterval)
		defer ticker.Stop()

		for range ticker.C {
			cleanupExpiredEntries(manager)
		}
	}()

	// Start SSE cleanup routine
	go func() {
		sseTicker := time.NewTicker(SSECleanupInterval)
		defer sseTicker.Stop()

		for range sseTicker.C {
			cleanupSSEConnections()
		}
	}()

	// Start separate database pool cleanup routine
	go func() {
		dbTicker := time.NewTicker(DBPoolCleanupInterval)
		defer dbTicker.Stop()

		for range dbTicker.C {
			cleanupDatabasePools()
		}
	}()
}

// cleanupExpiredEntries removes expired cache entries
// LOCKING: Acquires manager.Mu.RLock() to get tenant list, then releases before processing
func cleanupExpiredEntries(manager *Manager) {
	// Get snapshot of tenant IDs while holding read lock
	manager.Mu.RLock()
	tenantIDs := make([]string, 0, len(manager.ContentCache))
	for tenantID := range manager.ContentCache {
		tenantIDs = append(tenantIDs, tenantID)
	}
	manager.Mu.RUnlock()

	// Process each tenant without holding manager lock
	for _, tenantID := range tenantIDs {
		cleanupTenantExpiredEntries(manager, tenantID)
	}
}

// cleanupExpiredSessions removes expired sessions from user state cache
// LOCKING: Only acquires individual cache mutex (userCache.Mu)
// ASSUMES: Manager locks NOT held by caller
func cleanupExpiredSessions(manager *Manager, tenantID string) {
	// Get cache reference safely
	manager.Mu.RLock()
	userCache, exists := manager.UserStateCache[tenantID]
	manager.Mu.RUnlock()

	if !exists || userCache == nil {
		return
	}

	// Only acquire the individual cache lock
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
// LOCKING: Only acquires individual cache mutexes (NOT manager.Mu)
// ASSUMES: Manager locks NOT held by caller
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

	// Clean up expired sessions
	cleanupExpiredSessions(manager, tenantID)

	// Clean up analytics bins
	manager.Mu.RLock()
	analyticsCache, analyticsExists := manager.AnalyticsCache[tenantID]
	manager.Mu.RUnlock()

	if analyticsExists && analyticsCache != nil {
		analyticsCache.Mu.Lock()

		// Clean up epinet bins
		for key, bin := range analyticsCache.EpinetBins {
			if IsAnalyticsBinExpired(bin) {
				delete(analyticsCache.EpinetBins, key)
			}
		}

		// Clean up content bins
		for key, bin := range analyticsCache.ContentBins {
			if IsExpired(bin.ComputedAt, bin.TTL) {
				delete(analyticsCache.ContentBins, key)
			}
		}

		// Clean up site bins
		for key, bin := range analyticsCache.SiteBins {
			if IsExpired(bin.ComputedAt, bin.TTL) {
				delete(analyticsCache.SiteBins, key)
			}
		}

		// Clean up computed metrics
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
	// Get all tenant IDs that have active SSE connections
	// We'll need to expose this from the SSEBroadcaster
	tenantIDs := models.Broadcaster.GetActiveTenantIDs()

	totalCleaned := 0
	totalActive := 0

	for _, tenantID := range tenantIDs {
		// Get dead channels for this tenant (need new method)
		deadChannels := models.Broadcaster.GetDeadChannelsForTenant(tenantID)

		if len(deadChannels) > 0 {
			models.Broadcaster.CleanupDeadChannels(tenantID, deadChannels)
			totalCleaned += len(deadChannels)
		}

		totalActive += models.Broadcaster.GetActiveConnectionCount(tenantID)
	}

	if totalCleaned > 0 {
		log.Printf("SSE cleanup: removed %d dead connections across all tenants, %d active remaining",
			totalCleaned, totalActive)
	}
}
