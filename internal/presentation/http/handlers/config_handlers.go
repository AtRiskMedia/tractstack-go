// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ConfigHandlers contains all configuration-related HTTP handlers
type ConfigHandlers struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewConfigHandlers creates config handlers
func NewConfigHandlers(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ConfigHandlers {
	return &ConfigHandlers{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// GetBrandConfig returns tenant brand configuration
func (h *ConfigHandlers) GetBrandConfig(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_brand_config_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received get brand config request", "method", c.Request.Method, "path", c.Request.URL.Path)

	// Check if brand config is loaded
	if tenantCtx.Config.BrandConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "brand configuration not loaded"})
		return
	}

	h.logger.System().Info("Get brand config request completed", "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetBrandConfig request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, tenantCtx.Config.BrandConfig)
}
