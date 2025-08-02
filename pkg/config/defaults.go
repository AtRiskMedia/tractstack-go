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

func loadEnvFile() {
	envLoaded.Do(func() {
		file, err := os.Open(".env")
		if err != nil {
			return
		}
		defer file.Close()

		log.Println("Loading configuration overrides from .env file...")
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	})
}

func getEnvInt(key string, defaultValue int) int {
	if valStr := os.Getenv(key); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil {
			if val != defaultValue {
				log.Printf("Config override: %s=%d (default: %d)", key, val, defaultValue)
			}
			return val
		}
	}
	return defaultValue
}

func getEnvString(key string, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		if val != defaultValue {
			log.Printf("Config override: %s=%s (default: %s)", key, val, defaultValue)
		}
		return val
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if valStr := os.Getenv(key); valStr != "" {
		if val, err := strconv.ParseBool(valStr); err == nil {
			if val != defaultValue {
				log.Printf("Config override: %s=%t (default: %t)", key, val, defaultValue)
			}
			return val
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if valStr := os.Getenv(key); valStr != "" {
		if val, err := time.ParseDuration(valStr); err == nil {
			if val != defaultValue {
				log.Printf("Config override: %s=%s (default: %s)", key, val, defaultValue)
			}
			return val
		}
	}
	return defaultValue
}

var (
	// Server Configuration
	Port               string
	ServerReadTimeout  time.Duration
	ServerWriteTimeout time.Duration
	ServerIdleTimeout  time.Duration

	// Cache Configuration
	MaxTenants           int
	MaxMemoryMB          int
	MaxSessionsPerTenant int

	// Database Pool
	DBMaxOpenConns           int
	DBMaxIdleConns           int
	DBConnMaxLifetimeMinutes int
	DBConnMaxIdleMinutes     int

	// SSE Configuration
	MaxSessionsPerClient        int
	MaxSessionConnections       int
	SSEConnectionTimeoutMinutes int
	SSEHeartbeatIntervalSeconds int
	SSEInactivityTimeoutMinutes int

	// TTL Configuration
	ContentCacheTTL time.Duration
	UserStateTTL    time.Duration
	HTMLChunkTTL    time.Duration
	AnalyticsBinTTL time.Duration
	CurrentHourTTL  time.Duration
	LeadMetricsTTL  time.Duration
	DashboardTTL    time.Duration

	// Cleanup Intervals
	CleanupInterval           time.Duration
	TenantTimeout             time.Duration
	SSECleanupInterval        time.Duration
	DBPoolCleanupInterval     time.Duration
	RepositoryCleanupInterval time.Duration
	RepositoryCleanupVerbose  bool
)

func init() {
	loadEnvFile()

	// Server Configuration
	Port = getEnvString("PORT", "8080")
	ServerReadTimeout = getEnvDuration("SERVER_READ_TIMEOUT", 15*time.Second)
	ServerWriteTimeout = getEnvDuration("SERVER_WRITE_TIMEOUT", 15*time.Second)
	ServerIdleTimeout = getEnvDuration("SERVER_IDLE_TIMEOUT", 60*time.Second)

	// Memory Management
	MaxTenants = getEnvInt("MAX_TENANTS", 5)
	MaxMemoryMB = getEnvInt("MAX_MEMORY_MB", 768)
	MaxSessionsPerTenant = getEnvInt("MAX_SESSIONS_PER_TENANT", 5000)

	// Database Pool
	DBMaxOpenConns = getEnvInt("DB_MAX_OPEN_CONNS", 10)
	DBMaxIdleConns = getEnvInt("DB_MAX_IDLE_CONNS", 3)
	DBConnMaxLifetimeMinutes = getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 30)
	DBConnMaxIdleMinutes = getEnvInt("DB_CONN_MAX_IDLE_MINUTES", 3)

	// SSE Configuration
	MaxSessionsPerClient = getEnvInt("MAX_SESSIONS_PER_CLIENT", 10000)
	MaxSessionConnections = getEnvInt("MAX_SESSION_CONNECTIONS", 3)
	SSEConnectionTimeoutMinutes = getEnvInt("SSE_CONNECTION_TIMEOUT_MINUTES", 30)
	SSEHeartbeatIntervalSeconds = getEnvInt("SSE_HEARTBEAT_INTERVAL_SECONDS", 30)
	SSEInactivityTimeoutMinutes = getEnvInt("SSE_INACTIVITY_TIMEOUT_MINUTES", 5)

	// TTL Configuration
	ContentCacheTTL = time.Duration(getEnvInt("CONTENT_CACHE_TTL_HOURS", 24)) * time.Hour
	UserStateTTL = time.Duration(getEnvInt("USER_STATE_TTL_HOURS", 168)) * time.Hour
	HTMLChunkTTL = time.Duration(getEnvInt("HTML_CHUNK_TTL_HOURS", 1)) * time.Hour
	AnalyticsBinTTL = time.Duration(getEnvInt("ANALYTICS_BIN_TTL_DAYS", 28)) * 24 * time.Hour
	CurrentHourTTL = time.Duration(getEnvInt("CURRENT_HOUR_TTL_MINUTES", 15)) * time.Minute
	LeadMetricsTTL = time.Duration(getEnvInt("LEAD_METRICS_TTL_MINUTES", 5)) * time.Minute
	DashboardTTL = time.Duration(getEnvInt("DASHBOARD_TTL_MINUTES", 10)) * time.Minute

	// Cleanup Intervals
	CleanupInterval = time.Duration(getEnvInt("CACHE_CLEANUP_INTERVAL_MINUTES", 30)) * time.Minute
	TenantTimeout = time.Duration(getEnvInt("TENANT_TIMEOUT_HOURS", 4)) * time.Hour
	SSECleanupInterval = time.Duration(getEnvInt("SSE_CLEANUP_INTERVAL_MINUTES", 5)) * time.Minute
	DBPoolCleanupInterval = time.Duration(getEnvInt("DB_POOL_CLEANUP_INTERVAL_MINUTES", 5)) * time.Minute
	RepositoryCleanupInterval = time.Duration(getEnvInt("REPOSITORY_CLEANUP_INTERVAL", 30)) * time.Minute
	RepositoryCleanupVerbose = getEnvString("REPOSITORY_CLEANUP_VERBOSE", "true") == "true"
}
