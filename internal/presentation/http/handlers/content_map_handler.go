// Package handlers provides HTTP handlers for content map endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ContentMapHandlers contains all content map-related HTTP handlers
type ContentMapHandlers struct {
	contentMapService *services.ContentMapService
}

// NewContentMapHandlers creates content map handlers with injected dependencies
func NewContentMapHandlers(contentMapService *services.ContentMapService) *ContentMapHandlers {
	return &ContentMapHandlers{
		contentMapService: contentMapService,
	}
}

// GetContentMap handles GET /api/v1/content/full-map
func (h *ContentMapHandlers) GetContentMap(c *gin.Context) {
	// Get tenant context from middleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Get client's lastUpdated parameter for timestamp comparison
	clientLastUpdated := c.Query("lastUpdated")

	// FIXED: Pass cache manager as 3rd parameter
	response, notModified, err := h.contentMapService.GetContentMap(tenantCtx, clientLastUpdated, tenantCtx.CacheManager)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Handle 304 Not Modified
	if notModified {
		c.Status(http.StatusNotModified)
		return
	}

	// Return content map data with wrapper that works with TractStackAPI
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"data":        response.Data,
			"lastUpdated": response.LastUpdated,
		},
	})
}
