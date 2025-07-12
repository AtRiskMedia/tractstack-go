// Package api provide epinet handlers
package api

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/gin-gonic/gin"
)

// EpinetIDsRequest represents the request body for bulk epinet loading
type EpinetIDsRequest struct {
	EpinetIDs []string `json:"epinetIds" binding:"required"`
}

// GetAllEpinetIDsHandler returns all epinet IDs using cache-first pattern
func GetAllEpinetIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Use cache-first epinet service with global cache manager
	epinetService := content.NewEpinetService(ctx, cache.GetGlobalManager())
	epinetIDs, err := epinetService.GetAllIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"epinetIds": epinetIDs,
		"count":     len(epinetIDs),
	})
}

// GetEpinetsByIDsHandler returns multiple epinets by IDs using cache-first pattern
func GetEpinetsByIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Parse request body
	var req EpinetIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.EpinetIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "epinetIds array cannot be empty"})
		return
	}

	// Use cache-first epinet service with global cache manager
	epinetService := content.NewEpinetService(ctx, cache.GetGlobalManager())
	epinets, err := epinetService.GetByIDs(req.EpinetIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"epinets": epinets,
		"count":   len(epinets),
	})
}

// GetEpinetByIDHandler returns a specific epinet by ID using cache-first pattern
func GetEpinetByIDHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	epinetID := c.Param("id")
	if epinetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "epinet ID is required"})
		return
	}

	// Use cache-first epinet service with global cache manager
	epinetService := content.NewEpinetService(ctx, cache.GetGlobalManager())
	epinetNode, err := epinetService.GetByID(epinetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if epinetNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "epinet not found"})
		return
	}

	c.JSON(http.StatusOK, epinetNode)
}
