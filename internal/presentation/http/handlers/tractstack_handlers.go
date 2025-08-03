// Package handlers provides HTTP handlers for tractstack endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
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
}

// NewTractStackHandlers creates tractstack handlers with injected dependencies
func NewTractStackHandlers(tractStackService *services.TractStackService, logger *logging.ChanneledLogger) *TractStackHandlers {
	return &TractStackHandlers{
		tractStackService: tractStackService,
		logger:            logger,
	}
}

// GetAllTractStackIDs returns all tractstack IDs using cache-first pattern
func (h *TractStackHandlers) GetAllTractStackIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get all tractstack IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	tractStackIDs, err := h.tractStackService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get all tractstack IDs request completed", "count", len(tractStackIDs), "duration", time.Since(start))

	c.JSON(http.StatusOK, gin.H{
		"tractStackIds": tractStackIDs,
		"count":         len(tractStackIDs),
	})
}

// GetTractStacksByIDs returns multiple tractstacks by IDs using cache-first pattern
func (h *TractStackHandlers) GetTractStacksByIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get tractstacks by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

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

	c.JSON(http.StatusOK, gin.H{
		"tractStacks": tractStacks,
		"count":       len(tractStacks),
	})
}

// GetTractStackByID returns a specific tractstack by ID using cache-first pattern
func (h *TractStackHandlers) GetTractStackByID(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get tractstack by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "tractStackId", c.Param("id"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

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

	c.JSON(http.StatusOK, tractStackNode)
}

// GetTractStackBySlug returns a specific tractstack by slug using cache-first pattern
func (h *TractStackHandlers) GetTractStackBySlug(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get tractstack by slug request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

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

	c.JSON(http.StatusOK, tractStackNode)
}
