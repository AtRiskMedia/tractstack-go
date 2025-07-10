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

// HandleEpinetSankey handles GET /api/v1/analytics/epinet/:id
func HandleEpinetSankey(c *gin.Context) {
	log.Printf("DEBUG: Starting HandleEpinetSankey")

	// Get tenant context
	ctx, err := getTenantContext(c)
	if err != nil {
		log.Printf("ERROR: getTenantContext failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	log.Printf("DEBUG: Got tenant context: %s", ctx.TenantID)

	// Get epinet ID
	epinetID := c.Param("id")
	if epinetID == "" {
		log.Printf("ERROR: Epinet ID is empty")
		c.JSON(http.StatusBadRequest, gin.H{"error": "epinet ID is required"})
		return
	}
	log.Printf("DEBUG: Epinet ID: %s", epinetID)

	// Parse query parameters
	visitorType := c.DefaultQuery("visitorType", "all")
	var selectedUserID *string
	if userID := c.Query("selectedUserId"); userID != "" {
		selectedUserID = &userID
	}

	// Parse startHour and endHour
	var startHour, endHour *int
	hoursBack := 168 // Default to 168 hours (7 days)
	startHourStr := c.Query("startHour")
	endHourStr := c.Query("endHour")

	if startHourStr != "" && endHourStr != "" {
		start, err := strconv.Atoi(startHourStr)
		if err == nil && start > 0 {
			end, err := strconv.Atoi(endHourStr)
			if err == nil && end >= 0 && start > end {
				startHour = &start
				endHour = &end
				hoursBack = start - end
				log.Printf("DEBUG: Parsed startHour=%d, endHour=%d, hoursBack=%d", start, end, hoursBack)
			} else {
				log.Printf("DEBUG: Invalid endHour '%s' or startHour (%d) <= endHour (%d), defaulting to hoursBack=168", endHourStr, start, end)
			}
		} else {
			log.Printf("DEBUG: Invalid startHour '%s', defaulting to hoursBack=168", startHourStr)
		}
	} else {
		log.Printf("DEBUG: startHour or endHour missing, defaulting to hoursBack=168")
	}

	// Load analytics data
	log.Printf("DEBUG: About to call LoadHourlyEpinetData with hoursBack=%d", hoursBack)
	err = analytics.LoadHourlyEpinetData(ctx, hoursBack)
	if err != nil {
		log.Printf("ERROR: LoadHourlyEpinetData failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load analytics data"})
		return
	}
	log.Printf("DEBUG: LoadHourlyEpinetData completed successfully")

	// Create filters
	filters := &analytics.SankeyFilters{
		VisitorType:    visitorType,
		SelectedUserID: selectedUserID,
		StartHour:      startHour,
		EndHour:        endHour,
	}

	// Compute sankey diagram
	log.Printf("DEBUG: About to call ComputeEpinetSankey with hoursBack=%d", hoursBack)
	sankey, err := analytics.ComputeEpinetSankey(ctx, epinetID, hoursBack, filters)
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
