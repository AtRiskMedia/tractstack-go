// Package analytics provides dashboard analytics computation functionality.
package analytics

import (
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// ComputeDashboardAnalytics computes dashboard analytics from cached epinet data for custom range
func ComputeDashboardAnalytics(ctx *tenant.Context, startHour, endHour int) (*models.DashboardAnalytics, error) {
	// Get all epinets for the tenant
	epinets, err := getEpinets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets: %w", err)
	}

	if len(epinets) == 0 {
		return createEmptyDashboardAnalytics(), nil
	}

	// Get hour keys for the custom time range (for line data and hot content)
	hourKeys := utils.GetHourKeysForCustomRange(startHour, endHour)

	// Calculate stats for FIXED time periods (not custom range)
	// Each stat should represent its specific time period
	dailyHourKeys := utils.GetHourKeysForCustomRange(24, 0)    // Last 24 hours
	weeklyHourKeys := utils.GetHourKeysForCustomRange(168, 0)  // Last 7 days (168 hours)
	monthlyHourKeys := utils.GetHourKeysForCustomRange(672, 0) // Last 28 days (672 hours)

	stats := models.TimeRangeStats{
		Daily:   computeAllEvents(ctx, epinets, dailyHourKeys),
		Weekly:  computeAllEvents(ctx, epinets, weeklyHourKeys),
		Monthly: computeAllEvents(ctx, epinets, monthlyHourKeys),
	}

	// Generate timeline data for the custom range (this should use the requested range)
	line := computeLineData(ctx, epinets, hourKeys, "custom")

	// Identify most active content for the custom range
	hotContent := computeHotContent(ctx, epinets, hourKeys)

	return &models.DashboardAnalytics{
		Stats:      stats,
		Line:       line,
		HotContent: hotContent,
	}, nil
}

// computeAllEvents calculates total events for a custom time period, using epinet data
func computeAllEvents(ctx *tenant.Context, epinets []EpinetConfig, hourKeys []string) int {
	total := 0
	cacheManager := cache.GetGlobalManager()

	// Iterate through all epinets
	for _, epinet := range epinets {
		// For each requested hour
		for _, hourKey := range hourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}

			// Count events from all steps in this hour
			for _, stepData := range bin.Data.Steps {
				total += len(stepData.Visitors)
			}
		}
	}

	return total
}

// computeLineData creates timeline visualization data for custom time range
func computeLineData(ctx *tenant.Context, epinets []EpinetConfig, hourKeys []string, timeframe string) []models.LineDataSeries {
	cacheManager := cache.GetGlobalManager()

	// Aggregate data by hour across all epinets
	hourlyTotals := make(map[string]int)

	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}

			// Count events for this hour
			hourTotal := 0
			for _, stepData := range bin.Data.Steps {
				hourTotal += len(stepData.Visitors)
			}

			hourlyTotals[hourKey] += hourTotal
		}
	}

	// Convert to line data format
	var dataPoints []models.LineDataPoint
	for _, hourKey := range hourKeys {
		total := hourlyTotals[hourKey]
		dataPoints = append(dataPoints, models.LineDataPoint{
			X: hourKey,
			Y: total,
		})
	}

	return []models.LineDataSeries{
		{
			ID:   "events",
			Data: dataPoints,
		},
	}
}

// computeHotContent identifies the most active content for custom time range
func computeHotContent(ctx *tenant.Context, epinets []EpinetConfig, hourKeys []string) []models.HotItem {
	cacheManager := cache.GetGlobalManager()
	contentCounts := make(map[string]int)

	// Aggregate content activity across all epinets and hours
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}

			// Count events by content ID
			for stepID, stepData := range bin.Data.Steps {
				contentID := extractContentIDFromNodeID(stepID)
				if contentID != "" {
					contentCounts[contentID] += len(stepData.Visitors)
				}
			}
		}
	}

	// Format and sort the result
	var sortedContent []models.HotItem
	for id, totalEvents := range contentCounts {
		sortedContent = append(sortedContent, models.HotItem{
			ID:          id,
			TotalEvents: totalEvents,
		})
	}

	// Sort by total events descending
	for i := 0; i < len(sortedContent)-1; i++ {
		for j := i + 1; j < len(sortedContent); j++ {
			if sortedContent[j].TotalEvents > sortedContent[i].TotalEvents {
				sortedContent[i], sortedContent[j] = sortedContent[j], sortedContent[i]
			}
		}
	}

	return sortedContent
}

// extractEventTypeFromNodeID extracts the event type (verb) from a node ID (exact V1 pattern)
func extractEventTypeFromNodeID(nodeID string) string {
	parts := strings.Split(nodeID, "-")

	if len(parts) < 2 {
		return ""
	}

	switch parts[0] {
	case "belief":
		// Format: belief-VERB-contentID
		if len(parts) >= 2 {
			return parts[1]
		}
	case "identifyAs":
		// Format: identifyAs-OBJECT-contentID
		return "IDENTIFY_AS"
	case "commitmentAction", "conversionAction":
		// Format: commitmentAction-StoryFragment-VERB-contentID
		if len(parts) >= 3 {
			return parts[2]
		}
	}

	return ""
}

// extractContentIDFromNodeID extracts the content ID from a node ID (exact V1 pattern)
func extractContentIDFromNodeID(nodeID string) string {
	parts := strings.Split(nodeID, "-")

	if len(parts) < 2 {
		return ""
	}

	// Content ID is always the last part (exact V1 pattern)
	return parts[len(parts)-1]
}

// createEmptyDashboardAnalytics creates an empty dashboard analytics object (exact V1 pattern)
func createEmptyDashboardAnalytics() *models.DashboardAnalytics {
	return &models.DashboardAnalytics{
		Stats: models.TimeRangeStats{
			Daily:   0,
			Weekly:  0,
			Monthly: 0,
		},
		Line:       []models.LineDataSeries{},
		HotContent: []models.HotItem{},
	}
}
