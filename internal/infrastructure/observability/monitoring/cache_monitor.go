// Package monitoring provides cache performance monitoring and health tracking
// for TractStack's multi-layered cache architecture with detailed analytics.
package monitoring

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// CachePerformanceMonitor tracks performance metrics across all cache layers
type CachePerformanceMonitor struct {
	// Cache layer metrics
	contentCacheMetrics   *CacheLayerMetrics
	userStateCacheMetrics *CacheLayerMetrics
	htmlChunkCacheMetrics *CacheLayerMetrics
	analyticsCacheMetrics *CacheLayerMetrics

	// Overall cache metrics
	overallMetrics *OverallCacheMetrics

	// Tenant-specific metrics
	tenantMetrics map[string]*TenantCacheMetrics

	// Eviction and warming metrics
	evictionStats *EvictionStats
	warmingStats  *WarmingStats

	// Configuration and synchronization
	config  *CacheMonitorConfig
	mu      sync.RWMutex
	started time.Time
}

// CacheLayerMetrics represents performance metrics for a single cache layer
type CacheLayerMetrics struct {
	LayerName   string    `json:"layerName"`
	LastUpdated time.Time `json:"lastUpdated"`

	// Hit/miss statistics
	TotalRequests int64   `json:"totalRequests"`
	CacheHits     int64   `json:"cacheHits"`
	CacheMisses   int64   `json:"cacheMisses"`
	HitRatio      float64 `json:"hitRatio"`

	// Performance metrics
	AvgHitLatency  time.Duration `json:"avgHitLatency"`
	AvgMissLatency time.Duration `json:"avgMissLatency"`
	P95HitLatency  time.Duration `json:"p95HitLatency"`
	P99HitLatency  time.Duration `json:"p99HitLatency"`

	// Storage metrics
	TotalItems       int64 `json:"totalItems"`
	TotalSizeBytes   int64 `json:"totalSizeBytes"`
	AvgItemSizeBytes int64 `json:"avgItemSizeBytes"`
	MaxItemSizeBytes int64 `json:"maxItemSizeBytes"`

	// Eviction and TTL metrics
	TotalEvictions  int64 `json:"totalEvictions"`
	TTLEvictions    int64 `json:"ttlEvictions"`
	MemoryEvictions int64 `json:"memoryEvictions"`
	ManualEvictions int64 `json:"manualEvictions"`

	// Recent performance (last 5 minutes)
	RecentHitRatio     float64 `json:"recentHitRatio"`
	RecentRequestRate  float64 `json:"recentRequestRate"`
	RecentEvictionRate float64 `json:"recentEvictionRate"`
}

// OverallCacheMetrics represents system-wide cache performance
type OverallCacheMetrics struct {
	LastUpdated time.Time `json:"lastUpdated"`

	// Aggregate statistics
	TotalLayers      int     `json:"totalLayers"`
	TotalRequests    int64   `json:"totalRequests"`
	TotalCacheHits   int64   `json:"totalCacheHits"`
	TotalCacheMisses int64   `json:"totalCacheMisses"`
	OverallHitRatio  float64 `json:"overallHitRatio"`

	// Memory usage
	TotalMemoryUsageBytes int64   `json:"totalMemoryUsageBytes"`
	TotalMemoryUsageMB    int64   `json:"totalMemoryUsageMB"`
	MemoryPressure        float64 `json:"memoryPressure"` // 0.0 to 1.0

	// Performance distribution
	BestPerformingLayer     string         `json:"bestPerformingLayer"`
	WorstPerformingLayer    string         `json:"worstPerformingLayer"`
	LayerPerformanceRanking []LayerRanking `json:"layerPerformanceRanking"`

	// Health indicators
	OverallHealth  CacheHealthStatus `json:"overallHealth"`
	CriticalLayers []string          `json:"criticalLayers"`
	WarningLayers  []string          `json:"warningLayers"`
}

// LayerRanking represents performance ranking of cache layers
type LayerRanking struct {
	LayerName  string        `json:"layerName"`
	HitRatio   float64       `json:"hitRatio"`
	AvgLatency time.Duration `json:"avgLatency"`
	Score      float64       `json:"score"` // Composite performance score
}

// TenantCacheMetrics represents cache performance for a specific tenant
type TenantCacheMetrics struct {
	TenantID    string    `json:"tenantId"`
	LastUpdated time.Time `json:"lastUpdated"`

	// Per-layer metrics for this tenant
	LayerMetrics map[string]*TenantLayerMetrics `json:"layerMetrics"`

	// Tenant-specific aggregates
	TotalRequests       int64   `json:"totalRequests"`
	TotalHits           int64   `json:"totalHits"`
	TotalMisses         int64   `json:"totalMisses"`
	TenantHitRatio      float64 `json:"tenantHitRatio"`
	TenantMemoryUsageMB int64   `json:"tenantMemoryUsageMB"`

	// Business context metrics
	ContentCacheHitRatio   float64 `json:"contentCacheHitRatio"`
	AnalyticsCacheHitRatio float64 `json:"analyticsCacheHitRatio"`
	HTMLFragmentHitRatio   float64 `json:"htmlFragmentHitRatio"`
	SessionCacheHitRatio   float64 `json:"sessionCacheHitRatio"`

	// Health status
	TenantCacheHealth CacheHealthStatus `json:"tenantCacheHealth"`
}

// TenantLayerMetrics represents a specific cache layer's performance for a tenant
type TenantLayerMetrics struct {
	LayerName        string        `json:"layerName"`
	Requests         int64         `json:"requests"`
	Hits             int64         `json:"hits"`
	Misses           int64         `json:"misses"`
	HitRatio         float64       `json:"hitRatio"`
	AvgLatency       time.Duration `json:"avgLatency"`
	ItemCount        int64         `json:"itemCount"`
	MemoryUsageBytes int64         `json:"memoryUsageBytes"`
}

// EvictionStats tracks cache eviction patterns and performance
type EvictionStats struct {
	LastUpdated time.Time `json:"lastUpdated"`

	// Global eviction metrics
	TotalEvictions     int64   `json:"totalEvictions"`
	EvictionsPerMinute float64 `json:"evictionsPerMinute"`

	// Eviction reasons
	TTLExpiredEvictions     int64 `json:"ttlExpiredEvictions"`
	MemoryPressureEvictions int64 `json:"memoryPressureEvictions"`
	ManualEvictions         int64 `json:"manualEvictions"`
	CapacityEvictions       int64 `json:"capacityEvictions"`

	// Per-layer eviction breakdown
	LayerEvictions map[string]*LayerEvictionStats `json:"layerEvictions"`

	// Eviction efficiency metrics
	AvgEvictionTime    time.Duration `json:"avgEvictionTime"`
	MemoryReclaimedMB  int64         `json:"memoryReclaimedMB"`
	EvictionEfficiency float64       `json:"evictionEfficiency"` // Memory reclaimed per eviction
}

// LayerEvictionStats tracks evictions for a specific cache layer
type LayerEvictionStats struct {
	LayerName            string  `json:"layerName"`
	TotalEvictions       int64   `json:"totalEvictions"`
	TTLEvictions         int64   `json:"ttlEvictions"`
	MemoryEvictions      int64   `json:"memoryEvictions"`
	ManualEvictions      int64   `json:"manualEvictions"`
	EvictionRate         float64 `json:"evictionRate"` // Evictions per minute
	MemoryReclaimedBytes int64   `json:"memoryReclaimedBytes"`
}

// WarmingStats tracks cache warming performance and effectiveness
type WarmingStats struct {
	LastUpdated time.Time `json:"lastUpdated"`

	// Warming operation metrics
	TotalWarmingOperations int64   `json:"totalWarmingOperations"`
	SuccessfulWarmings     int64   `json:"successfulWarmings"`
	FailedWarmings         int64   `json:"failedWarmings"`
	WarmingSuccessRate     float64 `json:"warmingSuccessRate"`

	// Performance metrics
	AvgWarmingDuration time.Duration `json:"avgWarmingDuration"`
	TotalItemsWarmed   int64         `json:"totalItemsWarmed"`
	WarmingThroughput  float64       `json:"warmingThroughput"` // Items per second

	// Per-tenant warming stats
	TenantWarmingStats map[string]*TenantWarmingStats `json:"tenantWarmingStats"`

	// Content type warming breakdown
	ContentWarmingStats *ContentTypeWarmingStats `json:"contentWarmingStats"`

	// Recent warming activity (last hour)
	RecentWarmingCount    int64         `json:"recentWarmingCount"`
	RecentWarmingDuration time.Duration `json:"recentWarmingDuration"`
}

// TenantWarmingStats tracks cache warming for a specific tenant
type TenantWarmingStats struct {
	TenantID             string        `json:"tenantId"`
	WarmingOperations    int64         `json:"warmingOperations"`
	ItemsWarmed          int64         `json:"itemsWarmed"`
	AvgWarmingTime       time.Duration `json:"avgWarmingTime"`
	LastWarmingTime      time.Time     `json:"lastWarmingTime"`
	WarmingEffectiveness float64       `json:"warmingEffectiveness"` // Hit ratio improvement after warming
}

// ContentTypeWarmingStats tracks warming performance by content type
type ContentTypeWarmingStats struct {
	PanesWarmed          int64 `json:"panesWarmed"`
	StoryFragmentsWarmed int64 `json:"storyFragmentsWarmed"`
	MenusWarmed          int64 `json:"menusWarmed"`
	ResourcesWarmed      int64 `json:"resourcesWarmed"`
	BeliefsWarmed        int64 `json:"beliefsWarmed"`
	AnalyticsBinsWarmed  int64 `json:"analyticsBinsWarmed"`
	HTMLFragmentsWarmed  int64 `json:"htmlFragmentsWarmed"`
	ContentMapWarmed     int64 `json:"contentMapWarmed"`
}

// CacheHealthStatus represents the health of cache operations
type CacheHealthStatus string

const (
	CacheHealthy   CacheHealthStatus = "healthy"   // Performing optimally
	CacheDegraded  CacheHealthStatus = "degraded"  // Some performance issues
	CacheUnhealthy CacheHealthStatus = "unhealthy" // Significant issues
	CacheCritical  CacheHealthStatus = "critical"  // Critical performance problems
	CacheUnknown   CacheHealthStatus = "unknown"   // Unable to determine health
)

// CacheMonitorConfig contains configuration for the cache monitor
type CacheMonitorConfig struct {
	// Monitoring intervals
	MetricsUpdateInterval time.Duration `json:"metricsUpdateInterval"` // How often to update metrics
	HealthCheckInterval   time.Duration `json:"healthCheckInterval"`   // How often to check health
	CleanupInterval       time.Duration `json:"cleanupInterval"`       // How often to cleanup old data

	// Health thresholds
	MinHealthyHitRatio     float64       `json:"minHealthyHitRatio"`     // 0.85
	MinDegradedHitRatio    float64       `json:"minDegradedHitRatio"`    // 0.70
	MaxHealthyEvictionRate float64       `json:"maxHealthyEvictionRate"` // 10 per minute
	MaxHealthyLatency      time.Duration `json:"maxHealthyLatency"`      // 10ms

	// Memory thresholds
	MemoryWarningThreshold  float64 `json:"memoryWarningThreshold"`  // 0.80 (80%)
	MemoryCriticalThreshold float64 `json:"memoryCriticalThreshold"` // 0.95 (95%)

	// Alert configuration
	EnableAlerts   bool                 `json:"enableAlerts"`
	AlertCallbacks []CacheAlertCallback `json:"-"` // Not serialized
}

// DefaultCacheMonitorConfig returns sensible defaults
func DefaultCacheMonitorConfig() *CacheMonitorConfig {
	return &CacheMonitorConfig{
		MetricsUpdateInterval:   time.Second * 30,
		HealthCheckInterval:     time.Minute * 1,
		CleanupInterval:         time.Minute * 10,
		MinHealthyHitRatio:      0.85,
		MinDegradedHitRatio:     0.70,
		MaxHealthyEvictionRate:  10.0,
		MaxHealthyLatency:       time.Millisecond * 10,
		MemoryWarningThreshold:  0.80,
		MemoryCriticalThreshold: 0.95,
		EnableAlerts:            true,
	}
}

// CacheAlertCallback is called when cache alerts are generated
type CacheAlertCallback func(alert *CacheAlert)

// CacheAlert represents a cache performance alert
type CacheAlert struct {
	ID           string             `json:"id"`
	Timestamp    time.Time          `json:"timestamp"`
	Severity     AlertSeverity      `json:"severity"`
	Category     CacheAlertCategory `json:"category"`
	LayerName    string             `json:"layerName"`
	TenantID     string             `json:"tenantId,omitempty"`
	Message      string             `json:"message"`
	CurrentValue any                `json:"currentValue"`
	Threshold    any                `json:"threshold"`
	Metadata     map[string]any     `json:"metadata"`
}

// CacheAlertCategory represents the type of cache alert
type CacheAlertCategory string

const (
	CacheAlertHitRatio CacheAlertCategory = "hit_ratio"
	CacheAlertLatency  CacheAlertCategory = "latency"
	CacheAlertMemory   CacheAlertCategory = "memory"
	CacheAlertEviction CacheAlertCategory = "eviction"
	CacheAlertWarming  CacheAlertCategory = "warming"
	CacheAlertHealth   CacheAlertCategory = "health"
)

// NewCachePerformanceMonitor creates a new cache performance monitor
func NewCachePerformanceMonitor(config *CacheMonitorConfig) *CachePerformanceMonitor {
	if config == nil {
		config = DefaultCacheMonitorConfig()
	}

	return &CachePerformanceMonitor{
		contentCacheMetrics:   createEmptyLayerMetrics("content"),
		userStateCacheMetrics: createEmptyLayerMetrics("user_state"),
		htmlChunkCacheMetrics: createEmptyLayerMetrics("html_chunk"),
		analyticsCacheMetrics: createEmptyLayerMetrics("analytics"),
		overallMetrics:        createEmptyOverallMetrics(),
		tenantMetrics:         make(map[string]*TenantCacheMetrics),
		evictionStats:         createEmptyEvictionStats(),
		warmingStats:          createEmptyWarmingStats(),
		config:                config,
		started:               time.Now(),
	}
}

// createEmptyLayerMetrics creates an empty metrics structure for a cache layer
func createEmptyLayerMetrics(layerName string) *CacheLayerMetrics {
	return &CacheLayerMetrics{
		LayerName:   layerName,
		LastUpdated: time.Now(),
	}
}

// createEmptyOverallMetrics creates an empty overall metrics structure
func createEmptyOverallMetrics() *OverallCacheMetrics {
	return &OverallCacheMetrics{
		LastUpdated:             time.Now(),
		TotalLayers:             4, // content, user_state, html_chunk, analytics
		OverallHealth:           CacheUnknown,
		LayerPerformanceRanking: make([]LayerRanking, 0),
		CriticalLayers:          make([]string, 0),
		WarningLayers:           make([]string, 0),
	}
}

// createEmptyEvictionStats creates an empty eviction stats structure
func createEmptyEvictionStats() *EvictionStats {
	return &EvictionStats{
		LastUpdated:    time.Now(),
		LayerEvictions: make(map[string]*LayerEvictionStats),
	}
}

// createEmptyWarmingStats creates an empty warming stats structure
func createEmptyWarmingStats() *WarmingStats {
	return &WarmingStats{
		LastUpdated:         time.Now(),
		TenantWarmingStats:  make(map[string]*TenantWarmingStats),
		ContentWarmingStats: &ContentTypeWarmingStats{},
	}
}

// RecordCacheOperation records a cache operation for performance tracking
func (cpm *CachePerformanceMonitor) RecordCacheOperation(
	layerName, tenantID string,
	hit bool,
	latency time.Duration,
	itemSizeBytes int64,
) {
	cpm.mu.Lock()
	defer cpm.mu.Unlock()

	// Update layer metrics
	layerMetrics := cpm.getLayerMetrics(layerName)
	if layerMetrics != nil {
		cpm.updateLayerMetrics(layerMetrics, hit, latency, itemSizeBytes)
	}

	// Update tenant metrics
	cpm.updateTenantMetrics(tenantID, layerName, hit, latency)

	// Update overall metrics
	cpm.updateOverallMetrics()
}

// getLayerMetrics returns metrics for a specific cache layer
func (cpm *CachePerformanceMonitor) getLayerMetrics(layerName string) *CacheLayerMetrics {
	switch layerName {
	case "content":
		return cpm.contentCacheMetrics
	case "user_state":
		return cpm.userStateCacheMetrics
	case "html_chunk":
		return cpm.htmlChunkCacheMetrics
	case "analytics":
		return cpm.analyticsCacheMetrics
	default:
		return nil
	}
}

// updateLayerMetrics updates metrics for a cache layer
func (cpm *CachePerformanceMonitor) updateLayerMetrics(
	metrics *CacheLayerMetrics,
	hit bool,
	latency time.Duration,
	itemSizeBytes int64,
) {
	metrics.TotalRequests++

	if hit {
		metrics.CacheHits++
		// Update hit latency (exponential moving average)
		if metrics.AvgHitLatency == 0 {
			metrics.AvgHitLatency = latency
		} else {
			metrics.AvgHitLatency = time.Duration(
				float64(metrics.AvgHitLatency)*0.9 + float64(latency)*0.1,
			)
		}
	} else {
		metrics.CacheMisses++
		// Update miss latency (exponential moving average)
		if metrics.AvgMissLatency == 0 {
			metrics.AvgMissLatency = latency
		} else {
			metrics.AvgMissLatency = time.Duration(
				float64(metrics.AvgMissLatency)*0.9 + float64(latency)*0.1,
			)
		}
	}

	// Update hit ratio
	metrics.HitRatio = float64(metrics.CacheHits) / float64(metrics.TotalRequests)

	// Update item size statistics
	if itemSizeBytes > 0 {
		metrics.TotalItems++
		metrics.TotalSizeBytes += itemSizeBytes
		metrics.AvgItemSizeBytes = metrics.TotalSizeBytes / metrics.TotalItems

		if itemSizeBytes > metrics.MaxItemSizeBytes {
			metrics.MaxItemSizeBytes = itemSizeBytes
		}
	}

	metrics.LastUpdated = time.Now()
}

// updateTenantMetrics updates cache metrics for a specific tenant
func (cpm *CachePerformanceMonitor) updateTenantMetrics(
	tenantID, layerName string,
	hit bool,
	latency time.Duration,
) {
	if _, exists := cpm.tenantMetrics[tenantID]; !exists {
		cpm.tenantMetrics[tenantID] = &TenantCacheMetrics{
			TenantID:          tenantID,
			LastUpdated:       time.Now(),
			LayerMetrics:      make(map[string]*TenantLayerMetrics),
			TenantCacheHealth: CacheUnknown,
		}
	}

	tenantMetrics := cpm.tenantMetrics[tenantID]

	// Update layer-specific metrics for this tenant
	if _, exists := tenantMetrics.LayerMetrics[layerName]; !exists {
		tenantMetrics.LayerMetrics[layerName] = &TenantLayerMetrics{
			LayerName: layerName,
		}
	}

	layerMetrics := tenantMetrics.LayerMetrics[layerName]
	layerMetrics.Requests++

	if hit {
		layerMetrics.Hits++
		tenantMetrics.TotalHits++
	} else {
		layerMetrics.Misses++
		tenantMetrics.TotalMisses++
	}

	// Update ratios
	layerMetrics.HitRatio = float64(layerMetrics.Hits) / float64(layerMetrics.Requests)
	tenantMetrics.TotalRequests++
	tenantMetrics.TenantHitRatio = float64(tenantMetrics.TotalHits) / float64(tenantMetrics.TotalRequests)

	// Update latency (exponential moving average)
	if layerMetrics.AvgLatency == 0 {
		layerMetrics.AvgLatency = latency
	} else {
		layerMetrics.AvgLatency = time.Duration(
			float64(layerMetrics.AvgLatency)*0.9 + float64(latency)*0.1,
		)
	}

	// Update business context hit ratios
	switch layerName {
	case "content":
		tenantMetrics.ContentCacheHitRatio = layerMetrics.HitRatio
	case "analytics":
		tenantMetrics.AnalyticsCacheHitRatio = layerMetrics.HitRatio
	case "html_chunk":
		tenantMetrics.HTMLFragmentHitRatio = layerMetrics.HitRatio
	case "user_state":
		tenantMetrics.SessionCacheHitRatio = layerMetrics.HitRatio
	}

	tenantMetrics.LastUpdated = time.Now()
}

// updateOverallMetrics updates system-wide cache metrics
func (cpm *CachePerformanceMonitor) updateOverallMetrics() {
	metrics := cpm.overallMetrics

	// Aggregate statistics from all layers
	totalRequests := int64(0)
	totalHits := int64(0)
	totalMisses := int64(0)
	totalMemory := int64(0)

	layers := []*CacheLayerMetrics{
		cpm.contentCacheMetrics,
		cpm.userStateCacheMetrics,
		cpm.htmlChunkCacheMetrics,
		cpm.analyticsCacheMetrics,
	}

	for _, layer := range layers {
		totalRequests += layer.TotalRequests
		totalHits += layer.CacheHits
		totalMisses += layer.CacheMisses
		totalMemory += layer.TotalSizeBytes
	}

	metrics.TotalRequests = totalRequests
	metrics.TotalCacheHits = totalHits
	metrics.TotalCacheMisses = totalMisses
	metrics.TotalMemoryUsageBytes = totalMemory
	metrics.TotalMemoryUsageMB = totalMemory / (1024 * 1024)

	if totalRequests > 0 {
		metrics.OverallHitRatio = float64(totalHits) / float64(totalRequests)
	}

	// Update performance ranking
	cpm.updateLayerRanking()

	// Update health status
	cpm.updateOverallHealth()

	metrics.LastUpdated = time.Now()
}

// updateLayerRanking calculates and updates performance ranking of cache layers
func (cpm *CachePerformanceMonitor) updateLayerRanking() {
	layers := map[string]*CacheLayerMetrics{
		"content":    cpm.contentCacheMetrics,
		"user_state": cpm.userStateCacheMetrics,
		"html_chunk": cpm.htmlChunkCacheMetrics,
		"analytics":  cpm.analyticsCacheMetrics,
	}

	rankings := make([]LayerRanking, 0, len(layers))

	for name, metrics := range layers {
		if metrics.TotalRequests == 0 {
			continue
		}

		// Calculate composite performance score
		// Score = (hit_ratio * 0.6) + (1 - normalized_latency * 0.4)
		// Higher score = better performance

		avgLatency := metrics.AvgHitLatency
		if avgLatency == 0 && metrics.AvgMissLatency > 0 {
			avgLatency = metrics.AvgMissLatency
		}

		normalizedLatency := float64(avgLatency.Milliseconds()) / 100.0 // Normalize against 100ms
		if normalizedLatency > 1.0 {
			normalizedLatency = 1.0
		}

		score := (metrics.HitRatio * 0.6) + ((1.0 - normalizedLatency) * 0.4)

		rankings = append(rankings, LayerRanking{
			LayerName:  name,
			HitRatio:   metrics.HitRatio,
			AvgLatency: avgLatency,
			Score:      score,
		})
	}

	// Sort by score (descending)
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].Score > rankings[j].Score
	})

	cpm.overallMetrics.LayerPerformanceRanking = rankings

	if len(rankings) > 0 {
		cpm.overallMetrics.BestPerformingLayer = rankings[0].LayerName
		cpm.overallMetrics.WorstPerformingLayer = rankings[len(rankings)-1].LayerName
	}
}

// updateOverallHealth determines the overall cache health status
func (cpm *CachePerformanceMonitor) updateOverallHealth() {
	criticalLayers := make([]string, 0)
	warningLayers := make([]string, 0)

	layers := map[string]*CacheLayerMetrics{
		"content":    cpm.contentCacheMetrics,
		"user_state": cpm.userStateCacheMetrics,
		"html_chunk": cpm.htmlChunkCacheMetrics,
		"analytics":  cpm.analyticsCacheMetrics,
	}

	for name, metrics := range layers {
		if metrics.TotalRequests == 0 {
			continue
		}

		// Check hit ratio thresholds
		if metrics.HitRatio < cpm.config.MinDegradedHitRatio {
			criticalLayers = append(criticalLayers, name)
		} else if metrics.HitRatio < cpm.config.MinHealthyHitRatio {
			warningLayers = append(warningLayers, name)
		}

		// Check latency thresholds
		avgLatency := metrics.AvgHitLatency
		if avgLatency == 0 {
			avgLatency = metrics.AvgMissLatency
		}

		if avgLatency > cpm.config.MaxHealthyLatency*2 {
			if !contains(criticalLayers, name) {
				criticalLayers = append(criticalLayers, name)
			}
		} else if avgLatency > cpm.config.MaxHealthyLatency {
			if !contains(criticalLayers, name) && !contains(warningLayers, name) {
				warningLayers = append(warningLayers, name)
			}
		}
	}

	// Determine overall health
	var overallHealth CacheHealthStatus
	if len(criticalLayers) > 0 {
		if len(criticalLayers) >= 2 {
			overallHealth = CacheCritical
		} else {
			overallHealth = CacheUnhealthy
		}
	} else if len(warningLayers) > 0 {
		if len(warningLayers) >= 2 {
			overallHealth = CacheUnhealthy
		} else {
			overallHealth = CacheDegraded
		}
	} else {
		overallHealth = CacheHealthy
	}

	cpm.overallMetrics.OverallHealth = overallHealth
	cpm.overallMetrics.CriticalLayers = criticalLayers
	cpm.overallMetrics.WarningLayers = warningLayers
}

// RecordEviction records a cache eviction event
func (cpm *CachePerformanceMonitor) RecordEviction(layerName, reason string, itemSizeBytes int64) {
	cpm.mu.Lock()
	defer cpm.mu.Unlock()

	// Update overall eviction stats
	cpm.evictionStats.TotalEvictions++

	switch reason {
	case "ttl":
		cpm.evictionStats.TTLExpiredEvictions++
	case "memory":
		cpm.evictionStats.MemoryPressureEvictions++
	case "manual":
		cpm.evictionStats.ManualEvictions++
	case "capacity":
		cpm.evictionStats.CapacityEvictions++
	}

	// Update layer-specific eviction stats
	if _, exists := cpm.evictionStats.LayerEvictions[layerName]; !exists {
		cpm.evictionStats.LayerEvictions[layerName] = &LayerEvictionStats{
			LayerName: layerName,
		}
	}

	layerStats := cpm.evictionStats.LayerEvictions[layerName]
	layerStats.TotalEvictions++

	switch reason {
	case "ttl":
		layerStats.TTLEvictions++
	case "memory":
		layerStats.MemoryEvictions++
	case "manual":
		layerStats.ManualEvictions++
	}

	if itemSizeBytes > 0 {
		layerStats.MemoryReclaimedBytes += itemSizeBytes
		cpm.evictionStats.MemoryReclaimedMB += itemSizeBytes / (1024 * 1024)
	}

	// Update layer metrics
	layerMetrics := cpm.getLayerMetrics(layerName)
	if layerMetrics != nil {
		layerMetrics.TotalEvictions++

		switch reason {
		case "ttl":
			layerMetrics.TTLEvictions++
		case "memory":
			layerMetrics.MemoryEvictions++
		case "manual":
			layerMetrics.ManualEvictions++
		}
	}

	cpm.evictionStats.LastUpdated = time.Now()
}

// RecordWarmingOperation records a cache warming operation
func (cpm *CachePerformanceMonitor) RecordWarmingOperation(
	tenantID string,
	itemsWarmed int64,
	duration time.Duration,
	success bool,
	contentType string,
) {
	cpm.mu.Lock()
	defer cpm.mu.Unlock()

	// Update overall warming stats
	cpm.warmingStats.TotalWarmingOperations++
	cpm.warmingStats.TotalItemsWarmed += itemsWarmed

	if success {
		cpm.warmingStats.SuccessfulWarmings++
	} else {
		cpm.warmingStats.FailedWarmings++
	}

	// Update success rate
	if cpm.warmingStats.TotalWarmingOperations > 0 {
		cpm.warmingStats.WarmingSuccessRate = float64(cpm.warmingStats.SuccessfulWarmings) /
			float64(cpm.warmingStats.TotalWarmingOperations)
	}

	// Update average duration (exponential moving average)
	if cpm.warmingStats.AvgWarmingDuration == 0 {
		cpm.warmingStats.AvgWarmingDuration = duration
	} else {
		cpm.warmingStats.AvgWarmingDuration = time.Duration(
			float64(cpm.warmingStats.AvgWarmingDuration)*0.9 + float64(duration)*0.1,
		)
	}

	// Update throughput
	if duration > 0 {
		throughput := float64(itemsWarmed) / duration.Seconds()
		if cpm.warmingStats.WarmingThroughput == 0 {
			cpm.warmingStats.WarmingThroughput = throughput
		} else {
			cpm.warmingStats.WarmingThroughput = cpm.warmingStats.WarmingThroughput*0.9 + throughput*0.1
		}
	}

	// Update tenant-specific warming stats
	if _, exists := cpm.warmingStats.TenantWarmingStats[tenantID]; !exists {
		cpm.warmingStats.TenantWarmingStats[tenantID] = &TenantWarmingStats{
			TenantID: tenantID,
		}
	}

	tenantStats := cpm.warmingStats.TenantWarmingStats[tenantID]
	tenantStats.WarmingOperations++
	tenantStats.ItemsWarmed += itemsWarmed
	tenantStats.LastWarmingTime = time.Now()

	// Update tenant average warming time
	if tenantStats.AvgWarmingTime == 0 {
		tenantStats.AvgWarmingTime = duration
	} else {
		tenantStats.AvgWarmingTime = time.Duration(
			float64(tenantStats.AvgWarmingTime)*0.9 + float64(duration)*0.1,
		)
	}

	// Update content type warming stats
	contentStats := cpm.warmingStats.ContentWarmingStats
	switch contentType {
	case "pane":
		contentStats.PanesWarmed += itemsWarmed
	case "storyfragment":
		contentStats.StoryFragmentsWarmed += itemsWarmed
	case "menu":
		contentStats.MenusWarmed += itemsWarmed
	case "resource":
		contentStats.ResourcesWarmed += itemsWarmed
	case "belief":
		contentStats.BeliefsWarmed += itemsWarmed
	case "analytics":
		contentStats.AnalyticsBinsWarmed += itemsWarmed
	case "html":
		contentStats.HTMLFragmentsWarmed += itemsWarmed
	case "contentmap":
		contentStats.ContentMapWarmed += itemsWarmed
	}

	cpm.warmingStats.LastUpdated = time.Now()
}

// GetLayerMetrics returns performance metrics for a specific cache layer
func (cpm *CachePerformanceMonitor) GetLayerMetrics(layerName string) *CacheLayerMetrics {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	metrics := cpm.getLayerMetrics(layerName)
	if metrics == nil {
		return nil
	}

	// Return a copy to avoid concurrent modification
	metricsCopy := *metrics
	return &metricsCopy
}

// GetOverallMetrics returns overall cache performance metrics
func (cpm *CachePerformanceMonitor) GetOverallMetrics() *OverallCacheMetrics {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	metricsCopy := *cpm.overallMetrics
	return &metricsCopy
}

// GetTenantMetrics returns cache metrics for a specific tenant
func (cpm *CachePerformanceMonitor) GetTenantMetrics(tenantID string) *TenantCacheMetrics {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	if metrics, exists := cpm.tenantMetrics[tenantID]; exists {
		// Return a copy to avoid concurrent modification
		metricsCopy := *metrics

		// Deep copy layer metrics
		metricsCopy.LayerMetrics = make(map[string]*TenantLayerMetrics)
		for layerName, layerMetrics := range metrics.LayerMetrics {
			layerCopy := *layerMetrics
			metricsCopy.LayerMetrics[layerName] = &layerCopy
		}

		return &metricsCopy
	}
	return nil
}

// GetEvictionStats returns cache eviction statistics
func (cpm *CachePerformanceMonitor) GetEvictionStats() *EvictionStats {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	statsCopy := *cpm.evictionStats

	// Deep copy layer evictions
	statsCopy.LayerEvictions = make(map[string]*LayerEvictionStats)
	for layerName, layerStats := range cpm.evictionStats.LayerEvictions {
		layerCopy := *layerStats
		statsCopy.LayerEvictions[layerName] = &layerCopy
	}

	return &statsCopy
}

// GetWarmingStats returns cache warming statistics
func (cpm *CachePerformanceMonitor) GetWarmingStats() *WarmingStats {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	statsCopy := *cpm.warmingStats

	// Deep copy tenant warming stats
	statsCopy.TenantWarmingStats = make(map[string]*TenantWarmingStats)
	for tenantID, tenantStats := range cpm.warmingStats.TenantWarmingStats {
		tenantCopy := *tenantStats
		statsCopy.TenantWarmingStats[tenantID] = &tenantCopy
	}

	// Deep copy content warming stats
	contentCopy := *cpm.warmingStats.ContentWarmingStats
	statsCopy.ContentWarmingStats = &contentCopy

	return &statsCopy
}

// GetCacheHealth returns the overall health status of the cache system
func (cpm *CachePerformanceMonitor) GetCacheHealth() map[string]any {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	return map[string]any{
		"overallHealth":      cpm.overallMetrics.OverallHealth,
		"overallHitRatio":    cpm.overallMetrics.OverallHitRatio,
		"totalMemoryMB":      cpm.overallMetrics.TotalMemoryUsageMB,
		"criticalLayers":     cpm.overallMetrics.CriticalLayers,
		"warningLayers":      cpm.overallMetrics.WarningLayers,
		"bestLayer":          cpm.overallMetrics.BestPerformingLayer,
		"worstLayer":         cpm.overallMetrics.WorstPerformingLayer,
		"totalEvictions":     cpm.evictionStats.TotalEvictions,
		"warmingSuccessRate": cpm.warmingStats.WarmingSuccessRate,
		"monitorUptime":      time.Since(cpm.started),
		"lastUpdated":        cpm.overallMetrics.LastUpdated,
	}
}

// GetDetailedHealthReport returns a comprehensive health report
func (cpm *CachePerformanceMonitor) GetDetailedHealthReport() map[string]any {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	// Get layer-by-layer health
	layerHealth := make(map[string]any)

	layers := map[string]*CacheLayerMetrics{
		"content":    cpm.contentCacheMetrics,
		"user_state": cpm.userStateCacheMetrics,
		"html_chunk": cpm.htmlChunkCacheMetrics,
		"analytics":  cpm.analyticsCacheMetrics,
	}

	for name, metrics := range layers {
		health := CacheHealthy
		issues := make([]string, 0)

		if metrics.HitRatio < cpm.config.MinDegradedHitRatio {
			health = CacheCritical
			issues = append(issues, fmt.Sprintf("Hit ratio critically low: %.2f%%", metrics.HitRatio*100))
		} else if metrics.HitRatio < cpm.config.MinHealthyHitRatio {
			health = CacheDegraded
			issues = append(issues, fmt.Sprintf("Hit ratio below optimal: %.2f%%", metrics.HitRatio*100))
		}

		avgLatency := metrics.AvgHitLatency
		if avgLatency == 0 {
			avgLatency = metrics.AvgMissLatency
		}

		if avgLatency > cpm.config.MaxHealthyLatency*2 {
			health = CacheCritical
			issues = append(issues, fmt.Sprintf("Latency critically high: %v", avgLatency))
		} else if avgLatency > cpm.config.MaxHealthyLatency {
			if health == CacheHealthy {
				health = CacheDegraded
			}
			issues = append(issues, fmt.Sprintf("Latency above optimal: %v", avgLatency))
		}

		layerHealth[name] = map[string]any{
			"health":     health,
			"hitRatio":   metrics.HitRatio,
			"avgLatency": avgLatency,
			"totalItems": metrics.TotalItems,
			"memoryMB":   metrics.TotalSizeBytes / (1024 * 1024),
			"evictions":  metrics.TotalEvictions,
			"issues":     issues,
		}
	}

	return map[string]any{
		"timestamp":     time.Now(),
		"overallHealth": cpm.overallMetrics.OverallHealth,
		"layerHealth":   layerHealth,
		"summary": map[string]any{
			"totalRequests":      cpm.overallMetrics.TotalRequests,
			"overallHitRatio":    cpm.overallMetrics.OverallHitRatio,
			"totalMemoryMB":      cpm.overallMetrics.TotalMemoryUsageMB,
			"totalEvictions":     cpm.evictionStats.TotalEvictions,
			"warmingOps":         cpm.warmingStats.TotalWarmingOperations,
			"warmingSuccessRate": cpm.warmingStats.WarmingSuccessRate,
		},
		"alerts": map[string]any{
			"criticalLayers": cpm.overallMetrics.CriticalLayers,
			"warningLayers":  cpm.overallMetrics.WarningLayers,
		},
		"performance": map[string]any{
			"ranking":    cpm.overallMetrics.LayerPerformanceRanking,
			"bestLayer":  cpm.overallMetrics.BestPerformingLayer,
			"worstLayer": cpm.overallMetrics.WorstPerformingLayer,
		},
	}
}

// AddAlertCallback adds a callback for cache alerts
func (cpm *CachePerformanceMonitor) AddAlertCallback(callback CacheAlertCallback) {
	cpm.config.AlertCallbacks = append(cpm.config.AlertCallbacks, callback)
}

// triggerAlert sends an alert to all registered callbacks
func (cpm *CachePerformanceMonitor) triggerAlert(alert *CacheAlert) {
	if !cpm.config.EnableAlerts {
		return
	}

	for _, callback := range cpm.config.AlertCallbacks {
		go callback(alert)
	}
}

// Start begins background monitoring tasks
func (cpm *CachePerformanceMonitor) Start() {
	go cpm.metricsUpdateLoop()
	go cpm.healthCheckLoop()
	go cpm.cleanupLoop()
}

// metricsUpdateLoop periodically updates cache metrics
func (cpm *CachePerformanceMonitor) metricsUpdateLoop() {
	ticker := time.NewTicker(cpm.config.MetricsUpdateInterval)
	defer ticker.Stop()

	for range ticker.C {
		cpm.mu.Lock()
		cpm.updateOverallMetrics()
		cpm.mu.Unlock()
	}
}

// healthCheckLoop periodically checks cache health and generates alerts
func (cpm *CachePerformanceMonitor) healthCheckLoop() {
	ticker := time.NewTicker(cpm.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		cpm.performHealthCheck()
	}
}

// cleanupLoop periodically cleans up old metrics data
func (cpm *CachePerformanceMonitor) cleanupLoop() {
	ticker := time.NewTicker(cpm.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		cpm.cleanupOldData()
	}
}

// performHealthCheck evaluates cache health and generates alerts if needed
func (cpm *CachePerformanceMonitor) performHealthCheck() {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	// Check each layer for health issues
	layers := map[string]*CacheLayerMetrics{
		"content":    cpm.contentCacheMetrics,
		"user_state": cpm.userStateCacheMetrics,
		"html_chunk": cpm.htmlChunkCacheMetrics,
		"analytics":  cpm.analyticsCacheMetrics,
	}

	for name, metrics := range layers {
		if metrics.TotalRequests == 0 {
			continue
		}

		// Check hit ratio
		if metrics.HitRatio < cpm.config.MinDegradedHitRatio {
			alert := &CacheAlert{
				ID:           fmt.Sprintf("cache_alert_%d", time.Now().UnixNano()),
				Timestamp:    time.Now(),
				Severity:     AlertCritical,
				Category:     CacheAlertHitRatio,
				LayerName:    name,
				Message:      fmt.Sprintf("Cache hit ratio critically low for layer %s", name),
				CurrentValue: metrics.HitRatio,
				Threshold:    cpm.config.MinDegradedHitRatio,
				Metadata: map[string]any{
					"totalRequests": metrics.TotalRequests,
					"cacheHits":     metrics.CacheHits,
					"cacheMisses":   metrics.CacheMisses,
				},
			}
			cpm.triggerAlert(alert)
		}

		// Check latency
		avgLatency := metrics.AvgHitLatency
		if avgLatency == 0 {
			avgLatency = metrics.AvgMissLatency
		}

		if avgLatency > cpm.config.MaxHealthyLatency*2 {
			alert := &CacheAlert{
				ID:           fmt.Sprintf("cache_alert_%d", time.Now().UnixNano()),
				Timestamp:    time.Now(),
				Severity:     AlertCritical,
				Category:     CacheAlertLatency,
				LayerName:    name,
				Message:      fmt.Sprintf("Cache latency critically high for layer %s", name),
				CurrentValue: avgLatency,
				Threshold:    cpm.config.MaxHealthyLatency,
				Metadata: map[string]any{
					"avgHitLatency":  metrics.AvgHitLatency,
					"avgMissLatency": metrics.AvgMissLatency,
				},
			}
			cpm.triggerAlert(alert)
		}
	}
}

// cleanupOldData removes old metrics data to prevent memory leaks
func (cpm *CachePerformanceMonitor) cleanupOldData() {
	cpm.mu.Lock()
	defer cpm.mu.Unlock()

	// Clean up old tenant metrics (keep last 24 hours)
	cutoff := time.Now().Add(-24 * time.Hour)

	for tenantID, metrics := range cpm.tenantMetrics {
		if metrics.LastUpdated.Before(cutoff) {
			delete(cpm.tenantMetrics, tenantID)
		}
	}

	// Clean up old warming stats for inactive tenants
	for tenantID, stats := range cpm.warmingStats.TenantWarmingStats {
		if stats.LastWarmingTime.Before(cutoff) {
			delete(cpm.warmingStats.TenantWarmingStats, tenantID)
		}
	}
}

// contains is a helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
