// Package handlers provides HTTP handlers for imagefile endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ImageFileIDsRequest represents the request body for bulk imagefile loading
type ImageFileIDsRequest struct {
	FileIDs []string `json:"fileIds" binding:"required"`
}

// ImageFileHandlers contains all imagefile-related HTTP handlers
type ImageFileHandlers struct {
	imageFileService *services.ImageFileService
	logger           *logging.ChanneledLogger
	perfTracker      *performance.Tracker
}

// NewImageFileHandlers creates imagefile handlers with injected dependencies
func NewImageFileHandlers(imageFileService *services.ImageFileService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ImageFileHandlers {
	return &ImageFileHandlers{
		imageFileService: imageFileService,
		logger:           logger,
		perfTracker:      perfTracker,
	}
}

// GetAllFileIDs returns all imagefile IDs using cache-first pattern
func (h *ImageFileHandlers) GetAllFileIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_all_file_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get all file IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	fileIDs, err := h.imageFileService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get all file IDs request completed", "count", len(fileIDs), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetAllFileIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"fileIds": fileIDs,
		"count":   len(fileIDs),
	})
}

// GetFilesByIDs returns multiple imagefiles by IDs using cache-first pattern
func (h *ImageFileHandlers) GetFilesByIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_files_by_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get files by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req ImageFileIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.FileIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fileIds array cannot be empty"})
		return
	}

	files, err := h.imageFileService.GetByIDs(tenantCtx, req.FileIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get files by IDs request completed", "requestedCount", len(req.FileIDs), "foundCount", len(files), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetFilesByIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(req.FileIDs))

	c.JSON(http.StatusOK, gin.H{
		"files": files,
		"count": len(files),
	})
}

// GetFileByID returns a specific imagefile by ID using cache-first pattern
func (h *ImageFileHandlers) GetFileByID(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_file_by_id_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get file by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "fileId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	fileID := c.Param("id")
	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file ID is required"})
		return
	}

	fileNode, err := h.imageFileService.GetByID(tenantCtx, fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if fileNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	h.logger.Content().Info("Get file by ID request completed", "fileId", fileID, "found", fileNode != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetFileByID request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "fileId", fileID)

	c.JSON(http.StatusOK, fileNode)
}
