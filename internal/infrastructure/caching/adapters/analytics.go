// Package adapters provides cache adapters for different access patterns.
package adapters

import (
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// WriteOnlyAnalyticsCacheAdapter provides a write-only view of the cache manager,
// satisfying the WriteOnlyAnalyticsCache interface for the WarmingService.
type WriteOnlyAnalyticsCacheAdapter struct {
	manager *manager.Manager
}

// NewWriteOnlyAnalyticsCacheAdapter creates a new write-only analytics cache adapter.
func NewWriteOnlyAnalyticsCacheAdapter(m *manager.Manager) interfaces.WriteOnlyAnalyticsCache {
	return &WriteOnlyAnalyticsCacheAdapter{manager: m}
}

// SetHourlyEpinetBin stores the hourly analytics bin for a specific Epinet.
func (a *WriteOnlyAnalyticsCacheAdapter) SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *types.HourlyEpinetBin) {
	a.manager.SetHourlyEpinetBin(tenantID, epinetID, hourKey, bin)
}

// SetHourlyContentBin stores the hourly analytics bin for a specific content item.
func (a *WriteOnlyAnalyticsCacheAdapter) SetHourlyContentBin(tenantID, contentID, hourKey string, bin *types.HourlyContentBin) {
	a.manager.SetHourlyContentBin(tenantID, contentID, hourKey, bin)
}

// SetHourlySiteBin stores the hourly analytics bin for the entire site.
func (a *WriteOnlyAnalyticsCacheAdapter) SetHourlySiteBin(tenantID, hourKey string, bin *types.HourlySiteBin) {
	a.manager.SetHourlySiteBin(tenantID, hourKey, bin)
}

// SetLeadMetrics calls the underlying manager's ETag-aware method with placeholder values.
func (a *WriteOnlyAnalyticsCacheAdapter) SetLeadMetrics(tenantID string, metrics *types.LeadMetricsCache) {
	a.manager.SetLeadMetricsWithETag(tenantID, "", metrics.Data, "")
}

// SetDashboardData calls the underlying manager's ETag-aware method with placeholder values.
func (a *WriteOnlyAnalyticsCacheAdapter) SetDashboardData(tenantID string, data *types.DashboardCache) {
	a.manager.SetDashboardDataWithETag(tenantID, "", data.Data, "")
}

// PurgeExpiredBins removes analytics bins that are older than the specified retention period.
func (a *WriteOnlyAnalyticsCacheAdapter) PurgeExpiredBins(tenantID string, olderThan string) {
	a.manager.PurgeExpiredBins(tenantID, olderThan)
}

// InvalidateAnalyticsCache clears all analytics-related cache entries for a tenant.
func (a *WriteOnlyAnalyticsCacheAdapter) InvalidateAnalyticsCache(tenantID string) {
	a.manager.InvalidateAnalyticsCache(tenantID)
}

// UpdateLastFullHour updates the marker indicating the last fully processed hour for analytics.
func (a *WriteOnlyAnalyticsCacheAdapter) UpdateLastFullHour(tenantID, hourKey string) {
	a.manager.UpdateLastFullHour(tenantID, hourKey)
}
