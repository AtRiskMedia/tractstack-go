// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/events"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// StateHandlers contains all state-related HTTP handlers
type StateHandlers struct {
	stateService *services.StateService
	logger       *logging.ChanneledLogger
	perfTracker  *performance.Tracker
}

// NewStateHandlers creates state handlers with injected dependencies
func NewStateHandlers(stateService *services.StateService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *StateHandlers {
	return &StateHandlers{
		stateService: stateService,
		logger:       logger,
		perfTracker:  perfTracker,
	}
}

// PostState handles POST /api/v1/state - processes widget state updates and belief events
func (h *StateHandlers) PostState(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_state_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received post state request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Extract session ID from header
	sessionID := c.GetHeader("X-TractStack-Session-ID")
	if sessionID == "" {
		h.logger.System().Error("State request missing session ID", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID required"})
		return
	}

	// Parse form data into request structure
	req := &services.StateRequest{
		BeliefID:       c.PostForm("beliefId"),
		BeliefType:     c.PostForm("beliefType"),
		BeliefValue:    c.PostForm("beliefValue"),
		BeliefObject:   c.PostForm("beliefObject"),
		PaneID:         c.PostForm("paneId"),
		SessionID:      sessionID,
		UnsetBeliefIds: c.PostForm("unsetBeliefIds"),
		GotoPaneID:     c.PostForm("gotoPaneID"),
	}

	// Log the request for debugging
	if req.UnsetBeliefIds != "" {
		h.logger.System().Debug("Processing bulk unset request",
			"tenantId", tenantCtx.TenantID,
			"sessionId", sessionID,
			"beliefIds", req.UnsetBeliefIds,
			"paneId", req.PaneID,
			"gotoPaneId", req.GotoPaneID)
	} else {
		h.logger.System().Debug("Processing single belief state request",
			"tenantId", tenantCtx.TenantID,
			"sessionId", sessionID,
			"beliefId", req.BeliefID,
			"beliefType", req.BeliefType,
			"beliefValue", req.BeliefValue,
			"beliefObject", req.BeliefObject,
			"paneId", req.PaneID)
	}

	// Validate the request
	if err := h.stateService.ValidateStateRequest(req); err != nil {
		h.logger.System().Error("State request validation failed", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Process the state update
	result := h.stateService.ProcessStateUpdate(req, tenantCtx)

	// Handle the result
	if !result.Success {
		h.logger.System().Error("State processing failed",
			"tenantId", tenantCtx.TenantID,
			"sessionId", sessionID,
			"error", result.Error,
			"duration", time.Since(start))
		marker.SetSuccess(false)
		h.logger.Perf().Info("Performance for PostState request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", false)

		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error})
		return
	}

	// Log successful processing
	if result.Events != nil {
		h.logger.System().Info("Bulk state processing completed",
			"tenantId", tenantCtx.TenantID,
			"sessionId", sessionID,
			"processedCount", len(result.Events),
			"duration", time.Since(start))
	} else {
		h.logger.System().Info("Single belief state processing completed",
			"tenantId", tenantCtx.TenantID,
			"sessionId", sessionID,
			"beliefId", req.BeliefID,
			"duration", time.Since(start))
	}

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostState request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	// Return successful result
	response := gin.H{
		"status":   result.Status,
		"tenantId": result.TenantID,
	}

	if result.Event != nil {
		response["event"] = result.Event
	}

	if result.Events != nil {
		response["events"] = result.Events
		response["processedCount"] = len(result.Events)
	}

	c.JSON(http.StatusOK, response)
}

// PostBulkState handles POST /api/v1/state/bulk - processes bulk state operations
func (h *StateHandlers) PostBulkState(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_bulk_state_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received post bulk state request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Extract session ID from header
	sessionID := c.GetHeader("X-TractStack-Session-ID")
	if sessionID == "" {
		h.logger.System().Error("Bulk state request missing session ID", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID required"})
		return
	}

	// Parse JSON request body
	var bulkRequest BulkStateRequest
	if err := c.ShouldBindJSON(&bulkRequest); err != nil {
		h.logger.System().Error("Bulk state request JSON binding failed", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Convert to state request format
	beliefIdsStr := strings.Join(bulkRequest.BeliefIds, ",")
	req := &services.StateRequest{
		SessionID:      sessionID,
		UnsetBeliefIds: beliefIdsStr,
		PaneID:         bulkRequest.PaneID,
		GotoPaneID:     bulkRequest.GotoPaneID,
	}

	h.logger.System().Debug("Processing bulk state request",
		"tenantId", tenantCtx.TenantID,
		"sessionId", sessionID,
		"action", bulkRequest.Action,
		"beliefCount", len(bulkRequest.BeliefIds),
		"paneId", bulkRequest.PaneID,
		"gotoPaneId", bulkRequest.GotoPaneID)

	// Validate the request
	if err := h.stateService.ValidateStateRequest(req); err != nil {
		h.logger.System().Error("Bulk state request validation failed", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Process the bulk state update
	result := h.stateService.ProcessStateUpdate(req, tenantCtx)

	// Handle the result
	if !result.Success {
		h.logger.System().Error("Bulk state processing failed",
			"tenantId", tenantCtx.TenantID,
			"sessionId", sessionID,
			"action", bulkRequest.Action,
			"error", result.Error,
			"duration", time.Since(start))
		marker.SetSuccess(false)
		h.logger.Perf().Info("Performance for PostBulkState request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", false)

		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error})
		return
	}

	h.logger.System().Info("Bulk state processing completed",
		"tenantId", tenantCtx.TenantID,
		"sessionId", sessionID,
		"action", bulkRequest.Action,
		"processedCount", len(result.Events),
		"duration", time.Since(start))

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostBulkState request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	// Return successful result
	c.JSON(http.StatusOK, gin.H{
		"status":         result.Status,
		"tenantId":       result.TenantID,
		"action":         bulkRequest.Action,
		"processedCount": len(result.Events),
		"events":         result.Events,
	})
}

// GetStateValidation handles GET /api/v1/state/validate - validates state data without processing
func (h *StateHandlers) GetStateValidation(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_state_validation_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received get state validation request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Extract query parameters
	beliefID := c.Query("beliefId")
	beliefType := c.Query("beliefType")
	beliefValue := c.Query("beliefValue")
	beliefObject := c.Query("beliefObject")

	if beliefID == "" || beliefType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "beliefId and beliefType are required"})
		return
	}

	// Create a state request for validation
	req := &services.StateRequest{
		BeliefID:     beliefID,
		BeliefType:   beliefType,
		BeliefValue:  beliefValue,
		BeliefObject: beliefObject,
		SessionID:    "validation", // Dummy session ID for validation
	}

	// Validate the request
	err := h.stateService.ValidateStateRequest(req)

	validationResult := map[string]any{
		"valid":      err == nil,
		"beliefId":   beliefID,
		"beliefType": beliefType,
	}

	if err != nil {
		validationResult["error"] = err.Error()
	}

	// Also validate the event that would be created
	if err == nil {
		// This is a bit of a hack - we convert to event and validate it
		// In a real system, you might want a separate validation method
		event := h.convertToEventForValidation(req)
		eventValidation := h.stateService.ValidateEvent(event)
		validationResult["eventValidation"] = eventValidation
	}

	h.logger.System().Info("State validation completed",
		"tenantId", tenantCtx.TenantID,
		"beliefId", beliefID,
		"beliefType", beliefType,
		"valid", err == nil,
		"duration", time.Since(start))

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetStateValidation request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, validationResult)
}

// convertToEventForValidation converts a state request to an event for validation purposes
func (h *StateHandlers) convertToEventForValidation(req *services.StateRequest) events.Event {
	// This mirrors the logic in StateService.convertToEvent
	if req.BeliefObject != "" {
		return events.Event{
			ID:     req.BeliefID,
			Type:   req.BeliefType,
			Verb:   "IDENTIFY_AS",
			Object: req.BeliefObject,
		}
	}

	if req.BeliefValue != "" && req.BeliefValue != "0" {
		return events.Event{
			ID:     req.BeliefID,
			Type:   req.BeliefType,
			Verb:   req.BeliefValue,
			Object: req.BeliefID,
		}
	}

	return events.Event{
		ID:     req.BeliefID,
		Type:   req.BeliefType,
		Verb:   "UNSET",
		Object: req.BeliefID,
	}
}

// StateRequestForm represents form data structure for state requests
type StateRequestForm struct {
	BeliefID       string `form:"beliefId"`
	BeliefType     string `form:"beliefType"`
	BeliefValue    string `form:"beliefValue"`
	BeliefObject   string `form:"beliefObject"`
	PaneID         string `form:"paneId"`
	UnsetBeliefIds string `form:"unsetBeliefIds"`
	GotoPaneID     string `form:"gotoPaneID"`
}

// BulkStateRequest represents JSON structure for bulk state requests
type BulkStateRequest struct {
	BeliefIds  []string `json:"beliefIds" binding:"required"`
	Action     string   `json:"action" binding:"required"`
	PaneID     string   `json:"paneId,omitempty"`
	GotoPaneID string   `json:"gotoPaneId,omitempty"`
}

// StateResponse represents the response structure for state operations
type StateResponse struct {
	Status         string         `json:"status"`
	TenantID       string         `json:"tenantId"`
	Event          *events.Event  `json:"event,omitempty"`
	Events         []events.Event `json:"events,omitempty"`
	Action         string         `json:"action,omitempty"`
	ProcessedCount int            `json:"processedCount,omitempty"`
}

// ValidationResponse represents the response structure for validation requests
type ValidationResponse struct {
	Valid           bool           `json:"valid"`
	BeliefID        string         `json:"beliefId"`
	BeliefType      string         `json:"beliefType"`
	Error           string         `json:"error,omitempty"`
	EventValidation map[string]any `json:"eventValidation,omitempty"`
}
