// Package analytics provides dashboard analytics computation functionality.
package analytics

import (
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// ComputeDashboardAnalytics computes dashboard analytics from cached epinet data (exact V1 pattern)
func ComputeDashboardAnalytics(ctx *tenant.Context) (*models.DashboardAnalytics, error) {
	// Get all epinets for the tenant
	epinets, err := getEpinets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets: %w", err)
	}

	if len(epinets) == 0 {
		return createEmptyDashboardAnalytics(), nil
	}

	// Get hour keys for different time periods (exact V1 pattern)
	hours24 := getHourKeysForTimeRange(24)
	hours7d := getHourKeysForTimeRange(168)  // 7 days
	hours28d := getHourKeysForTimeRange(672) // 28 days

	// Calculate stats for different time periods (exact V1 pattern)
	stats := models.TimeRangeStats{
		Daily:   computeAllEvents(ctx, epinets, hours24),
		Weekly:  computeAllEvents(ctx, epinets, hours7d),
		Monthly: computeAllEvents(ctx, epinets, hours28d),
	}

	// Generate timeline data (exact V1 pattern)
	line := computeLineData(ctx, epinets, hours7d, "weekly")

	// Identify most active content (exact V1 pattern)
	hotContent := computeHotContent(ctx, epinets, hours7d)

	return &models.DashboardAnalytics{
		Stats:      stats,
		Line:       line,
		HotContent: hotContent,
	}, nil
}

// computeAllEvents calculates total events for a given time period, using epinet data (exact V1 pattern)
func computeAllEvents(ctx *tenant.Context, epinets []EpinetConfig, hourKeys []string) int {
	total := 0
	cacheManager := cache.GetGlobalManager()

	// Iterate through all epinets (exact V1 pattern)
	for _, epinet := range epinets {
		// For each requested hour
		for _, hourKey := range hourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}

			// Count events from all steps in this hour (exact V1 pattern)
			for _, stepData := range bin.Data.Steps {
				// Each visitor counts as one event
				total += len(stepData.Visitors)
			}
		}
	}

	return total
}

// computeLineData generates line chart data for events over time (exact V1 pattern)
func computeLineData(ctx *tenant.Context, epinets []EpinetConfig, hourKeys []string, duration string) []models.LineDataSeries {
	// Determine periods based on duration (exact V1 pattern)
	periodsToDisplay := 24
	if duration == "weekly" {
		periodsToDisplay = 7
	} else if duration == "monthly" {
		periodsToDisplay = 28
	}

	// Find all event types by parsing node IDs across all epinets (exact V1 pattern)
	eventTypes := make(map[string]bool)
	eventCountsByPeriod := make(map[string][]models.LineDataPoint)

	cacheManager := cache.GetGlobalManager()
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// First, collect all event types and initialize the data structure (exact V1 pattern)
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}

			// Analyze step IDs to extract event types (verbs) (exact V1 pattern)
			for stepID := range bin.Data.Steps {
				eventType := extractEventTypeFromNodeID(stepID)
				if eventType != "" {
					eventTypes[eventType] = true
				}
			}
		}
	}

	// Initialize data structure for all event types (exact V1 pattern)
	for eventType := range eventTypes {
		eventCountsByPeriod[eventType] = make([]models.LineDataPoint, periodsToDisplay)
		for i := 0; i < periodsToDisplay; i++ {
			eventCountsByPeriod[eventType][i] = models.LineDataPoint{
				X: i,
				Y: 0,
			}
		}
	}

	// Now process hourly data to count events (exact V1 pattern)
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}

			// Calculate the period index for this hour (exact V1 pattern)
			hourTime, err := parseHourKeyToDate(hourKey)
			if err != nil {
				continue
			}

			var periodIndex int
			if duration == "daily" {
				hoursAgo := int(now.Sub(hourTime).Hours())
				if hoursAgo >= 0 && hoursAgo < periodsToDisplay {
					periodIndex = periodsToDisplay - 1 - hoursAgo
				} else {
					continue
				}
			} else {
				// Days ago (0-6 or 0-27)
				dateOnly := time.Date(hourTime.Year(), hourTime.Month(), hourTime.Day(), 0, 0, 0, 0, time.UTC)
				daysAgo := int(todayStart.Sub(dateOnly).Hours() / 24)
				if daysAgo >= 0 && daysAgo < periodsToDisplay {
					periodIndex = periodsToDisplay - 1 - daysAgo
				} else {
					continue
				}
			}

			// Process each step in this hour (exact V1 pattern)
			for stepID, stepData := range bin.Data.Steps {
				eventType := extractEventTypeFromNodeID(stepID)
				if eventType != "" && eventCountsByPeriod[eventType] != nil {
					// Add visitors to the event count for this period
					eventCountsByPeriod[eventType][periodIndex].Y += len(stepData.Visitors)
				}
			}
		}
	}

	// Format the result as LineDataSeries (exact V1 pattern)
	var result []models.LineDataSeries
	for eventType, data := range eventCountsByPeriod {
		result = append(result, models.LineDataSeries{
			ID:   eventType,
			Data: data,
		})
	}

	return result
}

// computeHotContent computes hot content by analyzing epinet steps and counting events per content ID (exact V1 pattern)
func computeHotContent(ctx *tenant.Context, epinets []EpinetConfig, hourKeys []string) []models.HotItem {
	contentCounts := make(map[string]int)
	cacheManager := cache.GetGlobalManager()

	// Process each epinet (exact V1 pattern)
	for _, epinet := range epinets {
		// For each requested hour
		for _, hourKey := range hourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				continue
			}

			// Process each step in this hour to extract content ID (exact V1 pattern)
			for stepID, stepData := range bin.Data.Steps {
				contentID := extractContentIDFromNodeID(stepID)
				if contentID != "" {
					contentCounts[contentID] += len(stepData.Visitors)
				}
			}
		}
	}

	// Format and sort the result (exact V1 pattern)
	var sortedContent []models.HotItem
	for id, totalEvents := range contentCounts {
		sortedContent = append(sortedContent, models.HotItem{
			ID:          id,
			TotalEvents: totalEvents,
		})
	}

	// Sort by total events descending (exact V1 pattern)
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
