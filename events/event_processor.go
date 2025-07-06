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

// getSessionData retrieves session data for the current session
func (ep *EventProcessor) getSessionData() (*models.SessionData, error) {
	sessionData, exists := ep.cacheManager.GetSession(ep.tenantID, ep.sessionID)
	if !exists {
		return nil, fmt.Errorf("session not found: %s", ep.sessionID)
	}
	return sessionData, nil
}

// ProcessEvents processes an array of events, handling each type appropriately
func (ep *EventProcessor) ProcessEvents(events []models.Event, currentPaneID string) error {
	var changedBeliefs []string

	// NEW: Capture visibility snapshot before processing belief events
	var visibilitySnapshot map[string]map[string]bool
	if currentPaneID != "" {
		visibilitySnapshot = ep.captureVisibilitySnapshot(events)
		log.Printf("DEBUG: Captured visibility snapshot for pane %s: %v", currentPaneID, visibilitySnapshot)
	}

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
		ep.triggerSSE(changedBeliefs, visibilitySnapshot, currentPaneID)
	}

	return nil
}

// captureVisibilitySnapshot captures current pane visibility before belief processing
func (ep *EventProcessor) captureVisibilitySnapshot(events []models.Event) map[string]map[string]bool {
	snapshot := make(map[string]map[string]bool)

	// Extract belief slugs from events
	var changedBeliefs []string
	for _, event := range events {
		if event.Type == "Belief" {
			changedBeliefs = append(changedBeliefs, event.ID)
		}
	}

	log.Printf("DEBUG: Capturing snapshot for changed beliefs: %v", changedBeliefs)

	// Use the working BeliefBroadcastService to find affected storyfragments
	broadcastService := services.NewBeliefBroadcastService(ep.cacheManager, ep.sessionID)
	affectedStoryfragmentMap := broadcastService.FindAffectedStoryfragments(ep.tenantID, changedBeliefs)

	log.Printf("DEBUG: Affected storyfragments: %v", affectedStoryfragmentMap)

	// For each affected storyfragment, capture current pane visibility
	for storyfragmentID := range affectedStoryfragmentMap {
		log.Printf("DEBUG: Capturing visibility for storyfragment: %s", storyfragmentID)

		registry, exists := ep.cacheManager.GetStoryfragmentBeliefRegistry(ep.tenantID, storyfragmentID)
		if !exists {
			log.Printf("DEBUG: No registry found for storyfragment: %s", storyfragmentID)
			continue
		}

		sessionContext, exists := ep.cacheManager.GetSessionBeliefContext(ep.tenantID, ep.sessionID, storyfragmentID)
		var userBeliefs map[string][]string
		if exists {
			userBeliefs = sessionContext.UserBeliefs
		} else {
			userBeliefs = make(map[string][]string)
		}

		log.Printf("DEBUG: User beliefs for snapshot: %v", userBeliefs)

		snapshot[storyfragmentID] = make(map[string]bool)
		for paneID, paneBeliefs := range registry.PaneBeliefPayloads {
			beliefEngine := services.NewBeliefEvaluationEngine()
			visibilityResult := beliefEngine.EvaluatePaneVisibility(paneBeliefs, userBeliefs)
			snapshot[storyfragmentID][paneID] = (visibilityResult == "visible" || visibilityResult == "true")

			log.Printf("DEBUG: Pane %s visibility: %v", paneID, snapshot[storyfragmentID][paneID])
		}
	}

	return snapshot
}

// triggerSSE now includes visibility snapshot and current pane
func (ep *EventProcessor) triggerSSE(changedBeliefs []string, visibilitySnapshot map[string]map[string]bool, currentPaneID string) {
	log.Printf("SSE Trigger: beliefs changed for session %s: %v", ep.sessionID, changedBeliefs)

	broadcastService := services.NewBeliefBroadcastService(ep.cacheManager, ep.sessionID)
	broadcastService.BroadcastBeliefChange(ep.tenantID, ep.sessionID, changedBeliefs, visibilitySnapshot, currentPaneID)
}
