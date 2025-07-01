// Package api provide belief handlers
package api

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/gin-gonic/gin"
)

// BeliefIDsRequest represents the request body for bulk belief loading
type BeliefIDsRequest struct {
	BeliefIDs []string `json:"beliefIds" binding:"required"`
}

// GetAllBeliefIDsHandler returns all belief IDs using cache-first pattern
func GetAllBeliefIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Use cache-first belief service with global cache manager
	beliefService := content.NewBeliefService(ctx, cache.GetGlobalManager())
	beliefIDs, err := beliefService.GetAllIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"beliefIds": beliefIDs,
		"count":     len(beliefIDs),
	})
}

// GetBeliefsByIDsHandler returns multiple beliefs by IDs using cache-first pattern
func GetBeliefsByIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Parse request body
	var req BeliefIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.BeliefIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "beliefIds array cannot be empty"})
		return
	}

	// Use cache-first belief service with global cache manager
	beliefService := content.NewBeliefService(ctx, cache.GetGlobalManager())
	beliefs, err := beliefService.GetByIDs(req.BeliefIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"beliefs": beliefs,
		"count":   len(beliefs),
	})
}

// GetBeliefByIDHandler returns a specific belief by ID using cache-first pattern
func GetBeliefByIDHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	beliefID := c.Param("id")
	if beliefID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "belief ID is required"})
		return
	}

	// Use cache-first belief service with global cache manager
	beliefService := content.NewBeliefService(ctx, cache.GetGlobalManager())
	beliefNode, err := beliefService.GetByID(beliefID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if beliefNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "belief not found"})
		return
	}

	c.JSON(http.StatusOK, beliefNode)
}

// GetBeliefBySlugHandler returns a specific belief by slug using cache-first pattern
func GetBeliefBySlugHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "belief slug is required"})
		return
	}

	// Use cache-first belief service with global cache manager
	beliefService := content.NewBeliefService(ctx, cache.GetGlobalManager())
	beliefNode, err := beliefService.GetBySlug(slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if beliefNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "belief not found"})
		return
	}

	c.JSON(http.StatusOK, beliefNode)
}
