// Package cache provides utility functions for cache management, TTL handling, and cleanup operations.
package cache

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

const (
	// TTL constants for different cache types
	ContentCacheTTL = 24 * time.Hour      // Content changes infrequently
	UserStateTTL    = 2 * time.Hour       // User state changes more frequently
	HTMLChunkTTL    = 1 * time.Hour       // HTML chunks can be regenerated quickly
	AnalyticsBinTTL = 28 * 24 * time.Hour // Analytics bins are kept for 28 days
	CurrentHourTTL  = 15 * time.Minute    // Current hour bins refresh more frequently
	LeadMetricsTTL  = 5 * time.Minute     // Lead metrics computed frequently
	DashboardTTL    = 10 * time.Minute    // Dashboard data refreshed regularly

	// Cleanup intervals
	CleanupInterval = 30 * time.Minute // How often to run cleanup
	TenantTimeout   = 4 * time.Hour    // Evict inactive tenants after this time
)

// CacheLock provides cache management for cache refresh operations
type CacheLock struct {
	AcquiredAt time.Time
	ExpiresAt  time.Time
	TenantID   string
	Key        string
}

var (
	cacheLocks = make(map[string]*CacheLock)
	locksMutex = &sync.RWMutex{}
	lockTTL    = 30 * time.Second
)

// TryAcquireLock attempts to acquire a lock for cache operations
func TryAcquireLock(lockType, tenantID, key string) bool {
	locksMutex.Lock()
	defer locksMutex.Unlock()

	// Clean expired locks first
	cleanExpiredLocks()

	lockKey := fmt.Sprintf("%s-%s-%s", lockType, tenantID, key)

	// Check if lock already exists
	if _, exists := cacheLocks[lockKey]; exists {
		return false
	}

	// Acquire the lock
	now := time.Now()
	cacheLocks[lockKey] = &CacheLock{
		AcquiredAt: now,
		ExpiresAt:  now.Add(lockTTL),
		TenantID:   tenantID,
		Key:        key,
	}

	return true
}

// ReleaseLock releases a previously acquired lock
func ReleaseLock(lockType, tenantID, key string) {
	locksMutex.Lock()
	defer locksMutex.Unlock()

	lockKey := fmt.Sprintf("%s-%s-%s", lockType, tenantID, key)
	delete(cacheLocks, lockKey)
}

// cleanExpiredLocks removes expired locks (internal function)
func cleanExpiredLocks() {
	now := time.Now()
	for key, lock := range cacheLocks {
		if lock.ExpiresAt.Before(now) {
			delete(cacheLocks, key)
		}
	}
}

// FormatHourKey creates a consistent hour key for analytics
func FormatHourKey(date time.Time) string {
	// Use UTC to match database timestamps
	year := date.UTC().Year()
	month := int(date.UTC().Month())
	day := date.UTC().Day()
	hour := date.UTC().Hour()
	return fmt.Sprintf("%04d-%02d-%02d-%02d", year, month, day, hour)
}

// ParseHourKey parses an hour key back to a time
func ParseHourKey(hourKey string) (time.Time, error) {
	var year, month, day, hour int
	_, err := fmt.Sscanf(hourKey, "%d-%d-%d-%d", &year, &month, &day, &hour)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid hour key format: %s", hourKey)
	}
	return time.Date(year, time.Month(month), day, hour, 0, 0, 0, time.UTC), nil
}

// GetCurrentHourKey returns the current hour key
func GetCurrentHourKey() string {
	now := time.Now().UTC()
	return FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))
}

// GetHourKeysForRange returns hour keys for a time range
func GetHourKeysForRange(hours int) []string {
	keys := make([]string, 0, hours)
	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)

	for i := 0; i < hours; i++ {
		hourDate := currentHour.Add(-time.Duration(i) * time.Hour)
		keys = append(keys, FormatHourKey(hourDate))
	}

	return keys
}

// IsExpired checks if a cached item has exceeded its TTL
func IsExpired(cachedAt time.Time, ttl time.Duration) bool {
	return time.Since(cachedAt) > ttl
}

// IsAnalyticsBinExpired checks if an analytics bin has expired
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

// StartCleanupRoutine starts a background goroutine for cache cleanup
func StartCleanupRoutine(manager *Manager) {
	go func() {
		ticker := time.NewTicker(CleanupInterval)
		defer ticker.Stop()

		for tenantID := range manager.ContentCache {
			cleanupTenantExpiredEntries(manager, tenantID)
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

// cleanupTenantExpiredEntries cleans up expired entries for a specific tenant
func cleanupTenantExpiredEntries(manager *Manager, tenantID string) {
	// Clean up HTML chunks
	if htmlCache, exists := manager.HTMLChunkCache[tenantID]; exists {
		htmlCache.Mu.Lock()
		for key, chunk := range htmlCache.Chunks {
			if IsExpired(chunk.CachedAt, HTMLChunkTTL) {
				delete(htmlCache.Chunks, key)
			}
		}
		htmlCache.Mu.Unlock()
	}

	// Clean up analytics bins
	if analyticsCache, exists := manager.AnalyticsCache[tenantID]; exists {
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

// cleanupInactiveTenants removes cache data for tenants that haven't been accessed recently
func cleanupInactiveTenants(manager *Manager) {
	manager.Mu.Lock()
	defer manager.Mu.Unlock()

	now := time.Now()
	for tenantID, lastAccessed := range manager.LastAccessed {
		if now.Sub(lastAccessed) > TenantTimeout {
			// Remove tenant caches
			delete(manager.ContentCache, tenantID)
			delete(manager.UserStateCache, tenantID)
			delete(manager.HTMLChunkCache, tenantID)
			delete(manager.AnalyticsCache, tenantID)
			delete(manager.LastAccessed, tenantID)
		}
	}
}

// CreateEmptyHourlyEpinetData creates an empty epinet data structure
func CreateEmptyHourlyEpinetData() *models.HourlyEpinetData {
	return &models.HourlyEpinetData{
		Steps:       make(map[string]*models.HourlyEpinetStepData),
		Transitions: make(map[string]map[string]*models.HourlyEpinetTransitionData),
	}
}

// CreateEmptyHourlyContentData creates an empty content data structure
func CreateEmptyHourlyContentData() *models.HourlyContentData {
	return &models.HourlyContentData{
		UniqueVisitors:    make(map[string]bool),
		KnownVisitors:     make(map[string]bool),
		AnonymousVisitors: make(map[string]bool),
		Actions:           0,
		EventCounts:       make(map[string]int),
	}
}

// CreateEmptyHourlySiteData creates an empty site data structure
func CreateEmptyHourlySiteData() *models.HourlySiteData {
	return &models.HourlySiteData{
		TotalVisits:       0,
		KnownVisitors:     make(map[string]bool),
		AnonymousVisitors: make(map[string]bool),
		EventCounts:       make(map[string]int),
	}
}

// GetCacheKey creates a consistent cache key
func GetCacheKey(parts ...string) string {
	return strings.Join(parts, ":")
}

// ValidateTenantID checks if a tenant ID is valid
func ValidateTenantID(tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}
	if len(tenantID) > 64 {
		return fmt.Errorf("tenant ID too long: %d characters (max 64)", len(tenantID))
	}
	return nil
}

// GetMemoryStats returns memory usage statistics for the cache
func GetMemoryStats(manager *Manager) map[string]interface{} {
	manager.Mu.RLock()
	defer manager.Mu.RUnlock()

	stats := make(map[string]interface{})

	totalTenants := len(manager.ContentCache)
	totalContent := 0
	totalUserStates := 0
	totalHTMLChunks := 0
	totalAnalyticsBins := 0

	for tenantID := range manager.ContentCache {
		if contentCache := manager.ContentCache[tenantID]; contentCache != nil {
			contentCache.Mu.RLock()
			totalContent += len(contentCache.TractStacks)
			totalContent += len(contentCache.StoryFragments)
			totalContent += len(contentCache.Panes)
			totalContent += len(contentCache.Menus)
			totalContent += len(contentCache.Resources)
			totalContent += len(contentCache.Beliefs)
			totalContent += len(contentCache.Files)
			contentCache.Mu.RUnlock()
		}

		if userCache := manager.UserStateCache[tenantID]; userCache != nil {
			userCache.Mu.RLock()
			totalUserStates += len(userCache.FingerprintStates)
			totalUserStates += len(userCache.VisitStates)
			userCache.Mu.RUnlock()
		}

		if htmlCache := manager.HTMLChunkCache[tenantID]; htmlCache != nil {
			htmlCache.Mu.RLock()
			totalHTMLChunks += len(htmlCache.Chunks)
			htmlCache.Mu.RUnlock()
		}

		if analyticsCache := manager.AnalyticsCache[tenantID]; analyticsCache != nil {
			analyticsCache.Mu.RLock()
			totalAnalyticsBins += len(analyticsCache.EpinetBins)
			totalAnalyticsBins += len(analyticsCache.ContentBins)
			totalAnalyticsBins += len(analyticsCache.SiteBins)
			analyticsCache.Mu.RUnlock()
		}
	}

	stats["totalTenants"] = totalTenants
	stats["totalContentNodes"] = totalContent
	stats["totalUserStates"] = totalUserStates
	stats["totalHTMLChunks"] = totalHTMLChunks
	stats["totalAnalyticsBins"] = totalAnalyticsBins
	stats["activeLocks"] = len(cacheLocks)

	return stats
}
