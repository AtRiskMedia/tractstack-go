// Package api provide resource handlers
package api

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/gin-gonic/gin"
)

// ResourceIDsRequest represents the request body for bulk resource loading
type ResourceIDsRequest struct {
	ResourceIDs []string `json:"resourceIds,omitempty"` // Made optional
	Categories  []string `json:"categories,omitempty"`  // New - filter by categories
	Slugs       []string `json:"slugs,omitempty"`       // New - filter by slugs
}

// GetResourcesByIDsHandler returns multiple resources by IDs, categories, or slugs using cache-first pattern
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

	// Validate that at least one filtering method is provided
	if len(req.ResourceIDs) == 0 && len(req.Categories) == 0 && len(req.Slugs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one of resourceIds, categories, or slugs must be provided"})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	var allResources []*models.ResourceNode
	resourceMap := make(map[string]*models.ResourceNode) // To avoid duplicates

	// Fetch by IDs if provided
	if len(req.ResourceIDs) > 0 {
		resources, err := resourceService.GetByIDs(req.ResourceIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for _, resource := range resources {
			resourceMap[resource.ID] = resource
		}
	}

	// Fetch by categories if provided
	if len(req.Categories) > 0 {
		for _, category := range req.Categories {
			resources, err := resourceService.GetByCategory(category)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			for _, resource := range resources {
				resourceMap[resource.ID] = resource
			}
		}
	}

	// Fetch by slugs if provided
	if len(req.Slugs) > 0 {
		for _, slug := range req.Slugs {
			resource, err := resourceService.GetBySlug(slug)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if resource != nil {
				resourceMap[resource.ID] = resource
			}
		}
	}

	// Convert map to slice
	for _, resource := range resourceMap {
		allResources = append(allResources, resource)
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": allResources,
		"count":     len(allResources),
	})
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
