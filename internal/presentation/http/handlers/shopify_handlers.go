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

	// Fetch products using the service which handles secrets and GraphQL communication
	productsJSON, err := h.shopifyService.FetchProducts(tenantCtx)
	if err != nil {
		h.logger.System().Error("Failed to fetch Shopify products", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// We return the raw JSON from Shopify directly to the frontend
	// The frontend handles the caching (NanoStore TTL)
	c.Data(http.StatusOK, "application/json", productsJSON)

	h.logger.System().Info("Shopify products fetched successfully", "duration", time.Since(start))
	marker.SetSuccess(true)
}

// HandleWebhook processes incoming webhooks from Shopify (product updates/creation).
// It verifies the HMAC signature and upserts the resource into the local database.
func (h *ShopifyHandlers) HandleWebhook(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("shopify_webhook", tenantCtx.TenantID)
	defer marker.Complete()

	// 1. Read the raw body for HMAC verification
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.System().Error("Failed to read webhook body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	// 2. Verify Signature
	signature := c.GetHeader("X-Shopify-Hmac-Sha256")
	if !h.shopifyService.VerifySignature(tenantCtx, body, signature) {
		h.logger.System().Warn("Invalid Shopify webhook signature", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	// 3. Parse the payload into a ResourceNode
	resource, err := h.shopifyService.ParseWebhook(body)
	if err != nil {
		h.logger.System().Error("Failed to parse Shopify webhook", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload format"})
		return
	}

	// 4. Upsert the resource using the optimized in-memory lookup
	// Capture the operation status to match the new (string, error) signature
	op, err := h.resourceService.UpsertShopifyResource(tenantCtx, resource)
	if err != nil {
		h.logger.System().Error("Failed to upsert Shopify resource", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync resource"})
		return
	}

	h.logger.System().Info("Shopify webhook processed successfully",
		"tenantId", tenantCtx.TenantID,
		"resourceSlug", resource.Slug,
		"operation", op,
		"duration", time.Since(start))

	marker.SetSuccess(true)
	c.Status(http.StatusOK)
}
