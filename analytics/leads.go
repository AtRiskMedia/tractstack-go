// Package analytics provides lead metrics computation functionality.
package analytics

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// ComputeLeadMetrics computes lead metrics using FIXED time periods (like dashboard analytics)
// The startHour/endHour parameters are used for TotalVisits calculation only.
// All other metrics (24h/7d/28d) use their respective fixed time periods.
func ComputeLeadMetrics(ctx *tenant.Context, startHour, endHour int) (*models.LeadMetrics, error) {
	// Get all epinets for the tenant
	epinets, err := getEpinets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets: %w", err)
	}

	if len(epinets) == 0 {
		return createEmptyLeadMetrics(), nil
	}

	// Get known fingerprints for visitor classification
	knownFingerprints, err := getKnownFingerprints(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get known fingerprints: %w", err)
	}

	// Calculate metrics for FIXED time periods (matching dashboard.go pattern)
	dailyHourKeys := utils.GetHourKeysForCustomRange(24, 0)    // Last 24 hours
	weeklyHourKeys := utils.GetHourKeysForCustomRange(168, 0)  // Last 7 days (168 hours)
	monthlyHourKeys := utils.GetHourKeysForCustomRange(672, 0) // Last 28 days (672 hours)

	// Calculate visitor metrics for each time period
	dailyMetrics := aggregateHourlyVisitorMetrics(ctx, epinets, dailyHourKeys, knownFingerprints)
	weeklyMetrics := aggregateHourlyVisitorMetrics(ctx, epinets, weeklyHourKeys, knownFingerprints)
	monthlyMetrics := aggregateHourlyVisitorMetrics(ctx, epinets, monthlyHourKeys, knownFingerprints)

	// Calculate all-time metrics for TotalVisits (using custom range for now, could be all-time)
	customHourKeys := utils.GetHourKeysForCustomRange(startHour, endHour)
	customMetrics := aggregateHourlyVisitorMetrics(ctx, epinets, customHourKeys, knownFingerprints)

	// Calculate percentages for each time period
	daily24hTotal := len(dailyMetrics.AnonymousVisitors) + len(dailyMetrics.KnownVisitors)
	var firstTime24hPercentage, returning24hPercentage float64
	if daily24hTotal > 0 {
		firstTime24hPercentage = float64(len(dailyMetrics.AnonymousVisitors)) / float64(daily24hTotal) * 100
		returning24hPercentage = float64(len(dailyMetrics.KnownVisitors)) / float64(daily24hTotal) * 100
	}

	weekly7dTotal := len(weeklyMetrics.AnonymousVisitors) + len(weeklyMetrics.KnownVisitors)
	var firstTime7dPercentage, returning7dPercentage float64
	if weekly7dTotal > 0 {
		firstTime7dPercentage = float64(len(weeklyMetrics.AnonymousVisitors)) / float64(weekly7dTotal) * 100
		returning7dPercentage = float64(len(weeklyMetrics.KnownVisitors)) / float64(weekly7dTotal) * 100
	}

	monthly28dTotal := len(monthlyMetrics.AnonymousVisitors) + len(monthlyMetrics.KnownVisitors)
	var firstTime28dPercentage, returning28dPercentage float64
	if monthly28dTotal > 0 {
		firstTime28dPercentage = float64(len(monthlyMetrics.AnonymousVisitors)) / float64(monthly28dTotal) * 100
		returning28dPercentage = float64(len(monthlyMetrics.KnownVisitors)) / float64(monthly28dTotal) * 100
	}

	// Get ACTUAL lead count from database (not visitor count)
	actualLeadCount, err := getActualLeadCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get actual lead count: %w", err)
	}

	// Get last activity time
	lastActivity := time.Now().Format(time.RFC3339)

	return &models.LeadMetrics{
		TotalVisits:  customMetrics.TotalVisitors, // Based on custom range (for now)
		LastActivity: lastActivity,

		// 24-hour metrics (using ACTUAL 24-hour data)
		FirstTime24h:           len(dailyMetrics.AnonymousVisitors),
		Returning24h:           len(dailyMetrics.KnownVisitors),
		FirstTime24hPercentage: firstTime24hPercentage,
		Returning24hPercentage: returning24hPercentage,

		// 7-day metrics (using ACTUAL 7-day data)
		FirstTime7d:           len(weeklyMetrics.AnonymousVisitors),
		Returning7d:           len(weeklyMetrics.KnownVisitors),
		FirstTime7dPercentage: firstTime7dPercentage,
		Returning7dPercentage: returning7dPercentage,

		// 28-day metrics (using ACTUAL 28-day data)
		FirstTime28d:           len(monthlyMetrics.AnonymousVisitors),
		Returning28d:           len(monthlyMetrics.KnownVisitors),
		FirstTime28dPercentage: firstTime28dPercentage,
		Returning28dPercentage: returning28dPercentage,

		// ACTUAL lead count from database
		TotalLeads: actualLeadCount,
	}, nil
}

// getActualLeadCount queries the actual leads table for the real lead count
func getActualLeadCount(ctx *tenant.Context) (int, error) {
	query := `SELECT COUNT(*) FROM leads`

	var count int
	err := ctx.Database.Conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to query lead count: %w", err)
	}

	return count, nil
}

// TimeRangeMetrics holds visitor metrics for a time range (exact V1 pattern)
type TimeRangeMetrics struct {
	AnonymousVisitors map[string]bool
	KnownVisitors     map[string]bool
	TotalVisitors     int
	TotalVisits       int
	EventCounts       map[string]int
}

// aggregateHourlyVisitorMetrics aggregates visitor metrics across hours (exact V1 pattern)
func aggregateHourlyVisitorMetrics(ctx *tenant.Context, epinets []EpinetConfig, hourKeys []string, knownFingerprints map[string]bool) *TimeRangeMetrics {
	anonymousVisitors := make(map[string]bool)
	knownVisitorsMap := make(map[string]bool)
	totalVisits := 0
	eventCounts := make(map[string]int)

	cacheManager := cache.GetGlobalManager()

	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}

			for stepID, stepData := range bin.Data.Steps {
				eventType := extractEventTypeFromNodeID(stepID)
				if eventType != "" {
					eventCounts[eventType] += len(stepData.Visitors)
				}

				for visitorID := range stepData.Visitors {
					if knownFingerprints[visitorID] {
						knownVisitorsMap[visitorID] = true
					} else {
						anonymousVisitors[visitorID] = true
					}
					totalVisits++
				}
			}
		}
	}

	return &TimeRangeMetrics{
		AnonymousVisitors: anonymousVisitors,
		KnownVisitors:     knownVisitorsMap,
		TotalVisitors:     len(anonymousVisitors) + len(knownVisitorsMap),
		TotalVisits:       totalVisits,
		EventCounts:       eventCounts,
	}
}

// getAllHourKeys gets all available hour keys from cache (exact V1 pattern)
func getAllHourKeys(ctx *tenant.Context, epinets []EpinetConfig) []string {
	hourKeySet := make(map[string]bool)
	cacheManager := cache.GetGlobalManager()

	// Collect all hour keys from all epinets
	for _, epinet := range epinets {
		// This is a simplified approach - in practice, you'd need to scan the cache
		// or maintain an index of available hour keys
		hourKeys := getHourKeysForTimeRange(8760) // 1 year worth of hours
		for _, hourKey := range hourKeys {
			_, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if exists {
				hourKeySet[hourKey] = true
			}
		}
	}

	var allHourKeys []string
	for hourKey := range hourKeySet {
		allHourKeys = append(allHourKeys, hourKey)
	}

	return allHourKeys
}

// createEmptyLeadMetrics creates empty lead metrics (exact V1 pattern)
func createEmptyLeadMetrics() *models.LeadMetrics {
	return &models.LeadMetrics{
		TotalVisits:            0,
		LastActivity:           time.Now().Format(time.RFC3339),
		FirstTime24h:           0,
		Returning24h:           0,
		FirstTime7d:            0,
		Returning7d:            0,
		FirstTime28d:           0,
		Returning28d:           0,
		FirstTime24hPercentage: 0,
		Returning24hPercentage: 0,
		FirstTime7dPercentage:  0,
		Returning7dPercentage:  0,
		FirstTime28dPercentage: 0,
		Returning28dPercentage: 0,
		TotalLeads:             0,
	}
}
