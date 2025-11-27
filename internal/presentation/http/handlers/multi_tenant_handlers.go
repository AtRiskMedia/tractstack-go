package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
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
	Domain            string `json:"domain"`
	AAIAPIKey         string `json:"aaiApiKey"`
}

type MultiTenantHandlers struct {
	service     *services.MultiTenantService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

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

		if exists && defaultInfo.Status == "active" {
			marker.SetError(fmt.Errorf("setup not available - tenant status: %s", defaultInfo.Status))
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Setup not available",
				"details": "System is already configured or not in fresh install state",
			})
			return
		}
	}

	h.logger.System().Info("Starting tenant initialization", "tenantId", targetID)

	domains := []string{"*"}
	if req.Domain != "" {
		domains = []string{req.Domain}
		if parts := strings.Split(req.Domain, "."); len(parts) >= 3 {
			baseDomain := strings.Join(parts[1:], ".")
			domains = append(domains, baseDomain)
		}
	}

	provisionReq := services.ProvisionRequest{
		TenantID:          targetID,
		AdminEmail:        req.AdminEmail,
		AdminPassword:     req.AdminPassword,
		AdminPasswordHash: req.AdminPasswordHash,
		Domains:           domains,
		TursoDatabaseURL:  req.TursoDatabaseURL,
		TursoAuthToken:    req.TursoAuthToken,
		HydrationToken:    req.HydrationToken,
		AAIAPIKey:         req.AAIAPIKey,
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

func (h *MultiTenantHandlers) HandleResolveDomain(c *gin.Context) {
	host := c.Query("host")
	if host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host query parameter is required"})
		return
	}

	tenantManager := h.getTenantManager()
	tenantID, err := tenantManager.GetDetector().ResolveTenantByDomain(host)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tenantId": tenantID})
}

func (h *MultiTenantHandlers) getTenantManager() *tenant.Manager {
	return h.service.GetTenantManager()
}

func (h *MultiTenantHandlers) HandleFetchSuitcase(c *gin.Context) {
	// Context is now provided by TenantMiddleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Tenant context not found"})
		return
	}
	tenantID := tenantCtx.TenantID

	// 1. Security Guard: Only allow access if the site is NOT yet initialized.
	if tenantCtx.Config.BrandConfig.SiteInit {
		c.JSON(http.StatusForbidden, gin.H{"error": "Site is already initialized."})
		return
	}

	// 2. Retrieve the token directly from the already-loaded tenant configuration
	token := tenantCtx.Config.HydrationToken

	if token == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No suitcase available"})
		return
	}

	// 3. Fetch the suitcase using the resolved token from the local agent
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:8081/api/local/suitcase/%s", token))
	if err != nil {
		h.logger.System().Error("Failed to fetch suitcase from agent", "error", err, "tenantId", tenantID)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to connect to local agent"})
		return
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("Fetch Suitcase fail.")
	}

	c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, nil)
}

func (h *MultiTenantHandlers) HandleSetupComplete(c *gin.Context) {
	// Context is now provided by TenantMiddleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Tenant context not found"})
		return
	}
	tenantID := tenantCtx.TenantID

	// 1. Security Guard: Only allow completion if site is NOT initialized
	if tenantCtx.Config.BrandConfig.SiteInit {
		c.JSON(http.StatusForbidden, gin.H{"error": "Site is already initialized"})
		return
	}

	// 2. Use the internal token for authorization
	token := tenantCtx.Config.HydrationToken

	marker := h.perfTracker.StartOperation("handler_setup_complete", tenantID)
	defer marker.Complete()

	// 3. CompleteSetup handles token verification (if any) and final removal from disk
	if err := h.service.CompleteSetup(tenantID, token); err != nil {
		marker.SetError(err)
		h.logger.System().Error("Failed to complete setup", "error", err, "tenantId", tenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete setup", "details": err.Error()})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"success": true})
}
