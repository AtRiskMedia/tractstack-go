// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// HoldSlotRequest defines the payload for creating a temporary booking hold.
type HoldSlotRequest struct {
	TraceID     string    `json:"traceId" binding:"required"`
	ResourceIDs []string  `json:"resourceIds" binding:"required"`
	LeadID      string    `json:"leadId" binding:"required"`
	StartTime   time.Time `json:"startTime" binding:"required"`
	EndTime     time.Time `json:"endTime" binding:"required"`
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

	resourceIDsParam := c.Query("resourceIds")
	if resourceIDsParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resourceIds parameter is required"})
		return
	}

	// Split the comma-separated list of resource IDs
	resourceIDs := strings.Split(resourceIDsParam, ",")

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

	bookings, err := h.bookingService.GetAvailability(tenantCtx, resourceIDs, start, end)
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
		tenantCtx,
		req.TraceID,
		req.ResourceIDs,
		req.LeadID,
		req.StartTime.UTC(),
		req.EndTime.UTC(),
	)
	if err != nil {
		h.logger.System().Warn("Failed to hold booking slot", "error", err, "traceId", req.TraceID)
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	h.logger.System().Info("Booking hold created", "traceId", req.TraceID, "tenantId", tenantCtx.TenantID)
	c.Status(http.StatusCreated)
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
