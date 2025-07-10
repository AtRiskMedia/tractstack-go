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
