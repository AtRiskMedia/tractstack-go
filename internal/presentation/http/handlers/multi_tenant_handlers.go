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
	token := c.GetHeader("X-Hydration-Token")

	tenantManager := h.getTenantManager()
	registry := tenantManager.GetDetector().GetRegistry()

	info, ok := registry.Tenants["default"]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
		return
	}

	// Security Logic:
	// 1. If a token is provided, it MUST match.
	// 2. If NO token is provided, we check if we are in a safe "Bootstrap" state.
	//    Bootstrap State = Tenant is Active (files exist) AND Site is NOT Initialized (BrandConfig.SiteInit == false).
	if token != "" {
		if info.HydrationToken != token {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}
	} else {
		// Tokenless Access (Server-Side Bootstrap)
		if info.Status != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Tenant must be active to bootstrap"})
			return
		}

		// Load Brand Config to check initialization state
		brandConfig, err := tenant.LoadBrandConfig("default")
		if err != nil {
			h.logger.System().Error("Failed to load brand config during bootstrap check", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Configuration error"})
			return
		}

		if brandConfig.SiteInit {
			c.JSON(http.StatusForbidden, gin.H{"error": "Site is already initialized. Token required."})
			return
		}

		// Allowed! Use the internal token.
		token = info.HydrationToken
	}

	if token == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No suitcase available"})
		return
	}

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:8081/api/local/suitcase/%s", token))
	if err != nil {
		h.logger.System().Error("Failed to fetch suitcase from agent", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to connect to local agent"})
		return
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("Fetch Suitcase fail.")
	}

	c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, nil)
}

func (h *MultiTenantHandlers) HandleSetupComplete(c *gin.Context) {
	tenantID := c.GetHeader("X-Tenant-Id")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "X-Tenant-Id header is required"})
		return
	}

	token := c.GetHeader("X-Hydration-Token")

	marker := h.perfTracker.StartOperation("handler_setup_complete", tenantID)
	defer marker.Complete()

	if err := h.service.CompleteSetup(tenantID, token); err != nil {
		marker.SetError(err)
		h.logger.System().Error("Failed to complete setup", "error", err, "tenantId", tenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete setup", "details": err.Error()})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"success": true})
}
