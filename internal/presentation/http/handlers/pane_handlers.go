// Package handlers provides HTTP handlers for pane endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// PaneIDsRequest represents the request body for bulk pane loading
type PaneIDsRequest struct {
	PaneIDs []string `json:"paneIds" binding:"required"`
}

// PaneHandlers contains all pane-related HTTP handlers
type PaneHandlers struct {
	paneService *services.PaneService
	logger      *logging.ChanneledLogger
}

// NewPaneHandlers creates pane handlers with injected dependencies
func NewPaneHandlers(paneService *services.PaneService, logger *logging.ChanneledLogger) *PaneHandlers {
	return &PaneHandlers{
		paneService: paneService,
		logger:      logger,
	}
}

// GetAllPaneIDs returns all pane IDs using cache-first pattern
func (h *PaneHandlers) GetAllPaneIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	paneIDs, err := h.paneService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"paneIds": paneIDs,
		"count":   len(paneIDs),
	})
}

// GetPanesByIDs returns multiple panes by IDs using cache-first pattern
func (h *PaneHandlers) GetPanesByIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get all pane IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req PaneIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.PaneIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paneIds array cannot be empty"})
		return
	}

	panes, err := h.paneService.GetByIDs(tenantCtx, req.PaneIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get panes by IDs request completed", "requestedCount", len(req.PaneIDs), "foundCount", len(panes), "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"panes": panes,
		"count": len(panes),
	})
}

// GetPaneByID returns a specific pane by ID using cache-first pattern
func (h *PaneHandlers) GetPaneByID(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get pane by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "paneId", c.Param("id"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	paneID := c.Param("id")
	if paneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pane ID is required"})
		return
	}

	paneNode, err := h.paneService.GetByID(tenantCtx, paneID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if paneNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pane not found"})
		return
	}

	h.logger.Content().Info("Get pane by ID request completed", "paneId", paneID, "found", paneNode != nil, "duration", time.Since(start))
	c.JSON(http.StatusOK, paneNode)
}

// GetPaneBySlug returns a specific pane by slug using cache-first pattern
func (h *PaneHandlers) GetPaneBySlug(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get pane by slug request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pane slug is required"})
		return
	}

	paneNode, err := h.paneService.GetBySlug(tenantCtx, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if paneNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pane not found"})
		return
	}

	h.logger.Content().Info("Get pane by slug request completed", "slug", slug, "found", paneNode != nil, "duration", time.Since(start))
	c.JSON(http.StatusOK, paneNode)
}

// GetContextPanes returns all context panes using cache-first pattern
func (h *PaneHandlers) GetContextPanes(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get context panes request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	contextPanes, err := h.paneService.GetContextPanes(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get context panes request completed", "count", len(contextPanes), "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"contextPanes": contextPanes,
		"count":        len(contextPanes),
	})
}
