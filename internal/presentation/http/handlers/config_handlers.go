// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// AdvancedConfigStatusResponse represents the response structure for advanced config status
type AdvancedConfigStatusResponse struct {
	TursoConfigured   bool `json:"tursoConfigured"`
	TursoTokenSet     bool `json:"tursoTokenSet"`
	AdminPasswordSet  bool `json:"adminPasswordSet"`
	EditorPasswordSet bool `json:"editorPasswordSet"`
	AAIAPIKeySet      bool `json:"aaiApiKeySet"`
	TursoEnabled      bool `json:"tursoEnabled"`
}

// ConfigHandlers contains all config-related HTTP handlers
type ConfigHandlers struct {
	configService *services.ConfigService
	logger        *logging.ChanneledLogger
	perfTracker   *performance.Tracker
}

// NewConfigHandlers creates config handlers with injected dependencies
func NewConfigHandlers(
	configService *services.ConfigService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ConfigHandlers {
	return &ConfigHandlers{
		configService: configService,
		logger:        logger,
		perfTracker:   perfTracker,
	}
}

// GetBrandConfig handles GET /api/v1/config/brand
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

	// Return current brand configuration
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetBrandConfig request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)
	h.logger.System().Info("Get brand config request completed", "duration", time.Since(start))

	c.JSON(http.StatusOK, tenantCtx.Config.BrandConfig)
}

// UpdateBrandConfig handles PUT /api/v1/config/brand
func (h *ConfigHandlers) UpdateBrandConfig(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("update_brand_config_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received update brand config request", "method", c.Request.Method, "path", c.Request.URL.Path)

	// Validate permissions
	authHeader := c.GetHeader("Authorization")
	if err := h.configService.ValidateEditorPermissions(authHeader, tenantCtx); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Parse request
	var request services.BrandConfigUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	currentConfig := tenantCtx.Config.BrandConfig
	if currentConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Brand configuration not loaded"})
		return
	}

	// Get media path
	mediaPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantCtx.Config.TenantID, "media")

	// Process brand config update through service
	updatedConfig, err := h.configService.ProcessBrandConfigUpdate(mediaPath, &request, currentConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Save configuration through service
	if err := h.configService.SaveBrandConfig(tenantCtx.Config.TenantID, updatedConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update in-memory config
	tenantCtx.Config.BrandConfig = updatedConfig

	h.logger.System().Info("Update brand config request completed", "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for UpdateBrandConfig request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{"message": "Brand configuration updated successfully"})
}

// GetAdvancedConfig handles GET /api/v1/config/advanced
func (h *ConfigHandlers) GetAdvancedConfig(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_advanced_config_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received get advanced config request", "method", c.Request.Method, "path", c.Request.URL.Path)

	// Validate admin permissions
	authHeader := c.GetHeader("Authorization")
	if err := h.configService.ValidateAdminPermissions(authHeader, tenantCtx); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Check configuration status (never expose actual values)
	status := AdvancedConfigStatusResponse{
		TursoConfigured:   tenantCtx.Config.TursoDatabase != "",
		TursoTokenSet:     tenantCtx.Config.TursoToken != "",
		AdminPasswordSet:  tenantCtx.Config.AdminPassword != "",
		EditorPasswordSet: tenantCtx.Config.EditorPassword != "",
		AAIAPIKeySet:      tenantCtx.Config.AAIAPIKey != "",
		TursoEnabled:      tenantCtx.Config.TursoEnabled,
	}

	h.logger.System().Info("Get advanced config request completed", "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetAdvancedConfig request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{"data": status})
}

// UpdateAdvancedConfig handles PUT /api/v1/config/advanced
func (h *ConfigHandlers) UpdateAdvancedConfig(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("update_advanced_config_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received update advanced config request", "method", c.Request.Method, "path", c.Request.URL.Path)

	// Validate admin permissions
	authHeader := c.GetHeader("Authorization")
	if err := h.configService.ValidateAdminPermissions(authHeader, tenantCtx); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Parse request
	var request services.AdvancedConfigUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Validate Turso pair requirement
	hasTursoURL := request.TursoDatabaseURL != ""
	hasTursoToken := request.TursoAuthToken != ""
	if hasTursoURL != hasTursoToken {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Both Turso Database URL and Auth Token must be provided together"})
		return
	}

	// Test Turso connection if credentials provided
	if hasTursoURL && hasTursoToken {
		if err := h.configService.TestTursoConnection(request.TursoDatabaseURL, request.TursoAuthToken); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Turso connection test failed: " + err.Error()})
			return
		}
	}

	// Process advanced config update through service
	if err := h.configService.ProcessAdvancedConfigUpdate(&request, tenantCtx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Save configuration through service
	if err := h.configService.SaveAdvancedConfig(tenantCtx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.System().Info("Update advanced config request completed", "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for UpdateAdvancedConfig request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{"message": "Configuration updated successfully"})
}
