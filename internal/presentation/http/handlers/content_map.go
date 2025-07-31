// Package handlers provides HTTP handlers for content map endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/bulk"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// GetContentMapHandler handles GET /api/v1/content/full-map
func GetContentMapHandler(c *gin.Context) {
	// Get tenant context from middleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Create bulk repository for this request
	db := &database.DB{DB: tenantCtx.Database.Conn}
	bulkRepo := bulk.NewRepository(db)

	// Create service per-request with tenant's cache manager
	contentMapService := services.NewContentMapService(bulkRepo)

	// Get client's lastUpdated parameter for timestamp comparison
	clientLastUpdated := c.Query("lastUpdated")

	// Get content map with caching logic using tenant's cache manager
	response, notModified, err := contentMapService.GetContentMap(tenantCtx.TenantID, clientLastUpdated, tenantCtx.CacheManager)
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
