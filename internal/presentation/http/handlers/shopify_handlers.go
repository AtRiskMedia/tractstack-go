// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// CreateCheckoutRequest handles cart lines, attributes, and user identity.
type CreateCheckoutRequest struct {
	Lines      []services.CartLineInput  `json:"lines"`
	Attributes []services.AttributeInput `json:"attributes"`
	Email      string                    `json:"email"`
}

// ShopifyHandlers provides endpoints for Shopify integration.
type ShopifyHandlers struct {
	shopifyService  *services.ShopifyService
	resourceService *services.ResourceService
	bookingService  *services.BookingService
	emailWorker     *services.EmailWorker
	logger          *logging.ChanneledLogger
	perfTracker     *performance.Tracker
}

// NewShopifyHandlers creates a new instance of ShopifyHandlers.
func NewShopifyHandlers(
	shopifyService *services.ShopifyService,
	resourceService *services.ResourceService,
	bookingService *services.BookingService,
	emailWorker *services.EmailWorker,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ShopifyHandlers {
	return &ShopifyHandlers{
		shopifyService:  shopifyService,
		resourceService: resourceService,
		bookingService:  bookingService,
		emailWorker:     emailWorker,
		logger:          logger,
		perfTracker:     perfTracker,
	}
}

// HandleGetProducts proxies the request to Shopify GraphQL via the backend service.
// This endpoint supports search and cursor pagination for dynamic discovery.
func (h *ShopifyHandlers) HandleGetProducts(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		h.logger.System().Error("PRODUCTS ABORT: No tenant context found", "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("shopify_get_products", tenantCtx.TenantID)
	defer marker.Complete()

	queryStr := c.Query("q")
	var cursor *string
	if cVal := c.Query("cursor"); cVal != "" {
		cursor = &cVal
	}

	h.logger.System().Debug("Handling Shopify get products request", "tenantId", tenantCtx.TenantID, "query", queryStr)

	result, err := h.shopifyService.FetchProducts(tenantCtx, queryStr, cursor)
	if err != nil {
		h.logger.System().Error("Failed to fetch Shopify products", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)

	h.logger.System().Info("Shopify products fetched successfully", "duration", time.Since(start))
	marker.SetSuccess(true)
}

// HandleWebhook processes incoming webhooks from Shopify, directing product updates to the
// high-fidelity GraphQL sync pipeline and processing order payments via the REST payload.
func (h *ShopifyHandlers) HandleWebhook(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("shopify_webhook", tenantCtx.TenantID)
	defer marker.Complete()

	topic := c.GetHeader("X-Shopify-Topic")
	if topic == "" {
		h.logger.System().Warn("Missing X-Shopify-Topic header", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing shopify topic header"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.System().Error("Failed to read webhook body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	signature := c.GetHeader("X-Shopify-Hmac-Sha256")
	if !h.shopifyService.VerifySignature(tenantCtx, body, signature) {
		h.logger.System().Warn("Invalid Shopify webhook signature", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	switch topic {
	case "orders/paid":
		// AUTHORITATIVE REST PATH: Headless Storefront API cannot query Order objects by ID.
		var order struct {
			ID             int64 `json:"id"`
			NoteAttributes []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"note_attributes"`
		}
		if err := json.Unmarshal(body, &order); err != nil {
			h.logger.System().Error("Failed to parse orders/paid webhook", "error", err)
			return
		}

		orderID := fmt.Sprintf("%d", order.ID)
		for _, attr := range order.NoteAttributes {
			if attr.Name == "bookingId" || attr.Name == "Trace ID" {
				if err := h.bookingService.ConfirmBooking(tenantCtx, attr.Value, &orderID); err != nil {
					if errors.Is(err, services.ErrBookingNotFound) {
						h.logger.System().Error("ORPHANED PAYMENT: User paid for an expired booking hold", "traceId", attr.Value, "shopifyOrderId", orderID)

						if h.emailWorker != nil && tenantCtx.Config.BrandConfig != nil && tenantCtx.Config.BrandConfig.AdminEmail != "" {
							_ = h.emailWorker.Enqueue(services.EmailJob{
								TenantID:     tenantCtx.TenantID,
								To:           []string{tenantCtx.Config.BrandConfig.AdminEmail},
								Category:     "shopify",
								TemplateName: "orphaned-payment",
								Data: map[string]any{
									"TraceID":        attr.Value,
									"ShopifyOrderID": orderID,
								},
							})
						}
						return
					}
					h.logger.System().Error("Failed to confirm booking from webhook", "error", err, "traceId", attr.Value)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "database lock or confirmation failure, forcing retry"})
					return
				}
			}
		}

	case "products/delete", "products/update":
		// EXTRACT GID: Parse minimal ID from REST payload
		gid, err := h.shopifyService.ParseWebhook(body)
		if err != nil {
			h.logger.System().Error("Failed to extract GID from Shopify webhook", "error", err, "topic", topic)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload format"})
			return
		}

		// FILTER: Only process updates for resources currently tracked in the database
		existsLocally, err := h.resourceService.ExistsByShopifyGID(tenantCtx, gid)
		if err != nil {
			h.logger.System().Error("Failed to check local Shopify resource existence", "error", err, "gid", gid)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal database error"})
			return
		}

		if !existsLocally {
			h.logger.System().Debug("Ignoring webhook for untracked Shopify resource", "gid", gid, "topic", topic)
			c.Status(http.StatusOK)
			return
		}

		var op string
		var syncErr error

		if topic == "products/delete" {
			op, syncErr = h.resourceService.SyncShopifyDeletion(tenantCtx, gid)
		} else {
			// TARGETED GRAPHQL SYNC: Fetch high-fidelity data and perform non-destructive merge
			op, syncErr = h.shopifyService.SyncProductByGID(tenantCtx, gid)
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

	default:
		// Accept unsupported topics without processing to prevent Shopify from retrying noise
		h.logger.System().Debug("Unsupported Shopify webhook topic received", "topic", topic, "tenantId", tenantCtx.TenantID)
		c.Status(http.StatusAccepted)
		return
	}

	marker.SetSuccess(true)
	c.Status(http.StatusOK)
}

// HandleCreateCheckout creates a new Shopify cart/checkout via the service.
func (h *ShopifyHandlers) HandleCreateCheckout(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		h.logger.System().Error("CHECKOUT ABORT: No tenant context found", "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("shopify_create_checkout", tenantCtx.TenantID)
	defer marker.Complete()

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

	checkoutURL, err := h.shopifyService.CreateCart(tenantCtx, req.Lines, req.Attributes, req.Email)
	if err != nil {
		h.logger.System().Error("Failed to create shopify checkout", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.System().Info("Shopify checkout created", "url", checkoutURL, "tenantId", tenantCtx.TenantID)
	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"checkoutUrl": checkoutURL})
}
