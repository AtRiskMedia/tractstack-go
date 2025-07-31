// Package handlers provides HTTP handlers for imagefile endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
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
}

// NewImageFileHandlers creates imagefile handlers with injected dependencies
func NewImageFileHandlers(imageFileService *services.ImageFileService) *ImageFileHandlers {
	return &ImageFileHandlers{
		imageFileService: imageFileService,
	}
}

// GetAllFileIDs returns all imagefile IDs using cache-first pattern
func (h *ImageFileHandlers) GetAllFileIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	fileIDs, err := h.imageFileService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"fileIds": fileIDs,
		"count":   len(fileIDs),
	})
}

// GetFilesByIDs returns multiple imagefiles by IDs using cache-first pattern
func (h *ImageFileHandlers) GetFilesByIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
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

	c.JSON(http.StatusOK, gin.H{
		"files": files,
		"count": len(files),
	})
}

// GetFileByID returns a specific imagefile by ID using cache-first pattern
func (h *ImageFileHandlers) GetFileByID(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
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

	c.JSON(http.StatusOK, fileNode)
}
