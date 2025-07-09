// Package api provides HTTP handlers for analytics endpoints.
package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/AtRiskMedia/tractstack-go/analytics"
	"github.com/gin-gonic/gin"
)

// HandleDashboardAnalytics handles GET /api/v1/analytics/dashboard (exact V1 pattern)
func HandleDashboardAnalytics(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Load analytics data
	err = analytics.LoadHourlyEpinetData(ctx, 672) // Load 28 days
	if err != nil {
		log.Printf("ERROR: LoadHourlyEpinetData failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load analytics data"})
		return
	}

	// Compute dashboard analytics
	dashboard, err := analytics.ComputeDashboardAnalytics(ctx)
	if err != nil {
		log.Printf("ERROR: ComputeDashboardAnalytics failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute dashboard analytics"})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

// HandleEpinetSankey handles GET /api/v1/analytics/epinet/:id (exact V1 pattern)
func HandleEpinetSankey(c *gin.Context) {
	log.Printf("DEBUG: Starting HandleEpinetSankey")

	ctx, err := getTenantContext(c)
	if err != nil {
		log.Printf("ERROR: getTenantContext failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	log.Printf("DEBUG: Got tenant context: %s", ctx.TenantID)

	epinetID := c.Param("id")
	log.Printf("DEBUG: Epinet ID: %s", epinetID)

	// Load analytics data
	log.Printf("DEBUG: About to call LoadHourlyEpinetData")
	err = analytics.LoadHourlyEpinetData(ctx, 168) // Load 7 days
	if err != nil {
		log.Printf("ERROR: LoadHourlyEpinetData failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load analytics data"})
		return
	}
	log.Printf("DEBUG: LoadHourlyEpinetData completed successfully")

	// Parse query parameters (exact V1 pattern)
	visitorType := c.DefaultQuery("visitorType", "all")

	var selectedUserID *string
	if userID := c.Query("selectedUserId"); userID != "" {
		selectedUserID = &userID
	}

	var startHour, endHour *int
	if startHourStr := c.Query("startHour"); startHourStr != "" {
		if start, err := strconv.Atoi(startHourStr); err == nil {
			startHour = &start
		}
	}
	if endHourStr := c.Query("endHour"); endHourStr != "" {
		if end, err := strconv.Atoi(endHourStr); err == nil {
			endHour = &end
		}
	}

	// Create filters (exact V1 pattern)
	filters := &analytics.SankeyFilters{
		VisitorType:    visitorType,
		SelectedUserID: selectedUserID,
		StartHour:      startHour,
		EndHour:        endHour,
	}

	// Compute sankey diagram
	sankey, err := analytics.ComputeEpinetSankey(ctx, epinetID, 168, filters)
	if err != nil {
		log.Printf("ERROR: ComputeEpinetSankey failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute epinet sankey"})
		return
	}

	log.Printf("DEBUG: Sankey computed successfully with %d nodes and %d links", len(sankey.Nodes), len(sankey.Links))
	c.JSON(http.StatusOK, sankey)
}

// HandleLeadMetrics handles GET /api/v1/analytics/leads (exact V1 pattern)
func HandleLeadMetrics(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Load analytics data
	err = analytics.LoadHourlyEpinetData(ctx, 672) // Load 28 days
	if err != nil {
		log.Printf("ERROR: LoadHourlyEpinetData failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load analytics data"})
		return
	}

	// Compute lead metrics
	metrics, err := analytics.ComputeLeadMetrics(ctx)
	if err != nil {
		log.Printf("ERROR: ComputeLeadMetrics failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute lead metrics"})
		return
	}

	c.JSON(http.StatusOK, metrics)
}
