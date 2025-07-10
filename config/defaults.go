// Package config provides centralized default values for TractStack
package config

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var envLoaded sync.Once

// loadEnvFile loads environment variables from .env file
func loadEnvFile() {
	envLoaded.Do(func() {
		loadEnvFileOnce()
	})
}

func loadEnvFileOnce() {
	file, err := os.Open(".env")
	if err != nil {
		// .env file is optional, don't error if it doesn't exist
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first = sign
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Only set if not already set in environment
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

func init() {
	// Ensure .env is loaded before any config access
	loadEnvFile()
}

// getEnvInt reads environment variable with fallback to default
func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getEnvString reads environment variable with string fallback
func getEnvString(key string, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// getEnvDuration reads environment variable as duration with fallback
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil {
			return parsed
		}
		// Try as integer seconds
		if seconds, err := strconv.Atoi(val); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultValue
}

// Server Configuration
var (
	Port = getEnvString("PORT", "8080")
)

// Cache Configuration
var (
	// Memory Management
	MaxTenants           = getEnvInt("MAX_TENANTS", 5)
	MaxMemoryMB          = getEnvInt("MAX_MEMORY_MB", 768)
	MaxSessionsPerTenant = getEnvInt("MAX_SESSIONS_PER_TENANT", 5000)

	// Database Pool
	DBMaxOpenConns           = getEnvInt("DB_MAX_OPEN_CONNS", 10)
	DBMaxIdleConns           = getEnvInt("DB_MAX_IDLE_CONNS", 3)
	DBConnMaxLifetimeMinutes = getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 30)
	DBConnMaxIdleMinutes     = getEnvInt("DB_CONN_MAX_IDLE_MINUTES", 3)

	// SSE Configuration
	MaxSessionsPerClient        = getEnvInt("MAX_SESSIONS_PER_CLIENT", 10000)
	MaxSessionConnections       = getEnvInt("MAX_SESSION_CONNECTIONS", 3)
	SSEConnectionTimeoutMinutes = getEnvInt("SSE_CONNECTION_TIMEOUT_MINUTES", 30)
	SSEHeartbeatIntervalSeconds = getEnvInt("SSE_HEARTBEAT_INTERVAL_SECONDS", 30)
	SSEInactivityTimeoutMinutes = getEnvInt("SSE_INACTIVITY_TIMEOUT_MINUTES", 5)
)

// TTL Configuration
var (
	ContentCacheTTL = time.Duration(getEnvInt("CONTENT_CACHE_TTL_HOURS", 24)) * time.Hour
	UserStateTTL    = time.Duration(getEnvInt("USER_STATE_TTL_HOURS", 2)) * time.Hour
	HTMLChunkTTL    = time.Duration(getEnvInt("HTML_CHUNK_TTL_HOURS", 1)) * time.Hour
	AnalyticsBinTTL = time.Duration(getEnvInt("ANALYTICS_BIN_TTL_DAYS", 28)) * 24 * time.Hour
	CurrentHourTTL  = time.Duration(getEnvInt("CURRENT_HOUR_TTL_MINUTES", 15)) * time.Minute
	LeadMetricsTTL  = time.Duration(getEnvInt("LEAD_METRICS_TTL_MINUTES", 5)) * time.Minute
	DashboardTTL    = time.Duration(getEnvInt("DASHBOARD_TTL_MINUTES", 10)) * time.Minute
)

// Cleanup Intervals
var (
	CleanupInterval       = time.Duration(getEnvInt("CACHE_CLEANUP_INTERVAL_MINUTES", 30)) * time.Minute
	TenantTimeout         = time.Duration(getEnvInt("TENANT_TIMEOUT_HOURS", 4)) * time.Hour
	SSECleanupInterval    = time.Duration(getEnvInt("SSE_CLEANUP_INTERVAL_MINUTES", 5)) * time.Minute
	DBPoolCleanupInterval = time.Duration(getEnvInt("DB_POOL_CLEANUP_INTERVAL_MINUTES", 5)) * time.Minute
)

func init() {
	log.Printf("DEBUG: Config loaded CURRENT_HOUR_TTL_MINUTES env, CurrentHourTTL=%s", CurrentHourTTL)
}
