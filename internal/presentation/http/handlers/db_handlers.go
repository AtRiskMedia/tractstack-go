// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// DatabaseHandlers contains all database-related HTTP handlers
type DatabaseHandlers struct {
	dbService   *services.DBService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewDBHandlers creates database handlers with injected dependencies
func NewDBHandlers(dbService *services.DBService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *DatabaseHandlers {
	return &DatabaseHandlers{
		dbService:   dbService,
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// GetDatabaseStatus handles GET /api/v1/db/status - checks tenant database status
func (h *DatabaseHandlers) GetDatabaseStatus(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_database_status_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received get database status request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Use database service to check status
	status := h.dbService.CheckStatus(tenantCtx)

	// Log the result
	if status["error"] != nil && status["error"].(string) != "" {
		h.logger.System().Error("Database status check failed", "tenantId", tenantCtx.TenantID, "error", status["error"], "duration", time.Since(start))
		marker.SetSuccess(false)
		h.logger.Perf().Info("Performance for GetDatabaseStatus request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", false)

		// Return error status but still return 200 OK with error details
		c.JSON(http.StatusOK, gin.H{
			"tenantId":       status["tenantId"],
			"status":         status["status"],
			"allTablesExist": false,
			"error":          status["error"],
			"responseTime":   status["timestamp"],
		})
		return
	}

	h.logger.System().Info("Database status check completed", "tenantId", tenantCtx.TenantID, "status", status["status"], "allTablesExist", status["allTablesExist"], "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetDatabaseStatus request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	// Return successful status
	c.JSON(http.StatusOK, status)
}

// GetDatabaseHealth handles GET /api/v1/db/health - comprehensive health check
func (h *DatabaseHandlers) GetDatabaseHealth(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_database_health_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received get database health request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Perform comprehensive health check
	healthResult := h.dbService.PerformHealthCheck(tenantCtx)

	// Log the result
	h.logger.System().Info("Database health check completed",
		"tenantId", tenantCtx.TenantID,
		"overallStatus", healthResult["status"],
		"duration", time.Since(start))

	// Set marker success based on overall status
	success := healthResult["status"] == "healthy"
	marker.SetSuccess(success)
	h.logger.Perf().Info("Performance for GetDatabaseHealth request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", success)

	// Return health check results
	c.JSON(http.StatusOK, healthResult)
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
	filteredTests := make(map[string]map[string]interface{})
	if tests, ok := healthResult["tests"].(map[string]interface{}); ok {
		switch testRequest.Level {
		case "basic":
			// Return only connectivity and tables tests
			if result, exists := tests["connectivity"]; exists {
				filteredTests["connectivity"] = result.(map[string]interface{})
			}
			if result, exists := tests["tables"]; exists {
				filteredTests["tables"] = result.(map[string]interface{})
			}
		case "performance":
			// Return performance-related tests
			if result, exists := tests["performance"]; exists {
				filteredTests["performance"] = result.(map[string]interface{})
			}
			if result, exists := tests["write_read"]; exists {
				filteredTests["write_read"] = result.(map[string]interface{})
			}
		case "full":
			// Return all tests
			for k, v := range tests {
				filteredTests[k] = v.(map[string]interface{})
			}
		default:
			// Default to basic
			if result, exists := tests["connectivity"]; exists {
				filteredTests["connectivity"] = result.(map[string]interface{})
			}
			if result, exists := tests["tables"]; exists {
				filteredTests["tables"] = result.(map[string]interface{})
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
	TenantID       string                 `json:"tenantId"`
	Status         string                 `json:"status"`
	Database       map[string]interface{} `json:"database"`
	AllTablesExist bool                   `json:"allTablesExist"`
	TableStatus    map[string]bool        `json:"tableStatus,omitempty"`
	ConnectionInfo string                 `json:"connectionInfo,omitempty"`
	ResponseTime   string                 `json:"responseTime"`
	LastChecked    time.Time              `json:"lastChecked"`
	Error          string                 `json:"error,omitempty"`
}

// HealthResponse represents the response structure for health checks
type HealthResponse struct {
	TenantID      string                            `json:"tenantId"`
	OverallStatus string                            `json:"overallStatus"`
	Tests         map[string]map[string]interface{} `json:"tests"`
	Duration      string                            `json:"duration"`
	StartTime     time.Time                         `json:"startTime"`
	CompletedAt   time.Time                         `json:"completedAt"`
}

// StatsResponse represents the response structure for connection stats
type StatsResponse struct {
	TenantID string                 `json:"tenantId"`
	Stats    map[string]interface{} `json:"stats"`
}

// TestResponse represents the response structure for database tests
type TestResponse struct {
	TenantID      string                            `json:"tenantId"`
	Level         string                            `json:"level"`
	OverallStatus string                            `json:"overallStatus"`
	Tests         map[string]map[string]interface{} `json:"tests"`
	Duration      string                            `json:"duration"`
	CompletedAt   time.Time                         `json:"completedAt"`
}
