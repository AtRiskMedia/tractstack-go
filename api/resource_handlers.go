// Package api provide resource handlers
package api

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/gin-gonic/gin"
)

// ResourceIDsRequest represents the request body for bulk resource loading
type ResourceIDsRequest struct {
	ResourceIDs []string `json:"resourceIds" binding:"required"`
}

// GetAllResourceIDsHandler returns all resource IDs using cache-first pattern
func GetAllResourceIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	resourceIDs, err := resourceService.GetAllIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resourceIds": resourceIDs,
		"count":       len(resourceIDs),
	})
}

// GetResourcesByIDsHandler returns multiple resources by IDs using cache-first pattern
func GetResourcesByIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Parse request body
	var req ResourceIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.ResourceIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resourceIds array cannot be empty"})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	resources, err := resourceService.GetByIDs(req.ResourceIDs)
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
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource ID is required"})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	resourceNode, err := resourceService.GetByID(resourceID)
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
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource slug is required"})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	resourceNode, err := resourceService.GetBySlug(slug)
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

// GetResourcesByCategoryHandler returns resources by category using cache-first pattern
func GetResourcesByCategoryHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	category := c.Param("category")
	if category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource category is required"})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	resources, err := resourceService.GetByCategory(category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"category":  category,
		"count":     len(resources),
	})
}
