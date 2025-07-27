// Package api provides HTTP handlers for analytics endpoints.
package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/services"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
)

// HandleDashboardAnalytics handles GET /api/v1/analytics/dashboard
func HandleDashboardAnalytics(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	startHour, endHour := parseTimeRange(c)
	epinetIDs, err := getEpinetIDs(ctx)
	if err != nil || len(epinetIDs) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get epinet IDs"})
		return
	}

	cacheStatus := cache.GetRangeCacheStatus(ctx, epinetIDs[0], startHour, endHour)

	if cacheStatus.Action != "proceed" {
		locker := cache.GetGlobalWarmingLock()
		lockKey := fmt.Sprintf("warm:hourly:%s:%d", ctx.TenantID, startHour)

		if locker.TryLock(lockKey) {
			log.Printf("Lock acquired for '%s'. Starting background cache warming.", lockKey)
			tenantID := ctx.TenantID
			go func(id string, sh int, lk string) {
				defer locker.Unlock(lk)
				bgCtx, err := tenant.NewContextFromID(id)
				if err != nil {
					log.Printf("ERROR: Failed to create background context for tenant %s: %v", id, err)
					return
				}
				defer bgCtx.Close()
				wCache := cache.NewWriteOnlyAnalyticsCacheAdapter(cache.GetGlobalManager())
				warmer := services.NewCacheWarmingService(wCache, bgCtx)
				if err := warmer.WarmHourlyEpinetData(sh); err != nil {
					log.Printf("ERROR: Background cache warming for key '%s' failed: %v", lk, err)
				}
			}(tenantID, startHour, lockKey)
		} else {
			log.Printf("Cache warming already in progress for key '%s'. Skipping new task.", lockKey)
		}

		c.JSON(http.StatusOK, gin.H{
			"dashboard": gin.H{
				"status":  "loading",
				"message": "Cache warming in progress, please retry in a few moments",
			},
		})
		return
	}

	rCache := cache.NewReadOnlyAnalyticsCacheAdapter(cache.GetGlobalManager())
	analyticsService := services.NewAnalyticsService(rCache, ctx.TenantID)
	analyticsService.SetContext(ctx)
	dashboard, err := analyticsService.ComputeDashboard(startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute dashboard analytics"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"dashboard": dashboard})
}

// HandleEpinetSankey handles GET /api/v1/analytics/epinet/:id
func HandleEpinetSankey(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	epinetID := c.Param("id")
	startHour, _ := strconv.Atoi(c.DefaultQuery("startHour", "168"))
	endHour, _ := strconv.Atoi(c.DefaultQuery("endHour", "0"))

	cacheStatus := cache.GetRangeCacheStatus(ctx, epinetID, startHour, endHour)

	if cacheStatus.Action != "proceed" {
		locker := cache.GetGlobalWarmingLock()
		lockKey := fmt.Sprintf("warm:hourly:%s:%d", ctx.TenantID, startHour)

		if locker.TryLock(lockKey) {
			log.Printf("Lock acquired for '%s'. Starting background cache warming.", lockKey)
			tenantID := ctx.TenantID
			go func(id string, sh int, lk string) {
				defer locker.Unlock(lk)
				bgCtx, err := tenant.NewContextFromID(id)
				if err != nil {
					log.Printf("ERROR: Failed to create background context for tenant %s: %v", id, err)
					return
				}
				defer bgCtx.Close()
				wCache := cache.NewWriteOnlyAnalyticsCacheAdapter(cache.GetGlobalManager())
				warmer := services.NewCacheWarmingService(wCache, bgCtx)
				if err := warmer.WarmHourlyEpinetData(sh); err != nil {
					log.Printf("ERROR: Background cache warming for key '%s' failed: %v", lk, err)
				}
			}(tenantID, startHour, lockKey)
		} else {
			log.Printf("Cache warming already in progress for key '%s'. Skipping new task.", lockKey)
		}

		c.JSON(http.StatusOK, gin.H{
			"epinet": gin.H{
				"status":  "loading",
				"message": "Cache warming in progress, please retry in a few moments",
			},
			"userCounts":         []models.UserCount{},
			"hourlyNodeActivity": models.HourlyActivity{},
		})
		return
	}

	rCache := cache.NewReadOnlyAnalyticsCacheAdapter(cache.GetGlobalManager())
	analyticsService := services.NewAnalyticsService(rCache, ctx.TenantID)
	analyticsService.SetContext(ctx)

	visitorType := c.DefaultQuery("visitorType", "all")
	selectedUserID := c.Query("userId")
	var startHourPtr, endHourPtr *int
	var selectedUserIDPtr *string
	startHourPtr = &startHour
	endHourPtr = &endHour
	if selectedUserID != "" {
		selectedUserIDPtr = &selectedUserID
	}

	epinet, err := analyticsService.ComputeEpinetSankey(epinetID, &models.SankeyFilters{
		VisitorType: visitorType, SelectedUserID: selectedUserIDPtr, StartHour: startHourPtr, EndHour: endHourPtr,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute epinet sankey"})
		return
	}
	userCounts, err := analyticsService.GetFilteredVisitorCounts(epinetID, visitorType, startHourPtr, endHourPtr)
	if err != nil {
		userCounts = []models.UserCount{}
	}
	hourlyNodeActivity, err := analyticsService.GetHourlyNodeActivity(epinetID, startHourPtr, endHourPtr)
	if err != nil {
		hourlyNodeActivity = models.HourlyActivity{}
	}

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
	epinetIDs, err := getEpinetIDs(ctx)
	if err != nil || len(epinetIDs) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get epinet IDs"})
		return
	}
	cacheStatus := cache.GetRangeCacheStatus(ctx, epinetIDs[0], 672, 0)
	if cacheStatus.Action != "proceed" {
		c.JSON(http.StatusOK, gin.H{
			"storyfragments": gin.H{"status": "loading", "message": "Cache warming may be in progress."},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"storyfragments": []models.StoryfragmentAnalytics{}})
}

// HandleLeadMetrics handles GET /api/v1/analytics/leads
func HandleLeadMetrics(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	startHour, endHour := parseTimeRange(c)
	epinetIDs, err := getEpinetIDs(ctx)
	if err != nil || len(epinetIDs) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get epinet IDs"})
		return
	}
	cacheStatus := cache.GetRangeCacheStatus(ctx, epinetIDs[0], startHour, endHour)
	if cacheStatus.Action != "proceed" {
		c.JSON(http.StatusOK, gin.H{
			"leads": gin.H{"status": "loading", "message": "Cache warming may be in progress."},
		})
		return
	}
	rCache := cache.NewReadOnlyAnalyticsCacheAdapter(cache.GetGlobalManager())
	analyticsService := services.NewAnalyticsService(rCache, ctx.TenantID)
	analyticsService.SetContext(ctx)
	metrics, err := analyticsService.ComputeLeadMetrics(startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute lead metrics"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"leads": metrics})
}

func HandleAllAnalytics(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	startHour, endHour := parseTimeRange(c)
	visitorType := c.DefaultQuery("visitorType", "all")
	selectedUserID := c.Query("userId")

	cacheManager := cache.GetGlobalManager()
	epinetService := content.NewEpinetService(ctx, cacheManager)
	epinetIDs, err := epinetService.GetAllIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get epinet IDs"})
		return
	}

	var epinetID string
	if len(epinetIDs) > 0 {
		epinets, err := epinetService.GetByIDs(epinetIDs)
		if err == nil {
			for _, epinet := range epinets {
				if epinet != nil && epinet.Promoted {
					epinetID = epinet.ID
					break
				}
				if epinetID == "" && epinet != nil {
					epinetID = epinet.ID
				}
			}
		}
	}

	cacheStatus := cache.GetRangeCacheStatus(ctx, epinetID, startHour, endHour)

	if cacheStatus.Action != "proceed" {
		locker := cache.GetGlobalWarmingLock()
		lockKey := fmt.Sprintf("warm:hourly:%s:%d", ctx.TenantID, startHour)

		if locker.TryLock(lockKey) {
			log.Printf("Lock acquired for '%s' from /all. Starting background cache warming.", lockKey)
			tenantID := ctx.TenantID
			go func(id string, sh int, lk string) {
				defer locker.Unlock(lk)
				bgCtx, err := tenant.NewContextFromID(id)
				if err != nil {
					log.Printf("ERROR: Failed to create background context for tenant %s: %v", id, err)
					return
				}
				defer bgCtx.Close()
				wCache := cache.NewWriteOnlyAnalyticsCacheAdapter(cache.GetGlobalManager())
				warmer := services.NewCacheWarmingService(wCache, bgCtx)
				if err := warmer.WarmHourlyEpinetData(sh); err != nil {
					log.Printf("ERROR: Background cache warming for key '%s' failed: %v", lk, err)
				}
			}(tenantID, startHour, lockKey)
		} else {
			log.Printf("Cache warming already in progress for key '%s'. Skipping new task.", lockKey)
		}

		c.JSON(http.StatusOK, gin.H{
			"dashboard":          gin.H{"status": "loading", "message": "Cache warming in progress..."},
			"leads":              gin.H{"status": "loading", "message": "Cache warming in progress..."},
			"epinet":             gin.H{"status": "loading", "message": "Cache warming in progress..."},
			"userCounts":         []models.UserCount{},
			"hourlyNodeActivity": models.HourlyActivity{},
		})
		return
	}

	rCache := cache.NewReadOnlyAnalyticsCacheAdapter(cache.GetGlobalManager())
	analyticsService := services.NewAnalyticsService(rCache, ctx.TenantID)
	analyticsService.SetContext(ctx)

	dashboard, err := analyticsService.ComputeDashboard(startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute dashboard analytics"})
		return
	}

	leadMetrics, err := analyticsService.ComputeLeadMetrics(startHour, endHour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute lead metrics"})
		return
	}

	var epinet *models.SankeyDiagram
	var userCounts []models.UserCount
	var hourlyNodeActivity models.HourlyActivity

	if epinetID != "" {
		var startHourPtr, endHourPtr *int
		var selectedUserIDPtr *string

		startHourPtr = &startHour
		endHourPtr = &endHour
		if selectedUserID != "" {
			selectedUserIDPtr = &selectedUserID
		}

		epinet, err = analyticsService.ComputeEpinetSankey(epinetID, &models.SankeyFilters{
			VisitorType:    visitorType,
			SelectedUserID: selectedUserIDPtr,
			StartHour:      startHourPtr,
			EndHour:        endHourPtr,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute epinet sankey"})
			return
		}

		userCounts, err = analyticsService.GetFilteredVisitorCounts(epinetID, visitorType, startHourPtr, endHourPtr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get visitor counts"})
			return
		}

		hourlyNodeActivity, err = analyticsService.GetHourlyNodeActivity(epinetID, startHourPtr, endHourPtr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get hourly node activity"})
			return
		}
	} else {
		userCounts = []models.UserCount{}
		hourlyNodeActivity = models.HourlyActivity{}
	}

	c.JSON(http.StatusOK, gin.H{
		"dashboard":          dashboard,
		"leads":              leadMetrics,
		"epinet":             epinet,
		"userCounts":         userCounts,
		"hourlyNodeActivity": hourlyNodeActivity,
	})
}

func parseTimeRange(c *gin.Context) (startHour, endHour int) {
	startHourStr := c.Query("startHour")
	endHourStr := c.Query("endHour")
	if startHourStr != "" && endHourStr != "" {
		var err error
		startHour, err = strconv.Atoi(startHourStr)
		if err != nil {
			return 168, 0
		}
		endHour, err = strconv.Atoi(endHourStr)
		if err != nil {
			return 168, 0
		}
		return startHour, endHour
	}
	duration := c.DefaultQuery("duration", "weekly")
	switch duration {
	case "daily":
		return 24, 0
	case "weekly":
		return 168, 0
	case "monthly":
		return 672, 0
	default:
		return 168, 0
	}
}

func getEpinetIDs(ctx *tenant.Context) ([]string, error) {
	epinetService := content.NewEpinetService(ctx, cache.GetGlobalManager())
	return epinetService.GetAllIDs()
}
