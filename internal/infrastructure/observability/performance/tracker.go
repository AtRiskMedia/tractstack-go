// Package performance provides performance tracking and monitoring capabilities
// for TractStack operations with multi-tenant support and real-time metrics.
package performance

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// Tracker manages performance markers and provides metrics aggregation
type Tracker struct {
	markers    map[string]*Marker     // Active and completed markers by unique ID
	snapshots  []*PerformanceSnapshot // Historical performance snapshots
	alerts     []*PerformanceAlert    // Active performance alerts
	thresholds *AlertThresholds       // Configurable alert thresholds
	mu         sync.RWMutex           // Protects concurrent access
	started    time.Time              // When tracking started
	config     *TrackerConfig         // Tracker configuration
}

// TrackerConfig contains configuration options for the performance tracker
type TrackerConfig struct {
	MaxMarkers          int           `json:"maxMarkers"`          // Maximum number of markers to retain
	MaxSnapshots        int           `json:"maxSnapshots"`        // Maximum number of snapshots to retain
	MaxAlerts           int           `json:"maxAlerts"`           // Maximum number of alerts to retain
	SnapshotInterval    time.Duration `json:"snapshotInterval"`    // How often to take performance snapshots
	CleanupInterval     time.Duration `json:"cleanupInterval"`     // How often to clean up old data
	EnableDetailedStats bool          `json:"enableDetailedStats"` // Whether to collect detailed memory stats
	EnableAlerts        bool          `json:"enableAlerts"`        // Whether to generate performance alerts
}

// DefaultTrackerConfig returns a sensible default configuration
func DefaultTrackerConfig() *TrackerConfig {
	return &TrackerConfig{
		MaxMarkers:          10000,
		MaxSnapshots:        100,
		MaxAlerts:           500,
		SnapshotInterval:    time.Minute * 5,
		CleanupInterval:     time.Minute * 10,
		EnableDetailedStats: true,
		EnableAlerts:        true,
	}
}

// AlertThresholds defines performance thresholds for generating alerts
type AlertThresholds struct {
	// Response time thresholds
	SlowResponseThreshold     time.Duration `json:"slowResponseThreshold"`     // 500ms
	VerySlowResponseThreshold time.Duration `json:"verySlowResponseThreshold"` // 2s
	CriticalResponseThreshold time.Duration `json:"criticalResponseThreshold"` // 5s

	// Cache performance thresholds
	LowCacheHitRatio      float64 `json:"lowCacheHitRatio"`      // 0.85 (85%)
	CriticalCacheHitRatio float64 `json:"criticalCacheHitRatio"` // 0.70 (70%)

	// Memory thresholds (in MB)
	HighMemoryUsage     int64 `json:"highMemoryUsage"`     // 500MB
	CriticalMemoryUsage int64 `json:"criticalMemoryUsage"` // 1GB

	// Operation-specific thresholds
	AuthOperationThreshold      time.Duration `json:"authOperationThreshold"`      // 200ms
	FragmentGenerationThreshold time.Duration `json:"fragmentGenerationThreshold"` // 100ms
	AnalyticsQueryThreshold     time.Duration `json:"analyticsQueryThreshold"`     // 1s
	DatabaseQueryThreshold      time.Duration `json:"databaseQueryThreshold"`      // 50ms
}

// DefaultAlertThresholds returns sensible default alert thresholds
func DefaultAlertThresholds() *AlertThresholds {
	return &AlertThresholds{
		SlowResponseThreshold:       time.Millisecond * 500,
		VerySlowResponseThreshold:   time.Second * 2,
		CriticalResponseThreshold:   time.Second * 5,
		LowCacheHitRatio:            0.85,
		CriticalCacheHitRatio:       0.70,
		HighMemoryUsage:             500 * 1024 * 1024,  // 500MB
		CriticalMemoryUsage:         1024 * 1024 * 1024, // 1GB
		AuthOperationThreshold:      time.Millisecond * 200,
		FragmentGenerationThreshold: time.Millisecond * 100,
		AnalyticsQueryThreshold:     time.Second * 1,
		DatabaseQueryThreshold:      time.Millisecond * 50,
	}
}

// NewTracker creates a new performance tracker with the given configuration
func NewTracker(config *TrackerConfig) *Tracker {
	if config == nil {
		config = DefaultTrackerConfig()
	}

	tracker := &Tracker{
		markers:    make(map[string]*Marker),
		snapshots:  make([]*PerformanceSnapshot, 0),
		alerts:     make([]*PerformanceAlert, 0),
		thresholds: DefaultAlertThresholds(),
		started:    time.Now(),
		config:     config,
	}

	return tracker
}

// StartOperation creates and tracks a new performance marker for an operation
func (t *Tracker) StartOperation(operation, tenantID string) *Marker {
	marker := &Marker{
		Operation: operation,
		TenantID:  tenantID,
		StartTime: time.Now(),
		Metadata:  make(map[string]any),
		Success:   true, // Assume success until proven otherwise
	}

	// Generate unique ID for this marker
	markerID := fmt.Sprintf("%s_%s_%d", tenantID, operation, time.Now().UnixNano())

	t.mu.Lock()
	t.markers[markerID] = marker
	t.mu.Unlock()

	return marker
}

// StartOperationWithContext creates a performance marker with context cancellation support
func (t *Tracker) StartOperationWithContext(ctx context.Context, operation, tenantID string) *Marker {
	marker := t.StartOperation(operation, tenantID)

	// Monitor context cancellation
	go func() {
		<-ctx.Done()
		if !marker.Completed {
			marker.SetError(ctx.Err())
			marker.Complete()
		}
	}()

	return marker
}

// CompleteOperation manually completes an operation and checks for alerts
func (t *Tracker) CompleteOperation(marker *Marker) {
	if marker == nil || marker.Completed {
		return
	}

	marker.Complete()

	// Check for performance alerts if enabled
	if t.config.EnableAlerts {
		t.checkForAlerts(marker)
	}
}

// checkForAlerts evaluates a completed marker against alert thresholds
func (t *Tracker) checkForAlerts(marker *Marker) {
	if marker == nil || !marker.Completed {
		return
	}

	alerts := t.evaluateThresholds(marker)

	t.mu.Lock()
	for _, alert := range alerts {
		t.alerts = append(t.alerts, alert)

		// Maintain max alerts limit
		if len(t.alerts) > t.config.MaxAlerts {
			// Remove oldest alerts
			t.alerts = t.alerts[len(t.alerts)-t.config.MaxAlerts:]
		}
	}
	t.mu.Unlock()
}

// evaluateThresholds checks a marker against all relevant thresholds
func (t *Tracker) evaluateThresholds(marker *Marker) []*PerformanceAlert {
	var alerts []*PerformanceAlert

	// Check general response time thresholds
	if marker.Duration > t.thresholds.CriticalResponseThreshold {
		alerts = append(alerts, t.createAlert(marker, AlertCritical,
			"Operation exceeded critical response time threshold"))
	} else if marker.Duration > t.thresholds.VerySlowResponseThreshold {
		alerts = append(alerts, t.createAlert(marker, AlertWarning,
			"Operation exceeded slow response time threshold"))
	}

	// Check operation-specific thresholds
	switch {
	case contains(marker.Operation, "auth"):
		if marker.Duration > t.thresholds.AuthOperationThreshold {
			alerts = append(alerts, t.createAlert(marker, AlertWarning,
				"Authentication operation exceeded threshold"))
		}
	case contains(marker.Operation, "fragment"):
		if marker.Duration > t.thresholds.FragmentGenerationThreshold {
			alerts = append(alerts, t.createAlert(marker, AlertWarning,
				"Fragment generation exceeded threshold"))
		}
	case contains(marker.Operation, "analytics"):
		if marker.Duration > t.thresholds.AnalyticsQueryThreshold {
			alerts = append(alerts, t.createAlert(marker, AlertWarning,
				"Analytics query exceeded threshold"))
		}
	}

	// Check cache hit ratio
	if marker.CacheHits+marker.CacheMisses > 0 {
		hitRatio := marker.GetCacheHitRatio()
		if hitRatio < t.thresholds.CriticalCacheHitRatio {
			alerts = append(alerts, t.createAlert(marker, AlertCritical,
				"Cache hit ratio critically low"))
		} else if hitRatio < t.thresholds.LowCacheHitRatio {
			alerts = append(alerts, t.createAlert(marker, AlertWarning,
				"Cache hit ratio below optimal"))
		}
	}

	// Check memory usage
	memoryMB := marker.MemoryUsage / (1024 * 1024)
	if marker.MemoryUsage > t.thresholds.CriticalMemoryUsage {
		alerts = append(alerts, t.createAlert(marker, AlertCritical,
			fmt.Sprintf("Critical memory usage: %d MB", memoryMB)))
	} else if marker.MemoryUsage > t.thresholds.HighMemoryUsage {
		alerts = append(alerts, t.createAlert(marker, AlertWarning,
			fmt.Sprintf("High memory usage: %d MB", memoryMB)))
	}

	return alerts
}

// createAlert creates a new performance alert
func (t *Tracker) createAlert(marker *Marker, severity AlertSeverity, message string) *PerformanceAlert {
	return &PerformanceAlert{
		ID:        fmt.Sprintf("alert_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		TenantID:  marker.TenantID,
		Severity:  severity,
		Operation: marker.Operation,
		Actual:    marker.Duration,
		Message:   message,
		Metadata: map[string]any{
			"cacheHitRatio": marker.GetCacheHitRatio(),
			"memoryUsageMB": marker.MemoryUsage / (1024 * 1024),
			"success":       marker.Success,
		},
	}
}

// GetMetrics returns performance metrics for a specific tenant
func (t *Tracker) GetMetrics(tenantID string) []Marker {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var metrics []Marker
	for _, marker := range t.markers {
		if marker.TenantID == tenantID && marker.Completed {
			metrics = append(metrics, *marker)
		}
	}
	return metrics
}

// GetRecentMetrics returns metrics for operations completed within the specified duration
func (t *Tracker) GetRecentMetrics(tenantID string, within time.Duration) []Marker {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cutoff := time.Now().Add(-within)
	var metrics []Marker

	for _, marker := range t.markers {
		if marker.TenantID == tenantID && marker.Completed && marker.EndTime.After(cutoff) {
			metrics = append(metrics, *marker)
		}
	}
	return metrics
}

// GetActiveOperations returns currently running operations for a tenant
func (t *Tracker) GetActiveOperations(tenantID string) []Marker {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var active []Marker
	for _, marker := range t.markers {
		if marker.TenantID == tenantID && !marker.Completed {
			metrics := *marker
			// Calculate current duration for active operations
			metrics.Duration = time.Since(marker.StartTime)
			active = append(active, metrics)
		}
	}
	return active
}

// GetAlerts returns performance alerts for a specific tenant
func (t *Tracker) GetAlerts(tenantID string) []*PerformanceAlert {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var alerts []*PerformanceAlert
	for _, alert := range t.alerts {
		if alert.TenantID == tenantID {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// TakeSnapshot creates a performance snapshot for the specified tenant
func (t *Tracker) TakeSnapshot(tenantID string) *PerformanceSnapshot {
	metrics := t.GetRecentMetrics(tenantID, time.Minute*5)
	activeOps := t.GetActiveOperations(tenantID)

	snapshot := &PerformanceSnapshot{
		Timestamp:           time.Now(),
		TenantID:            tenantID,
		ActiveOperations:    len(activeOps),
		CompletedOperations: len(metrics),
		OverallHealth:       t.calculateHealth(metrics, activeOps),
	}

	// Categorize metrics by operation type
	snapshot.Auth = t.extractAuthMetrics(metrics)
	snapshot.Content = t.extractContentMetrics(metrics)
	snapshot.Analytics = t.extractAnalyticsMetrics(metrics)

	t.mu.Lock()
	t.snapshots = append(t.snapshots, snapshot)

	// Maintain max snapshots limit
	if len(t.snapshots) > t.config.MaxSnapshots {
		t.snapshots = t.snapshots[len(t.snapshots)-t.config.MaxSnapshots:]
	}
	t.mu.Unlock()

	return snapshot
}

// calculateHealth determines overall system health based on recent metrics
func (t *Tracker) calculateHealth(metrics, activeOps []Marker) HealthStatus {
	if len(metrics) == 0 && len(activeOps) == 0 {
		return HealthUnknown
	}

	criticalIssues := 0
	warningIssues := 0
	totalOps := len(metrics) + len(activeOps)

	allOps := append(metrics, activeOps...)

	for _, op := range allOps {
		duration := op.Duration
		if !op.Completed {
			duration = time.Since(op.StartTime)
		}

		if duration > t.thresholds.CriticalResponseThreshold || !op.Success {
			criticalIssues++
		} else if duration > t.thresholds.VerySlowResponseThreshold {
			warningIssues++
		}
	}

	criticalRatio := float64(criticalIssues) / float64(totalOps)
	warningRatio := float64(warningIssues) / float64(totalOps)

	if criticalRatio > 0.1 { // More than 10% critical issues
		return HealthUnhealthy
	} else if criticalRatio > 0.05 || warningRatio > 0.2 { // More than 5% critical or 20% warning
		return HealthDegraded
	}

	return HealthHealthy
}

// extractAuthMetrics filters metrics for authentication operations
func (t *Tracker) extractAuthMetrics(metrics []Marker) *AuthPerformanceTracker {
	tracker := &AuthPerformanceTracker{}

	for _, metric := range metrics {
		switch {
		case contains(metric.Operation, "fingerprint"):
			if tracker.FingerprintCreation == nil || metric.EndTime.After(tracker.FingerprintCreation.EndTime) {
				m := metric
				tracker.FingerprintCreation = &m
			}
		case contains(metric.Operation, "visit"):
			if tracker.VisitCreation == nil || metric.EndTime.After(tracker.VisitCreation.EndTime) {
				m := metric
				tracker.VisitCreation = &m
			}
		case contains(metric.Operation, "session"):
			if tracker.SessionManagement == nil || metric.EndTime.After(tracker.SessionManagement.EndTime) {
				m := metric
				tracker.SessionManagement = &m
			}
		case contains(metric.Operation, "belief"):
			if tracker.BeliefEvaluation == nil || metric.EndTime.After(tracker.BeliefEvaluation.EndTime) {
				m := metric
				tracker.BeliefEvaluation = &m
			}
		case contains(metric.Operation, "sse"):
			if tracker.SSEBroadcast == nil || metric.EndTime.After(tracker.SSEBroadcast.EndTime) {
				m := metric
				tracker.SSEBroadcast = &m
			}
		case contains(metric.Operation, "jwt"):
			if tracker.JWTGeneration == nil || metric.EndTime.After(tracker.JWTGeneration.EndTime) {
				m := metric
				tracker.JWTGeneration = &m
			}
		}
	}

	return tracker
}

// extractContentMetrics filters metrics for content operations
func (t *Tracker) extractContentMetrics(metrics []Marker) *ContentPerformanceTracker {
	tracker := &ContentPerformanceTracker{}

	for _, metric := range metrics {
		switch {
		case contains(metric.Operation, "repository"):
			if tracker.RepositoryQuery == nil || metric.EndTime.After(tracker.RepositoryQuery.EndTime) {
				m := metric
				tracker.RepositoryQuery = &m
			}
		case contains(metric.Operation, "fragment"):
			if tracker.FragmentGeneration == nil || metric.EndTime.After(tracker.FragmentGeneration.EndTime) {
				m := metric
				tracker.FragmentGeneration = &m
			}
		case contains(metric.Operation, "template"):
			if tracker.TemplateRendering == nil || metric.EndTime.After(tracker.TemplateRendering.EndTime) {
				m := metric
				tracker.TemplateRendering = &m
			}
		case contains(metric.Operation, "contentmap"):
			if tracker.ContentMapBuild == nil || metric.EndTime.After(tracker.ContentMapBuild.EndTime) {
				m := metric
				tracker.ContentMapBuild = &m
			}
		}
	}

	return tracker
}

// extractAnalyticsMetrics filters metrics for analytics operations
func (t *Tracker) extractAnalyticsMetrics(metrics []Marker) *AnalyticsPerformanceTracker {
	tracker := &AnalyticsPerformanceTracker{}

	for _, metric := range metrics {
		switch {
		case contains(metric.Operation, "dashboard"):
			if tracker.DashboardQuery == nil || metric.EndTime.After(tracker.DashboardQuery.EndTime) {
				m := metric
				tracker.DashboardQuery = &m
			}
		case contains(metric.Operation, "sankey"):
			if tracker.SankeyGeneration == nil || metric.EndTime.After(tracker.SankeyGeneration.EndTime) {
				m := metric
				tracker.SankeyGeneration = &m
			}
		case contains(metric.Operation, "warming"):
			if tracker.CacheWarming == nil || metric.EndTime.After(tracker.CacheWarming.EndTime) {
				m := metric
				tracker.CacheWarming = &m
			}
		}
	}

	return tracker
}

// Cleanup removes old markers and snapshots to prevent memory leaks
func (t *Tracker) Cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clean up old completed markers
	cutoff := time.Now().Add(-time.Hour) // Keep last hour of markers
	for id, marker := range t.markers {
		if marker.Completed && marker.EndTime.Before(cutoff) {
			delete(t.markers, id)
		}
	}

	// Maintain max markers limit
	if len(t.markers) > t.config.MaxMarkers {
		// This is a simple approach - in production you might want more sophisticated cleanup
		count := 0
		for id := range t.markers {
			if count > t.config.MaxMarkers/2 {
				delete(t.markers, id)
			}
			count++
		}
	}
}

// GetOverallStats returns overall tracker statistics
func (t *Tracker) GetOverallStats() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	activeCount := 0
	completedCount := 0

	for _, marker := range t.markers {
		if marker.Completed {
			completedCount++
		} else {
			activeCount++
		}
	}

	return map[string]any{
		"trackerUptime":       time.Since(t.started),
		"totalMarkers":        len(t.markers),
		"activeOperations":    activeCount,
		"completedOperations": completedCount,
		"totalSnapshots":      len(t.snapshots),
		"totalAlerts":         len(t.alerts),
		"memoryUsageMB":       memStats.Alloc / (1024 * 1024),
		"systemMemoryMB":      memStats.Sys / (1024 * 1024),
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && s[len(s)-len(substr):] == substr ||
		(len(s) > len(substr) && len(substr) > 0 &&
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())
}
