// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	logger          *logging.ChanneledLogger
	perfTracker     *performance.Tracker
}

// NewShopifyHandlers creates a new instance of ShopifyHandlers.
func NewShopifyHandlers(
	shopifyService *services.ShopifyService,
	resourceService *services.ResourceService,
	bookingService *services.BookingService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ShopifyHandlers {
	return &ShopifyHandlers{
		shopifyService:  shopifyService,
		resourceService: resourceService,
		bookingService:  bookingService,
		logger:          logger,
		perfTracker:     perfTracker,
	}
}

// HandleGetProducts proxies the request to Shopify GraphQL via the backend service.
// This endpoint supports search and cursor pagination for dynamic discovery.
func (h *ShopifyHandlers) HandleGetProducts(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
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

// HandleWebhook processes incoming webhooks from Shopify (product updates/creation/deletion and order payments).
// It verifies the HMAC signature, checks local existence for products, and confirms bookings for paid orders.
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
					h.logger.System().Error("Failed to confirm booking from webhook", "error", err, "traceId", attr.Value)
				}
			}
		}

	case "products/delete", "products/create", "products/update":
		var rawData map[string]any
		if err := json.Unmarshal(body, &rawData); err != nil {
			h.logger.System().Error("Failed to unmarshal minimal webhook body", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json payload"})
			return
		}

		var idStr string
		if v, ok := rawData["id"].(float64); ok {
			idStr = fmt.Sprintf("%.0f", v)
		} else if v, ok := rawData["id"].(string); ok {
			idStr = v
		}

		if idStr == "" {
			h.logger.System().Warn("Missing ID in Shopify webhook", "tenantId", tenantCtx.TenantID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing id in payload"})
			return
		}

		gid := idStr
		if !strings.HasPrefix(idStr, "gid://") {
			gid = fmt.Sprintf("gid://shopify/Product/%s", idStr)
		}

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
			resource, err := h.shopifyService.ParseWebhook(body)
			if err != nil {
				h.logger.System().Error("Failed to parse Shopify webhook", "error", err, "topic", topic)
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload format"})
				return
			}
			op, syncErr = h.resourceService.UpsertShopifyResource(tenantCtx, resource)
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
		h.logger.System().Warn("Unsupported Shopify webhook topic", "topic", topic, "tenantId", tenantCtx.TenantID)
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
