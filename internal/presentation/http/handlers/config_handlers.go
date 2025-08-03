// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ConfigHandlers contains all configuration-related HTTP handlers
type ConfigHandlers struct {
	logger *logging.ChanneledLogger
}

// NewConfigHandlers creates config handlers
func NewConfigHandlers(logger *logging.ChanneledLogger) *ConfigHandlers {
	return &ConfigHandlers{
		logger: logger,
	}
}

// GetBrandConfig returns tenant brand configuration
func (h *ConfigHandlers) GetBrandConfig(c *gin.Context) {
	start := time.Now()
	h.logger.System().Debug("Received get brand config request", "method", c.Request.Method, "path", c.Request.URL.Path)
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

	h.logger.System().Info("Get brand config request completed", "duration", time.Since(start))

	c.JSON(http.StatusOK, tenantCtx.Config.BrandConfig)
}
