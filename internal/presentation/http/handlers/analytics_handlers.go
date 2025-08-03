// Package handlers provides HTTP handlers for analytics endpoints
package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/adapters"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
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
	tenantManager             *tenant.Manager
	logger                    *logging.ChanneledLogger
}

// NewAnalyticsHandlers creates analytics handlers with injected dependencies
func NewAnalyticsHandlers(
	analyticsService *services.AnalyticsService,
	dashboardAnalyticsService *services.DashboardAnalyticsService,
	epinetAnalyticsService *services.EpinetAnalyticsService,
	leadAnalyticsService *services.LeadAnalyticsService,
	contentAnalyticsService *services.ContentAnalyticsService,
	warmingService *services.WarmingService,
	tenantManager *tenant.Manager,
	logger *logging.ChanneledLogger,
) *AnalyticsHandlers {
	return &AnalyticsHandlers{
		analyticsService:          analyticsService,
		dashboardAnalyticsService: dashboardAnalyticsService,
		epinetAnalyticsService:    epinetAnalyticsService,
		leadAnalyticsService:      leadAnalyticsService,
		contentAnalyticsService:   contentAnalyticsService,
		warmingService:            warmingService,
		tenantManager:             tenantManager,
		logger:                    logger,
	}
}

// HandleDashboardAnalytics handles GET /api/v1/analytics/dashboard
func (h *AnalyticsHandlers) HandleDashboardAnalytics(c *gin.Context) {
	start := time.Now()
	h.logger.Analytics().Debug("Received dashboard analytics request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	startHour, endHour := h.parseTimeRange(c)
	epinetIDs, err := h.getEpinetIDs(tenantCtx)
	if err != nil || len(epinetIDs) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get epinet IDs for analytics"})
		return
	}
	epinetID := epinetIDs[0]

	cacheStatus := tenantCtx.CacheManager.GetRangeCacheStatus(tenantCtx.TenantID, epinetID, startHour, endHour)

	if cacheStatus.Action != "proceed" {
		h.triggerBackgroundWarming(tenantCtx, startHour, cacheStatus)
		c.JSON(http.StatusOK, gin.H{"dashboard": gin.H{"status": "loading"}})
		return
	}

	dashboard, err := h.dashboardAnalyticsService.ComputeDashboard(tenantCtx, startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Analytics().Info("Dashboard analytics request completed", "startHour", startHour, "endHour", endHour, "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{"dashboard": dashboard})
}

// HandleEpinetSankey handles GET /api/v1/analytics/epinets/:id
func (h *AnalyticsHandlers) HandleEpinetSankey(c *gin.Context) {
	start := time.Now()
	h.logger.Analytics().Debug("Received epinet analytics request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	epinetID := c.Param("id")
	startHour, endHour := h.parseTimeRange(c)

	cacheStatus := tenantCtx.CacheManager.GetRangeCacheStatus(tenantCtx.TenantID, epinetID, startHour, endHour)
	if cacheStatus.Action != "proceed" {
		h.triggerBackgroundWarming(tenantCtx, startHour, cacheStatus)
		c.JSON(http.StatusOK, gin.H{
			"epinet":             gin.H{"status": "loading"},
			"userCounts":         []services.UserCount{},
			"hourlyNodeActivity": make(services.HourlyActivity),
		})
		return
	}

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

	userCounts, _ := h.analyticsService.GetFilteredVisitorCounts(tenantCtx, epinetID, visitorType, &startHour, &endHour)
	hourlyNodeActivity, _ := h.contentAnalyticsService.GetHourlyNodeActivity(tenantCtx, epinetID, &startHour, &endHour)

	h.logger.Analytics().Info("Epinet analytics request completed", "epinetId", epinetID, "startHour", startHour, "endHour", endHour, "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"epinet":             epinet,
		"userCounts":         userCounts,
		"hourlyNodeActivity": hourlyNodeActivity,
	})
}

// HandleStoryfragmentAnalytics handles GET /api/v1/analytics/storyfragments
func (h *AnalyticsHandlers) HandleStoryfragmentAnalytics(c *gin.Context) {
	start := time.Now()
	h.logger.Analytics().Debug("Received storyfragment analytics request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	startHour, endHour := h.parseTimeRange(c)
	epinetIDs, err := h.getEpinetIDs(tenantCtx)
	if err != nil || len(epinetIDs) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get epinet IDs"})
		return
	}
	epinetID := epinetIDs[0]

	cacheStatus := tenantCtx.CacheManager.GetRangeCacheStatus(tenantCtx.TenantID, epinetID, startHour, endHour)
	if cacheStatus.Action != "proceed" {
		h.triggerBackgroundWarming(tenantCtx, startHour, cacheStatus)
		c.JSON(http.StatusOK, gin.H{"storyfragments": gin.H{"status": "loading"}})
		return
	}

	storyfragments, err := h.contentAnalyticsService.GetStoryfragmentAnalytics(tenantCtx, epinetIDs, startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Analytics().Info("Storyfragment analytics request completed", "epinetId", epinetID, "startHour", startHour, "endHour", endHour, "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{"storyfragments": storyfragments})
}

// HandleLeadMetrics handles GET /api/v1/analytics/leads
func (h *AnalyticsHandlers) HandleLeadMetrics(c *gin.Context) {
	start := time.Now()
	h.logger.Analytics().Debug("Received lead analytics request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	startHour, endHour := h.parseTimeRange(c)
	epinetIDs, err := h.getEpinetIDs(tenantCtx)
	if err != nil || len(epinetIDs) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get epinet IDs"})
		return
	}
	epinetID := epinetIDs[0]

	cacheStatus := tenantCtx.CacheManager.GetRangeCacheStatus(tenantCtx.TenantID, epinetID, startHour, endHour)
	if cacheStatus.Action != "proceed" {
		h.triggerBackgroundWarming(tenantCtx, startHour, cacheStatus)
		c.JSON(http.StatusOK, gin.H{"leads": gin.H{"status": "loading"}})
		return
	}

	leadMetrics, err := h.leadAnalyticsService.ComputeLeadMetrics(tenantCtx, startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Analytics().Info("Lead analytics request completed", "startHour", startHour, "endHour", endHour, "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{"leads": leadMetrics})
}

// HandleAllAnalytics provides a composite analytics response.
func (h *AnalyticsHandlers) HandleAllAnalytics(c *gin.Context) {
	start := time.Now()
	h.logger.Analytics().Debug("Received all analytics request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	startHour, endHour := h.parseTimeRange(c)
	epinetIDs, err := h.getEpinetIDs(tenantCtx)
	if err != nil || len(epinetIDs) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get epinet IDs"})
		return
	}
	epinetID := epinetIDs[0]

	cacheStatus := tenantCtx.CacheManager.GetRangeCacheStatus(tenantCtx.TenantID, epinetID, startHour, endHour)
	if cacheStatus.Action != "proceed" {
		h.triggerBackgroundWarming(tenantCtx, startHour, cacheStatus)
		c.JSON(http.StatusOK, gin.H{
			"dashboard":          gin.H{"status": "loading"},
			"leads":              gin.H{"status": "loading"},
			"epinet":             gin.H{"status": "loading"},
			"userCounts":         []services.UserCount{},
			"hourlyNodeActivity": make(services.HourlyActivity),
		})
		return
	}

	var dashboard *services.DashboardAnalytics
	var leadMetrics *services.LeadMetrics
	var epinet *services.SankeyDiagram
	var userCounts []services.UserCount
	var hourlyNodeActivity services.HourlyActivity
	var wg sync.WaitGroup
	errChan := make(chan error, 5)

	wg.Add(5)

	go func() {
		defer wg.Done()
		var err error
		dashboard, err = h.dashboardAnalyticsService.ComputeDashboard(tenantCtx, startHour, endHour)
		if err != nil {
			errChan <- fmt.Errorf("dashboard error: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		leadMetrics, err = h.leadAnalyticsService.ComputeLeadMetrics(tenantCtx, startHour, endHour)
		if err != nil {
			errChan <- fmt.Errorf("lead metrics error: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		visitorType := c.DefaultQuery("visitorType", "all")
		selectedUserID := c.Query("userId")
		var selectedUserIDPtr *string
		if selectedUserID != "" {
			selectedUserIDPtr = &selectedUserID
		}
		filters := &services.SankeyFilters{VisitorType: visitorType, SelectedUserID: selectedUserIDPtr, StartHour: &startHour, EndHour: &endHour}
		epinet, err = h.epinetAnalyticsService.ComputeEpinetSankey(tenantCtx, epinetID, filters)
		if err != nil {
			errChan <- fmt.Errorf("epinet sankey error: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		visitorType := c.DefaultQuery("visitorType", "all")
		userCounts, err = h.analyticsService.GetFilteredVisitorCounts(tenantCtx, epinetID, visitorType, &startHour, &endHour)
		if err != nil {
			errChan <- fmt.Errorf("user counts error: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		hourlyNodeActivity, err = h.contentAnalyticsService.GetHourlyNodeActivity(tenantCtx, epinetID, &startHour, &endHour)
		if err != nil {
			errChan <- fmt.Errorf("hourly activity error: %w", err)
		}
	}()

	wg.Wait()
	close(errChan)

	for err := range errChan {
		log.Printf("Error during parallel analytics computation: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute all analytics"})
		return
	}

	h.logger.Analytics().Info("All analytics request completed", "startHour", startHour, "endHour", endHour, "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"dashboard":          dashboard,
		"leads":              leadMetrics,
		"epinet":             epinet,
		"userCounts":         userCounts,
		"hourlyNodeActivity": hourlyNodeActivity,
	})
}

// --- Helper Methods ---

func (h *AnalyticsHandlers) parseTimeRange(c *gin.Context) (int, int) {
	startHour, _ := strconv.Atoi(c.DefaultQuery("startHour", "168"))
	endHour, _ := strconv.Atoi(c.DefaultQuery("endHour", "0"))
	return startHour, endHour
}

func (h *AnalyticsHandlers) getEpinetIDs(tenantCtx *tenant.Context) ([]string, error) {
	epinetRepo := tenantCtx.EpinetRepo()
	epinets, err := epinetRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range epinets {
		if e != nil {
			ids = append(ids, e.ID)
		}
	}
	return ids, nil
}

func (h *AnalyticsHandlers) triggerBackgroundWarming(tenantCtx *tenant.Context, startHour int, status types.RangeCacheStatus) {
	locker := caching.GetGlobalWarmingLock()
	lockKey := fmt.Sprintf("warm:hourly:%s:%d", tenantCtx.TenantID, startHour)

	if locker.TryLock(lockKey) {
		log.Printf("Lock acquired for '%s'. Starting background analytics warming.", lockKey)
		go func() {
			defer locker.Unlock(lockKey)
			bgCtx, err := h.tenantManager.NewContextFromID(tenantCtx.TenantID)
			if err != nil {
				log.Printf("ERROR: Failed to create background context for warming tenant %s: %v", tenantCtx.TenantID, err)
				return
			}
			defer bgCtx.Close()

			writeCache := adapters.NewWriteOnlyAnalyticsCacheAdapter(bgCtx.CacheManager)
			if status.Action == "refresh_current" {
				if err := h.warmingService.WarmRecentHours(bgCtx, writeCache, status.MissingHours); err != nil {
					log.Printf("ERROR: Rapid refresh for key '%s' failed: %v", lockKey, err)
				}
			} else {
				if err := h.warmingService.WarmHourlyEpinetData(bgCtx, writeCache, startHour); err != nil {
					log.Printf("ERROR: Full warming for key '%s' failed: %v", lockKey, err)
				}
			}
		}()
	} else {
		log.Printf("Cache warming already in progress for key '%s'. Skipping new task.", lockKey)
	}
}
