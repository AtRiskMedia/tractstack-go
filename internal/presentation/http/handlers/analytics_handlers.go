// Package handlers provides HTTP handlers for analytics endpoints
package handlers

import (
	"net/http"
	"strconv"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/gin-gonic/gin"
)

// AnalyticsHandlers contains all analytics-related HTTP handlers
type AnalyticsHandlers struct {
	// REMOVED: Analytics services (not yet migrated to clean architecture)
	// TODO: Add analytics service fields when they're migrated from legacy /services to /internal/application/services
	// dashboardAnalyticsService    *services.DashboardAnalyticsService
	// epinetAnalyticsService       *services.EpinetAnalyticsService
	// leadAnalyticsService         *services.LeadAnalyticsService
	// contentAnalyticsService      *services.ContentAnalyticsService
	// analyticsOrchestratorService *services.AnalyticsOrchestratorService

	warmingService *services.WarmingService
}

// NewAnalyticsHandlers creates analytics handlers with injected dependencies
func NewAnalyticsHandlers(
	// REMOVED: Analytics service parameters (not yet migrated)
	// TODO: Add analytics service parameters when they're migrated
	// dashboardAnalyticsService *services.DashboardAnalyticsService,
	// epinetAnalyticsService *services.EpinetAnalyticsService,
	// leadAnalyticsService *services.LeadAnalyticsService,
	// contentAnalyticsService *services.ContentAnalyticsService,
	// analyticsOrchestratorService *services.AnalyticsOrchestratorService,
	warmingService *services.WarmingService,
) *AnalyticsHandlers {
	return &AnalyticsHandlers{
		// REMOVED: Analytics service assignments (not yet migrated)
		// TODO: Add analytics service assignments when they're migrated
		// dashboardAnalyticsService:    dashboardAnalyticsService,
		// epinetAnalyticsService:       epinetAnalyticsService,
		// leadAnalyticsService:         leadAnalyticsService,
		// contentAnalyticsService:      contentAnalyticsService,
		// analyticsOrchestratorService: analyticsOrchestratorService,
		warmingService: warmingService,
	}
}

// HandleDashboardAnalytics handles GET /api/v1/analytics/dashboard
func (h *AnalyticsHandlers) HandleDashboardAnalytics(c *gin.Context) {
	// tenantCtx, exists := middleware.GetTenantContext(c)
	// if !exists {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
	// 	return
	// }

	// TODO: Implement using migrated analytics services when available
	// For now, return placeholder response
	c.JSON(http.StatusOK, gin.H{
		"dashboard": gin.H{
			"status":  "not_implemented",
			"message": "Analytics services not yet migrated to clean architecture",
		},
	})
}

// HandleEpinetSankey handles GET /api/v1/analytics/epinets/:id
func (h *AnalyticsHandlers) HandleEpinetSankey(c *gin.Context) {
	// tenantCtx, exists := middleware.GetTenantContext(c)
	// if !exists {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
	// 	return
	// }

	// epinetID := c.Param("id")
	// startHour, _ := strconv.Atoi(c.DefaultQuery("startHour", "168"))
	// endHour, _ := strconv.Atoi(c.DefaultQuery("endHour", "0"))
	// visitorType := c.DefaultQuery("visitorType", "all")
	// selectedUserID := c.Query("selectedUserId")

	// TODO: Implement using migrated analytics services when available
	// For now, return placeholder response
	c.JSON(http.StatusOK, gin.H{
		"dashboard":          gin.H{"status": "not_implemented"},
		"leads":              gin.H{"status": "not_implemented"},
		"epinet":             gin.H{"status": "not_implemented"},
		"userCounts":         []interface{}{},
		"hourlyNodeActivity": gin.H{},
	})
}

// HandleStoryfragmentAnalytics handles GET /api/v1/analytics/storyfragments
func (h *AnalyticsHandlers) HandleStoryfragmentAnalytics(c *gin.Context) {
	// tenantCtx, exists := middleware.GetTenantContext(c)
	// if !exists {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
	// 	return
	// }

	// TODO: Implement using migrated analytics services when available
	c.JSON(http.StatusOK, gin.H{
		"storyfragments": []interface{}{},
		"status":         "not_implemented",
	})
}

// HandleLeadMetrics handles GET /api/v1/analytics/leads
func (h *AnalyticsHandlers) HandleLeadMetrics(c *gin.Context) {
	// tenantCtx, exists := middleware.GetTenantContext(c)
	// if !exists {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
	// 	return
	// }

	// startHour, endHour := h.parseTimeRange(c)

	// TODO: Implement using migrated analytics services when available
	c.JSON(http.StatusOK, gin.H{
		"leads":  gin.H{"status": "not_implemented"},
		"status": "analytics_services_not_migrated",
	})
}

// Helper methods

func (h *AnalyticsHandlers) parseTimeRange(c *gin.Context) (int, int) {
	startHour, _ := strconv.Atoi(c.DefaultQuery("startHour", "168"))
	endHour, _ := strconv.Atoi(c.DefaultQuery("endHour", "0"))
	return startHour, endHour
}
