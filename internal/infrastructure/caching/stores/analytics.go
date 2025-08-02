// Package stores provides concrete cache store implementations
package stores

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
)

// AnalyticsStore implements analytics caching operations with tenant isolation
type AnalyticsStore struct {
	tenantCaches map[string]*types.TenantAnalyticsCache
	mu           sync.RWMutex
	logger       *logging.ChanneledLogger
}

// NewAnalyticsStore creates a new analytics cache store
func NewAnalyticsStore(logger *logging.ChanneledLogger) *AnalyticsStore {
	if logger != nil {
		logger.Cache().Info("Initializing analytics cache store")
	}
	return &AnalyticsStore{
		tenantCaches: make(map[string]*types.TenantAnalyticsCache),
		logger:       logger,
	}
}

// InitializeTenant creates cache structures for a tenant
func (as *AnalyticsStore) InitializeTenant(tenantID string) {
	start := time.Now()
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.logger != nil {
		as.logger.Cache().Debug("Initializing tenant analytics cache", "tenantId", tenantID)
	}

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

		if as.logger != nil {
			as.logger.Cache().Info("Tenant analytics cache initialized", "tenantId", tenantID, "duration", time.Since(start))
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
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "epinet_bin", "tenantId", tenantID, "epinetId", epinetID, "hourKey", hourKey, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	binKey := epinetID + ":" + hourKey
	bin, found := cache.EpinetBins[binKey]
	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "epinet_bin", "tenantId", tenantID, "epinetId", epinetID, "hourKey", hourKey, "hit", found, "duration", time.Since(start))
	}
	return bin, found
}

// SetHourlyEpinetBin stores an hourly epinet bin
func (as *AnalyticsStore) SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *types.HourlyEpinetBin) {
	start := time.Now()
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

	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "set", "type", "epinet_bin", "tenantId", tenantID, "epinetId", epinetID, "hourKey", hourKey, "duration", time.Since(start))
	}
}

// GetHourlyEpinetRange retrieves multiple hourly epinet bins
func (as *AnalyticsStore) GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*types.HourlyEpinetBin, []string) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "get_range", "type", "epinet_bin", "tenantId", tenantID, "epinetId", epinetID, "requested", len(hourKeys), "found", 0, "missing", len(hourKeys), "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
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

	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "get_range", "type", "epinet_bin", "tenantId", tenantID, "epinetId", epinetID, "requested", len(hourKeys), "found", len(found), "missing", len(missing), "duration", time.Since(start))
	}

	return found, missing
}

// =============================================================================
// Hourly Content Bin Operations
// =============================================================================

// GetHourlyContentBin retrieves an hourly content bin
func (as *AnalyticsStore) GetHourlyContentBin(tenantID, contentID, hourKey string) (*types.HourlyContentBin, bool) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "content_bin", "tenantId", tenantID, "contentId", contentID, "hourKey", hourKey, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	binKey := contentID + ":" + hourKey
	bin, found := cache.ContentBins[binKey]
	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "content_bin", "tenantId", tenantID, "contentId", contentID, "hourKey", hourKey, "hit", found, "duration", time.Since(start))
	}
	return bin, found
}

// SetHourlyContentBin stores an hourly content bin
func (as *AnalyticsStore) SetHourlyContentBin(tenantID, contentID, hourKey string, bin *types.HourlyContentBin) {
	start := time.Now()
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

	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "set", "type", "content_bin", "tenantId", tenantID, "contentId", contentID, "hourKey", hourKey, "duration", time.Since(start))
	}
}

// =============================================================================
// Hourly Site Bin Operations
// =============================================================================

// GetHourlySiteBin retrieves an hourly site bin
func (as *AnalyticsStore) GetHourlySiteBin(tenantID, hourKey string) (*types.HourlySiteBin, bool) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "site_bin", "tenantId", tenantID, "hourKey", hourKey, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	bin, found := cache.SiteBins[hourKey]
	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "site_bin", "tenantId", tenantID, "hourKey", hourKey, "hit", found, "duration", time.Since(start))
	}
	return bin, found
}

// SetHourlySiteBin stores an hourly site bin
func (as *AnalyticsStore) SetHourlySiteBin(tenantID, hourKey string, bin *types.HourlySiteBin) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.SiteBins[hourKey] = bin
	cache.LastUpdated = time.Now().UTC()

	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "set", "type", "site_bin", "tenantId", tenantID, "hourKey", hourKey, "duration", time.Since(start))
	}
}

// =============================================================================
// Computed Metrics Operations
// =============================================================================

// GetLeadMetrics retrieves cached lead metrics
func (as *AnalyticsStore) GetLeadMetrics(tenantID string) (*types.LeadMetricsCache, bool) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "lead_metrics", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.LeadMetrics == nil {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "lead_metrics", "tenantId", tenantID, "hit", false, "reason", "nil", "duration", time.Since(start))
		}
		return nil, false
	}

	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "lead_metrics", "tenantId", tenantID, "hit", true, "duration", time.Since(start))
	}

	// TTL check is handled by the manager
	return cache.LeadMetrics, true
}

// SetLeadMetrics stores computed lead metrics - CORRECTED SIGNATURE
func (as *AnalyticsStore) SetLeadMetrics(tenantID string, metrics *types.LeadMetricsCache) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.LeadMetrics = metrics
	cache.LastUpdated = time.Now().UTC()

	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "set", "type", "lead_metrics", "tenantId", tenantID, "duration", time.Since(start))
	}
}

// GetDashboardData retrieves cached dashboard data
func (as *AnalyticsStore) GetDashboardData(tenantID string) (*types.DashboardCache, bool) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "dashboard_data", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.DashboardData == nil {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "dashboard_data", "tenantId", tenantID, "hit", false, "reason", "nil", "duration", time.Since(start))
		}
		return nil, false
	}

	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "dashboard_data", "tenantId", tenantID, "hit", true, "duration", time.Since(start))
	}

	// TTL check is handled by the manager
	return cache.DashboardData, true
}

// SetDashboardData stores computed dashboard data - CORRECTED SIGNATURE
func (as *AnalyticsStore) SetDashboardData(tenantID string, data *types.DashboardCache) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.DashboardData = data
	cache.LastUpdated = time.Now().UTC()

	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "set", "type", "dashboard_data", "tenantId", tenantID, "duration", time.Since(start))
	}
}

// GetEpinetSankey retrieves a cached Sankey diagram
func (as *AnalyticsStore) GetEpinetSankey(tenantID, epinetID string, filters string) (*types.SankeyDiagram, string, bool) {
	// This functionality is not part of the immediate plan, returning not found.
	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "get", "type", "epinet_sankey", "tenantId", tenantID, "epinetId", epinetID, "hit", false, "reason", "not_implemented")
	}
	return nil, "", false
}

// SetEpinetSankey stores a computed Sankey diagram
func (as *AnalyticsStore) SetEpinetSankey(tenantID, epinetID string, filters string, data *types.SankeyDiagram, etag string) {
	// This functionality is not part of the immediate plan.
	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "set", "type", "epinet_sankey", "tenantId", tenantID, "epinetId", epinetID, "reason", "not_implemented")
	}
}

// =============================================================================
// Cache Management Operations
// =============================================================================

// PurgeExpiredBins removes hourly bins older than specified hour key
func (as *AnalyticsStore) PurgeExpiredBins(tenantID string, olderThan string) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "purge", "type", "expired_bins", "tenantId", tenantID, "olderThan", olderThan, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return
	}

	if as.logger != nil {
		as.logger.Cache().Debug("Starting cache purge", "tenantId", tenantID, "olderThan", olderThan)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	purgeCounts := map[string]int{
		"epinet_bins":  0,
		"content_bins": 0,
		"site_bins":    0,
	}

	// Purge epinet bins
	for binKey := range cache.EpinetBins {
		parts := splitBinKey(binKey)
		if len(parts) == 2 && parts[1] < olderThan {
			delete(cache.EpinetBins, binKey)
			purgeCounts["epinet_bins"]++
		}
	}

	// Purge content bins
	for binKey := range cache.ContentBins {
		parts := splitBinKey(binKey)
		if len(parts) == 2 && parts[1] < olderThan {
			delete(cache.ContentBins, binKey)
			purgeCounts["content_bins"]++
		}
	}

	// Purge site bins
	for hourKey := range cache.SiteBins {
		if hourKey < olderThan {
			delete(cache.SiteBins, hourKey)
			purgeCounts["site_bins"]++
		}
	}

	cache.LastUpdated = time.Now().UTC()

	if as.logger != nil {
		as.logger.Cache().Info("Cache purge completed", "tenantId", tenantID, "olderThan", olderThan, "purged_epinet_bins", purgeCounts["epinet_bins"], "purged_content_bins", purgeCounts["content_bins"], "purged_site_bins", purgeCounts["site_bins"], "duration", time.Since(start))
	}
}

// UpdateLastFullHour updates the last processed hour for a tenant
func (as *AnalyticsStore) UpdateLastFullHour(tenantID, hourKey string) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		as.InitializeTenant(tenantID)
		cache, _ = as.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.LastFullHour = hourKey
	cache.LastUpdated = time.Now().UTC()

	if as.logger != nil {
		as.logger.Cache().Debug("Cache operation", "operation", "update_last_hour", "tenantId", tenantID, "hourKey", hourKey, "duration", time.Since(start))
	}
}

// InvalidateAnalyticsCache clears computed metrics for a tenant
func (as *AnalyticsStore) InvalidateAnalyticsCache(tenantID string) {
	start := time.Now()
	cache, exists := as.GetTenantCache(tenantID)
	if !exists {
		if as.logger != nil {
			as.logger.Cache().Debug("Cache operation", "operation", "invalidate", "type", "analytics", "tenantId", tenantID, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return
	}

	if as.logger != nil {
		as.logger.Cache().Debug("Invalidating analytics cache", "tenantId", tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.LeadMetrics = nil
	cache.DashboardData = nil
	cache.LastUpdated = time.Now().UTC()

	if as.logger != nil {
		as.logger.Cache().Info("Analytics cache invalidated", "tenantId", tenantID, "duration", time.Since(start))
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// splitBinKey splits a bin key like "epinetID:hourKey" into parts
func splitBinKey(binKey string) []string {
	parts := make([]string, 0, 2)
	colonIndex := -1

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
