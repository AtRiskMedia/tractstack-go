// Package events provides event processing functionality for TractStack
package events

import (
	"fmt"
	"log"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/services"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// EventProcessor coordinates event processing across different event types
type EventProcessor struct {
	tenantID           string
	sessionID          string
	ctx                *tenant.Context
	cacheManager       *cache.Manager
	beliefProcessor    *BeliefProcessor
	analyticsProcessor *AnalyticsProcessor
}

// NewEventProcessor creates a new event processor
func NewEventProcessor(tenantID, sessionID string, ctx *tenant.Context) *EventProcessor {
	cacheManager := cache.GetGlobalManager()

	return &EventProcessor{
		tenantID:           tenantID,
		sessionID:          sessionID,
		ctx:                ctx,
		cacheManager:       cacheManager,
		beliefProcessor:    NewBeliefProcessor(tenantID, sessionID, ctx, cacheManager),
		analyticsProcessor: NewAnalyticsProcessor(tenantID, sessionID, ctx, cacheManager),
	}
}

// ProcessEvents processes an array of events, handling each type appropriately
func (ep *EventProcessor) ProcessEvents(events []models.Event) error {
	var changedBeliefs []string

	// Process each event based on type
	for _, event := range events {
		switch event.Type {
		case "Belief":
			changed, err := ep.beliefProcessor.ProcessBelief(event)
			if err != nil {
				log.Printf("Error processing belief event: %v", err)
				continue // Don't fail entire array
			}
			if changed {
				changedBeliefs = append(changedBeliefs, event.ID)
			}

		case "Pane", "StoryFragment":
			err := ep.analyticsProcessor.ProcessAnalyticsEvent(event)
			if err != nil {
				log.Printf("Error processing analytics event: %v", err)
				continue // Don't fail entire array
			}

		default:
			log.Printf("Unknown event type: %s", event.Type)
		}
	}

	// Trigger SSE notifications after all events processed
	if len(changedBeliefs) > 0 {
		ep.triggerSSE(changedBeliefs)
	}

	return nil
}

// getSessionData retrieves session data for the current session
func (ep *EventProcessor) getSessionData() (*models.SessionData, error) {
	sessionData, exists := ep.cacheManager.GetSession(ep.tenantID, ep.sessionID)
	if !exists {
		return nil, fmt.Errorf("session not found: %s", ep.sessionID)
	}
	return sessionData, nil
}

// triggerSSE triggers real-time UI updates via Server-Sent Events
func (ep *EventProcessor) triggerSSE(changedBeliefs []string) {
	log.Printf("SSE Trigger: beliefs changed for session %s: %v", ep.sessionID, changedBeliefs)

	broadcastService := services.NewBeliefBroadcastService(ep.cacheManager)
	broadcastService.BroadcastBeliefChange(ep.tenantID, ep.sessionID, changedBeliefs)
}
