// Package stores provides concrete cache store implementations
package stores

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// AnalyticsStore implements analytics caching operations with tenant isolation
type AnalyticsStore struct {
	tenantCaches map[string]*types.TenantAnalyticsCache
	mu           sync.RWMutex
}

// NewAnalyticsStore creates a new analytics cache store
func NewAnalyticsStore() *AnalyticsStore {
	return &AnalyticsStore{
		tenantCaches: make(map[string]*types.TenantAnalyticsCache),
	}
}

// InitializeTenant creates cache structures for a tenant
func (as *AnalyticsStore) InitializeTenant(tenantID string) {
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.tenantCaches[tenantID] == nil {
		as.tenantCaches[tenantID] = &types.TenantAnalyticsCache{
			EpinetBins:    make(map[string]*types.HourlyEpinetBin),
			ContentBins:   make(map[string]*types.HourlyContentBin),
			SiteBins:      make(map[string]*types.HourlySiteBin),
			LeadMetrics:   nil,
			DashboardData: nil,
			LastFullHour:  "",
			LastUpdated:   time.Now().UTC(),
		}
	}
}

// GetTenantCache safely retrieves a tenant's analytics cache
func (as *AnalyticsStore) GetTenantCache(tenantID string) (*types.TenantAnalyticsCache, bool) {
	as.mu.RLock()
	defer as.mu.RUnlock()
	cache, exists := as.tenantCaches[tenantID]
	return cache, exists
}

// =============================================================================
// Hourly Epinet Bin Operations
// =============================================================================

// GetHourlyEpinetBin retrieves an hourly epinet bin
func (as *AnalyticsStore) GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*types.HourlyEpinetBin, bool) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	binKey := epinetID + ":" + hourKey
	bin, exists := cache.EpinetBins[binKey]
	return bin, exists
}

// SetHourlyEpinetBin stores an hourly epinet bin
func (as *AnalyticsStore) SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *types.HourlyEpinetBin) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	binKey := epinetID + ":" + hourKey
	cache.EpinetBins[binKey] = bin
	cache.LastUpdated = time.Now().UTC()
}

// GetHourlyEpinetRange retrieves multiple hourly epinet bins
func (as *AnalyticsStore) GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*types.HourlyEpinetBin, []string) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return make(map[string]*types.HourlyEpinetBin), hourKeys
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	found := make(map[string]*types.HourlyEpinetBin)
	missing := make([]string, 0)

	for _, hourKey := range hourKeys {
		binKey := epinetID + ":" + hourKey
		if bin, exists := cache.EpinetBins[binKey]; exists {
			found[hourKey] = bin
		} else {
			missing = append(missing, hourKey)
		}
	}

	return found, missing
}

// =============================================================================
// Hourly Content Bin Operations
// =============================================================================

// GetHourlyContentBin retrieves an hourly content bin
func (as *AnalyticsStore) GetHourlyContentBin(tenantID, contentID, hourKey string) (*types.HourlyContentBin, bool) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	binKey := contentID + ":" + hourKey
	bin, exists := cache.ContentBins[binKey]
	return bin, exists
}

// SetHourlyContentBin stores an hourly content bin
func (as *AnalyticsStore) SetHourlyContentBin(tenantID, contentID, hourKey string, bin *types.HourlyContentBin) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	binKey := contentID + ":" + hourKey
	cache.ContentBins[binKey] = bin
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Hourly Site Bin Operations
// =============================================================================

// GetHourlySiteBin retrieves an hourly site bin
func (as *AnalyticsStore) GetHourlySiteBin(tenantID, hourKey string) (*types.HourlySiteBin, bool) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	bin, exists := cache.SiteBins[hourKey]
	return bin, exists
}

// SetHourlySiteBin stores an hourly site bin
func (as *AnalyticsStore) SetHourlySiteBin(tenantID, hourKey string, bin *types.HourlySiteBin) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.SiteBins[hourKey] = bin
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Computed Metrics Operations
// =============================================================================

// GetLeadMetrics retrieves cached lead metrics
func (as *AnalyticsStore) GetLeadMetrics(tenantID string) (*types.LeadMetricsCache, bool) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.LeadMetrics == nil {
		return nil, false
	}

	// Check if metrics are expired (1 hour TTL for computed metrics)
	if time.Since(cache.LeadMetrics.LastComputed) > time.Hour {
		return nil, false
	}

	return cache.LeadMetrics, true
}

// SetLeadMetrics stores computed lead metrics
func (as *AnalyticsStore) SetLeadMetrics(tenantID string, metrics *types.LeadMetricsCache) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.LeadMetrics = metrics
	cache.LastUpdated = time.Now().UTC()
}

// GetDashboardData retrieves cached dashboard data
func (as *AnalyticsStore) GetDashboardData(tenantID string) (*types.DashboardCache, bool) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.DashboardData == nil {
		return nil, false
	}

	// Check if dashboard data is expired (1 hour TTL for computed metrics)
	if time.Since(cache.DashboardData.LastComputed) > time.Hour {
		return nil, false
	}

	return cache.DashboardData, true
}

// SetDashboardData stores computed dashboard data
func (as *AnalyticsStore) SetDashboardData(tenantID string, data *types.DashboardCache) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.DashboardData = data
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Cache Management Operations
// =============================================================================

// PurgeExpiredBins removes hourly bins older than specified hour key
func (as *AnalyticsStore) PurgeExpiredBins(tenantID string, olderThan string) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	// Purge epinet bins
	for binKey := range cache.EpinetBins {
		// Extract hour key from binKey (format: "epinetID:hourKey")
		parts := splitBinKey(binKey)
		if len(parts) == 2 && parts[1] < olderThan {
			delete(cache.EpinetBins, binKey)
		}
	}

	// Purge content bins
	for binKey := range cache.ContentBins {
		// Extract hour key from binKey (format: "contentID:hourKey")
		parts := splitBinKey(binKey)
		if len(parts) == 2 && parts[1] < olderThan {
			delete(cache.ContentBins, binKey)
		}
	}

	// Purge site bins
	for hourKey := range cache.SiteBins {
		if hourKey < olderThan {
			delete(cache.SiteBins, hourKey)
		}
	}

	cache.LastUpdated = time.Now().UTC()
}

// UpdateLastFullHour updates the last processed hour for a tenant
func (as *AnalyticsStore) UpdateLastFullHour(tenantID, hourKey string) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.LastFullHour = hourKey
	cache.LastUpdated = time.Now().UTC()
}

// GetLastFullHour retrieves the last processed hour for a tenant
func (as *AnalyticsStore) GetLastFullHour(tenantID string) string {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return ""
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	return cache.LastFullHour
}

// InvalidateAnalyticsCache clears computed metrics for a tenant
func (as *AnalyticsStore) InvalidateAnalyticsCache(tenantID string) {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	// Clear computed metrics but keep raw hourly bins
	cache.LeadMetrics = nil
	cache.DashboardData = nil
	cache.LastUpdated = time.Now().UTC()
}

// GetAnalyticsSummary returns cache status summary for debugging
func (as *AnalyticsStore) GetAnalyticsSummary(tenantID string) map[string]any {
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		return map[string]any{
			"exists": false,
		}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	return map[string]any{
		"exists":         true,
		"epinetBins":     len(cache.EpinetBins),
		"contentBins":    len(cache.ContentBins),
		"siteBins":       len(cache.SiteBins),
		"hasLeadMetrics": cache.LeadMetrics != nil,
		"hasDashboard":   cache.DashboardData != nil,
		"lastFullHour":   cache.LastFullHour,
		"lastUpdated":    cache.LastUpdated,
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// splitBinKey splits a bin key like "epinetID:hourKey" into parts
func splitBinKey(binKey string) []string {
	parts := make([]string, 0, 2)
	colonIndex := -1

	// Find the last colon to handle IDs that might contain colons
	for i := len(binKey) - 1; i >= 0; i-- {
		if binKey[i] == ':' {
			colonIndex = i
			break
		}
	}

	if colonIndex == -1 {
		return []string{binKey}
	}

	parts = append(parts, binKey[:colonIndex])
	parts = append(parts, binKey[colonIndex+1:])
	return parts
}
