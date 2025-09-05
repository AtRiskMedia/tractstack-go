// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// DatabaseHandlers contains all database-related HTTP handlers
type DatabaseHandlers struct {
	dbService     *services.DBService
	logger        *logging.ChanneledLogger
	perfTracker   *performance.Tracker
	tenantManager *tenant.Manager
}

// NewDBHandlers creates database handlers with injected dependencies
func NewDBHandlers(dbService *services.DBService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker, tenantManager *tenant.Manager) *DatabaseHandlers {
	return &DatabaseHandlers{
		dbService:     dbService,
		logger:        logger,
		perfTracker:   perfTracker,
		tenantManager: tenantManager,
	}
}

// GetConnectionStats handles GET /api/v1/db/stats - database connection statistics
func (h *DatabaseHandlers) GetConnectionStats(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_connection_stats_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received get connection stats request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Get connection statistics
	stats := h.dbService.GetConnectionStats(tenantCtx)

	h.logger.System().Info("Connection stats retrieved",
		"tenantId", tenantCtx.TenantID,
		"available", stats["available"],
		"openConns", stats["openConns"],
		"inUse", stats["inUse"],
		"idle", stats["idle"],
		"duration", time.Since(start))

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetConnectionStats request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"tenantId": tenantCtx.TenantID,
		"stats":    stats,
	})
}

// PostDatabaseTest handles POST /api/v1/db/test - test database operations
func (h *DatabaseHandlers) PostDatabaseTest(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_database_test_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received post database test request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Parse request body for test parameters
	var testRequest struct {
		Tests []string `json:"tests,omitempty"` // Optional: specify which tests to run
		Level string   `json:"level,omitempty"` // Optional: "basic", "full", "performance"
	}

	if err := c.ShouldBindJSON(&testRequest); err != nil {
		// If no valid JSON, run basic tests
		testRequest.Level = "basic"
	}

	// Default to basic level if not specified
	if testRequest.Level == "" {
		testRequest.Level = "basic"
	}

	// Perform health check (which includes various test levels)
	healthResult := h.dbService.PerformHealthCheck(tenantCtx)

	// Filter results based on requested level
	filteredTests := make(map[string]map[string]any)
	if tests, ok := healthResult["tests"].(map[string]any); ok {
		switch testRequest.Level {
		case "basic":
			// Return only connectivity and tables tests
			if result, exists := tests["connectivity"]; exists {
				filteredTests["connectivity"] = result.(map[string]any)
			}
			if result, exists := tests["tables"]; exists {
				filteredTests["tables"] = result.(map[string]any)
			}
		case "performance":
			// Return performance-related tests
			if result, exists := tests["performance"]; exists {
				filteredTests["performance"] = result.(map[string]any)
			}
			if result, exists := tests["write_read"]; exists {
				filteredTests["write_read"] = result.(map[string]any)
			}
		case "full":
			// Return all tests
			for k, v := range tests {
				filteredTests[k] = v.(map[string]any)
			}
		default:
			// Default to basic
			if result, exists := tests["connectivity"]; exists {
				filteredTests["connectivity"] = result.(map[string]any)
			}
			if result, exists := tests["tables"]; exists {
				filteredTests["tables"] = result.(map[string]any)
			}
		}
	}

	h.logger.System().Info("Database test completed",
		"tenantId", tenantCtx.TenantID,
		"level", testRequest.Level,
		"testCount", len(filteredTests),
		"overallStatus", healthResult["status"],
		"duration", time.Since(start))

	// Determine success based on test results
	success := true
	for _, test := range filteredTests {
		if status, ok := test["status"].(string); ok && status == "fail" {
			success = false
			break
		}
	}

	marker.SetSuccess(success)
	h.logger.Perf().Info("Performance for PostDatabaseTest request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", success)

	// Return test results
	c.JSON(http.StatusOK, gin.H{
		"tenantId":      tenantCtx.TenantID,
		"level":         testRequest.Level,
		"overallStatus": healthResult["status"],
		"tests":         filteredTests,
		"duration":      time.Since(start).String(),
		"completedAt":   time.Now(),
	})
}

// DatabaseStatusResponse represents the response structure for database status
type DatabaseStatusResponse struct {
	TenantID       string          `json:"tenantId"`
	Status         string          `json:"status"`
	Database       map[string]any  `json:"database"`
	AllTablesExist bool            `json:"allTablesExist"`
	TableStatus    map[string]bool `json:"tableStatus,omitempty"`
	ConnectionInfo string          `json:"connectionInfo,omitempty"`
	ResponseTime   string          `json:"responseTime"`
	LastChecked    time.Time       `json:"lastChecked"`
	Error          string          `json:"error,omitempty"`
}

// HealthResponse represents the response structure for health checks
type HealthResponse struct {
	TenantID      string                    `json:"tenantId"`
	OverallStatus string                    `json:"overallStatus"`
	Tests         map[string]map[string]any `json:"tests"`
	Duration      string                    `json:"duration"`
	StartTime     time.Time                 `json:"startTime"`
	CompletedAt   time.Time                 `json:"completedAt"`
}

// StatsResponse represents the response structure for connection stats
type StatsResponse struct {
	TenantID string         `json:"tenantId"`
	Stats    map[string]any `json:"stats"`
}

// TestResponse represents the response structure for database tests
type TestResponse struct {
	TenantID      string                    `json:"tenantId"`
	Level         string                    `json:"level"`
	OverallStatus string                    `json:"overallStatus"`
	Tests         map[string]map[string]any `json:"tests"`
	Duration      string                    `json:"duration"`
	CompletedAt   time.Time                 `json:"completedAt"`
}

func (h *DatabaseHandlers) GetGeneralHealth(c *gin.Context) {
	// First, try to get tenant context using existing middleware pattern
	tenantCtx, exists := middleware.GetTenantContext(c)

	// If tenant context exists and tenant is active, return healthy status
	if exists && tenantCtx.Status == "active" {
		// Return healthy status in exact legacy format
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"healthy":   true,
			"timestamp": time.Now().UTC().Unix(),
			"tenantId":  tenantCtx.TenantID,
		})
		return
	}

	// If tenant context failed, check if this is a setup detection scenario
	if !exists {
		// Determine which tenant was being requested
		detector := h.tenantManager.GetDetector()
		tenantID, err := detector.DetectTenant(c)

		// CRITICAL: Only check for setup if the requested tenant is "default"
		if err == nil && tenantID == "default" {
			defaultTenantStatus := detector.GetTenantStatus("default")

			// If default tenant exists in registry but is inactive, setup is needed
			if defaultTenantStatus == "inactive" {
				c.JSON(http.StatusOK, gin.H{
					"needsSetup": true,
				})
				return
			}
		}
	}

	// Fallback for other error cases
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"status":  "error",
		"healthy": false,
		"error":   "tenant not available",
	})
}
