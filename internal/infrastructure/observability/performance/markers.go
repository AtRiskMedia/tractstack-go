// Package performance provides performance monitoring data structures and utilities
// for tracking operation performance across the TractStack application.
package performance

import (
	"runtime"
	"time"
)

// Marker represents a single performance measurement for an operation
type Marker struct {
	Operation   string         `json:"operation"`       // e.g., "auth:create_visit", "fragment:generate"
	TenantID    string         `json:"tenantId"`        // Tenant identifier for multi-tenant isolation
	StartTime   time.Time      `json:"startTime"`       // When the operation started
	EndTime     time.Time      `json:"endTime"`         // When the operation completed
	Duration    time.Duration  `json:"duration"`        // Total operation duration
	Success     bool           `json:"success"`         // Whether the operation completed successfully
	Error       string         `json:"error,omitempty"` // Error message if operation failed
	Metadata    map[string]any `json:"metadata"`        // Additional operation-specific data
	MemoryUsage int64          `json:"memoryUsage"`     // Memory allocated during operation (bytes)
	CacheHits   int            `json:"cacheHits"`       // Number of cache hits during operation
	CacheMisses int            `json:"cacheMisses"`     // Number of cache misses during operation
	Completed   bool           `json:"completed"`       // Whether Complete() has been called
}

// Complete marks the operation as finished and calculates final metrics
func (m *Marker) Complete() {
	if m.Completed {
		return // Prevent double completion
	}

	m.EndTime = time.Now()
	m.Duration = m.EndTime.Sub(m.StartTime)
	m.Completed = true

	// Capture memory usage at completion
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.MemoryUsage = int64(memStats.Alloc)
}

// SetSuccess marks the operation as successful or failed
func (m *Marker) SetSuccess(success bool) {
	m.Success = success
}

// SetError sets an error message and marks the operation as failed
func (m *Marker) SetError(err error) {
	if err != nil {
		m.Error = err.Error()
		m.Success = false
	}
}

// AddMetadata adds key-value metadata to the marker
func (m *Marker) AddMetadata(key string, value any) {
	if m.Metadata == nil {
		m.Metadata = make(map[string]any)
	}
	m.Metadata[key] = value
}

// AddCacheHit increments the cache hit counter
func (m *Marker) AddCacheHit() {
	m.CacheHits++
}

// AddCacheMiss increments the cache miss counter
func (m *Marker) AddCacheMiss() {
	m.CacheMisses++
}

// GetCacheHitRatio returns the cache hit ratio (0.0 to 1.0)
func (m *Marker) GetCacheHitRatio() float64 {
	total := m.CacheHits + m.CacheMisses
	if total == 0 {
		return 0.0
	}
	return float64(m.CacheHits) / float64(total)
}

// AuthPerformanceTracker contains markers for authentication-related operations
type AuthPerformanceTracker struct {
	FingerprintCreation *Marker `json:"fingerprintCreation,omitempty"`
	VisitCreation       *Marker `json:"visitCreation,omitempty"`
	SessionManagement   *Marker `json:"sessionManagement,omitempty"`
	BeliefEvaluation    *Marker `json:"beliefEvaluation,omitempty"`
	SSEBroadcast        *Marker `json:"sseBroadcast,omitempty"`
	JWTGeneration       *Marker `json:"jwtGeneration,omitempty"`
	ProfileDecoding     *Marker `json:"profileDecoding,omitempty"`
}

// ContentPerformanceTracker contains markers for content-related operations
type ContentPerformanceTracker struct {
	RepositoryQuery    *Marker `json:"repositoryQuery,omitempty"`
	CacheOperation     *Marker `json:"cacheOperation,omitempty"`
	FragmentGeneration *Marker `json:"fragmentGeneration,omitempty"`
	TemplateRendering  *Marker `json:"templateRendering,omitempty"`
	ContentMapBuild    *Marker `json:"contentMapBuild,omitempty"`
}

// AnalyticsPerformanceTracker contains markers for analytics-related operations
type AnalyticsPerformanceTracker struct {
	DashboardQuery     *Marker `json:"dashboardQuery,omitempty"`
	SankeyGeneration   *Marker `json:"sankeyGeneration,omitempty"`
	HourlyBinProcess   *Marker `json:"hourlyBinProcess,omitempty"`
	CacheWarming       *Marker `json:"cacheWarming,omitempty"`
	MetricsAggregation *Marker `json:"metricsAggregation,omitempty"`
}

// TenantPerformanceTracker contains markers for tenant-related operations
type TenantPerformanceTracker struct {
	TenantActivation    *Marker `json:"tenantActivation,omitempty"`
	DatabaseConnection  *Marker `json:"databaseConnection,omitempty"`
	CacheInitialization *Marker `json:"cacheInitialization,omitempty"`
	ConfigurationLoad   *Marker `json:"configurationLoad,omitempty"`
}

// SystemPerformanceTracker contains markers for system-wide operations
type SystemPerformanceTracker struct {
	ApplicationStartup   *Marker `json:"applicationStartup,omitempty"`
	DIContainerBuild     *Marker `json:"diContainerBuild,omitempty"`
	ServerInitialization *Marker `json:"serverInitialization,omitempty"`
	GracefulShutdown     *Marker `json:"gracefulShutdown,omitempty"`
}

// PerformanceSnapshot represents a point-in-time view of system performance
type PerformanceSnapshot struct {
	Timestamp           time.Time                    `json:"timestamp"`
	TenantID            string                       `json:"tenantId"`
	Auth                *AuthPerformanceTracker      `json:"auth,omitempty"`
	Content             *ContentPerformanceTracker   `json:"content,omitempty"`
	Analytics           *AnalyticsPerformanceTracker `json:"analytics,omitempty"`
	Tenant              *TenantPerformanceTracker    `json:"tenant,omitempty"`
	System              *SystemPerformanceTracker    `json:"system,omitempty"`
	OverallHealth       HealthStatus                 `json:"overallHealth"`
	ActiveOperations    int                          `json:"activeOperations"`
	CompletedOperations int                          `json:"completedOperations"`
}

// HealthStatus represents the overall health of a system component
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"   // All operations performing within normal parameters
	HealthDegraded  HealthStatus = "degraded"  // Some operations showing performance issues
	HealthUnhealthy HealthStatus = "unhealthy" // Significant performance problems detected
	HealthUnknown   HealthStatus = "unknown"   // Unable to determine health status
)

// PerformanceAlert represents a performance threshold violation
type PerformanceAlert struct {
	ID           string         `json:"id"`
	Timestamp    time.Time      `json:"timestamp"`
	TenantID     string         `json:"tenantId"`
	Severity     AlertSeverity  `json:"severity"`
	Operation    string         `json:"operation"`
	Threshold    time.Duration  `json:"threshold"`
	Actual       time.Duration  `json:"actual"`
	Message      string         `json:"message"`
	Metadata     map[string]any `json:"metadata"`
	Acknowledged bool           `json:"acknowledged"`
}

// AlertSeverity represents the severity level of a performance alert
type AlertSeverity string

const (
	AlertInfo     AlertSeverity = "info"     // Informational alert
	AlertWarning  AlertSeverity = "warning"  // Performance degradation detected
	AlertCritical AlertSeverity = "critical" // Serious performance issue
	AlertFatal    AlertSeverity = "fatal"    // System-threatening performance problem
)
