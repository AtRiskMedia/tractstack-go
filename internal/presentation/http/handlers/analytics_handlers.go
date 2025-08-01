// Package handlers provides HTTP handlers for analytics endpoints
package handlers

import (
	"net/http"
	"strconv"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// AnalyticsHandlers contains all analytics-related HTTP handlers
type AnalyticsHandlers struct {
	analyticsService          *services.AnalyticsService
	dashboardAnalyticsService *services.DashboardAnalyticsService
	epinetAnalyticsService    *services.EpinetAnalyticsService
	leadAnalyticsService      *services.LeadAnalyticsService
	contentAnalyticsService   *services.ContentAnalyticsService
	warmingService            *services.WarmingService
}

// NewAnalyticsHandlers creates analytics handlers with injected dependencies
func NewAnalyticsHandlers(
	analyticsService *services.AnalyticsService,
	dashboardAnalyticsService *services.DashboardAnalyticsService,
	epinetAnalyticsService *services.EpinetAnalyticsService,
	leadAnalyticsService *services.LeadAnalyticsService,
	contentAnalyticsService *services.ContentAnalyticsService,
	warmingService *services.WarmingService,
) *AnalyticsHandlers {
	return &AnalyticsHandlers{
		analyticsService:          analyticsService,
		dashboardAnalyticsService: dashboardAnalyticsService,
		epinetAnalyticsService:    epinetAnalyticsService,
		leadAnalyticsService:      leadAnalyticsService,
		contentAnalyticsService:   contentAnalyticsService,
		warmingService:            warmingService,
	}
}

// HandleDashboardAnalytics handles GET /api/v1/analytics/dashboard
func (h *AnalyticsHandlers) HandleDashboardAnalytics(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	startHour, _ := strconv.Atoi(c.DefaultQuery("startHour", "168"))
	endHour, _ := strconv.Atoi(c.DefaultQuery("endHour", "0"))

	dashboard, err := h.dashboardAnalyticsService.ComputeDashboard(tenantCtx, startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"dashboard": dashboard})
}

// HandleEpinetSankey handles GET /api/v1/analytics/epinets/:id
func (h *AnalyticsHandlers) HandleEpinetSankey(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	epinetID := c.Param("id")
	startHour, _ := strconv.Atoi(c.DefaultQuery("startHour", "168"))
	endHour, _ := strconv.Atoi(c.DefaultQuery("endHour", "0"))
	visitorType := c.DefaultQuery("visitorType", "all")
	selectedUserID := c.Query("selectedUserId")

	var selectedUserIDPtr *string
	if selectedUserID != "" {
		selectedUserIDPtr = &selectedUserID
	}

	filters := &services.SankeyFilters{
		VisitorType:    visitorType,
		SelectedUserID: selectedUserIDPtr,
		StartHour:      &startHour,
		EndHour:        &endHour,
	}

	epinet, err := h.epinetAnalyticsService.ComputeEpinetSankey(tenantCtx, epinetID, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	userCounts, err := h.analyticsService.GetFilteredVisitorCounts(tenantCtx, epinetID, visitorType, &startHour, &endHour)
	if err != nil {
		userCounts = []services.UserCount{}
	}

	hourlyNodeActivity, err := h.contentAnalyticsService.GetHourlyNodeActivity(tenantCtx, epinetID, &startHour, &endHour)
	if err != nil {
		hourlyNodeActivity = make(services.HourlyActivity)
	}

	c.JSON(http.StatusOK, gin.H{
		"epinet":             epinet,
		"userCounts":         userCounts,
		"hourlyNodeActivity": hourlyNodeActivity,
	})
}

// HandleStoryfragmentAnalytics handles GET /api/v1/analytics/storyfragments
func (h *AnalyticsHandlers) HandleStoryfragmentAnalytics(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	startHour, _ := strconv.Atoi(c.DefaultQuery("startHour", "672"))
	endHour, _ := strconv.Atoi(c.DefaultQuery("endHour", "0"))

	epinetRepo := tenantCtx.EpinetRepo()
	epinets, err := epinetRepo.FindAll(tenantCtx.TenantID)
	if err != nil || len(epinets) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get epinet IDs"})
		return
	}

	var epinetIDs []string
	for _, epinet := range epinets {
		if epinet != nil {
			epinetIDs = append(epinetIDs, epinet.ID)
		}
	}

	storyfragments, err := h.contentAnalyticsService.GetStoryfragmentAnalytics(tenantCtx, epinetIDs, startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"storyfragments": storyfragments})
}

// HandleLeadMetrics handles GET /api/v1/analytics/leads
func (h *AnalyticsHandlers) HandleLeadMetrics(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	startHour, _ := strconv.Atoi(c.DefaultQuery("startHour", "168"))
	endHour, _ := strconv.Atoi(c.DefaultQuery("endHour", "0"))

	leadMetrics, err := h.leadAnalyticsService.ComputeLeadMetrics(tenantCtx, startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"leads": leadMetrics})
}

// Helper methods

func (h *AnalyticsHandlers) parseTimeRange(c *gin.Context) (int, int) {
	startHour, _ := strconv.Atoi(c.DefaultQuery("startHour", "168"))
	endHour, _ := strconv.Atoi(c.DefaultQuery("endHour", "0"))
	return startHour, endHour
}
