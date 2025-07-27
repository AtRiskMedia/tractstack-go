// Package cache provides analytics caching implementation.
package cache

import (
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// =============================================================================
// Analytics Cache Implementation for Manager
// =============================================================================

// GetHourlyEpinetBin retrieves an hourly epinet bin from cache
func (m *Manager) GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*models.HourlyEpinetBin, bool) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		// log.Printf("CACHE_MISS_NO_CACHE: No analytics cache for tenant %s", tenantID)
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	binKey := fmt.Sprintf("%s:%s", epinetID, hourKey)
	bin, found := cache.EpinetBins[binKey]
	if !found {
		// log.Printf("CACHE_MISS_NO_BIN: No bin found for key %s", binKey)
		return nil, false
	}

	// Check if bin has expired using dynamic TTL calculation
	now := time.Now().UTC()
	currentHour := utils.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))

	ttl := func() time.Duration {
		if hourKey == currentHour {
			return config.CurrentHourTTL
		}
		return config.AnalyticsBinTTL
	}()

	age := now.Sub(bin.ComputedAt)
	expirationTime := bin.ComputedAt.Add(ttl)
	isExpired := expirationTime.Before(now)

	// Log detailed information about this TTL check
	// log.Printf("CACHE_TTL_CHECK: binKey=%s, currentHour=%s, isCurrentHour=%v",
	//	binKey, currentHour, hourKey == currentHour)
	// log.Printf("CACHE_TTL_TIMING: computedAt=%v, now=%v, age=%v",
	//	bin.ComputedAt.Format("15:04:05.000"), now.Format("15:04:05.000"), age)
	// log.Printf("CACHE_TTL_LOGIC: ttl=%v, expirationTime=%v, isExpired=%v",
	//	ttl, expirationTime.Format("15:04:05.000"), isExpired)

	if isExpired {
		// log.Printf("CACHE_MISS_EXPIRED: Bin for %s expired in GetHourlyEpinetBin (age=%v, ttl=%v)",
		//	binKey, age, ttl)

		// Log warning if a very new bin is being expired
		if age < 5*time.Minute {
			log.Printf("WARNING: Very fresh bin %s is being expired after only %v (ttl=%v)",
				binKey, age, ttl)
		}

		return nil, false
	}

	// log.Printf("CACHE_HIT: Bin %s found and valid (age=%v, ttl=%v)", binKey, age, ttl)
	return bin, true
}

// SetHourlyEpinetBin stores an hourly epinet bin in cache
func (m *Manager) SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *models.HourlyEpinetBin) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	if cache.EpinetBins == nil {
		cache.EpinetBins = make(map[string]*models.HourlyEpinetBin)
	}

	binKey := fmt.Sprintf("%s:%s", epinetID, hourKey)
	cache.EpinetBins[binKey] = bin
	cache.LastUpdated = time.Now()
}

// GetHourlyContentBin retrieves an hourly content bin from cache
func (m *Manager) GetHourlyContentBin(tenantID, contentID, hourKey string) (*models.HourlyContentBin, bool) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	binKey := fmt.Sprintf("%s:%s", contentID, hourKey)
	bin, found := cache.ContentBins[binKey]
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
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	if cache.ContentBins == nil {
		cache.ContentBins = make(map[string]*models.HourlyContentBin)
	}

	binKey := fmt.Sprintf("%s:%s", contentID, hourKey)
	cache.ContentBins[binKey] = bin
	cache.LastUpdated = time.Now()
}

// GetHourlySiteBin retrieves an hourly site bin from cache
func (m *Manager) GetHourlySiteBin(tenantID, hourKey string) (*models.HourlySiteBin, bool) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	bin, found := cache.SiteBins[hourKey]
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
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	if cache.SiteBins == nil {
		cache.SiteBins = make(map[string]*models.HourlySiteBin)
	}

	cache.SiteBins[hourKey] = bin
	cache.LastUpdated = time.Now()
}

// GetLeadMetrics retrieves lead metrics from cache
func (m *Manager) GetLeadMetrics(tenantID string) (*models.LeadMetricsCache, bool) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.LeadMetrics == nil {
		return nil, false
	}

	// Check if metrics have expired
	if IsExpired(cache.LeadMetrics.ComputedAt, cache.LeadMetrics.TTL) {
		return nil, false
	}

	return cache.LeadMetrics, true
}

// SetLeadMetrics stores lead metrics in cache
func (m *Manager) SetLeadMetrics(tenantID string, metrics *models.LeadMetricsCache) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.LeadMetrics = metrics
	cache.LastUpdated = time.Now()
}

// GetDashboardData retrieves dashboard data from cache
func (m *Manager) GetDashboardData(tenantID string) (*models.DashboardCache, bool) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.DashboardData == nil {
		return nil, false
	}

	// Check if dashboard data has expired
	if IsExpired(cache.DashboardData.ComputedAt, cache.DashboardData.TTL) {
		return nil, false
	}

	return cache.DashboardData, true
}

// SetDashboardData stores dashboard data in cache
func (m *Manager) SetDashboardData(tenantID string, data *models.DashboardCache) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.DashboardData = data
	cache.LastUpdated = time.Now()
}

// GetHourlyEpinetRange retrieves multiple hourly epinet bins in a single operation
func (m *Manager) GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*models.HourlyEpinetBin, []string) {
	found := make(map[string]*models.HourlyEpinetBin)
	missing := make([]string, 0)

	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return found, hourKeys // All are missing
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	for _, hourKey := range hourKeys {
		binKey := fmt.Sprintf("%s:%s", epinetID, hourKey)
		bin, exists := cache.EpinetBins[binKey]

		if exists && !IsAnalyticsBinExpired(bin, hourKey) {
			found[hourKey] = bin
		} else {
			missing = append(missing, hourKey)
		}
	}

	return found, missing
}

// PurgeExpiredBins removes expired analytics bins for a tenant
func (m *Manager) PurgeExpiredBins(tenantID string, olderThan string) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	now := time.Now()

	// Parse olderThan parameter
	cutoffTime := now.Add(-72 * time.Hour) // Default: 3 days
	if olderThan != "" {
		if parsedTime, err := utils.ParseHourKeyToDate(olderThan); err == nil {
			cutoffTime = parsedTime
		}
	}

	// Purge expired epinet bins
	for binKey, bin := range cache.EpinetBins {
		if bin.ComputedAt.Before(cutoffTime) || IsAnalyticsBinExpired(bin, binKey) {
			delete(cache.EpinetBins, binKey)
		}
	}

	// Purge expired content bins
	for binKey, bin := range cache.ContentBins {
		if bin.ComputedAt.Before(cutoffTime) || IsExpired(bin.ComputedAt, bin.TTL) {
			delete(cache.ContentBins, binKey)
		}
	}

	// Purge expired site bins
	for hourKey, bin := range cache.SiteBins {
		if bin.ComputedAt.Before(cutoffTime) || IsExpired(bin.ComputedAt, bin.TTL) {
			delete(cache.SiteBins, hourKey)
		}
	}

	// Check and clear expired computed metrics
	if cache.LeadMetrics != nil && IsExpired(cache.LeadMetrics.ComputedAt, cache.LeadMetrics.TTL) {
		cache.LeadMetrics = nil
	}

	if cache.DashboardData != nil && IsExpired(cache.DashboardData.ComputedAt, cache.DashboardData.TTL) {
		cache.DashboardData = nil
	}

	cache.LastUpdated = now
}
