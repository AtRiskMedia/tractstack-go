// Package handlers provides HTTP handlers for content map endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ContentMapHandler handles content map HTTP requests
type ContentMapHandler struct {
	contentMapService *services.ContentMapService
}

// NewContentMapHandler creates a new content map handler
func NewContentMapHandler(contentMapService *services.ContentMapService) *ContentMapHandler {
	return &ContentMapHandler{
		contentMapService: contentMapService,
	}
}

// GetContentMapHandler handles GET /api/v1/content/full-map
func (h *ContentMapHandler) GetContentMapHandler(c *gin.Context) {
	// Get tenant context from middleware
	tenantCtx, ok := middleware.GetTenantContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context required"})
		return
	}

	// Get client's lastUpdated parameter for timestamp comparison
	clientLastUpdated := c.Query("lastUpdated")

	// Get content map with caching logic
	response, notModified, err := h.contentMapService.GetContentMap(tenantCtx.TenantID, clientLastUpdated)
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
