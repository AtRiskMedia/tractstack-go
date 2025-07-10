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

// ComputeLeadMetrics computes lead metrics from hourly epinet data for custom range
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

	// Get hour keys for the custom time range
	hourKeys := utils.GetHourKeysForCustomRange(startHour, endHour)

	// Calculate metrics for the custom time period
	metrics := aggregateHourlyVisitorMetrics(ctx, epinets, hourKeys, knownFingerprints)

	// Calculate all-time metrics
	allHourKeys := getAllHourKeys(ctx, epinets)
	totalMetrics := aggregateHourlyVisitorMetrics(ctx, epinets, allHourKeys, knownFingerprints)

	// Calculate totals and percentages for the custom range
	totalCustomRange := len(metrics.AnonymousVisitors) + len(metrics.KnownVisitors)

	var firstTimePercentage, returningPercentage float64
	if totalCustomRange > 0 {
		firstTimePercentage = float64(len(metrics.AnonymousVisitors)) / float64(totalCustomRange) * 100
		returningPercentage = float64(len(metrics.KnownVisitors)) / float64(totalCustomRange) * 100
	}

	// Get last activity time
	lastActivity := time.Now().Format(time.RFC3339)

	return &models.LeadMetrics{
		TotalVisits:            totalMetrics.TotalVisitors,
		LastActivity:           lastActivity,
		FirstTime24h:           len(metrics.AnonymousVisitors),
		Returning24h:           len(metrics.KnownVisitors),
		FirstTime7d:            len(metrics.AnonymousVisitors),
		Returning7d:            len(metrics.KnownVisitors),
		FirstTime28d:           len(metrics.AnonymousVisitors),
		Returning28d:           len(metrics.KnownVisitors),
		FirstTime24hPercentage: firstTimePercentage,
		Returning24hPercentage: returningPercentage,
		FirstTime7dPercentage:  firstTimePercentage,
		Returning7dPercentage:  returningPercentage,
		FirstTime28dPercentage: firstTimePercentage,
		Returning28dPercentage: returningPercentage,
		TotalLeads:             totalMetrics.TotalVisitors,
	}, nil
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
