// Package analytics provides lead metrics computation functionality.
package analytics

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// ComputeLeadMetrics computes lead metrics from hourly epinet data (exact V1 pattern)
func ComputeLeadMetrics(ctx *tenant.Context) (*models.LeadMetrics, error) {
	// Get all epinets for the tenant
	epinets, err := getEpinets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets: %w", err)
	}

	if len(epinets) == 0 {
		return createEmptyLeadMetrics(), nil
	}

	// Get known fingerprints for visitor classification (exact V1 pattern)
	knownFingerprints, err := getKnownFingerprints(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get known fingerprints: %w", err)
	}

	// Get hour keys for different time periods (exact V1 pattern)
	hours24 := getHourKeysForTimeRange(24)
	hours7d := getHourKeysForTimeRange(168)  // 7 days
	hours28d := getHourKeysForTimeRange(672) // 28 days

	// Calculate metrics for each time period (exact V1 pattern)
	metrics24h := aggregateHourlyVisitorMetrics(ctx, epinets, hours24, knownFingerprints)
	metrics7d := aggregateHourlyVisitorMetrics(ctx, epinets, hours7d, knownFingerprints)
	metrics28d := aggregateHourlyVisitorMetrics(ctx, epinets, hours28d, knownFingerprints)

	// Calculate all-time metrics
	allHourKeys := getAllHourKeys(ctx, epinets)
	totalMetrics := aggregateHourlyVisitorMetrics(ctx, epinets, allHourKeys, knownFingerprints)

	// Calculate totals and percentages (exact V1 pattern)
	total24h := len(metrics24h.AnonymousVisitors) + len(metrics24h.KnownVisitors)
	total7d := len(metrics7d.AnonymousVisitors) + len(metrics7d.KnownVisitors)
	total28d := len(metrics28d.AnonymousVisitors) + len(metrics28d.KnownVisitors)

	var firstTime24hPercentage, returning24hPercentage float64
	var firstTime7dPercentage, returning7dPercentage float64
	var firstTime28dPercentage, returning28dPercentage float64

	if total24h > 0 {
		firstTime24hPercentage = float64(len(metrics24h.AnonymousVisitors)) / float64(total24h) * 100
		returning24hPercentage = float64(len(metrics24h.KnownVisitors)) / float64(total24h) * 100
	}

	if total7d > 0 {
		firstTime7dPercentage = float64(len(metrics7d.AnonymousVisitors)) / float64(total7d) * 100
		returning7dPercentage = float64(len(metrics7d.KnownVisitors)) / float64(total7d) * 100
	}

	if total28d > 0 {
		firstTime28dPercentage = float64(len(metrics28d.AnonymousVisitors)) / float64(total28d) * 100
		returning28dPercentage = float64(len(metrics28d.KnownVisitors)) / float64(total28d) * 100
	}

	// Get last activity time
	lastActivity := time.Now().Format(time.RFC3339)

	return &models.LeadMetrics{
		TotalVisits:            totalMetrics.TotalVisitors,
		LastActivity:           lastActivity,
		FirstTime24h:           len(metrics24h.AnonymousVisitors),
		Returning24h:           len(metrics24h.KnownVisitors),
		FirstTime7d:            len(metrics7d.AnonymousVisitors),
		Returning7d:            len(metrics7d.KnownVisitors),
		FirstTime28d:           len(metrics28d.AnonymousVisitors),
		Returning28d:           len(metrics28d.KnownVisitors),
		FirstTime24hPercentage: firstTime24hPercentage,
		Returning24hPercentage: returning24hPercentage,
		FirstTime7dPercentage:  firstTime7dPercentage,
		Returning7dPercentage:  returning7dPercentage,
		FirstTime28dPercentage: firstTime28dPercentage,
		Returning28dPercentage: returning28dPercentage,
		TotalLeads:             len(knownFingerprints),
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
