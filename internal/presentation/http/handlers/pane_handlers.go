// Package handlers provides HTTP handlers for pane endpoints
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

// BulkFilePaneRequest represents the request body for bulk file-pane relationship updates
type BulkFilePaneRequest struct {
	Relationships []struct {
		PaneID  string   `json:"paneId" binding:"required"`
		FileIDs []string `json:"fileIds"`
	} `json:"relationships" binding:"required"`
}

// PaneIDsRequest represents the request body for bulk pane loading
type PaneIDsRequest struct {
	PaneIDs []string `json:"paneIds" binding:"required"`
}

// PaneHandlers contains all pane-related HTTP handlers
type PaneHandlers struct {
	paneService *services.PaneService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewPaneHandlers creates pane handlers with injected dependencies
func NewPaneHandlers(paneService *services.PaneService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *PaneHandlers {
	return &PaneHandlers{
		paneService: paneService,
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// GetAllPaneIDs returns all pane IDs using cache-first pattern
func (h *PaneHandlers) GetAllPaneIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_all_pane_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	paneIDs, err := h.paneService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get pane IDs request completed", "foundCount", len(paneIDs), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetAllPaneIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"paneIds": paneIDs,
		"count":   len(paneIDs),
	})
}

// GetPanesByIDs returns multiple panes by IDs using cache-first pattern
func (h *PaneHandlers) GetPanesByIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_panes_by_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get all pane IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
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
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetPanesByIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(req.PaneIDs))

	c.JSON(http.StatusOK, gin.H{
		"panes": panes,
		"count": len(panes),
	})
}

// GetPaneByID returns a specific pane by ID using cache-first pattern
func (h *PaneHandlers) GetPaneByID(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_pane_by_id_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get pane by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "paneId", c.Param("id"))
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
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetPaneByID request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", paneID)

	c.JSON(http.StatusOK, paneNode)
}

// GetPaneBySlug returns a specific pane by slug using cache-first pattern
func (h *PaneHandlers) GetPaneBySlug(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_pane_by_slug_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get pane by slug request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
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
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetPaneBySlug request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	c.JSON(http.StatusOK, paneNode)
}

// GetContextPanes returns all context panes using cache-first pattern
func (h *PaneHandlers) GetContextPanes(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_context_panes_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get context panes request", "method", c.Request.Method, "path", c.Request.URL.Path)
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
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetContextPanes request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"contextPanes": contextPanes,
		"count":        len(contextPanes),
	})
}

// CreatePane creates a new pane
func (h *PaneHandlers) CreatePane(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("create_pane_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received create pane request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var pane content.PaneNode
	if err := c.ShouldBindJSON(&pane); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := h.paneService.Create(tenantCtx, &pane); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Create pane request completed", "paneId", pane.ID, "title", pane.Title, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for CreatePane request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", pane.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "pane created successfully",
		"paneId":  pane.ID,
	})
}

// UpdatePane updates an existing pane
func (h *PaneHandlers) UpdatePane(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("update_pane_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received update pane request", "method", c.Request.Method, "path", c.Request.URL.Path, "paneId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	paneID := c.Param("id")
	if paneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pane ID is required"})
		return
	}

	var pane content.PaneNode
	if err := c.ShouldBindJSON(&pane); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}
	pane.ID = paneID

	if err := h.paneService.Update(tenantCtx, &pane); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Update pane request completed", "paneId", pane.ID, "title", pane.Title, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for UpdatePane request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", pane.ID)

	c.JSON(http.StatusOK, gin.H{
		"message": "pane updated successfully",
		"paneId":  pane.ID,
	})
}

// DeletePane deletes a pane
func (h *PaneHandlers) DeletePane(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("delete_pane_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received delete pane request", "method", c.Request.Method, "path", c.Request.URL.Path, "paneId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	paneID := c.Param("id")
	if paneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pane ID is required"})
		return
	}

	if err := h.paneService.Delete(tenantCtx, paneID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Delete pane request completed", "paneId", paneID, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for DeletePane request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", paneID)

	c.JSON(http.StatusOK, gin.H{
		"message": "pane deleted successfully",
		"paneId":  paneID,
	})
}

// GetPaneTemplate returns a pane template in the same format as full-payload
func (h *PaneHandlers) GetPaneTemplate(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_pane_template_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get pane template request", "method", c.Request.Method, "path", c.Request.URL.Path, "paneId", c.Param("id"))

	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	paneID := c.Param("id")
	if paneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pane ID is required"})
		return
	}

	// Delegate to service layer for business logic
	templatePayload, err := h.paneService.GetPaneTemplate(tenantCtx, paneID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if templatePayload == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pane not found"})
		return
	}

	h.logger.Content().Info("Get pane template request completed", "paneId", paneID, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetPaneTemplate request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", paneID)

	c.JSON(http.StatusOK, templatePayload)
}

// BulkUpdateFilePaneRelationships handles bulk updates of file-pane relationships
func (h *PaneHandlers) BulkUpdateFilePaneRelationships(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("bulk_update_file_pane_relationships_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received bulk update file-pane relationships request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req BulkFilePaneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.Relationships) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "relationships array cannot be empty"})
		return
	}

	// Convert request format to service format (map[paneID][]fileIDs)
	relationships := make(map[string][]string)
	for _, rel := range req.Relationships {
		relationships[rel.PaneID] = rel.FileIDs
	}

	if err := h.paneService.BulkUpdateFilePaneRelationships(tenantCtx, relationships); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Bulk update file-pane relationships completed", "paneCount", len(req.Relationships), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for BulkUpdateFilePaneRelationships request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneCount", len(req.Relationships))

	c.JSON(http.StatusOK, gin.H{
		"message":   "file-pane relationships updated successfully",
		"paneCount": len(req.Relationships),
	})
}
