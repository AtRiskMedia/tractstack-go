package services

import (
	"fmt"
	"strings"

	domainEvents "github.com/AtRiskMedia/tractstack-go/internal/domain/events"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// StateService is now a clean orchestrator.
type StateService struct {
	eventProcessor *EventProcessingService
	logger         *logging.ChanneledLogger
	perfTracker    *performance.Tracker
}

// NewStateService creates a new, clean state service.
func NewStateService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *StateService {
	return &StateService{
		eventProcessor: NewEventProcessingService(),
		logger:         logger,
		perfTracker:    perfTracker,
	}
}

type StateRequest struct {
	BeliefID       string `form:"beliefId" json:"beliefId"`
	BeliefType     string `form:"beliefType" json:"beliefType"`
	BeliefValue    string `form:"beliefValue" json:"beliefValue"`
	BeliefObject   string `form:"beliefObject" json:"beliefObject"`
	PaneID         string `form:"paneId" json:"paneId"`
	SessionID      string `json:"sessionId"`
	UnsetBeliefIds string `form:"unsetBeliefIds" json:"unsetBeliefIds"`
	GotoPaneID     string `form:"gotoPaneID" json:"gotoPaneID"`
}

type StateResult struct {
	Status   string               `json:"status"`
	TenantID string               `json:"tenantId"`
	Event    *domainEvents.Event  `json:"event,omitempty"`
	Events   []domainEvents.Event `json:"events,omitempty"`
	Success  bool                 `json:"success"`
	Error    string               `json:"error,omitempty"`
}

// ValidateStateRequest validates a state request
func (s *StateService) ValidateStateRequest(req *StateRequest) error {
	if req.BeliefID == "" {
		return fmt.Errorf("beliefId is required")
	}
	if req.BeliefType == "" {
		return fmt.Errorf("beliefType is required")
	}
	return nil
}

// ValidateEvent validates an event
func (s *StateService) ValidateEvent(event domainEvents.Event) map[string]interface{} {
	return map[string]interface{}{
		"valid": true,
		"event": event,
	}
}

func (s *StateService) ProcessStateUpdate(req *StateRequest, tenantCtx *tenant.Context) *StateResult {
	if req.UnsetBeliefIds != "" {
		return s.processBulkUnset(req, tenantCtx)
	}

	event := s.convertToEvent(req)
	eventList := []domainEvents.Event{event}

	err := s.eventProcessor.ProcessEvents(tenantCtx, req.SessionID, eventList, req.PaneID, "")
	if err != nil {
		return &StateResult{Success: false, Error: err.Error()}
	}
	return &StateResult{Status: "ok", TenantID: tenantCtx.TenantID, Event: &event, Success: true}
}

func (s *StateService) processBulkUnset(req *StateRequest, tenantCtx *tenant.Context) *StateResult {
	beliefIDs := strings.Split(req.UnsetBeliefIds, ",")
	var eventList []domainEvents.Event
	for _, id := range beliefIDs {
		trimmedID := strings.TrimSpace(id)
		if trimmedID != "" {
			eventList = append(eventList, domainEvents.Event{ID: trimmedID, Type: "Belief", Verb: "UNSET", Object: trimmedID})
		}
	}

	err := s.eventProcessor.ProcessEvents(tenantCtx, req.SessionID, eventList, req.PaneID, req.GotoPaneID)
	if err != nil {
		return &StateResult{Success: false, Error: err.Error()}
	}
	return &StateResult{Status: "ok", TenantID: tenantCtx.TenantID, Events: eventList, Success: true}
}

func (s *StateService) convertToEvent(req *StateRequest) domainEvents.Event {
	if req.BeliefObject != "" {
		return domainEvents.Event{ID: req.BeliefID, Type: req.BeliefType, Verb: "IDENTIFY_AS", Object: req.BeliefObject}
	}
	if req.BeliefValue != "" && req.BeliefValue != "0" {
		return domainEvents.Event{ID: req.BeliefID, Type: req.BeliefType, Verb: req.BeliefValue, Object: req.BeliefID}
	}
	return domainEvents.Event{ID: req.BeliefID, Type: req.BeliefType, Verb: "UNSET", Object: req.BeliefID}
}
