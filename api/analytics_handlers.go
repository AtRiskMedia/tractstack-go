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
	// Get tenant context
	ctx, err := getTenantContext(c)
	if err != nil {
		log.Printf("ERROR: getTenantContext failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Get epinet ID
	epinetID := c.Param("id")
	if epinetID == "" {
		log.Printf("ERROR: Epinet ID is empty")
		c.JSON(http.StatusBadRequest, gin.H{"error": "epinet ID is required"})
		return
	}

	// Parse query parameters
	visitorType := c.DefaultQuery("visitorType", "all")
	var selectedUserID *string
	if userID := c.Query("userId"); userID != "" {
		selectedUserID = &userID
	}

	// Parse startHour and endHour
	var startHour, endHour *int
	startHourStr := c.Query("startHour")
	endHourStr := c.Query("endHour")

	if startHourStr != "" && endHourStr != "" {
		start, err := strconv.Atoi(startHourStr)
		if err == nil && start > 0 {
			end, err := strconv.Atoi(endHourStr)
			if err == nil && end >= 0 && start > end {
				startHour = &start
				endHour = &end
			} else {
				log.Printf("DEBUG: Invalid endHour '%s' or startHour (%d) <= endHour (%d)", endHourStr, start, end)
			}
		} else {
			log.Printf("DEBUG: Invalid startHour '%s'", startHourStr)
		}
	} else {
		log.Printf("DEBUG: startHour or endHour missing")
	}

	// Check cache status
	if startHour != nil && endHour != nil {
		status := analytics.GetRangeCacheStatus(ctx, epinetID, *startHour, *endHour)

		switch status.Action {
		// case "proceed":
		//	log.Printf("DEBUG: Range fully cached, proceeding with computation")
		case "refresh_current":
			// log.Printf("DEBUG: Current hour expired, refreshing current data")
			go func() {
				err := analytics.LoadCurrentHourData(ctx)
				if err != nil {
					log.Printf("ERROR: Background LoadCurrentHourData failed: %v", err)
					//} else {
					//	log.Printf("DEBUG: Current hour refresh completed")
				}
			}()
			c.JSON(http.StatusOK, gin.H{"status": "loading"})
			return
		case "load_range":
			// log.Printf("DEBUG: Range not fully cached, loading historical data")
			go func() {
				err := analytics.LoadHourlyEpinetData(ctx, 672)
				if err != nil {
					log.Printf("ERROR: Background LoadHourlyEpinetData failed: %v", err)
					//} else {
					//	log.Printf("DEBUG: Background warming completed")
				}
			}()
			c.JSON(http.StatusOK, gin.H{"status": "loading"})
			return
		}
	}

	// Create filters
	filters := &analytics.SankeyFilters{
		VisitorType:    visitorType,
		SelectedUserID: selectedUserID,
		StartHour:      startHour,
		EndHour:        endHour,
	}

	// Compute sankey diagram
	sankey, err := analytics.ComputeEpinetSankey(ctx, epinetID, filters)
	if err != nil {
		log.Printf("ERROR: ComputeEpinetSankey failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute epinet sankey"})
		return
	}

	log.Printf("DEBUG: Sankey computed successfully with %d nodes and %d links", len(sankey.Nodes), len(sankey.Links))
	c.JSON(http.StatusOK, sankey)
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
