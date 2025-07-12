// Package api provides HTTP handlers for analytics endpoints.
package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/AtRiskMedia/tractstack-go/analytics"
	"github.com/gin-gonic/gin"
)

// HandleDashboardAnalytics handles GET /api/v1/analytics/dashboard
func HandleDashboardAnalytics(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Parse time range parameters
	startHour, endHour := parseTimeRange(c)

	// Load analytics data for the requested range
	err = analytics.LoadHourlyEpinetData(ctx, startHour)
	if err != nil {
		log.Printf("ERROR: LoadHourlyEpinetData failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load analytics data"})
		return
	}

	// Compute dashboard analytics for the custom range
	dashboard, err := analytics.ComputeDashboardAnalytics(ctx, startHour, endHour)
	if err != nil {
		log.Printf("ERROR: ComputeDashboardAnalytics failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute dashboard analytics"})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

// HandleEpinetSankey handles GET /api/v1/analytics/epinet/:id
func HandleEpinetSankey(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	epinetID := c.Param("id")
	if epinetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "epinet ID is required"})
		return
	}

	// Parse time range parameters
	visitorType := c.DefaultQuery("visitorType", "all")
	selectedUserID := c.Query("userId")
	startHourStr := c.DefaultQuery("startHour", "168")
	endHourStr := c.DefaultQuery("endHour", "0")

	startHour, err := strconv.Atoi(startHourStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid startHour parameter"})
		return
	}

	endHour, err := strconv.Atoi(endHourStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid endHour parameter"})
		return
	}

	// Create pointers for optional parameters
	var startHourPtr, endHourPtr *int
	var selectedUserIDPtr *string

	if startHourStr != "" {
		startHourPtr = &startHour
	}
	if endHourStr != "" {
		endHourPtr = &endHour
	}
	if selectedUserID != "" {
		selectedUserIDPtr = &selectedUserID
	}

	// Load analytics data for the requested range
	err = analytics.LoadHourlyEpinetData(ctx, startHour)
	if err != nil {
		log.Printf("ERROR: LoadHourlyEpinetData failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load analytics data"})
		return
	}

	// Compute epinet sankey diagram
	epinet, err := analytics.ComputeEpinetSankey(ctx, epinetID, &analytics.SankeyFilters{
		VisitorType:    visitorType,
		SelectedUserID: selectedUserIDPtr,
		StartHour:      startHourPtr,
		EndHour:        endHourPtr,
	})
	if err != nil {
		log.Printf("ERROR: ComputeEpinetSankey failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute epinet sankey"})
		return
	}

	// Get filtered visitor counts
	userCounts, err := analytics.GetFilteredVisitorCounts(ctx, epinetID, visitorType, startHourPtr, endHourPtr)
	if err != nil {
		log.Printf("ERROR: GetFilteredVisitorCounts failed: %v", err)
		userCounts = []analytics.UserCount{}
	}

	// Get hourly node activity
	hourlyNodeActivity, err := analytics.GetHourlyNodeActivity(ctx, epinetID, visitorType, startHourPtr, endHourPtr, selectedUserIDPtr)
	if err != nil {
		log.Printf("ERROR: GetHourlyNodeActivity failed: %v", err)
		hourlyNodeActivity = analytics.HourlyActivity{}
	}

	// Return the combined response matching v1 shape
	c.JSON(http.StatusOK, gin.H{
		"epinet":             epinet,
		"userCounts":         userCounts,
		"hourlyNodeActivity": hourlyNodeActivity,
	})
}

// HandleStoryfragmentAnalytics handles GET /api/v1/analytics/storyfragments
func HandleStoryfragmentAnalytics(c *gin.Context) {
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

	// Compute storyfragment analytics
	storyfragmentAnalytics, err := analytics.ComputeStoryfragmentAnalytics(ctx)
	if err != nil {
		log.Printf("ERROR: ComputeStoryfragmentAnalytics failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute storyfragment analytics"})
		return
	}

	c.JSON(http.StatusOK, storyfragmentAnalytics)
}

// HandleLeadMetrics handles GET /api/v1/analytics/leads
func HandleLeadMetrics(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Parse time range parameters
	startHour, endHour := parseTimeRange(c)

	// Load analytics data for the requested range
	err = analytics.LoadHourlyEpinetData(ctx, startHour)
	if err != nil {
		log.Printf("ERROR: LoadHourlyEpinetData failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load analytics data"})
		return
	}

	// Compute lead metrics for the custom range
	metrics, err := analytics.ComputeLeadMetrics(ctx, startHour, endHour)
	if err != nil {
		log.Printf("ERROR: ComputeLeadMetrics failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute lead metrics"})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// parseTimeRange parses duration or startHour/endHour from query parameters
func parseTimeRange(c *gin.Context) (startHour, endHour int) {
	// Check for custom range first (priority)
	startHourStr := c.Query("startHour")
	endHourStr := c.Query("endHour")

	if startHourStr != "" && endHourStr != "" {
		var err error
		startHour, err = strconv.Atoi(startHourStr)
		if err != nil {
			// Default to weekly if invalid
			return 168, 0
		}
		endHour, err = strconv.Atoi(endHourStr)
		if err != nil {
			// Default to weekly if invalid
			return 168, 0
		}
		return startHour, endHour
	}

	// Check for duration parameter
	duration := c.DefaultQuery("duration", "weekly")

	switch duration {
	case "daily":
		return 24, 0
	case "weekly":
		return 168, 0
	case "monthly":
		return 672, 0
	default:
		return 168, 0 // Default to weekly
	}
}
