// Package monitoring provides tenant-specific performance monitoring and health tracking
// for multi-tenant TractStack operations with real-time metrics and alerting.
package monitoring

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// TenantMetrics represents comprehensive performance metrics for a single tenant
type TenantMetrics struct {
	TenantID    string    `json:"tenantId"`
	LastUpdated time.Time `json:"lastUpdated"`

	// Request performance metrics
	RequestsPerSecond float64       `json:"requestsPerSecond"`
	AvgResponseTime   time.Duration `json:"avgResponseTime"`
	P95ResponseTime   time.Duration `json:"p95ResponseTime"`
	P99ResponseTime   time.Duration `json:"p99ResponseTime"`
	TotalRequests     int64         `json:"totalRequests"`
	FailedRequests    int64         `json:"failedRequests"`
	ErrorRate         float64       `json:"errorRate"`

	// Cache performance metrics
	CacheHitRatio      float64 `json:"cacheHitRatio"`
	CacheHits          int64   `json:"cacheHits"`
	CacheMisses        int64   `json:"cacheMisses"`
	CacheEvictions     int64   `json:"cacheEvictions"`
	CacheMemoryUsageMB int64   `json:"cacheMemoryUsageMB"`

	// Database performance metrics
	DatabaseConnections int           `json:"databaseConnections"`
	AvgQueryTime        time.Duration `json:"avgQueryTime"`
	SlowQueries         int64         `json:"slowQueries"`
	DatabaseErrors      int64         `json:"databaseErrors"`

	// Real-time connection metrics
	SSEConnections     int `json:"sseConnections"`
	ActiveSessions     int `json:"activeSessions"`
	ActiveFingerprints int `json:"activeFingerprints"`
	ActiveVisits       int `json:"activeVisits"`

	// Memory and resource metrics
	MemoryUsageMB   int64   `json:"memoryUsageMB"`
	CPUUsagePercent float64 `json:"cpuUsagePercent"`
	GoroutineCount  int     `json:"goroutineCount"`

	// Business logic metrics
	BeliefEvaluations   int64 `json:"beliefEvaluations"`
	FragmentGenerations int64 `json:"fragmentGenerations"`
	AnalyticsQueries    int64 `json:"analyticsQueries"`
	ContentOperations   int64 `json:"contentOperations"`

	// Health status
	HealthStatus    HealthStatus `json:"healthStatus"`
	LastHealthCheck time.Time    `json:"lastHealthCheck"`
	AlertCount      int          `json:"alertCount"`

	// Time-windowed metrics (last 5 minutes)
	RecentMetrics *RecentMetrics `json:"recentMetrics"`
}

// RecentMetrics contains metrics calculated over a recent time window
type RecentMetrics struct {
	WindowDuration     time.Duration `json:"windowDuration"`
	RequestCount       int64         `json:"requestCount"`
	ErrorCount         int64         `json:"errorCount"`
	AvgResponseTime    time.Duration `json:"avgResponseTime"`
	MaxResponseTime    time.Duration `json:"maxResponseTime"`
	CacheHitRatio      float64       `json:"cacheHitRatio"`
	DatabaseQueryCount int64         `json:"databaseQueryCount"`
	SlowQueryCount     int64         `json:"slowQueryCount"`
}

// HealthStatus represents the health state of a tenant
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"   // All metrics within normal ranges
	HealthDegraded  HealthStatus = "degraded"  // Some metrics showing issues
	HealthUnhealthy HealthStatus = "unhealthy" // Significant issues detected
	HealthCritical  HealthStatus = "critical"  // Critical issues requiring immediate attention
	HealthUnknown   HealthStatus = "unknown"   // Unable to determine health status
)

// TenantMonitor manages performance monitoring for multiple tenants
type TenantMonitor struct {
	metrics         map[string]*TenantMetrics
	thresholds      *HealthThresholds
	alertCallbacks  []AlertCallback
	mu              sync.RWMutex
	started         time.Time
	updateInterval  time.Duration
	cleanupInterval time.Duration
}

// HealthThresholds defines the thresholds for determining tenant health
type HealthThresholds struct {
	// Response time thresholds
	WarningResponseTime  time.Duration `json:"warningResponseTime"`  // 500ms
	CriticalResponseTime time.Duration `json:"criticalResponseTime"` // 2s

	// Error rate thresholds
	WarningErrorRate  float64 `json:"warningErrorRate"`  // 5%
	CriticalErrorRate float64 `json:"criticalErrorRate"` // 15%

	// Cache performance thresholds
	WarningCacheHitRatio  float64 `json:"warningCacheHitRatio"`  // 80%
	CriticalCacheHitRatio float64 `json:"criticalCacheHitRatio"` // 60%

	// Memory thresholds (in MB)
	WarningMemoryUsage  int64 `json:"warningMemoryUsage"`  // 512MB
	CriticalMemoryUsage int64 `json:"criticalMemoryUsage"` // 1GB

	// Database thresholds
	WarningQueryTime  time.Duration `json:"warningQueryTime"`  // 100ms
	CriticalQueryTime time.Duration `json:"criticalQueryTime"` // 500ms
	MaxSlowQueries    int64         `json:"maxSlowQueries"`    // 10 per minute
}

// DefaultHealthThresholds returns sensible default health thresholds
func DefaultHealthThresholds() *HealthThresholds {
	return &HealthThresholds{
		WarningResponseTime:   time.Millisecond * 500,
		CriticalResponseTime:  time.Second * 2,
		WarningErrorRate:      0.05,               // 5%
		CriticalErrorRate:     0.15,               // 15%
		WarningCacheHitRatio:  0.80,               // 80%
		CriticalCacheHitRatio: 0.60,               // 60%
		WarningMemoryUsage:    512 * 1024 * 1024,  // 512MB
		CriticalMemoryUsage:   1024 * 1024 * 1024, // 1GB
		WarningQueryTime:      time.Millisecond * 100,
		CriticalQueryTime:     time.Millisecond * 500,
		MaxSlowQueries:        10,
	}
}

// AlertCallback is a function that gets called when alerts are generated
type AlertCallback func(tenantID string, alert *TenantAlert)

// TenantAlert represents a health alert for a tenant
type TenantAlert struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenantId"`
	Timestamp    time.Time      `json:"timestamp"`
	Severity     AlertSeverity  `json:"severity"`
	Category     AlertCategory  `json:"category"`
	Message      string         `json:"message"`
	CurrentValue any            `json:"currentValue"`
	Threshold    any            `json:"threshold"`
	Metadata     map[string]any `json:"metadata"`
}

// AlertSeverity represents the severity of an alert
type AlertSeverity string

const (
	AlertInfo     AlertSeverity = "info"
	AlertWarning  AlertSeverity = "warning"
	AlertCritical AlertSeverity = "critical"
	AlertFatal    AlertSeverity = "fatal"
)

// AlertCategory represents the category of an alert
type AlertCategory string

const (
	AlertCategoryPerformance  AlertCategory = "performance"
	AlertCategoryMemory       AlertCategory = "memory"
	AlertCategoryCache        AlertCategory = "cache"
	AlertCategoryDatabase     AlertCategory = "database"
	AlertCategoryConnectivity AlertCategory = "connectivity"
	AlertCategoryHealth       AlertCategory = "health"
)

// NewTenantMonitor creates a new tenant performance monitor
func NewTenantMonitor() *TenantMonitor {
	return &TenantMonitor{
		metrics:         make(map[string]*TenantMetrics),
		thresholds:      DefaultHealthThresholds(),
		alertCallbacks:  make([]AlertCallback, 0),
		started:         time.Now(),
		updateInterval:  time.Second * 30, // Update metrics every 30 seconds
		cleanupInterval: time.Minute * 10, // Cleanup old data every 10 minutes
	}
}

// Start begins the monitoring process with background tasks
func (tm *TenantMonitor) Start() {
	go tm.updateLoop()
	go tm.cleanupLoop()
}

// updateLoop periodically updates tenant metrics
func (tm *TenantMonitor) updateLoop() {
	ticker := time.NewTicker(tm.updateInterval)
	defer ticker.Stop()

	for range ticker.C {
		tm.updateAllTenantMetrics()
	}
}

// cleanupLoop periodically cleans up old metrics data
func (tm *TenantMonitor) cleanupLoop() {
	ticker := time.NewTicker(tm.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		tm.cleanupOldMetrics()
	}
}

// UpdateMetrics updates metrics for a specific tenant
func (tm *TenantMonitor) UpdateMetrics(tenantID string, update func(*TenantMetrics)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Get or create metrics for this tenant
	if _, exists := tm.metrics[tenantID]; !exists {
		tm.metrics[tenantID] = tm.createEmptyMetrics(tenantID)
	}

	// Apply the update
	update(tm.metrics[tenantID])
	tm.metrics[tenantID].LastUpdated = time.Now()

	// Update health status
	tm.updateHealthStatus(tenantID)
}

// createEmptyMetrics creates a new empty metrics structure for a tenant
func (tm *TenantMonitor) createEmptyMetrics(tenantID string) *TenantMetrics {
	return &TenantMetrics{
		TenantID:        tenantID,
		LastUpdated:     time.Now(),
		HealthStatus:    HealthUnknown,
		LastHealthCheck: time.Now(),
		RecentMetrics: &RecentMetrics{
			WindowDuration: time.Minute * 5,
		},
	}
}

// GetMetrics returns current metrics for a specific tenant
func (tm *TenantMonitor) GetMetrics(tenantID string) *TenantMetrics {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if metrics, exists := tm.metrics[tenantID]; exists {
		// Return a copy to avoid concurrent modification
		metricsCopy := *metrics
		return &metricsCopy
	}
	return nil
}

// GetAllMetrics returns metrics for all monitored tenants
func (tm *TenantMonitor) GetAllMetrics() map[string]*TenantMetrics {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make(map[string]*TenantMetrics)
	for tenantID, metrics := range tm.metrics {
		metricsCopy := *metrics
		result[tenantID] = &metricsCopy
	}
	return result
}

// RecordRequest records a request completion for performance tracking
func (tm *TenantMonitor) RecordRequest(tenantID string, duration time.Duration, success bool) {
	tm.UpdateMetrics(tenantID, func(metrics *TenantMetrics) {
		metrics.TotalRequests++
		if !success {
			metrics.FailedRequests++
		}

		// Update response time metrics (simple moving average)
		if metrics.TotalRequests == 1 {
			metrics.AvgResponseTime = duration
		} else {
			// Exponential moving average with alpha = 0.1
			metrics.AvgResponseTime = time.Duration(float64(metrics.AvgResponseTime)*0.9 + float64(duration)*0.1)
		}

		// Update error rate
		metrics.ErrorRate = float64(metrics.FailedRequests) / float64(metrics.TotalRequests)

		// Update requests per second (approximate)
		timeSinceStart := time.Since(tm.started).Seconds()
		if timeSinceStart > 0 {
			metrics.RequestsPerSecond = float64(metrics.TotalRequests) / timeSinceStart
		}
	})
}

// RecordCacheOperation records cache hit/miss for performance tracking
func (tm *TenantMonitor) RecordCacheOperation(tenantID string, hit bool) {
	tm.UpdateMetrics(tenantID, func(metrics *TenantMetrics) {
		if hit {
			metrics.CacheHits++
		} else {
			metrics.CacheMisses++
		}

		// Update cache hit ratio
		total := metrics.CacheHits + metrics.CacheMisses
		if total > 0 {
			metrics.CacheHitRatio = float64(metrics.CacheHits) / float64(total)
		}
	})
}

// RecordDatabaseQuery records database query performance
func (tm *TenantMonitor) RecordDatabaseQuery(tenantID string, duration time.Duration, slow bool) {
	tm.UpdateMetrics(tenantID, func(metrics *TenantMetrics) {
		// Update average query time (exponential moving average)
		if metrics.AvgQueryTime == 0 {
			metrics.AvgQueryTime = duration
		} else {
			metrics.AvgQueryTime = time.Duration(float64(metrics.AvgQueryTime)*0.9 + float64(duration)*0.1)
		}

		if slow {
			metrics.SlowQueries++
		}
	})
}

// RecordBusinessOperation records business logic operations
func (tm *TenantMonitor) RecordBusinessOperation(tenantID string, operation string) {
	tm.UpdateMetrics(tenantID, func(metrics *TenantMetrics) {
		switch operation {
		case "belief_evaluation":
			metrics.BeliefEvaluations++
		case "fragment_generation":
			metrics.FragmentGenerations++
		case "analytics_query":
			metrics.AnalyticsQueries++
		case "content_operation":
			metrics.ContentOperations++
		}
	})
}

// updateHealthStatus evaluates and updates the health status for a tenant
func (tm *TenantMonitor) updateHealthStatus(tenantID string) {
	metrics := tm.metrics[tenantID]
	if metrics == nil {
		return
	}

	oldStatus := metrics.HealthStatus
	newStatus := tm.calculateHealthStatus(metrics)

	if oldStatus != newStatus {
		metrics.HealthStatus = newStatus
		metrics.LastHealthCheck = time.Now()

		// Generate alert for health status change
		if newStatus == HealthDegraded || newStatus == HealthUnhealthy || newStatus == HealthCritical {
			alert := &TenantAlert{
				ID:        generateAlertID(),
				TenantID:  tenantID,
				Timestamp: time.Now(),
				Category:  AlertCategoryHealth,
				Message:   fmt.Sprintf("Tenant health status changed from %s to %s", oldStatus, newStatus),
				Metadata: map[string]any{
					"oldStatus": oldStatus,
					"newStatus": newStatus,
				},
			}

			switch newStatus {
			case HealthDegraded:
				alert.Severity = AlertWarning
			case HealthUnhealthy:
				alert.Severity = AlertCritical
			case HealthCritical:
				alert.Severity = AlertFatal
			}

			tm.triggerAlert(alert)
		}
	}
}

// calculateHealthStatus determines the health status based on current metrics
func (tm *TenantMonitor) calculateHealthStatus(metrics *TenantMetrics) HealthStatus {
	criticalIssues := 0
	warningIssues := 0

	// Check response time
	if metrics.AvgResponseTime > tm.thresholds.CriticalResponseTime {
		criticalIssues++
	} else if metrics.AvgResponseTime > tm.thresholds.WarningResponseTime {
		warningIssues++
	}

	// Check error rate
	if metrics.ErrorRate > tm.thresholds.CriticalErrorRate {
		criticalIssues++
	} else if metrics.ErrorRate > tm.thresholds.WarningErrorRate {
		warningIssues++
	}

	// Check cache hit ratio
	if metrics.CacheHitRatio < tm.thresholds.CriticalCacheHitRatio {
		criticalIssues++
	} else if metrics.CacheHitRatio < tm.thresholds.WarningCacheHitRatio {
		warningIssues++
	}

	// Check memory usage
	if metrics.MemoryUsageMB > tm.thresholds.CriticalMemoryUsage/(1024*1024) {
		criticalIssues++
	} else if metrics.MemoryUsageMB > tm.thresholds.WarningMemoryUsage/(1024*1024) {
		warningIssues++
	}

	// Check database query time
	if metrics.AvgQueryTime > tm.thresholds.CriticalQueryTime {
		criticalIssues++
	} else if metrics.AvgQueryTime > tm.thresholds.WarningQueryTime {
		warningIssues++
	}

	// Determine overall health
	if criticalIssues > 0 {
		if criticalIssues >= 3 {
			return HealthCritical
		}
		return HealthUnhealthy
	} else if warningIssues > 0 {
		if warningIssues >= 3 {
			return HealthUnhealthy
		}
		return HealthDegraded
	}

	return HealthHealthy
}

// updateAllTenantMetrics updates system-level metrics for all tenants
func (tm *TenantMonitor) updateAllTenantMetrics() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Get system memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	for tenantID, metrics := range tm.metrics {
		// Update memory usage (approximate per tenant)
		// In a real implementation, you would have per-tenant memory tracking
		tenantCount := len(tm.metrics)
		if tenantCount > 0 {
			metrics.MemoryUsageMB = int64(memStats.Alloc) / (1024 * 1024) / int64(tenantCount)
		}

		// Update goroutine count
		metrics.GoroutineCount = runtime.NumGoroutine()

		metrics.LastUpdated = time.Now()
		tm.updateHealthStatus(tenantID)
	}
}

// cleanupOldMetrics removes metrics for inactive tenants
func (tm *TenantMonitor) cleanupOldMetrics() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cutoff := time.Now().Add(-time.Hour) // Remove metrics older than 1 hour

	for tenantID, metrics := range tm.metrics {
		if metrics.LastUpdated.Before(cutoff) {
			delete(tm.metrics, tenantID)
		}
	}
}

// AddAlertCallback adds a callback function for alert notifications
func (tm *TenantMonitor) AddAlertCallback(callback AlertCallback) {
	tm.alertCallbacks = append(tm.alertCallbacks, callback)
}

// triggerAlert sends an alert to all registered callbacks
func (tm *TenantMonitor) triggerAlert(alert *TenantAlert) {
	for _, callback := range tm.alertCallbacks {
		go callback(alert.TenantID, alert)
	}
}

// GetSystemStats returns overall system statistics
func (tm *TenantMonitor) GetSystemStats() map[string]any {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	totalTenants := len(tm.metrics)
	healthyTenants := 0
	degradedTenants := 0
	unhealthyTenants := 0
	criticalTenants := 0

	for _, metrics := range tm.metrics {
		switch metrics.HealthStatus {
		case HealthHealthy:
			healthyTenants++
		case HealthDegraded:
			degradedTenants++
		case HealthUnhealthy:
			unhealthyTenants++
		case HealthCritical:
			criticalTenants++
		}
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]any{
		"monitorUptime":     time.Since(tm.started),
		"totalTenants":      totalTenants,
		"healthyTenants":    healthyTenants,
		"degradedTenants":   degradedTenants,
		"unhealthyTenants":  unhealthyTenants,
		"criticalTenants":   criticalTenants,
		"systemMemoryMB":    memStats.Sys / (1024 * 1024),
		"allocatedMemoryMB": memStats.Alloc / (1024 * 1024),
		"goroutineCount":    runtime.NumGoroutine(),
		"gcCycles":          memStats.NumGC,
	}
}

// generateAlertID generates a unique alert ID
func generateAlertID() string {
	return fmt.Sprintf("alert_%d", time.Now().UnixNano())
}

// SetThresholds updates the health thresholds
func (tm *TenantMonitor) SetThresholds(thresholds *HealthThresholds) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.thresholds = thresholds
}

// GetThresholds returns the current health thresholds
func (tm *TenantMonitor) GetThresholds() *HealthThresholds {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.thresholds
}
