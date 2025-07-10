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
	startHourStr := c.Query("startHour")
	endHourStr := c.Query("endHour")

	if startHourStr != "" && endHourStr != "" {
		start, err := strconv.Atoi(startHourStr)
		if err == nil && start > 0 {
			end, err := strconv.Atoi(endHourStr)
			if err == nil && end >= 0 && start > end {
				startHour = &start
				endHour = &end
				log.Printf("DEBUG: Parsed startHour=%d, endHour=%d", start, end)
			} else {
				log.Printf("DEBUG: Invalid endHour '%s' or startHour (%d) <= endHour (%d)", endHourStr, start, end)
			}
		} else {
			log.Printf("DEBUG: Invalid startHour '%s'", startHourStr)
		}
	} else {
		log.Printf("DEBUG: startHour or endHour missing")
	}

	// Check if range is fully cached
	if startHour != nil && endHour != nil {
		if analytics.IsRangeFullyCached(ctx, epinetID, *startHour, *endHour) {
			log.Printf("DEBUG: Range fully cached, proceeding with computation")
		} else {
			log.Printf("DEBUG: Range not fully cached, returning loading status and starting background warming")

			// Start background warming for gaps
			go func() {
				err := analytics.LoadHourlyEpinetData(ctx, 672)
				if err != nil {
					log.Printf("ERROR: Background LoadHourlyEpinetData failed: %v", err)
				} else {
					log.Printf("DEBUG: Background warming completed")
				}
			}()

			// Return loading status
			c.JSON(http.StatusOK, gin.H{"status": "loading"})
			return
		}
	} else {
		// No specific range provided, check if bulk cache is initialized
		if analytics.IsBulkCacheInitialized(ctx, epinetID) {
			log.Printf("DEBUG: Bulk cache initialized, proceeding with computation")
		} else {
			log.Printf("DEBUG: Bulk cache not initialized, returning loading status and starting background warming")

			// Start background warming
			go func() {
				err := analytics.LoadHourlyEpinetData(ctx, 672)
				if err != nil {
					log.Printf("ERROR: Background LoadHourlyEpinetData failed: %v", err)
				} else {
					log.Printf("DEBUG: Background warming completed")
				}
			}()

			// Return loading status
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
	log.Printf("DEBUG: About to call ComputeEpinetSankey")
	sankey, err := analytics.ComputeEpinetSankey(ctx, epinetID, filters)
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
