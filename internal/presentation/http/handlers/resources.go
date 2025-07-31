// Package handlers provides HTTP handlers for resource endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	persistence "github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ResourceIDsRequest represents the request body for bulk resource loading
type ResourceIDsRequest struct {
	ResourceIDs []string `json:"resourceIds,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	Slugs       []string `json:"slugs,omitempty"`
}

// GetAllResourceIDsHandler returns all resource IDs using cache-first pattern
func GetAllResourceIDsHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	resourceRepo := persistence.NewResourceRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	resourceService := services.NewResourceService(resourceRepo)

	resourceIDs, err := resourceService.GetAllIDs(tenantCtx.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resourceIds": resourceIDs,
		"count":       len(resourceIDs),
	})
}

// GetResourcesByIDsHandler returns multiple resources by IDs/filters using cache-first pattern
func GetResourcesByIDsHandler(c *gin.Context) {
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

	resourceRepo := persistence.NewResourceRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	resourceService := services.NewResourceService(resourceRepo)

	if len(req.ResourceIDs) > 0 || len(req.Categories) > 0 || len(req.Slugs) > 0 {
		// Multi-filter request
		resources, err = resourceService.GetByFilters(tenantCtx.TenantID, req.ResourceIDs, req.Categories, req.Slugs)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one filter (resourceIds, categories, or slugs) must be provided"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"count":     len(resources),
	})
}

// GetResourceByIDHandler returns a specific resource by ID using cache-first pattern
func GetResourceByIDHandler(c *gin.Context) {
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

	resourceRepo := persistence.NewResourceRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	resourceService := services.NewResourceService(resourceRepo)

	resourceNode, err := resourceService.GetByID(tenantCtx.TenantID, resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if resourceNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	c.JSON(http.StatusOK, resourceNode)
}

// GetResourceBySlugHandler returns a specific resource by slug using cache-first pattern
func GetResourceBySlugHandler(c *gin.Context) {
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

	resourceRepo := persistence.NewResourceRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	resourceService := services.NewResourceService(resourceRepo)

	resourceNode, err := resourceService.GetBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if resourceNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	c.JSON(http.StatusOK, resourceNode)
}

// CreateResourceHandler creates a new resource
func CreateResourceHandler(c *gin.Context) {
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

	resourceRepo := persistence.NewResourceRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	resourceService := services.NewResourceService(resourceRepo)

	err := resourceService.Create(tenantCtx.TenantID, &resource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":    "resource created successfully",
		"resourceId": resource.ID,
	})
}

// UpdateResourceHandler updates an existing resource
func UpdateResourceHandler(c *gin.Context) {
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

	resourceRepo := persistence.NewResourceRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	resourceService := services.NewResourceService(resourceRepo)

	err := resourceService.Update(tenantCtx.TenantID, &resource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "resource updated successfully",
		"resourceId": resource.ID,
	})
}

// DeleteResourceHandler deletes a resource
func DeleteResourceHandler(c *gin.Context) {
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

	resourceRepo := persistence.NewResourceRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	resourceService := services.NewResourceService(resourceRepo)

	err := resourceService.Delete(tenantCtx.TenantID, resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "resource deleted successfully",
		"resourceId": resourceID,
	})
}
