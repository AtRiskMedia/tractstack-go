// Package handlers provides HTTP handlers for imagefile endpoints
package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/media"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// OGImageUploadRequest represents the request body for OG image uploads
type OGImageUploadRequest struct {
	Data     string `json:"data" binding:"required"`
	Filename string `json:"filename" binding:"required"`
}

// OGImageDeleteRequest represents the request body for OG image deletion
type OGImageDeleteRequest struct {
	Path string `json:"path" binding:"required"`
}

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

// CreateFile creates a new imagefile
func (h *ImageFileHandlers) CreateFile(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("create_imagefile_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received create imagefile request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var imageFile content.ImageFileNode
	if err := c.ShouldBindJSON(&imageFile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := h.imageFileService.Create(tenantCtx, &imageFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Create imagefile request completed", "fileId", imageFile.ID, "filename", imageFile.Filename, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for CreateFile request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "fileId", imageFile.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "imagefile created successfully",
		"fileId":  imageFile.ID,
	})
}

// UpdateFile updates an existing imagefile
func (h *ImageFileHandlers) UpdateFile(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("update_imagefile_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received update imagefile request", "method", c.Request.Method, "path", c.Request.URL.Path, "fileId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	fileID := c.Param("id")
	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imagefile ID is required"})
		return
	}

	var imageFile content.ImageFileNode
	if err := c.ShouldBindJSON(&imageFile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}
	imageFile.ID = fileID

	if err := h.imageFileService.Update(tenantCtx, &imageFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Update imagefile request completed", "fileId", imageFile.ID, "filename", imageFile.Filename, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for UpdateFile request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "fileId", imageFile.ID)

	c.JSON(http.StatusOK, gin.H{
		"message": "imagefile updated successfully",
		"fileId":  imageFile.ID,
	})
}

// DeleteFile deletes an imagefile
func (h *ImageFileHandlers) DeleteFile(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("delete_imagefile_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received delete imagefile request", "method", c.Request.Method, "path", c.Request.URL.Path, "fileId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	fileID := c.Param("id")
	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imagefile ID is required"})
		return
	}

	if err := h.imageFileService.Delete(tenantCtx, fileID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Delete imagefile request completed", "fileId", fileID, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for DeleteFile request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "fileId", fileID)

	c.JSON(http.StatusOK, gin.H{
		"message": "imagefile deleted successfully",
		"fileId":  fileID,
	})
}

// UploadOGImage handles OG image uploads for StoryFragments
func (h *ImageFileHandlers) UploadOGImage(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("upload_og_image_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received upload OG image request", "method", c.Request.Method, "path", c.Request.URL.Path)

	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req OGImageUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Validate base64 data format
	if !strings.Contains(req.Data, "data:image/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 image data"})
		return
	}

	// Validate filename format (should be nodeID-timestamp.ext)
	if req.Filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename cannot be empty"})
		return
	}

	// Extract nodeID from filename (everything before first dash)
	nodeID := req.Filename
	if dashIndex := strings.Index(req.Filename, "-"); dashIndex != -1 {
		nodeID = req.Filename[:dashIndex]
	}

	// Get tenant's media path and create ImageProcessor
	mediaPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantCtx.TenantID, "media")
	processor := media.NewImageProcessor(mediaPath)

	// Process the image and generate thumbnails
	originalPath, thumbnailPaths, err := processor.ProcessOGImageWithThumbnails(req.Data, nodeID)
	if err != nil {
		h.logger.Content().Error("Failed to process OG image", "error", err, "nodeID", nodeID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process image", "details": err.Error()})
		return
	}

	// Invalidate StoryFragment cache if this is for a specific story fragment
	if nodeID != "" {
		tenantCtx.CacheManager.InvalidateStoryFragment(tenantCtx.TenantID, nodeID)
		tenantCtx.CacheManager.InvalidateFullContentMap(tenantCtx.TenantID)
	}

	h.logger.Content().Info("Upload OG image request completed", "nodeID", nodeID, "originalPath", originalPath, "thumbnailCount", len(thumbnailPaths), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for UploadOGImage request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "nodeID", nodeID)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"path":       originalPath,
		"thumbnails": thumbnailPaths,
		"message":    "OG image uploaded successfully",
	})
}

// DeleteOGImage handles OG image deletion for StoryFragments
func (h *ImageFileHandlers) DeleteOGImage(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("delete_og_image_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received delete OG image request", "method", c.Request.Method, "path", c.Request.URL.Path)

	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req OGImageDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Validate image path
	if req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image path cannot be empty"})
		return
	}

	// Extract nodeID from path for cache invalidation
	filename := filepath.Base(req.Path)
	nodeID := filename
	if dashIndex := strings.Index(filename, "-"); dashIndex != -1 {
		nodeID = filename[:dashIndex]
	}

	// Get tenant's media path and create ImageProcessor
	mediaPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantCtx.TenantID, "media")
	processor := media.NewImageProcessor(mediaPath)

	// Delete the image and its thumbnails
	err := processor.DeleteOGImageAndThumbnails(req.Path)
	if err != nil {
		h.logger.Content().Error("Failed to delete OG image", "error", err, "path", req.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete image", "details": err.Error()})
		return
	}

	// Invalidate StoryFragment cache
	if nodeID != "" {
		tenantCtx.CacheManager.InvalidateStoryFragment(tenantCtx.TenantID, nodeID)
		tenantCtx.CacheManager.InvalidateFullContentMap(tenantCtx.TenantID)
	}

	h.logger.Content().Info("Delete OG image request completed", "path", req.Path, "nodeID", nodeID, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for DeleteOGImage request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "path", req.Path)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "OG image and thumbnails deleted successfully",
	})
}
