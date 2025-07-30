// Package cleanup provides configuration for cache cleanup operations
package cleanup

import (
	"os"
	"strconv"
	"time"
)

// Config holds cleanup worker configuration
type Config struct {
	CleanupInterval   time.Duration
	VerboseReporting  bool
	ContentCacheTTL   time.Duration
	SessionCacheTTL   time.Duration
	AnalyticsCacheTTL time.Duration
	FragmentCacheTTL  time.Duration
}

// NewConfig creates cleanup configuration from environment variables
func NewConfig() *Config {
	return &Config{
		CleanupInterval:   getCleanupInterval(),
		VerboseReporting:  getVerboseReporting(),
		ContentCacheTTL:   getContentCacheTTL(),
		SessionCacheTTL:   getSessionCacheTTL(),
		AnalyticsCacheTTL: getAnalyticsCacheTTL(),
		FragmentCacheTTL:  getFragmentCacheTTL(),
	}
}

// getCleanupInterval reads REPOSITORY_CLEANUP_INTERVAL from env (default: 5 minutes)
func getCleanupInterval() time.Duration {
	intervalStr := os.Getenv("REPOSITORY_CLEANUP_INTERVAL")
	if intervalStr == "" {
		return 5 * time.Minute // Default
	}

	interval, err := strconv.Atoi(intervalStr)
	if err != nil {
		return 5 * time.Minute // Default on error
	}

	return time.Duration(interval) * time.Minute
}

// getVerboseReporting reads REPOSITORY_CLEANUP_VERBOSE from env (default: false)
func getVerboseReporting() bool {
	verboseStr := os.Getenv("REPOSITORY_CLEANUP_VERBOSE")
	if verboseStr == "" {
		return false // Default
	}

	verbose, err := strconv.ParseBool(verboseStr)
	if err != nil {
		return false // Default on error
	}

	return verbose
}

// getContentCacheTTL gets content cache TTL (default: 24 hours)
func getContentCacheTTL() time.Duration {
	ttlStr := os.Getenv("CONTENT_CACHE_TTL")
	if ttlStr == "" {
		return 24 * time.Hour // Default
	}

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return 24 * time.Hour // Default on error
	}

	return ttl
}

// getSessionCacheTTL gets session cache TTL (default: 2 hours)
func getSessionCacheTTL() time.Duration {
	ttlStr := os.Getenv("SESSION_CACHE_TTL")
	if ttlStr == "" {
		return 2 * time.Hour // Default
	}

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return 2 * time.Hour // Default on error
	}

	return ttl
}

// getAnalyticsCacheTTL gets analytics cache TTL (default: 6 hours)
func getAnalyticsCacheTTL() time.Duration {
	ttlStr := os.Getenv("ANALYTICS_CACHE_TTL")
	if ttlStr == "" {
		return 6 * time.Hour // Default
	}

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return 6 * time.Hour // Default on error
	}

	return ttl
}

// getFragmentCacheTTL gets fragment cache TTL (default: 24 hours)
func getFragmentCacheTTL() time.Duration {
	ttlStr := os.Getenv("FRAGMENT_CACHE_TTL")
	if ttlStr == "" {
		return 24 * time.Hour // Default
	}

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return 24 * time.Hour // Default on error
	}

	return ttl
}
