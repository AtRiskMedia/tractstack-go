// Package api provide storyfragment handlers
package api

import (
	"fmt"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
)

// StoryFragmentIDsRequest represents the request body for bulk storyfragment loading
type StoryFragmentIDsRequest struct {
	StoryFragmentIDs []string `json:"storyFragmentIds" binding:"required"`
}

// GetAllStoryFragmentIDsHandler returns all storyfragment IDs using cache-first pattern
func GetAllStoryFragmentIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Activate tenant if needed
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}
	}

	// Use cache-first storyfragment service with global cache manager
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragmentIDs, err := storyFragmentService.GetAllIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"storyFragmentIds": storyFragmentIDs,
		"count":            len(storyFragmentIDs),
	})
}

// GetStoryFragmentsByIDsHandler returns multiple storyfragments by IDs using cache-first pattern
func GetStoryFragmentsByIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Activate tenant if needed
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}
	}

	// Parse request body
	var req StoryFragmentIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.StoryFragmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyFragmentIds array cannot be empty"})
		return
	}

	// Use cache-first storyfragment service with global cache manager
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragments, err := storyFragmentService.GetByIDs(req.StoryFragmentIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"storyFragments": storyFragments,
		"count":          len(storyFragments),
	})
}

// GetStoryFragmentByIDHandler returns a specific storyfragment by ID using cache-first pattern
func GetStoryFragmentByIDHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Activate tenant if needed
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}
	}

	storyFragmentID := c.Param("id")
	if storyFragmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment ID is required"})
		return
	}

	// Use cache-first storyfragment service with global cache manager
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragmentNode, err := storyFragmentService.GetByID(storyFragmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	c.JSON(http.StatusOK, storyFragmentNode)
}

// GetStoryFragmentBySlugHandler returns a specific storyfragment by slug using cache-first pattern
func GetStoryFragmentBySlugHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Activate tenant if needed
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment slug is required"})
		return
	}

	// Use cache-first storyfragment service with global cache manager
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragmentNode, err := storyFragmentService.GetBySlug(slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	c.JSON(http.StatusOK, storyFragmentNode)
}
