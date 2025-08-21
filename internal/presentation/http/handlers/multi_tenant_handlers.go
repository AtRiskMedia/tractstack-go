// Package handlers provides HTTP handlers for tenant lifecycle management.
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/gin-gonic/gin"
)

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
