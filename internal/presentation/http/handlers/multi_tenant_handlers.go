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
	AdminEmail       string `json:"adminEmail" binding:"required"`
	AdminPassword    string `json:"adminPassword" binding:"required"`
	TursoDatabaseURL string `json:"tursoDatabaseURL,omitempty"`
	TursoAuthToken   string `json:"tursoAuthToken,omitempty"`
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

// HandleProvisionTenant handles POST /api/v1/tenant/provision
func (h *MultiTenantHandlers) HandleProvisionTenant(c *gin.Context) {
	marker := h.perfTracker.StartOperation("handler_provision_tenant", "unknown")
	defer marker.Complete()

	var req services.ProvisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		marker.SetError(err)
		h.logger.System().Warn("Failed to bind JSON for tenant provision", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}
	marker.TenantID = req.TenantID

	activationToken, err := h.service.ProvisionTenant(req)
	if err != nil {
		marker.SetError(err)
		h.logger.System().Error("Tenant provisioning failed", "error", err, "tenantId", req.TenantID)
		// Use HTTP 409 Conflict for business logic failures like "tenant already exists".
		c.JSON(http.StatusConflict, gin.H{"error": "Tenant provisioning failed", "details": err.Error()})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusCreated, gin.H{
		"status":  "ok",
		"message": "Tenant provisioned successfully. Activation email sent.",
		"token":   activationToken,
	})
}

// HandleActivateTenant handles POST /api/v1/tenant/activation
func (h *MultiTenantHandlers) HandleActivateTenant(c *gin.Context) {
	marker := h.perfTracker.StartOperation("handler_activate_tenant", "unknown")
	defer marker.Complete()

	var req services.ActivationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		marker.SetError(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := h.service.ActivateTenant(req.Token); err != nil {
		marker.SetError(err)
		h.logger.System().Error("Tenant activation failed", "error", err)
		c.JSON(http.StatusConflict, gin.H{"error": "Tenant activation failed", "details": err.Error()})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "Tenant activated successfully."})
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
	marker := h.perfTracker.StartOperation("handler_setup_initialize", "default")
	defer marker.Complete()

	// SECURITY: Only allow setup for inactive default tenant
	// We need to check tenant status without going through tenant middleware
	tenantManager := h.getTenantManager() // Will need this helper method
	detector := tenantManager.GetDetector()
	registry := detector.GetRegistry()

	// Check if default tenant exists and is inactive
	defaultInfo, exists := registry.Tenants["default"]
	if !exists || defaultInfo.Status != "inactive" {
		marker.SetError(fmt.Errorf("setup not available - tenant status: %s", defaultInfo.Status))
		c.JSON(http.StatusConflict, gin.H{
			"error":   "Setup not available",
			"details": "System is already configured or not in fresh install state",
		})
		return
	}

	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		marker.SetError(err)
		h.logger.System().Warn("Failed to bind JSON for setup initialize", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	// Use hardcoded tenantID = "default" for setup
	provisionReq := services.ProvisionRequest{
		TenantID:         "default",
		AdminEmail:       req.AdminEmail,
		AdminPassword:    req.AdminPassword,
		Domains:          []string{"*"},
		TursoDatabaseURL: req.TursoDatabaseURL,
		TursoAuthToken:   req.TursoAuthToken,
	}

	h.logger.System().Info("Starting fresh install setup", "tenantId", "default")

	// Provision tenant (creates config files, sets status to "reserved")
	activationToken, err := h.service.ProvisionTenant(provisionReq)
	if err != nil {
		marker.SetError(err)
		h.logger.System().Error("Setup provisioning failed", "error", err)
		c.JSON(http.StatusConflict, gin.H{"error": "Setup failed", "details": err.Error()})
		return
	}

	// Immediately activate (creates database schema, sets status to "active")
	if err := h.service.ActivateTenant(activationToken); err != nil {
		marker.SetError(err)
		h.logger.System().Error("Setup activation failed", "error", err)
		c.JSON(http.StatusConflict, gin.H{"error": "Activation failed", "details": err.Error()})
		return
	}

	marker.SetSuccess(true)
	h.logger.System().Info("Fresh install setup completed successfully")

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Setup completed successfully",
	})
}

// getTenantManager returns the tenant manager from the service
func (h *MultiTenantHandlers) getTenantManager() *tenant.Manager {
	// This requires adding a method to MultiTenantService to expose the tenant manager
	return h.service.GetTenantManager()
}
