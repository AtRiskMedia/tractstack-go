// Package analytics provides cache validation functions for epinet data.
package analytics

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

type RangeCacheStatus struct {
	Action             string // "proceed", "refresh_current", "load_range"
	CurrentHourExpired bool
	HistoricalComplete bool
	MissingHours       []string
}

func GetRangeCacheStatus(ctx *tenant.Context, epinetID string, startHour, endHour int) RangeCacheStatus {
	cacheManager := cache.GetGlobalManager()
	hourKeys := utils.GetHourKeysForCustomRange(startHour, endHour)

	now := time.Now().UTC()
	currentHour := utils.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))
	// log.Printf("DEBUG: Current hour key from utils.FormatHourKey: %s", currentHour)

	var missingHours []string
	currentHourExpired := false
	historicalMissing := false

	for _, hourKey := range hourKeys {
		bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey)

		ttl := func() time.Duration {
			if hourKey == currentHour {
				return config.CurrentHourTTL
			}
			return config.AnalyticsBinTTL
		}()

		isExpired := exists && bin.ComputedAt.Add(ttl).Before(time.Now().UTC())

		if !exists || isExpired {
			missingHours = append(missingHours, hourKey)
			if hourKey == currentHour {
				currentHourExpired = true
			} else {
				historicalMissing = true
			}
		}
	}

	// Determine action
	var action string
	if len(missingHours) == 0 {
		action = "proceed"
	} else if currentHourExpired && !historicalMissing {
		action = "refresh_current"
	} else {
		action = "load_range"
	}

	return RangeCacheStatus{
		Action:             action,
		CurrentHourExpired: currentHourExpired,
		HistoricalComplete: !historicalMissing,
		MissingHours:       missingHours,
	}
}

// FindCacheGap returns missing hour keys from 0 to first cached hour
func FindCacheGap(ctx *tenant.Context, epinetID string) []string {
	cacheManager := cache.GetGlobalManager()
	var gapHours []string

	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)
	currentHourKey := utils.FormatHourKey(currentHour)

	// Start from hour 0 and check forward until finding first cached hour
	for i := 0; i < 672; i++ { // Max 28 days (672 hours)
		hourTime := currentHour.Add(-time.Duration(i) * time.Hour)
		hourKey := utils.FormatHourKey(hourTime)

		bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey)

		// Use existing TTL expiration logic
		ttl := func() time.Duration {
			if hourKey == currentHourKey {
				return config.CurrentHourTTL
			}
			return config.AnalyticsBinTTL
		}()

		isExpired := exists && bin.ComputedAt.Add(ttl).Before(time.Now().UTC())

		if !exists || isExpired {
			// This hour is missing - add to gap
			gapHours = append(gapHours, hourKey)
		} else {
			// Found first cached hour - stop here
			break
		}
	}

	return gapHours
}

// IsBulkCacheInitialized checks if initial 672-hour bulk load has occurred
func IsBulkCacheInitialized(ctx *tenant.Context, epinetID string) bool {
	cacheManager := cache.GetGlobalManager()

	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)
	currentHourKey := utils.FormatHourKey(currentHour)

	cachedCount := 0

	// Check a sample of hours across the 672-hour range
	for i := 0; i < 672; i += 24 { // Check every 24th hour (daily sample)
		hourTime := currentHour.Add(-time.Duration(i) * time.Hour)
		hourKey := utils.FormatHourKey(hourTime)

		bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey)
		if !exists {
			continue
		}

		// Check if not expired
		ttl := func() time.Duration {
			if hourKey == currentHourKey {
				return config.CurrentHourTTL
			}
			return config.AnalyticsBinTTL
		}()

		if !bin.ComputedAt.Add(ttl).Before(time.Now().UTC()) {
			cachedCount++
		}
	}

	// Consider bulk cache initialized if we have at least 20 valid cached hours
	// out of our 28 daily samples (indicates significant initial loading occurred)
	return cachedCount >= 20
}
