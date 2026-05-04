// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ConfirmBookingRequest defines the payload for manually confirming a free booking hold.
type ConfirmBookingRequest struct {
	TraceID string `json:"traceId" binding:"required"`
}

// HoldSlotRequest defines the payload for creating a temporary booking hold.
type HoldSlotRequest struct {
	TraceID         string    `json:"traceId" binding:"required"`
	ResourceIDs     []string  `json:"resourceIds" binding:"required"`
	LeadID          string    `json:"leadId" binding:"required"`
	StartTime       time.Time `json:"startTime" binding:"required"`
	EndTime         time.Time `json:"endTime" binding:"required"`
	AppointmentMode string    `json:"appointmentMode" binding:"required"`
}

// BookingHandlers provides endpoints for the native booking engine.
type BookingHandlers struct {
	bookingService *services.BookingService
	logger         *logging.ChanneledLogger
	perfTracker    *performance.Tracker
}

// NewBookingHandlers creates a new instance of BookingHandlers.
func NewBookingHandlers(
	bookingService *services.BookingService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *BookingHandlers {
	return &BookingHandlers{
		bookingService: bookingService,
		logger:         logger,
		perfTracker:    perfTracker,
	}
}

// HandleGetAvailability returns a list of existing bookings to allow the frontend to calculate open slots.
func (h *BookingHandlers) HandleGetAvailability(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("booking_get_availability", tenantCtx.TenantID)
	defer marker.Complete()

	startStr := c.Query("start")
	endStr := c.Query("end")

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time format"})
		return
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time format"})
		return
	}

	bookings, err := h.bookingService.GetAvailability(tenantCtx, start, end)
	if err != nil {
		h.logger.System().Error("Failed to fetch availability", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch availability"})
		return
	}

	// We also provide the Scheduling config so the frontend has the source of truth for business hours and timezone
	c.JSON(http.StatusOK, gin.H{
		"bookings":   bookings,
		"scheduling": tenantCtx.Config.BrandConfig.Scheduling,
	})

	marker.SetSuccess(true)
}

// HandleHoldSlot attempts to create a PENDING booking hold for a specific time slot.
func (h *BookingHandlers) HandleHoldSlot(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("booking_hold_slot", tenantCtx.TenantID)
	defer marker.Complete()

	var req HoldSlotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request body: %v", err)})
		return
	}

	err := h.bookingService.HoldSlot(
		c.Request.Context(),
		tenantCtx,
		req.TraceID,
		req.ResourceIDs,
		req.LeadID,
		req.StartTime.UTC(),
		req.EndTime.UTC(),
		req.AppointmentMode,
	)
	if err != nil {
		h.logger.System().Warn("Failed to hold booking slot", "error", err, "traceId", req.TraceID)
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	h.logger.System().Info("Booking hold created", "traceId", req.TraceID, "tenantId", tenantCtx.TenantID)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"status":  "PENDING",
	})
	marker.SetSuccess(true)
}

// HandleReleaseHold drops a pending booking when explicitly cancelled by the frontend
func (h *BookingHandlers) HandleReleaseHold(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	traceID := c.Param("traceId")
	if traceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "traceId is required"})
		return
	}

	if err := h.bookingService.ReleaseHold(tenantCtx, traceID); err != nil {
		h.logger.System().Error("Failed to release hold", "error", err, "traceId", traceID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to release hold"})
		return
	}

	c.Status(http.StatusOK)
}

// HandleConfirmBooking manually transitions a pending hold to confirmed.
// This endpoint is explicitly used for free carts that bypass the Shopify checkout webhook flow.
func (h *BookingHandlers) HandleConfirmBooking(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("booking_confirm_free", tenantCtx.TenantID)
	defer marker.Complete()

	var req ConfirmBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// shopifyOrderID is nil because this route strictly handles transactions without payment
	if err := h.bookingService.ConfirmBooking(tenantCtx, req.TraceID, nil); err != nil {
		h.logger.System().Error("Failed to confirm free booking", "error", err, "traceId", req.TraceID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to confirm booking"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status":  "CONFIRMED",
	})
}

// HandleListBookings returns a paginated list of bookings for the administrative dashboard.
func (h *BookingHandlers) HandleListBookings(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("booking_list", tenantCtx.TenantID)
	defer marker.Complete()

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		limit = 50
	}

	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil {
		offset = 0
	}

	status := c.DefaultQuery("status", "ALL")

	bookings, totalCount, err := h.bookingService.ListBookings(tenantCtx, limit, offset, status)
	if err != nil {
		h.logger.System().Error("Failed to list bookings", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list bookings"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{
		"data":       bookings,
		"totalCount": totalCount,
	})
}

// HandleGetMetrics retrieves aggregated booking volume and conversion statistics.
func (h *BookingHandlers) HandleGetMetrics(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("booking_metrics", tenantCtx.TenantID)
	defer marker.Complete()

	metrics, err := h.bookingService.GetMetrics(tenantCtx)
	if err != nil {
		h.logger.System().Error("Failed to get booking metrics", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get booking metrics"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, metrics)
}

// HandleCancelBooking manually cancels an existing booking from the administrative dashboard.
func (h *BookingHandlers) HandleCancelBooking(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("booking_cancel", tenantCtx.TenantID)
	defer marker.Complete()

	traceID := c.Param("traceId")
	if traceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "traceId is required"})
		return
	}

	if err := h.bookingService.CancelBooking(tenantCtx, traceID); err != nil {
		h.logger.System().Error("Failed to cancel booking", "error", err, "traceId", traceID, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel booking"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status":  "CANCELLED",
	})
}
