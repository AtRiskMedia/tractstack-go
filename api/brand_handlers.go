// Package api provides brand configuration handlers
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetBrandConfigHandler returns tenant brand configuration
func GetBrandConfigHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if ctx.Config.BrandConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "brand configuration not loaded"})
		return
	}

	c.JSON(http.StatusOK, ctx.Config.BrandConfig)
}
