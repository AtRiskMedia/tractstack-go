// Package handlers provides HTTP handlers for tractstack endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// TractStackIDsRequest represents the request body for bulk tractstack loading
type TractStackIDsRequest struct {
	TractStackIDs []string `json:"tractStackIds" binding:"required"`
}

// TractStackHandlers contains all tractstack-related HTTP handlers
type TractStackHandlers struct {
	tractStackService *services.TractStackService
	logger            *logging.ChanneledLogger
	perfTracker       *performance.Tracker
}

// NewTractStackHandlers creates tractstack handlers with injected dependencies
func NewTractStackHandlers(tractStackService *services.TractStackService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *TractStackHandlers {
	return &TractStackHandlers{
		tractStackService: tractStackService,
		logger:            logger,
		perfTracker:       perfTracker,
	}
}

// GetAllTractStackIDs returns all tractstack IDs using cache-first pattern
func (h *TractStackHandlers) GetAllTractStackIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_all_tractstack_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get all tractstack IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)

	tractStackIDs, err := h.tractStackService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get all tractstack IDs request completed", "count", len(tractStackIDs), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetAllTractStackIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"tractStackIds": tractStackIDs,
		"count":         len(tractStackIDs),
	})
}

// GetTractStacksByIDs returns multiple tractstacks by IDs using cache-first pattern
func (h *TractStackHandlers) GetTractStacksByIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_tractstacks_by_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get tractstacks by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)

	var req TractStackIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.TractStackIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractStackIds array cannot be empty"})
		return
	}

	tractStacks, err := h.tractStackService.GetByIDs(tenantCtx, req.TractStackIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get tractstacks by IDs request completed", "requestedCount", len(req.TractStackIDs), "foundCount", len(tractStacks), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetTractStacksByIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(req.TractStackIDs))

	c.JSON(http.StatusOK, gin.H{
		"tractStacks": tractStacks,
		"count":       len(tractStacks),
	})
}

// GetTractStackByID returns a specific tractstack by ID using cache-first pattern
func (h *TractStackHandlers) GetTractStackByID(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_tractstack_by_id_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get tractstack by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "tractStackId", c.Param("id"))

	tractStackID := c.Param("id")
	if tractStackID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractstack ID is required"})
		return
	}

	tractStackNode, err := h.tractStackService.GetByID(tenantCtx, tractStackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if tractStackNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tractstack not found"})
		return
	}

	h.logger.Content().Info("Get tractstack by ID request completed", "tractStackId", tractStackID, "found", tractStackNode != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetTractStackByID request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "tractStackId", tractStackID)

	c.JSON(http.StatusOK, tractStackNode)
}

// GetTractStackBySlug returns a specific tractstack by slug using cache-first pattern
func (h *TractStackHandlers) GetTractStackBySlug(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_tractstack_by_slug_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get tractstack by slug request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractstack slug is required"})
		return
	}

	tractStackNode, err := h.tractStackService.GetBySlug(tenantCtx, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if tractStackNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tractstack not found"})
		return
	}

	h.logger.Content().Info("Get tractstack by slug request completed", "slug", slug, "found", tractStackNode != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetTractStackBySlug request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	c.JSON(http.StatusOK, tractStackNode)
}

// CreateTractStack creates a new tractstack
func (h *TractStackHandlers) CreateTractStack(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("create_tractstack_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received create tractstack request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var ts content.TractStackNode
	if err := c.ShouldBindJSON(&ts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := h.tractStackService.Create(tenantCtx, &ts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Create tractstack request completed", "tractStackId", ts.ID, "title", ts.Title, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for CreateTractStack request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "tractStackId", ts.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message":      "tractstack created successfully",
		"tractStackId": ts.ID,
	})
}

// UpdateTractStack updates an existing tractstack
func (h *TractStackHandlers) UpdateTractStack(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("update_tractstack_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received update tractstack request", "method", c.Request.Method, "path", c.Request.URL.Path, "tractStackId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	tractStackID := c.Param("id")
	if tractStackID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractstack ID is required"})
		return
	}

	var ts content.TractStackNode
	if err := c.ShouldBindJSON(&ts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}
	ts.ID = tractStackID

	if err := h.tractStackService.Update(tenantCtx, &ts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Update tractstack request completed", "tractStackId", ts.ID, "title", ts.Title, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for UpdateTractStack request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "tractStackId", ts.ID)

	c.JSON(http.StatusOK, gin.H{
		"message":      "tractstack updated successfully",
		"tractStackId": ts.ID,
	})
}

// DeleteTractStack deletes a tractstack
func (h *TractStackHandlers) DeleteTractStack(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("delete_tractstack_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received delete tractstack request", "method", c.Request.Method, "path", c.Request.URL.Path, "tractStackId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	tractStackID := c.Param("id")
	if tractStackID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractstack ID is required"})
		return
	}

	if err := h.tractStackService.Delete(tenantCtx, tractStackID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Delete tractstack request completed", "tractStackId", tractStackID, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for DeleteTractStack request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "tractStackId", tractStackID)

	c.JSON(http.StatusOK, gin.H{
		"message":      "tractstack deleted successfully",
		"tractStackId": tractStackID,
	})
}
