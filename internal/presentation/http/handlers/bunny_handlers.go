// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// BunnyImportRequest names the Bunny video to import as a bunny-video resource.
type BunnyImportRequest struct {
	GUID string `json:"guid"`
}

// BunnyApplyRecommendedRequest carries StoryKeep's PUBLIC_GO_BACKEND.
type BunnyApplyRecommendedRequest struct {
	GoBackend string `json:"goBackend"`
}

// BunnyHandlers provides endpoints for the Bunny Stream integration.
type BunnyHandlers struct {
	bunnyService *services.BunnyService
	logger       *logging.ChanneledLogger
	perfTracker  *performance.Tracker
}

// NewBunnyHandlers creates a new instance of BunnyHandlers.
func NewBunnyHandlers(
	bunnyService *services.BunnyService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *BunnyHandlers {
	return &BunnyHandlers{
		bunnyService: bunnyService,
		logger:       logger,
		perfTracker:  perfTracker,
	}
}

// HandleGetLibraries handles GET /api/v1/bunny/libraries (admin).
// It returns only the id and name of each library, never the keys Bunny sends.
func (h *BunnyHandlers) HandleGetLibraries(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("bunny_get_libraries", tenantCtx.TenantID)
	defer marker.Complete()

	if tenantCtx.Config.BunnyAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bunny account API key is not configured"})
		return
	}

	libraries, err := h.bunnyService.ListLibraries(tenantCtx)
	if err != nil {
		h.logger.System().Error("Failed to list Bunny libraries", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"libraries": libraries})
}

// HandleGetLibraryStatus handles GET /api/v1/bunny/library/status (admin).
func (h *BunnyHandlers) HandleGetLibraryStatus(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("bunny_get_library_status", tenantCtx.TenantID)
	defer marker.Complete()

	if tenantCtx.Config.BunnyAPIKey == "" || tenantCtx.Config.BunnyLibraryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No Bunny library is selected"})
		return
	}

	status, err := h.bunnyService.LibraryStatus(tenantCtx)
	if err != nil {
		h.logger.System().Error("Failed to read Bunny library status", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, status)
}

// HandleApplyRecommended handles POST /api/v1/bunny/library/apply-recommended (admin).
func (h *BunnyHandlers) HandleApplyRecommended(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("bunny_apply_recommended", tenantCtx.TenantID)
	defer marker.Complete()

	var req BunnyApplyRecommendedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	if tenantCtx.Config.BunnyAPIKey == "" || tenantCtx.Config.BunnyLibraryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No Bunny library is selected"})
		return
	}

	result, err := h.bunnyService.ApplyRecommendedSettings(tenantCtx, req.GoBackend)
	if err != nil {
		h.logger.System().Error("Failed to apply recommended Bunny settings", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, result)
}

// HandleGetVideos handles GET /api/v1/bunny/videos?q&page: a search of the tenant's
// Bunny library, mirroring the Shopify product search.
func (h *BunnyHandlers) HandleGetVideos(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("bunny_get_videos", tenantCtx.TenantID)
	defer marker.Complete()

	queryStr := c.Query("q")
	page := 1
	if pVal := c.Query("page"); pVal != "" {
		parsed, err := strconv.Atoi(pVal)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "page must be a positive number"})
			return
		}
		page = parsed
	}

	h.logger.System().Debug("Handling Bunny get videos request", "tenantId", tenantCtx.TenantID, "query", queryStr, "page", page)

	result, err := h.bunnyService.SearchVideos(tenantCtx, queryStr, page)
	if err != nil {
		h.logger.System().Error("Failed to fetch Bunny videos", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)

	h.logger.System().Info("Bunny videos fetched successfully", "duration", time.Since(start))
	marker.SetSuccess(true)
}

// HandleImport handles POST /api/v1/bunny/import {guid}: it fetches the video from
// Bunny, upserts its bunny-video resource and returns the resource.
func (h *BunnyHandlers) HandleImport(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("bunny_import_video", tenantCtx.TenantID)
	defer marker.Complete()

	var req BunnyImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}
	guid := strings.TrimSpace(req.GUID)
	if guid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "guid is required"})
		return
	}

	resource, err := h.bunnyService.ImportVideo(tenantCtx, guid)
	if err != nil {
		h.logger.System().Error("Failed to import Bunny video", "error", err, "tenantId", tenantCtx.TenantID, "guid", guid)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.System().Info("Bunny video imported", "tenantId", tenantCtx.TenantID, "guid", guid, "resourceId", resource.ID, "duration", time.Since(start))
	marker.SetSuccess(true)
	c.JSON(http.StatusOK, resource)
}
