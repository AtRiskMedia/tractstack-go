// Package events provides event processing functionality for TractStack
package events

import (
	"fmt"
	"log"
	"time"

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

// warmSessionBeliefContexts ensures session belief contexts are current with fingerprint state
// This mimics the pattern from storyfragment handler to ensure accurate belief state before processing
func (ep *EventProcessor) warmSessionBeliefContexts(events []models.Event) error {
	// Get current session data to find fingerprint ID
	sessionData, exists := ep.cacheManager.GetSession(ep.tenantID, ep.sessionID)
	if !exists {
		return fmt.Errorf("session not found: %s", ep.sessionID)
	}

	// Get current fingerprint state (the authoritative source of current beliefs)
	fingerprintState, fpExists := ep.cacheManager.GetFingerprintState(ep.tenantID, sessionData.FingerprintID)
	//if !fpExists {
	//	log.Printf("DEBUG: EventProcessor - no fingerprint state found for %s, using empty beliefs", sessionData.FingerprintID)
	//}

	var currentBeliefs map[string][]string
	if fpExists && fingerprintState.HeldBeliefs != nil {
		currentBeliefs = fingerprintState.HeldBeliefs
	} else {
		currentBeliefs = make(map[string][]string)
	}

	// Find all storyfragments that might be affected by these events
	// Extract belief slugs from events to determine scope
	var changedBeliefs []string
	for _, event := range events {
		if event.Type == "Belief" {
			changedBeliefs = append(changedBeliefs, event.ID)
		}
	}

	if len(changedBeliefs) == 0 {
		return nil
	}

	// Use the broadcast service to find affected storyfragments (same logic as snapshot)
	broadcastService := services.NewBeliefBroadcastService(ep.cacheManager, ep.sessionID)
	affectedStoryfragmentMap := broadcastService.FindAffectedStoryfragments(ep.tenantID, changedBeliefs)

	// Warm session belief context for each affected storyfragment
	for storyfragmentID := range affectedStoryfragmentMap {
		// Create fresh session belief context using current fingerprint beliefs
		// This mirrors the "SESSION BELIEF CONTEXT WARMING" from storyfragment_handlers.go
		sessionBeliefContext := &models.SessionBeliefContext{
			TenantID:        ep.tenantID,
			SessionID:       ep.sessionID,
			StoryfragmentID: storyfragmentID,
			UserBeliefs:     currentBeliefs, // Use current fingerprint state
			LastEvaluation:  time.Now().UTC(),
		}

		// Cache the refreshed session belief context
		ep.cacheManager.SetSessionBeliefContext(ep.tenantID, sessionBeliefContext)
	}

	return nil
}

// ProcessEvents processes an array of events, handling each type appropriately
func (ep *EventProcessor) ProcessEvents(events []models.Event, currentPaneID string, gotoPaneID string) error {
	var changedBeliefs []string
	var visibilitySnapshot map[string]map[string]bool
	if currentPaneID != "" {
		visibilitySnapshot = ep.captureVisibilitySnapshot(events)
	}

	// This ensures session contexts are in sync with fingerprint state for processing
	err := ep.warmSessionBeliefContexts(events)
	if err != nil {
		log.Printf("DEBUG: EventProcessor - context warming failed: %v, continuing with processing", err)
	}

	// Process each event based on type
	for _, event := range events {
		switch event.Type {
		case "Belief":
			changed, err := ep.beliefProcessor.ProcessBelief(event)
			if err != nil {
				log.Printf("ERROR: EventProcessor - error processing belief event %+v: %v", event, err)
				continue // Don't fail entire array
			}
			if changed {
				changedBeliefs = append(changedBeliefs, event.ID)
				//} else {
				//	log.Printf("DEBUG: EventProcessor - belief %s did not change (may not have existed)", event.ID)
			}

		case "Pane", "StoryFragment":
			err := ep.analyticsProcessor.ProcessAnalyticsEvent(event)
			if err != nil {
				log.Printf("ERROR: EventProcessor - error processing analytics event %+v: %v", event, err)
				continue // Don't fail entire array
			}

			// Check if this is a PAGEVIEWED event that needs belief state sync
			if event.Type == "StoryFragment" && event.Verb == "PAGEVIEWED" {
				storyfragmentID := event.ID

				// Check if user has existing beliefs for this storyfragment
				if sessionContext, exists := ep.cacheManager.GetSessionBeliefContext(ep.tenantID, ep.sessionID, storyfragmentID); exists {
					if len(sessionContext.UserBeliefs) > 0 {
						// Get belief registry
						if beliefRegistry, registryExists := ep.cacheManager.GetStoryfragmentBeliefRegistry(ep.tenantID, storyfragmentID); registryExists {

							// Create "before" snapshot (visibility with no beliefs = default state)
							emptyBeliefs := make(map[string][]string)

							beliefEngine := services.NewBeliefEvaluationEngine()
							var affectedPanes []string

							for paneID, paneBeliefs := range beliefRegistry.PaneBeliefPayloads {
								// Evaluate visibility without user beliefs (default state)
								beforeVisibility := beliefEngine.EvaluatePaneVisibility(paneBeliefs, emptyBeliefs)
								beforeVisible := (beforeVisibility == "visible")

								// Evaluate visibility with current user beliefs
								afterVisibility := beliefEngine.EvaluatePaneVisibility(paneBeliefs, sessionContext.UserBeliefs)
								afterVisible := (afterVisibility == "visible")

								// Only include panes whose visibility changed
								if beforeVisible != afterVisible {
									affectedPanes = append(affectedPanes, paneID)
								}
							}

							if len(affectedPanes) > 0 {
								// Trigger SSE broadcast to sync UI with existing belief state
								// Use the Broadcaster directly for session-specific broadcast
								models.Broadcaster.BroadcastToSpecificSession(ep.tenantID, ep.sessionID, storyfragmentID, affectedPanes, nil)
								//} else {
								//	log.Printf("DEBUG: EventProcessor - StoryFragment PAGEVIEWED sync found no affected panes")
							}
						} else {
							log.Printf("DEBUG: EventProcessor - no belief registry found for storyfragment %s", storyfragmentID)
						}
					} else {
						log.Printf("DEBUG: EventProcessor - session context exists but has no beliefs")
					}
				} else {
					log.Printf("DEBUG: EventProcessor - no session context found for storyfragment %s", storyfragmentID)
				}
			}

		default:
			log.Printf("WARNING: EventProcessor - unknown event type: %s for event: %+v", event.Type, event)
		}
	}

	// Trigger SSE notifications after all events processed
	if len(changedBeliefs) > 0 {
		ep.triggerSSE(changedBeliefs, visibilitySnapshot, currentPaneID, gotoPaneID)
		//} else {
		//	log.Printf("DEBUG: EventProcessor - no beliefs changed, skipping SSE trigger")
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

	// Use the working BeliefBroadcastService to find affected storyfragments
	broadcastService := services.NewBeliefBroadcastService(ep.cacheManager, ep.sessionID)
	affectedStoryfragmentMap := broadcastService.FindAffectedStoryfragments(ep.tenantID, changedBeliefs)

	// For each affected storyfragment, capture current pane visibility using CURRENT fingerprint state
	for storyfragmentID := range affectedStoryfragmentMap {
		registry, exists := ep.cacheManager.GetStoryfragmentBeliefRegistry(ep.tenantID, storyfragmentID)
		if !exists {
			continue
		}

		var userBeliefs map[string][]string

		sessionData, sessionExists := ep.cacheManager.GetSession(ep.tenantID, ep.sessionID)
		if sessionExists {
			fingerprintState, fpExists := ep.cacheManager.GetFingerprintState(ep.tenantID, sessionData.FingerprintID)
			if fpExists && fingerprintState.HeldBeliefs != nil {
				userBeliefs = fingerprintState.HeldBeliefs
			} else {
				userBeliefs = make(map[string][]string)
			}
		} else {
			userBeliefs = make(map[string][]string)
		}

		snapshot[storyfragmentID] = make(map[string]bool)
		for paneID, paneBeliefs := range registry.PaneBeliefPayloads {
			beliefEngine := services.NewBeliefEvaluationEngine()
			visibilityResult := beliefEngine.EvaluatePaneVisibility(paneBeliefs, userBeliefs)
			isVisible := (visibilityResult == "visible" || visibilityResult == "true")
			snapshot[storyfragmentID][paneID] = isVisible
		}
	}

	return snapshot
}

// triggerSSE now includes visibility snapshot and current pane
func (ep *EventProcessor) triggerSSE(changedBeliefs []string, visibilitySnapshot map[string]map[string]bool, currentPaneID string, gotoPaneID string) {
	broadcastService := services.NewBeliefBroadcastService(ep.cacheManager, ep.sessionID)
	broadcastService.BroadcastBeliefChange(ep.tenantID, ep.sessionID, changedBeliefs, visibilitySnapshot, currentPaneID, gotoPaneID)
}
