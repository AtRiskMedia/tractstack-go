// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"io"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// CreateCheckoutRequest handles to cart lines
type CreateCheckoutRequest struct {
	Lines []services.CartLineInput `json:"lines"`
}

// ShopifyHandlers provides endpoints for Shopify integration.
type ShopifyHandlers struct {
	shopifyService  *services.ShopifyService
	resourceService *services.ResourceService
	logger          *logging.ChanneledLogger
	perfTracker     *performance.Tracker
}

// NewShopifyHandlers creates a new instance of ShopifyHandlers.
func NewShopifyHandlers(
	shopifyService *services.ShopifyService,
	resourceService *services.ResourceService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ShopifyHandlers {
	return &ShopifyHandlers{
		shopifyService:  shopifyService,
		resourceService: resourceService,
		logger:          logger,
		perfTracker:     perfTracker,
	}
}

// HandleGetProducts proxies the request to Shopify GraphQL via the backend service.
// This endpoint is authenticated and used by the frontend dashboard.
func (h *ShopifyHandlers) HandleGetProducts(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("shopify_get_products", tenantCtx.TenantID)
	defer marker.Complete()

	h.logger.System().Debug("Handling Shopify get products request", "tenantId", tenantCtx.TenantID)

	products, err := h.shopifyService.FetchProducts(tenantCtx)
	if err != nil {
		h.logger.System().Error("Failed to fetch Shopify products", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"products": products})

	h.logger.System().Info("Shopify products fetched successfully", "duration", time.Since(start))
	marker.SetSuccess(true)
}

// HandleWebhook processes incoming webhooks from Shopify (product updates/creation/deletion).
// It verifies the HMAC signature and routes the operation to the appropriate service method.
func (h *ShopifyHandlers) HandleWebhook(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("shopify_webhook", tenantCtx.TenantID)
	defer marker.Complete()

	// 1. Identify the Shopify Topic
	topic := c.GetHeader("X-Shopify-Topic")
	if topic == "" {
		h.logger.System().Warn("Missing X-Shopify-Topic header", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing shopify topic header"})
		return
	}

	// 2. Read the raw body for HMAC verification
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.System().Error("Failed to read webhook body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	// 3. Verify Signature
	signature := c.GetHeader("X-Shopify-Hmac-Sha256")
	if !h.shopifyService.VerifySignature(tenantCtx, body, signature) {
		h.logger.System().Warn("Invalid Shopify webhook signature", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	// 4. Parse the payload into a ResourceNode
	// ParseWebhook will be updated to handle minimal deletion payloads (gid only).
	resource, err := h.shopifyService.ParseWebhook(body)
	if err != nil {
		h.logger.System().Error("Failed to parse Shopify webhook", "error", err, "topic", topic)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload format"})
		return
	}

	var op string
	var syncErr error

	// 5. Logic Switch: Branch based on Topic
	switch topic {
	case "products/delete":
		gid, _ := resource.OptionsPayload["gid"].(string)
		op, syncErr = h.resourceService.SyncShopifyDeletion(tenantCtx, gid)

	case "products/create", "products/update":
		op, syncErr = h.resourceService.UpsertShopifyResource(tenantCtx, resource)

	default:
		h.logger.System().Warn("Unsupported Shopify webhook topic", "topic", topic, "tenantId", tenantCtx.TenantID)
		c.Status(http.StatusAccepted) // Acknowledge but do nothing
		return
	}

	if syncErr != nil {
		h.logger.System().Error("Failed to sync Shopify webhook", "error", syncErr, "topic", topic)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync resource"})
		return
	}

	h.logger.System().Info("Shopify webhook processed successfully",
		"tenantId", tenantCtx.TenantID,
		"topic", topic,
		"operation", op,
		"duration", time.Since(start))

	marker.SetSuccess(true)
	c.Status(http.StatusOK)
}

// HandleCreateCheckout creates a new Shopify cart/checkout via the service.
func (h *ShopifyHandlers) HandleCreateCheckout(c *gin.Context) {
	// 1. Resolve Tenant Context using your existing middleware pattern
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("shopify_create_checkout", tenantCtx.TenantID)
	defer marker.Complete()

	// 2. Parse Request using Gin's binder
	var req CreateCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.System().Warn("Invalid checkout request body", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if len(req.Lines) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cart is empty"})
		return
	}

	// 3. Call Service
	checkoutURL, err := h.shopifyService.CreateCart(tenantCtx, req.Lines)
	if err != nil {
		h.logger.System().Error("Failed to create shopify checkout", "error", err, "tenantId", tenantCtx.TenantID)
		// Return 500 (Internal Server Error) with the error message
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 4. Return Success
	h.logger.System().Info("Shopify checkout created", "url", checkoutURL, "tenantId", tenantCtx.TenantID)
	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"checkoutUrl": checkoutURL})
}
