// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/events"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/messaging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// StateHandlers contains all state-related HTTP handlers with SSE broadcasting support
type StateHandlers struct {
	eventProcessor *services.EventProcessingService
	broadcaster    messaging.Broadcaster
	logger         *logging.ChanneledLogger
	perfTracker    *performance.Tracker
}

// NewStateHandlers creates state handlers with injected dependencies including broadcaster
func NewStateHandlers(
	eventProcessor *services.EventProcessingService,
	broadcaster messaging.Broadcaster,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *StateHandlers {
	return &StateHandlers{
		eventProcessor: eventProcessor,
		broadcaster:    broadcaster,
		logger:         logger,
		perfTracker:    perfTracker,
	}
}

// PostState handles POST /api/v1/state - processes widget state updates and belief events with SSE broadcasting
func (h *StateHandlers) PostState(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("post_state_request", tenantCtx.TenantID)
	defer marker.Complete()

	sessionID := c.GetHeader("X-TractStack-Session-ID")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID required"})
		return
	}

	// Read the story fragment ID from the header to provide context
	storyFragmentID := c.GetHeader("X-StoryFragment-ID")
	if storyFragmentID == "" {
		h.logger.System().Warn("Missing StoryFragment ID in state request", "sessionId", sessionID, "tenantId", tenantCtx.TenantID)
		storyFragmentID = "unknown"
	}

	var eventList []events.Event
	var paneID, gotoPaneID string

	// Handle bulk unset first, as it's a distinct form payload
	if unsetBeliefIds := c.PostForm("unsetBeliefIds"); unsetBeliefIds != "" {
		beliefIDs := strings.Split(unsetBeliefIds, ",")
		for _, beliefID := range beliefIDs {
			beliefID = strings.TrimSpace(beliefID)
			if beliefID != "" {
				eventList = append(eventList, events.Event{
					ID:     beliefID,
					Type:   "Belief",
					Verb:   "UNSET",
					Object: beliefID,
				})
			}
		}
		paneID = c.PostForm("paneId")
		gotoPaneID = c.PostForm("gotoPaneID")
	} else { // Handle single belief or action event
		var req struct {
			BeliefID     string `form:"beliefId"`
			BeliefType   string `form:"beliefType"`
			BeliefValue  string `form:"beliefValue"`
			BeliefObject string `form:"beliefObject"`
			Duration     int    `form:"duration"`
		}
		if err := c.ShouldBind(&req); err != nil {
			h.logger.System().Error("Invalid form data in state request", "error", err.Error(), "sessionId", sessionID, "tenantId", tenantCtx.TenantID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data"})
			return
		}

		if req.BeliefID == "" || req.BeliefType == "" {
			h.logger.System().Error("Missing required fields in state request", "beliefId", req.BeliefID, "beliefType", req.BeliefType, "sessionId", sessionID, "tenantId", tenantCtx.TenantID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "beliefId and beliefType are required"})
			return
		}
		eventList = append(eventList, convertRequestToEvent(&req))
		paneID = c.PostForm("paneId")
		gotoPaneID = c.PostForm("gotoPaneID")
	}

	h.logger.System().Debug("Processing state events",
		"sessionId", sessionID,
		"storyFragmentId", storyFragmentID,
		"eventCount", len(eventList),
		"paneId", paneID,
		"gotoPaneId", gotoPaneID,
		"tenantId", tenantCtx.TenantID)

	// Delegate all business logic to the application service
	if err := h.eventProcessor.ProcessEventsWithSSE(tenantCtx, sessionID, storyFragmentID, eventList, paneID, gotoPaneID, h.broadcaster); err != nil {
		h.logger.System().Error("State processing failed", "error", err, "sessionId", sessionID, "storyFragmentId", storyFragmentID, "tenantId", tenantCtx.TenantID)
		marker.SetError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Event processing failed"})
		return
	}

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostState request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{"status": "ok", "tenantId": tenantCtx.TenantID})
}

// convertRequestToEvent converts form data into a domain event.
func convertRequestToEvent(req *struct {
	BeliefID     string `form:"beliefId"`
	BeliefType   string `form:"beliefType"`
	BeliefValue  string `form:"beliefValue"`
	BeliefObject string `form:"beliefObject"`
	Duration     int    `form:"duration"`
},
) events.Event {
	// Handle Pane and StoryFragment Action Events (like READ, GLOSSED, PAGEVIEWED)
	if req.BeliefType == "Pane" || req.BeliefType == "StoryFragment" {
		return events.Event{
			ID:     req.BeliefID,
			Type:   req.BeliefType,
			Verb:   req.BeliefValue,
			Object: strconv.Itoa(req.Duration), // Pass duration as a string in the Object field
		}
	}

	// Handle Belief Events (existing logic)
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
