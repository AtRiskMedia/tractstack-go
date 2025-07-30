// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// GetBrandConfigHandler returns tenant brand configuration
func GetBrandConfigHandler(c *gin.Context) {
	// Get tenant context from middleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Check if brand config is loaded
	if tenantCtx.Config.BrandConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "brand configuration not loaded"})
		return
	}

	// Return brand configuration as JSON
	c.JSON(http.StatusOK, tenantCtx.Config.BrandConfig)
}
