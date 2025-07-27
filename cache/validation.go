// Package cache provides cache validation functions for analytics data.
package cache

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

func GetRangeCacheStatus(ctx *tenant.Context, epinetID string, startHour, endHour int) models.RangeCacheStatus {
	cacheManager := GetGlobalManager()
	hourKeys := utils.GetHourKeysForCustomRange(startHour, endHour)

	now := time.Now().UTC()
	currentHour := utils.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))

	var missingHours []string
	currentHourExpired := false
	historicalMissing := false

	for _, hourKey := range hourKeys {
		// GetHourlyEpinetBin is now the single source of truth. It returns false
		// if the bin doesn't exist OR if it's expired. We no longer need
		// to perform a second, redundant expiration check here.
		_, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey)

		if !exists {
			missingHours = append(missingHours, hourKey)
			if hourKey == currentHour {
				currentHourExpired = true
			} else {
				historicalMissing = true
			}
		}
	}

	// Determine action based on the findings
	var action string
	if len(missingHours) == 0 {
		action = "proceed"
	} else if currentHourExpired && !historicalMissing {
		// This action can be used to trigger a quick refresh of only the current hour's data
		action = "refresh_current"
	} else {
		// This action triggers the full range warming process
		action = "load_range"
	}

	return models.RangeCacheStatus{
		Action:             action,
		CurrentHourExpired: currentHourExpired,
		HistoricalComplete: !historicalMissing,
		MissingHours:       missingHours,
	}
}

// FindCacheGap returns missing hour keys from 0 to first cached hour
func FindCacheGap(ctx *tenant.Context, epinetID string) []string {
	cacheManager := GetGlobalManager()
	var gapHours []string

	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)

	// Start from hour 0 and check forward until finding first cached hour
	for i := 0; i < 672; i++ { // Max 28 days (672 hours)
		hourTime := currentHour.Add(-time.Duration(i) * time.Hour)
		hourKey := utils.FormatHourKey(hourTime)

		// Rely on the single source of truth for existence and expiration.
		_, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey)

		if !exists {
			gapHours = append(gapHours, hourKey)
		} else {
			// Found first cached hour, stop searching
			break
		}
	}

	return gapHours
}
