// Package cache provides analytics cache adapters for different access patterns.
package cache

import (
	"github.com/AtRiskMedia/tractstack-go/models"
)

// ReadOnlyAnalyticsCacheAdapter provides read-only access to analytics cache
type ReadOnlyAnalyticsCacheAdapter struct {
	manager *Manager
}

// NewReadOnlyAnalyticsCacheAdapter creates a new read-only analytics cache adapter
func NewReadOnlyAnalyticsCacheAdapter(manager *Manager) *ReadOnlyAnalyticsCacheAdapter {
	return &ReadOnlyAnalyticsCacheAdapter{manager: manager}
}

// GetHourlyEpinetBin retrieves an hourly epinet bin from cache
func (roca *ReadOnlyAnalyticsCacheAdapter) GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*models.HourlyEpinetBin, bool) {
	return roca.manager.GetHourlyEpinetBin(tenantID, epinetID, hourKey)
}

// GetHourlyContentBin retrieves an hourly content bin from cache
func (roca *ReadOnlyAnalyticsCacheAdapter) GetHourlyContentBin(tenantID, contentID, hourKey string) (*models.HourlyContentBin, bool) {
	return roca.manager.GetHourlyContentBin(tenantID, contentID, hourKey)
}

// GetHourlySiteBin retrieves an hourly site bin from cache
func (roca *ReadOnlyAnalyticsCacheAdapter) GetHourlySiteBin(tenantID, hourKey string) (*models.HourlySiteBin, bool) {
	return roca.manager.GetHourlySiteBin(tenantID, hourKey)
}

// GetLeadMetrics retrieves lead metrics from cache
func (roca *ReadOnlyAnalyticsCacheAdapter) GetLeadMetrics(tenantID string) (*models.LeadMetricsCache, bool) {
	return roca.manager.GetLeadMetrics(tenantID)
}

// GetDashboardData retrieves dashboard data from cache
func (roca *ReadOnlyAnalyticsCacheAdapter) GetDashboardData(tenantID string) (*models.DashboardCache, bool) {
	return roca.manager.GetDashboardData(tenantID)
}

// GetHourlyEpinetRange retrieves multiple hourly epinet bins in a single operation
func (roca *ReadOnlyAnalyticsCacheAdapter) GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*models.HourlyEpinetBin, []string) {
	return roca.manager.GetHourlyEpinetRange(tenantID, epinetID, hourKeys)
}

// WriteOnlyAnalyticsCacheAdapter provides write-only access to analytics cache
type WriteOnlyAnalyticsCacheAdapter struct {
	manager *Manager
}

// NewWriteOnlyAnalyticsCacheAdapter creates a new write-only analytics cache adapter
func NewWriteOnlyAnalyticsCacheAdapter(manager *Manager) *WriteOnlyAnalyticsCacheAdapter {
	return &WriteOnlyAnalyticsCacheAdapter{manager: manager}
}

// SetHourlyEpinetBin stores an hourly epinet bin in cache
func (woca *WriteOnlyAnalyticsCacheAdapter) SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *models.HourlyEpinetBin) {
	woca.manager.SetHourlyEpinetBin(tenantID, epinetID, hourKey, bin)
}

// SetHourlyContentBin stores an hourly content bin in cache
func (woca *WriteOnlyAnalyticsCacheAdapter) SetHourlyContentBin(tenantID, contentID, hourKey string, bin *models.HourlyContentBin) {
	woca.manager.SetHourlyContentBin(tenantID, contentID, hourKey, bin)
}

// SetHourlySiteBin stores an hourly site bin in cache
func (woca *WriteOnlyAnalyticsCacheAdapter) SetHourlySiteBin(tenantID, hourKey string, bin *models.HourlySiteBin) {
	woca.manager.SetHourlySiteBin(tenantID, hourKey, bin)
}

// SetLeadMetrics stores lead metrics in cache
func (woca *WriteOnlyAnalyticsCacheAdapter) SetLeadMetrics(tenantID string, metrics *models.LeadMetricsCache) {
	woca.manager.SetLeadMetrics(tenantID, metrics)
}

// SetDashboardData stores dashboard data in cache
func (woca *WriteOnlyAnalyticsCacheAdapter) SetDashboardData(tenantID string, data *models.DashboardCache) {
	woca.manager.SetDashboardData(tenantID, data)
}

// PurgeExpiredBins removes expired analytics bins for a tenant
func (woca *WriteOnlyAnalyticsCacheAdapter) PurgeExpiredBins(tenantID string, olderThan string) {
	woca.manager.PurgeExpiredBins(tenantID, olderThan)
}

// InvalidateAnalyticsCache clears analytics cache for a tenant
func (woca *WriteOnlyAnalyticsCacheAdapter) InvalidateAnalyticsCache(tenantID string) {
	woca.manager.InvalidateAnalyticsCache(tenantID)
}

// UpdateLastFullHour updates the last processed hour for analytics
func (woca *WriteOnlyAnalyticsCacheAdapter) UpdateLastFullHour(tenantID, hourKey string) {
	woca.manager.UpdateLastFullHour(tenantID, hourKey)
}
