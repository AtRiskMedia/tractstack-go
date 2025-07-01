// Package api provide tractstack handlers
package api

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/gin-gonic/gin"
)

// TractStackIDsRequest represents the request body for bulk tractstack loading
type TractStackIDsRequest struct {
	TractStackIDs []string `json:"tractStackIds" binding:"required"`
}

// GetAllTractStackIDsHandler returns all tractstack IDs using cache-first pattern
func GetAllTractStackIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tractStackService := content.NewTractStackService(ctx, cache.GetGlobalManager())
	tractStackIDs, err := tractStackService.GetAllIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tractStackIds": tractStackIDs,
		"count":         len(tractStackIDs),
	})
}

// GetTractStacksByIDsHandler returns multiple tractstacks by IDs using cache-first pattern
func GetTractStacksByIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Parse request body
	var req TractStackIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.TractStackIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractStackIds array cannot be empty"})
		return
	}

	// Use cache-first tractstack service with global cache manager
	tractStackService := content.NewTractStackService(ctx, cache.GetGlobalManager())
	tractstacks, err := tractStackService.GetByIDs(req.TractStackIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tractstacks": tractstacks,
		"count":       len(tractstacks),
	})
}

// GetTractStackByIDHandler returns a specific tractstack by ID using cache-first pattern
func GetTractStackByIDHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tractStackID := c.Param("id")
	if tractStackID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractstack ID is required"})
		return
	}

	// Use cache-first tractstack service with global cache manager
	tractStackService := content.NewTractStackService(ctx, cache.GetGlobalManager())
	tractStackNode, err := tractStackService.GetByID(tractStackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if tractStackNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tractstack not found"})
		return
	}

	c.JSON(http.StatusOK, tractStackNode)
}

// GetTractStackBySlugHandler returns a specific tractstack by slug using cache-first pattern
func GetTractStackBySlugHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractstack slug is required"})
		return
	}

	// Use cache-first tractstack service with global cache manager
	tractStackService := content.NewTractStackService(ctx, cache.GetGlobalManager())
	tractStackNode, err := tractStackService.GetBySlug(slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if tractStackNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tractstack not found"})
		return
	}

	c.JSON(http.StatusOK, tractStackNode)
}
