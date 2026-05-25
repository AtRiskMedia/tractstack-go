// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"net/http"
	"strconv"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// SaleHandlers provides endpoints for Shopify sales receipt logs.
type SaleHandlers struct {
	saleService *services.SaleService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewSaleHandlers creates a new instance of SaleHandlers.
func NewSaleHandlers(
	saleService *services.SaleService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *SaleHandlers {
	return &SaleHandlers{
		saleService: saleService,
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// HandleListSales returns a paginated list of Shopify paid receipts.
func (h *SaleHandlers) HandleListSales(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("sale_list", tenantCtx.TenantID)
	defer marker.Complete()

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		limit = 50
	}

	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil {
		offset = 0
	}

	sales, totalCount, err := h.saleService.List(tenantCtx, limit, offset)
	if err != nil {
		h.logger.System().Error("Failed to list sales", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sales"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{
		"data":       sales,
		"totalCount": totalCount,
	})
}

// HandleGetMetrics retrieves paid Shopify receipt aggregates.
func (h *SaleHandlers) HandleGetMetrics(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("sale_metrics", tenantCtx.TenantID)
	defer marker.Complete()

	metrics, err := h.saleService.GetMetrics(tenantCtx)
	if err != nil {
		h.logger.System().Error("Failed to get sales metrics", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get sales metrics"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, metrics)
}
