// Package config provides centralized default values for TractStack
package config

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
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
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				log.Printf("Warning: Failed to close .env file: %v", closeErr)
			}
		}()

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
			value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)

			if os.Getenv(key) == "" {
				if setErr := os.Setenv(key, value); setErr != nil {
					log.Printf("Warning: Failed to set environment variable %s: %v", key, setErr)
				}
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			log.Printf("Warning: Error reading .env file: %v", scanErr)
		}
	})
}

func getEnvInt(key string, defaultValue int) int {
	if valStr := os.Getenv(key); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil {
			if val != defaultValue {
				if strings.Contains(strings.ToUpper(key), "SECRET") {
					log.Printf("Config override: %s is set", key)
				} else {
					log.Printf("Config override: %s=%d (default: %d)", key, val, defaultValue)
				}
			}
			return val
		}
	}
	return defaultValue
}

func getEnvString(key string, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		if val != defaultValue {
			if strings.Contains(strings.ToUpper(key), "SECRET") {
				log.Printf("Config override: %s is set", key)
			} else {
				log.Printf("Config override: %s=%s (default: %s)", key, val, defaultValue)
			}
		}
		return val
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if valStr := os.Getenv(key); valStr != "" {
		if val, err := strconv.ParseBool(valStr); err == nil {
			if val != defaultValue {
				if strings.Contains(strings.ToUpper(key), "SECRET") {
					log.Printf("Config override: %s is set", key)
				} else {
					log.Printf("Config override: %s=%t (default: %t)", key, val, defaultValue)
				}
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
				if strings.Contains(strings.ToUpper(key), "SECRET") {
					log.Printf("Config override: %s is set", key)
				} else {
					log.Printf("Config override: %s=%s (default: %s)", key, val, defaultValue)
				}
			}
			return val
		}
	}
	return defaultValue
}

var (
	// BackendPath defines the root directory for backend storage and configuration.
	BackendPath string

	// Port specifies the network port the server listens on.
	Port string
	// ServerReadTimeout is the maximum duration for reading the entire request, including the body.
	ServerReadTimeout time.Duration
	// ServerWriteTimeout is the maximum duration the server will wait for a write to complete.
	ServerWriteTimeout time.Duration
	// ServerIdleTimeout is the maximum amount of time to wait for the next request when keep-alives are enabled.
	ServerIdleTimeout time.Duration

	// EnableMultiTenant indicates whether multi-tenant support is active.
	EnableMultiTenant bool

	// ShopifyReconcileInterval is the frequency at which the system performs a full reconciliation scan of Shopify products.
	ShopifyReconcileInterval time.Duration
	// ShopifyReconcileStartupDelay is the initial delay after server startup before the first reconciliation scan begins.
	ShopifyReconcileStartupDelay time.Duration
	// ShopifyDataTTL is the cache interval for shopify data
	ShopifyDataTTL time.Duration
	// ShopifyRequestTimeout is the maximum duration to wait for a Shopify API response
	ShopifyRequestTimeout time.Duration

	// MaxTenants sets the maximum number of tenants allowed in memory.
	MaxTenants int
	// MaxMemoryMB sets the soft memory limit for the application in megabytes.
	MaxMemoryMB int
	// MaxSessionsPerTenant is the limit on active sessions allowed for a single tenant.
	MaxSessionsPerTenant int

	// DBMaxOpenConns sets the maximum number of open connections to the database.
	DBMaxOpenConns int
	// DBMaxIdleConns defines the maximum number of idle connections allowed in the database pool.
	DBMaxIdleConns int
	// DBConnMaxLifetimeMinutes defines the maximum number of minutes a database connection may be reused.
	DBConnMaxLifetimeMinutes int
	// DBConnMaxIdleMinutes defines the maximum number of minutes a database connection can remain idle in the pool.
	DBConnMaxIdleMinutes int
	// SlowQueryThreshold defines the duration after which a database query is logged as a slow operation.
	SlowQueryThreshold time.Duration

	// MaxSessionsPerClient is the limit on active sessions allowed from a single client IP.
	MaxSessionsPerClient int
	// MaxSessionConnections defines the maximum number of concurrent active session connections allowed globally.
	MaxSessionConnections int
	// SSEConnectionTimeoutMinutes defines the maximum duration in minutes that a Server-Sent Events connection is allowed to remain open before being cycled.
	SSEConnectionTimeoutMinutes int
	// SSEHeartbeatIntervalSeconds defines the frequency at which keep-alive signals are sent over SSE connections.
	SSEHeartbeatIntervalSeconds int
	// SSEInactivityTimeoutMinutes defines the duration a Server-Sent Events connection can remain idle before being closed.
	SSEInactivityTimeoutMinutes int

	// ContentCacheTTL is the time-to-live for cached content items.
	ContentCacheTTL time.Duration
	// UserStateTTL defines the duration before cached visitor belief states are considered stale.
	UserStateTTL time.Duration
	// HTMLChunkTTL defines the duration that rendered HTML fragments remain in the cache before expiration.
	HTMLChunkTTL time.Duration
	// AnalyticsBinTTL defines the duration that aggregated analytics bins are kept in the cache.
	AnalyticsBinTTL time.Duration
	// CurrentHourTTL defines the time-to-live duration for analytics data pertaining to the current active hour.
	CurrentHourTTL time.Duration
	// LeadMetricsTTL defines the duration that calculated lead performance metrics remain valid in the cache.
	LeadMetricsTTL time.Duration
	// DashboardTTL defines the duration that aggregated dashboard data remains valid in the cache.
	DashboardTTL time.Duration

	// BookingHoldTimeout defines how long a pending booking is held
	BookingHoldTimeout time.Duration

	// CleanupInterval specifies how often background cleanup tasks run.
	CleanupInterval time.Duration
	// TenantTimeout is the duration of inactivity after which a tenant's resources are unloaded from memory.
	TenantTimeout time.Duration
	// SSECleanupInterval is the frequency at which the system checks for and removes stale SSE connections.
	SSECleanupInterval time.Duration
	// DBPoolCleanupInterval is the frequency at which the system performs maintenance on the database connection pool.
	DBPoolCleanupInterval time.Duration
	// RepositoryCleanupInterval is the frequency at which the system performs garbage collection on repository caches.
	RepositoryCleanupInterval time.Duration
	// RepositoryCleanupVerbose determines if detailed logs should be emitted during repository cleanup operations.
	RepositoryCleanupVerbose bool

	// SSLEnabled indicates if the server should run with SSL/TLS encryption.
	SSLEnabled bool
	// SSLCertPath is the file path to the SSL certificate.
	SSLCertPath string
	// SSLKeyPath is the file path to the SSL private key.
	SSLKeyPath string
	// BindAddress is the specific network interface address the server binds to.
	BindAddress string

	// CollectionRoutes defines the list of category slugs that should be treated as collections.
	CollectionRoutes []string

	// LogVerbosity sets the level of logging detail (e.g., DEBUG, INFO, WARN).
	LogVerbosity string

	// SysopPassword is the password required for accessing system operator endpoints.
	SysopPassword string

	// SandboxSecret is the secret key used to authorize access to sandbox features.
	SandboxSecret string

	// ExposeAnalytics indicates whether analytics data should be exposed via API.
	ExposeAnalytics bool
)

func parseCollectionRoutes(value string) []string {
	if value == "" {
		return []string{}
	}
	routes := strings.Split(value, ",")
	for i, route := range routes {
		routes[i] = strings.TrimSpace(route)
	}
	return routes
}

func init() {
	loadEnvFile()

	// Folder location
	homeDir, _ := os.UserHomeDir()
	BackendPath = getEnvString("GO_BACKEND_PATH", filepath.Join(homeDir, "t8k-go-server"))

	// Server Configuration
	Port = getEnvString("PORT", "8080")
	ServerReadTimeout = getEnvDuration("SERVER_READ_TIMEOUT", 15*time.Second)
	ServerWriteTimeout = getEnvDuration("SERVER_WRITE_TIMEOUT", 15*time.Second)
	ServerIdleTimeout = getEnvDuration("SERVER_IDLE_TIMEOUT", 60*time.Second)

	// Multi-tenant Configuration
	EnableMultiTenant = getEnvBool("ENABLE_MULTI_TENANT", false)

	// Shopify service
	ShopifyReconcileInterval = time.Duration(getEnvInt("SHOPIFY_RECONCILE_INTERVAL_HOURS", 48)) * time.Hour
	ShopifyReconcileStartupDelay = time.Duration(getEnvInt("SHOPIFY_RECONCILE_STARTUP_DELAY_MINUTES", 48)) * time.Minute
	ShopifyDataTTL = time.Duration(getEnvInt("SHOPIFY_DATA_TTL", 5)) * time.Minute
	ShopifyRequestTimeout = getEnvDuration("SHOPIFY_REQUEST_TIMEOUT", 30*time.Second)

	// Memory Management
	MaxTenants = getEnvInt("MAX_TENANTS", 0)
	MaxMemoryMB = getEnvInt("MAX_MEMORY_MB", 512)
	MaxSessionsPerTenant = getEnvInt("MAX_SESSIONS_PER_TENANT", 5000)

	// Database Pool
	DBMaxOpenConns = getEnvInt("DB_MAX_OPEN_CONNS", 0)
	DBMaxIdleConns = getEnvInt("DB_MAX_IDLE_CONNS", 50)
	DBConnMaxLifetimeMinutes = getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 15)
	DBConnMaxIdleMinutes = getEnvInt("DB_CONN_MAX_IDLE_MINUTES", 5)
	SlowQueryThreshold = getEnvDuration("SLOW_QUERY_THRESHOLD", 500*time.Millisecond)

	// Bookings
	BookingHoldTimeout = time.Duration(getEnvInt("BOOKING_HOLD_TIMEOUT_MINUTES", 50)) * time.Minute

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
	CurrentHourTTL = time.Duration(getEnvInt("CURRENT_HOUR_TTL_MINUTES", 5)) * time.Minute
	LeadMetricsTTL = time.Duration(getEnvInt("LEAD_METRICS_TTL_MINUTES", 5)) * time.Minute
	DashboardTTL = time.Duration(getEnvInt("DASHBOARD_TTL_MINUTES", 10)) * time.Minute

	// Cleanup Intervals
	CleanupInterval = time.Duration(getEnvInt("CACHE_CLEANUP_INTERVAL_MINUTES", 30)) * time.Minute
	TenantTimeout = time.Duration(getEnvInt("TENANT_TIMEOUT_HOURS", 4)) * time.Hour
	SSECleanupInterval = time.Duration(getEnvInt("SSE_CLEANUP_INTERVAL_MINUTES", 5)) * time.Minute
	DBPoolCleanupInterval = time.Duration(getEnvInt("DB_POOL_CLEANUP_INTERVAL_MINUTES", 5)) * time.Minute
	RepositoryCleanupInterval = time.Duration(getEnvInt("REPOSITORY_CLEANUP_INTERVAL", 30)) * time.Minute
	RepositoryCleanupVerbose = getEnvString("REPOSITORY_CLEANUP_VERBOSE", "true") == "false"

	// SSL Configuration
	SSLEnabled = getEnvBool("SSL_ENABLED", false)
	SSLCertPath = getEnvString("SSL_CERT_PATH", "")
	SSLKeyPath = getEnvString("SSL_KEY_PATH", "")
	BindAddress = getEnvString("BIND_ADDRESS", "127.0.0.1")

	// Collection Configuration
	CollectionRoutes = parseCollectionRoutes(getEnvString("COLLECTION_ROUTES", ""))

	// Logging Configuration
	LogVerbosity = getEnvString("LOG_VERBOSITY", "WARN")

	// SysOp Configuration
	SysopPassword = getEnvString("SYSOP_PASSWORD", "")

	// Sandbox Access
	SandboxSecret = getEnvString("SANDBOX_SECRET", "")

	// Analytics Configuration
	ExposeAnalytics = getEnvBool("EXPOSE_ANALYTICS", false)
}
