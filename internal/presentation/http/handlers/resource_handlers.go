// Package handlers provides HTTP handlers for resource endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ResourceIDsRequest represents the request body for bulk resource loading
type ResourceIDsRequest struct {
	ResourceIDs []string `json:"resourceIds,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	Slugs       []string `json:"slugs,omitempty"`
}

// ResourceHandlers contains all resource-related HTTP handlers
type ResourceHandlers struct {
	resourceService *services.ResourceService
	logger          *logging.ChanneledLogger
}

// NewResourceHandlers creates resource handlers with injected dependencies
func NewResourceHandlers(resourceService *services.ResourceService, logger *logging.ChanneledLogger) *ResourceHandlers {
	return &ResourceHandlers{
		resourceService: resourceService,
		logger:          logger,
	}
}

// GetAllResourceIDs returns all resource IDs using cache-first pattern
func (h *ResourceHandlers) GetAllResourceIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get all resource IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	resourceIDs, err := h.resourceService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get all resource IDs request completed", "count", len(resourceIDs), "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"resourceIds": resourceIDs,
		"count":       len(resourceIDs),
	})
}

// GetResourcesByIDs returns multiple resources by IDs/filters using cache-first pattern
func (h *ResourceHandlers) GetResourcesByIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get resources by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req ResourceIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Handle different request patterns
	var resources []*content.ResourceNode
	var err error

	if len(req.ResourceIDs) > 0 || len(req.Categories) > 0 || len(req.Slugs) > 0 {
		// Multi-filter request
		resources, err = h.resourceService.GetByFilters(tenantCtx, req.ResourceIDs, req.Categories, req.Slugs)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one filter (resourceIds, categories, or slugs) must be provided"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get resources by IDs request completed", "requestedCount", len(req.ResourceIDs), "foundCount", len(resources), "duration", time.Since(start))

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"count":     len(resources),
	})
}

// GetResourceByID returns a specific resource by ID using cache-first pattern
func (h *ResourceHandlers) GetResourceByID(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get resource by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "resourceId", c.Param("id"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource ID is required"})
		return
	}

	resourceNode, err := h.resourceService.GetByID(tenantCtx, resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if resourceNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	h.logger.Content().Info("Get resource by ID request completed", "resourceId", resourceID, "found", resourceNode != nil, "duration", time.Since(start))

	c.JSON(http.StatusOK, resourceNode)
}

// GetResourceBySlug returns a specific resource by slug using cache-first pattern
func (h *ResourceHandlers) GetResourceBySlug(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get resource by slug request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource slug is required"})
		return
	}

	resourceNode, err := h.resourceService.GetBySlug(tenantCtx, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if resourceNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	h.logger.Content().Info("Get resource by slug request completed", "slug", slug, "found", resourceNode != nil, "duration", time.Since(start))
	c.JSON(http.StatusOK, resourceNode)
}

// CreateResource creates a new resource
func (h *ResourceHandlers) CreateResource(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var resource content.ResourceNode
	if err := c.ShouldBindJSON(&resource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	err := h.resourceService.Create(tenantCtx, &resource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":    "resource created successfully",
		"resourceId": resource.ID,
	})
}

// UpdateResource updates an existing resource
func (h *ResourceHandlers) UpdateResource(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource ID is required"})
		return
	}

	var resource content.ResourceNode
	if err := c.ShouldBindJSON(&resource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Ensure ID matches URL parameter
	resource.ID = resourceID

	err := h.resourceService.Update(tenantCtx, &resource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "resource updated successfully",
		"resourceId": resource.ID,
	})
}

// DeleteResource deletes a resource
func (h *ResourceHandlers) DeleteResource(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource ID is required"})
		return
	}

	err := h.resourceService.Delete(tenantCtx, resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "resource deleted successfully",
		"resourceId": resourceID,
	})
}
