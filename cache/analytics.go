// Package cache provides analytics caching implementation.
package cache

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// =============================================================================
// Analytics Cache Implementation for Manager
// =============================================================================

// GetHourlyEpinetBin retrieves an hourly epinet bin from cache
func (m *Manager) GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*models.HourlyEpinetBin, bool) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	analyticsCache.Mu.RLock()
	defer analyticsCache.Mu.RUnlock()

	binKey := fmt.Sprintf("%s:%s", epinetID, hourKey)
	bin, found := analyticsCache.EpinetBins[binKey]
	if !found {
		return nil, false
	}

	// Check if bin has expired
	if IsAnalyticsBinExpired(bin) {
		return nil, false
	}

	return bin, true
}

// SetHourlyEpinetBin stores an hourly epinet bin in cache
func (m *Manager) SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *models.HourlyEpinetBin) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	analyticsCache.Mu.Lock()
	defer analyticsCache.Mu.Unlock()

	if analyticsCache.EpinetBins == nil {
		analyticsCache.EpinetBins = make(map[string]*models.HourlyEpinetBin)
	}

	binKey := fmt.Sprintf("%s:%s", epinetID, hourKey)
	analyticsCache.EpinetBins[binKey] = bin
	analyticsCache.LastUpdated = time.Now()
}

// GetHourlyContentBin retrieves an hourly content bin from cache
func (m *Manager) GetHourlyContentBin(tenantID, contentID, hourKey string) (*models.HourlyContentBin, bool) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	analyticsCache.Mu.RLock()
	defer analyticsCache.Mu.RUnlock()

	binKey := fmt.Sprintf("%s:%s", contentID, hourKey)
	bin, found := analyticsCache.ContentBins[binKey]
	if !found {
		return nil, false
	}

	// Check if bin has expired
	if IsExpired(bin.ComputedAt, bin.TTL) {
		return nil, false
	}

	return bin, true
}

// SetHourlyContentBin stores an hourly content bin in cache
func (m *Manager) SetHourlyContentBin(tenantID, contentID, hourKey string, bin *models.HourlyContentBin) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	analyticsCache.Mu.Lock()
	defer analyticsCache.Mu.Unlock()

	if analyticsCache.ContentBins == nil {
		analyticsCache.ContentBins = make(map[string]*models.HourlyContentBin)
	}

	binKey := fmt.Sprintf("%s:%s", contentID, hourKey)
	analyticsCache.ContentBins[binKey] = bin
	analyticsCache.LastUpdated = time.Now()
}

// GetHourlySiteBin retrieves an hourly site bin from cache
func (m *Manager) GetHourlySiteBin(tenantID, hourKey string) (*models.HourlySiteBin, bool) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	analyticsCache.Mu.RLock()
	defer analyticsCache.Mu.RUnlock()

	bin, found := analyticsCache.SiteBins[hourKey]
	if !found {
		return nil, false
	}

	// Check if bin has expired
	if IsExpired(bin.ComputedAt, bin.TTL) {
		return nil, false
	}

	return bin, true
}

// SetHourlySiteBin stores an hourly site bin in cache
func (m *Manager) SetHourlySiteBin(tenantID, hourKey string, bin *models.HourlySiteBin) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	analyticsCache.Mu.Lock()
	defer analyticsCache.Mu.Unlock()

	if analyticsCache.SiteBins == nil {
		analyticsCache.SiteBins = make(map[string]*models.HourlySiteBin)
	}

	analyticsCache.SiteBins[hourKey] = bin
	analyticsCache.LastUpdated = time.Now()
}

// GetLeadMetrics retrieves lead metrics from cache
func (m *Manager) GetLeadMetrics(tenantID string) (*models.LeadMetricsCache, bool) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	analyticsCache.Mu.RLock()
	defer analyticsCache.Mu.RUnlock()

	if analyticsCache.LeadMetrics == nil {
		return nil, false
	}

	// Check if metrics have expired
	if IsExpired(analyticsCache.LeadMetrics.ComputedAt, analyticsCache.LeadMetrics.TTL) {
		return nil, false
	}

	return analyticsCache.LeadMetrics, true
}

// SetLeadMetrics stores lead metrics in cache
func (m *Manager) SetLeadMetrics(tenantID string, metrics *models.LeadMetricsCache) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	analyticsCache.Mu.Lock()
	defer analyticsCache.Mu.Unlock()

	analyticsCache.LeadMetrics = metrics
	analyticsCache.LastUpdated = time.Now()
}

// GetDashboardData retrieves dashboard data from cache
func (m *Manager) GetDashboardData(tenantID string) (*models.DashboardCache, bool) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	analyticsCache.Mu.RLock()
	defer analyticsCache.Mu.RUnlock()

	if analyticsCache.DashboardData == nil {
		return nil, false
	}

	// Check if dashboard data has expired
	if IsExpired(analyticsCache.DashboardData.ComputedAt, analyticsCache.DashboardData.TTL) {
		return nil, false
	}

	return analyticsCache.DashboardData, true
}

// SetDashboardData stores dashboard data in cache
func (m *Manager) SetDashboardData(tenantID string, data *models.DashboardCache) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	analyticsCache.Mu.Lock()
	defer analyticsCache.Mu.Unlock()

	analyticsCache.DashboardData = data
	analyticsCache.LastUpdated = time.Now()
}

// GetHourlyEpinetRange retrieves multiple hourly epinet bins in a single operation
func (m *Manager) GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*models.HourlyEpinetBin, []string) {
	found := make(map[string]*models.HourlyEpinetBin)
	missing := make([]string, 0)

	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return found, hourKeys // All are missing
	}

	analyticsCache.Mu.RLock()
	defer analyticsCache.Mu.RUnlock()

	for _, hourKey := range hourKeys {
		binKey := fmt.Sprintf("%s:%s", epinetID, hourKey)
		bin, exists := analyticsCache.EpinetBins[binKey]

		if exists && !IsAnalyticsBinExpired(bin) {
			found[hourKey] = bin
		} else {
			missing = append(missing, hourKey)
		}
	}

	return found, missing
}

// PurgeExpiredBins removes expired analytics bins for a tenant
func (m *Manager) PurgeExpiredBins(tenantID string, olderThan string) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	analyticsCache.Mu.Lock()
	defer analyticsCache.Mu.Unlock()

	now := time.Now()

	// Parse olderThan parameter
	cutoffTime := now.Add(-72 * time.Hour) // Default: 3 days
	if olderThan != "" {
		if parsedTime, err := utils.ParseHourKeyToDate(olderThan); err == nil {
			cutoffTime = parsedTime
		}
	}

	// Purge expired epinet bins
	for binKey, bin := range analyticsCache.EpinetBins {
		if bin.ComputedAt.Before(cutoffTime) || IsAnalyticsBinExpired(bin) {
			delete(analyticsCache.EpinetBins, binKey)
		}
	}

	// Purge expired content bins
	for binKey, bin := range analyticsCache.ContentBins {
		if bin.ComputedAt.Before(cutoffTime) || IsExpired(bin.ComputedAt, bin.TTL) {
			delete(analyticsCache.ContentBins, binKey)
		}
	}

	// Purge expired site bins
	for hourKey, bin := range analyticsCache.SiteBins {
		if bin.ComputedAt.Before(cutoffTime) || IsExpired(bin.ComputedAt, bin.TTL) {
			delete(analyticsCache.SiteBins, hourKey)
		}
	}

	// Check and clear expired computed metrics
	if analyticsCache.LeadMetrics != nil && IsExpired(analyticsCache.LeadMetrics.ComputedAt, analyticsCache.LeadMetrics.TTL) {
		analyticsCache.LeadMetrics = nil
	}

	if analyticsCache.DashboardData != nil && IsExpired(analyticsCache.DashboardData.ComputedAt, analyticsCache.DashboardData.TTL) {
		analyticsCache.DashboardData = nil
	}

	analyticsCache.LastUpdated = now
}

// =============================================================================
// Analytics Cache Utility Functions
// =============================================================================

// GetAnalyticsSummary returns a summary of analytics cache usage for a tenant
func (m *Manager) GetAnalyticsSummary(tenantID string) map[string]interface{} {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return map[string]interface{}{
			"exists": false,
		}
	}

	analyticsCache.Mu.RLock()
	defer analyticsCache.Mu.RUnlock()

	return map[string]interface{}{
		"exists":         true,
		"episetBins":     len(analyticsCache.EpinetBins),
		"contentBins":    len(analyticsCache.ContentBins),
		"siteBins":       len(analyticsCache.SiteBins),
		"hasLeadMetrics": analyticsCache.LeadMetrics != nil,
		"hasDashboard":   analyticsCache.DashboardData != nil,
		"lastFullHour":   analyticsCache.LastFullHour,
		"lastUpdated":    analyticsCache.LastUpdated,
	}
}

// InvalidateAnalyticsCache clears all analytics data for a tenant
func (m *Manager) InvalidateAnalyticsCache(tenantID string) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	analyticsCache.Mu.Lock()
	defer analyticsCache.Mu.Unlock()

	// Clear all bins
	analyticsCache.EpinetBins = make(map[string]*models.HourlyEpinetBin)
	analyticsCache.ContentBins = make(map[string]*models.HourlyContentBin)
	analyticsCache.SiteBins = make(map[string]*models.HourlySiteBin)

	// Clear computed metrics
	analyticsCache.LeadMetrics = nil
	analyticsCache.DashboardData = nil

	// Reset metadata
	analyticsCache.LastFullHour = ""
	analyticsCache.LastUpdated = time.Now()
}

// UpdateLastFullHour updates the last processed hour for analytics
func (m *Manager) UpdateLastFullHour(tenantID, hourKey string) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	analyticsCache, exists := m.AnalyticsCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	analyticsCache.Mu.Lock()
	defer analyticsCache.Mu.Unlock()

	analyticsCache.LastFullHour = hourKey
	analyticsCache.LastUpdated = time.Now()
}
