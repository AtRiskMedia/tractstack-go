package cache

import (
	"github.com/AtRiskMedia/tractstack-go/models"
)

// ReadOnlyAnalyticsCacheAdapter implements read-only interface
type ReadOnlyAnalyticsCacheAdapter struct {
	manager *Manager
}

func NewReadOnlyAnalyticsCacheAdapter(manager *Manager) ReadOnlyAnalyticsCache {
	return &ReadOnlyAnalyticsCacheAdapter{manager: manager}
}

func (roca *ReadOnlyAnalyticsCacheAdapter) GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*models.HourlyEpinetBin, bool) {
	return roca.manager.GetHourlyEpinetBin(tenantID, epinetID, hourKey)
}

func (roca *ReadOnlyAnalyticsCacheAdapter) GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*models.HourlyEpinetBin, []string) {
	return roca.manager.GetHourlyEpinetRange(tenantID, epinetID, hourKeys)
}

func (roca *ReadOnlyAnalyticsCacheAdapter) GetHourlyContentBin(tenantID, contentID, hourKey string) (*models.HourlyContentBin, bool) {
	return roca.manager.GetHourlyContentBin(tenantID, contentID, hourKey)
}

func (roca *ReadOnlyAnalyticsCacheAdapter) GetHourlySiteBin(tenantID, hourKey string) (*models.HourlySiteBin, bool) {
	return roca.manager.GetHourlySiteBin(tenantID, hourKey)
}

func (roca *ReadOnlyAnalyticsCacheAdapter) GetLeadMetrics(tenantID string) (*models.LeadMetricsCache, bool) {
	return roca.manager.GetLeadMetrics(tenantID)
}

func (roca *ReadOnlyAnalyticsCacheAdapter) GetDashboardData(tenantID string) (*models.DashboardCache, bool) {
	return roca.manager.GetDashboardData(tenantID)
}

func (roca *ReadOnlyAnalyticsCacheAdapter) EnsureTenant(tenantID string) {
	roca.manager.EnsureTenant(tenantID)
}

// WriteOnlyAnalyticsCacheAdapter implements write-only interface
type WriteOnlyAnalyticsCacheAdapter struct {
	manager *Manager
}

func NewWriteOnlyAnalyticsCacheAdapter(manager *Manager) WriteOnlyAnalyticsCache {
	return &WriteOnlyAnalyticsCacheAdapter{manager: manager}
}

func (woca *WriteOnlyAnalyticsCacheAdapter) SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *models.HourlyEpinetBin) {
	woca.manager.SetHourlyEpinetBin(tenantID, epinetID, hourKey, bin)
}

func (woca *WriteOnlyAnalyticsCacheAdapter) SetHourlyContentBin(tenantID, contentID, hourKey string, bin *models.HourlyContentBin) {
	woca.manager.SetHourlyContentBin(tenantID, contentID, hourKey, bin)
}

func (woca *WriteOnlyAnalyticsCacheAdapter) SetHourlySiteBin(tenantID, hourKey string, bin *models.HourlySiteBin) {
	woca.manager.SetHourlySiteBin(tenantID, hourKey, bin)
}

func (woca *WriteOnlyAnalyticsCacheAdapter) SetLeadMetrics(tenantID string, metrics *models.LeadMetricsCache) {
	woca.manager.SetLeadMetrics(tenantID, metrics)
}

func (woca *WriteOnlyAnalyticsCacheAdapter) SetDashboardData(tenantID string, data *models.DashboardCache) {
	woca.manager.SetDashboardData(tenantID, data)
}

func (woca *WriteOnlyAnalyticsCacheAdapter) PurgeExpiredBins(tenantID string, olderThan string) {
	woca.manager.PurgeExpiredBins(tenantID, olderThan)
}

func (woca *WriteOnlyAnalyticsCacheAdapter) InvalidateAnalyticsCache(tenantID string) {
	woca.manager.InvalidateAnalyticsCache(tenantID)
}

func (woca *WriteOnlyAnalyticsCacheAdapter) UpdateLastFullHour(tenantID, hourKey string) {
	woca.manager.UpdateLastFullHour(tenantID, hourKey)
}

func (woca *WriteOnlyAnalyticsCacheAdapter) EnsureTenant(tenantID string) {
	woca.manager.EnsureTenant(tenantID)
}
