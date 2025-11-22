// Package handlers provides HTTP handlers for tenant lifecycle management.
package handlers

import (
	"fmt"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/gin-gonic/gin"
)

type SetupRequest struct {
	TenantID          string `json:"tenantId"`
	AdminEmail        string `json:"adminEmail" binding:"required"`
	AdminPassword     string `json:"adminPassword"`
	AdminPasswordHash string `json:"adminPasswordHash"`
	HydrationToken    string `json:"hydrationToken"`
	TursoDatabaseURL  string `json:"tursoDatabaseURL,omitempty"`
	TursoAuthToken    string `json:"tursoAuthToken,omitempty"`
}

// MultiTenantHandlers handles HTTP requests for tenant lifecycle management.
type MultiTenantHandlers struct {
	service     *services.MultiTenantService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewMultiTenantHandlers creates a new MultiTenantHandlers instance.
func NewMultiTenantHandlers(
	service *services.MultiTenantService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *MultiTenantHandlers {
	return &MultiTenantHandlers{
		service:     service,
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// HandleGetCapacity handles GET /api/v1/tenant/capacity
func (h *MultiTenantHandlers) HandleGetCapacity(c *gin.Context) {
	marker := h.perfTracker.StartOperation("handler_get_capacity", "system")
	defer marker.Complete()

	capacity, err := h.service.GetCapacity()
	if err != nil {
		marker.SetError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get capacity", "details": err.Error()})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, capacity)
}

// HandleSetupInitialize handles POST /api/v1/setup/initialize
func (h *MultiTenantHandlers) HandleSetupInitialize(c *gin.Context) {
	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.System().Warn("Failed to bind JSON for setup initialize", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	targetID := req.TenantID
	if targetID == "" {
		targetID = "default"
	}

	marker := h.perfTracker.StartOperation("handler_setup_initialize", targetID)
	defer marker.Complete()

	if targetID == "default" {
		tenantManager := h.getTenantManager()
		registry := tenantManager.GetDetector().GetRegistry()
		defaultInfo, exists := registry.Tenants["default"]

		if !exists || defaultInfo.Status != "inactive" {
			marker.SetError(fmt.Errorf("setup not available - tenant status: %s", defaultInfo.Status))
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Setup not available",
				"details": "System is already configured or not in fresh install state",
			})
			return
		}
	} else {
		config, err := tenant.LoadTenantConfig(targetID, h.logger)
		if err != nil {
			marker.SetError(fmt.Errorf("failed to load tenant config for auth: %w", err))
			c.JSON(http.StatusForbidden, gin.H{"error": "Authorization failed", "details": "Tenant not pre-allocated"})
			return
		}

		if config.HydrationToken == "" {
			marker.SetError(fmt.Errorf("tenant %s has no hydration token configured", targetID))
			c.JSON(http.StatusForbidden, gin.H{"error": "Authorization failed", "details": "Invalid tenant configuration"})
			return
		}

		if req.HydrationToken == "" || req.HydrationToken != config.HydrationToken {
			marker.SetError(fmt.Errorf("hydration token mismatch for tenant %s", targetID))
			c.JSON(http.StatusForbidden, gin.H{"error": "Authorization failed", "details": "Invalid provisioning token"})
			return
		}
	}

	h.logger.System().Info("Starting tenant initialization", "tenantId", targetID)

	provisionReq := services.ProvisionRequest{
		TenantID:          targetID,
		AdminEmail:        req.AdminEmail,
		AdminPassword:     req.AdminPassword,
		AdminPasswordHash: req.AdminPasswordHash,
		Domains:           []string{"*"},
		TursoDatabaseURL:  req.TursoDatabaseURL,
		TursoAuthToken:    req.TursoAuthToken,
	}

	if err := h.service.ProvisionTenant(provisionReq); err != nil {
		marker.SetError(err)
		h.logger.System().Error("Setup provisioning failed", "error", err, "tenantId", targetID)
		c.JSON(http.StatusConflict, gin.H{"error": "Setup failed", "details": err.Error()})
		return
	}

	if err := h.service.ActivateTenant(targetID); err != nil {
		marker.SetError(err)
		h.logger.System().Error("Setup activation failed", "error", err, "tenantId", targetID)
		c.JSON(http.StatusConflict, gin.H{"error": "Activation failed", "details": err.Error()})
		return
	}

	marker.SetSuccess(true)
	h.logger.System().Info("Tenant initialization completed successfully", "tenantId", targetID)

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Setup completed successfully",
	})
}

func (h *MultiTenantHandlers) getTenantManager() *tenant.Manager {
	return h.service.GetTenantManager()
}

// HandleHydrate acts as a stub for content ingestion.
func (h *MultiTenantHandlers) HandleHydrate(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
