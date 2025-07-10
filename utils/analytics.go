package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseHourKeyToDate parses an hour key back to a time
func ParseHourKeyToDate(hourKey string) (time.Time, error) {
	parts := strings.Split(hourKey, "-")
	if len(parts) != 4 {
		return time.Time{}, fmt.Errorf("invalid hour key format: %s", hourKey)
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid year in hour key: %s", hourKey)
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid month in hour key: %s", hourKey)
	}

	day, err := strconv.Atoi(parts[2])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day in hour key: %s", hourKey)
	}

	hour, err := strconv.Atoi(parts[3])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid hour in hour key: %s", hourKey)
	}

	return time.Date(year, time.Month(month), day, hour, 0, 0, 0, time.UTC), nil
}

// FormatHourKey formats a time as an hour key
func FormatHourKey(t time.Time) string {
	return fmt.Sprintf("%d-%02d-%02d-%02d", t.Year(), t.Month(), t.Day(), t.Hour())
}

// GetCurrentHourKey returns the current hour as a formatted key
func GetCurrentHourKey() string {
	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)
	return FormatHourKey(currentHour)
}

// GetHourKeysForTimeRange generates hour keys for the last N hours from now
func GetHourKeysForTimeRange(hours int) []string {
	var hourKeys []string
	now := time.Now().UTC()
	endHour := now.Truncate(time.Hour) // Current hour, inclusive
	startHour := endHour.Add(-time.Duration(hours) * time.Hour)

	for t := startHour; !t.After(endHour); t = t.Add(time.Hour) {
		hourKey := FormatHourKey(t)
		hourKeys = append(hourKeys, hourKey)
	}

	return hourKeys
}

// GetHourKeysForCustomRange generates hour keys for a custom range
func GetHourKeysForCustomRange(startHour, endHour int) []string {
	var hourKeys []string

	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)

	// Ensure proper order (min to max)
	minHour := endHour
	maxHour := startHour
	if startHour < endHour {
		minHour = startHour
		maxHour = endHour
	}

	for i := maxHour; i >= minHour; i-- {
		hourTime := currentHour.Add(-time.Duration(i) * time.Hour)
		hourKey := FormatHourKey(hourTime)
		hourKeys = append(hourKeys, hourKey)
	}

	return hourKeys
}

// GetMissingHoursFromZero finds missing hours from hour 0 to first cached hour
// Returns slice of hour keys that need to be loaded
func GetMissingHoursFromZero() []string {
	var missingHours []string
	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)

	// Start from hour 0 and work backwards, assuming we need to fill gap
	// This will be used by analytics code to determine what to warm
	for i := 0; i < 672; i++ { // Max 28 days back
		hourTime := currentHour.Add(-time.Duration(i) * time.Hour)
		hourKey := FormatHourKey(hourTime)
		missingHours = append(missingHours, hourKey)
	}

	return missingHours
}

// GetGapHourKeys calculates hour keys from 0 to a specific hour
func GetGapHourKeys(gapSize int) []string {
	var hourKeys []string
	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)

	for i := 0; i < gapSize; i++ {
		hourTime := currentHour.Add(-time.Duration(i) * time.Hour)
		hourKey := FormatHourKey(hourTime)
		hourKeys = append(hourKeys, hourKey)
	}

	return hourKeys
}
