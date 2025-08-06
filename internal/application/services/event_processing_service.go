// Package services provides event processing orchestration
package services

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	domainEvents "github.com/AtRiskMedia/tractstack-go/internal/domain/events"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/messaging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/oklog/ulid/v2"
)

// EventProcessingService contains the refactored business logic for handling events.
type EventProcessingService struct {
	beliefBroadcaster *BeliefBroadcastService
	beliefEvaluator   *BeliefEvaluationService
}

// NewEventProcessingService creates a new event processing service with its dependencies.
func NewEventProcessingService(broadcaster *BeliefBroadcastService, evaluator *BeliefEvaluationService) *EventProcessingService {
	return &EventProcessingService{
		beliefBroadcaster: broadcaster,
		beliefEvaluator:   evaluator,
	}
}

// ProcessEventsWithSSE is the main entry point for processing events with SSE broadcasting.
func (s *EventProcessingService) ProcessEventsWithSSE(
	tenantCtx *tenant.Context,
	sessionID string,
	events []domainEvents.Event,
	currentPaneID string,
	gotoPaneID string,
	broadcaster messaging.Broadcaster,
) error {
	var changedBeliefs []string
	visibilitySnapshot := s.captureVisibilitySnapshot(tenantCtx, sessionID, events)

	for _, event := range events {
		if event.Type == "Belief" {
			changed, err := s.processBelief(tenantCtx, sessionID, event)
			if err != nil {
				log.Printf("ERROR: EventProcessingService - error processing belief event %+v: %v", event, err)
				continue
			}
			if changed {
				changedBeliefs = append(changedBeliefs, event.ID)
			}
		} else if event.Type == "StoryFragment" && event.Verb == "PAGEVIEWED" {
			log.Printf("********************* PAGEVIEWED *********")
			storyfragmentID := event.ID

			// Check if user has existing beliefs for this storyfragment
			if sessionContext, exists := tenantCtx.CacheManager.GetSessionBeliefContext(tenantCtx.TenantID, sessionID, storyfragmentID); exists {
				log.Printf("********************* PAGEVIEWED: with context *********")
				if len(sessionContext.UserBeliefs) > 0 {
					log.Printf("********************* PAGEVIEWED: with context and beliefs *********")
					// Get belief registry
					if beliefRegistry, registryExists := tenantCtx.CacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID); registryExists {

						log.Printf("********************* PAGEVIEWED: with context and beliefs COMPUTING... *********")
						// Create "before" snapshot (visibility with no beliefs = default state)
						emptyBeliefs := make(map[string][]string)
						var affectedPanes []string

						for paneID, paneBeliefs := range beliefRegistry.PaneBeliefPayloads {
							// Evaluate visibility without user beliefs (default state)
							beforeVisibility := s.beliefEvaluator.EvaluatePaneVisibility(paneBeliefs, emptyBeliefs)
							beforeVisible := (beforeVisibility == "visible")

							// Evaluate visibility with current user beliefs
							afterVisibility := s.beliefEvaluator.EvaluatePaneVisibility(paneBeliefs, sessionContext.UserBeliefs)
							afterVisible := (afterVisibility == "visible")

							// Only include panes whose visibility changed
							if beforeVisible != afterVisible {
								affectedPanes = append(affectedPanes, paneID)
							}
						}

						if len(affectedPanes) > 0 {
							log.Printf("********************* PAGEVIEWED: with context and beliefs COMPUTING... WE GOT CONTACT !!!! *********")
							// Trigger SSE broadcast to sync UI with existing belief state
							broadcaster.BroadcastToSpecificSession(tenantCtx.TenantID, sessionID, storyfragmentID, affectedPanes, nil)
						} else {
							log.Printf("********************* PAGEVIEWED: with context and beliefs COMPUTING... ignoring *********")
						}

					}
				}
			}
		}
	}

	if len(changedBeliefs) > 0 {
		s.beliefBroadcaster.BroadcastBeliefChange(tenantCtx.TenantID, sessionID, changedBeliefs, visibilitySnapshot, currentPaneID, gotoPaneID, broadcaster)
	}

	return nil
}

func (s *EventProcessingService) captureVisibilitySnapshot(tenantCtx *tenant.Context, sessionID string, events []domainEvents.Event) map[string]map[string]bool {
	snapshot := make(map[string]map[string]bool)
	cacheManager := tenantCtx.CacheManager

	var changedBeliefs []string
	for _, event := range events {
		if event.Type == "Belief" {
			changedBeliefs = append(changedBeliefs, event.ID)
		}
	}

	affectedStoryfragmentMap := s.beliefBroadcaster.FindAffectedStoryfragments(tenantCtx.TenantID, changedBeliefs)

	for storyfragmentID := range affectedStoryfragmentMap {
		registry, exists := cacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID)
		if !exists {
			continue
		}

		sessionData, sessionExists := cacheManager.GetSession(tenantCtx.TenantID, sessionID)
		var userBeliefs map[string][]string
		if sessionExists {
			if fpState, fpExists := cacheManager.GetFingerprintState(tenantCtx.TenantID, sessionData.FingerprintID); fpExists {
				userBeliefs = fpState.HeldBeliefs
			}
		}
		if userBeliefs == nil {
			userBeliefs = make(map[string][]string)
		}

		snapshot[storyfragmentID] = make(map[string]bool)
		for paneID, paneBeliefs := range registry.PaneBeliefPayloads {
			visibilityResult := s.beliefEvaluator.EvaluatePaneVisibility(paneBeliefs, userBeliefs)
			snapshot[storyfragmentID][paneID] = (visibilityResult == "visible")
		}
	}

	return snapshot
}

func (s *EventProcessingService) processBelief(tenantCtx *tenant.Context, sessionID string, event domainEvents.Event) (bool, error) {
	cacheManager := tenantCtx.CacheManager
	beliefID, exists := cacheManager.GetContentBySlug(tenantCtx.TenantID, "belief:"+event.ID)
	if !exists {
		var foundID string
		err := tenantCtx.Database.Conn.QueryRow("SELECT id FROM beliefs WHERE slug = ?", event.ID).Scan(&foundID)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("WARNING: EventProcessingService - belief not found: %s", event.ID)
				return false, nil
			}
			return false, fmt.Errorf("failed to query belief by slug: %w", err)
		}
		beliefID = foundID
	}

	sessionData, exists := cacheManager.GetSession(tenantCtx.TenantID, sessionID)
	if !exists {
		return false, fmt.Errorf("session not found: %s", sessionID)
	}

	fingerprintState, exists := cacheManager.GetFingerprintState(tenantCtx.TenantID, sessionData.FingerprintID)
	if !exists {
		return false, fmt.Errorf("fingerprint state not found: %s", sessionData.FingerprintID)
	}

	if fingerprintState.HeldBeliefs == nil {
		fingerprintState.HeldBeliefs = make(map[string][]string)
	}

	changed := false
	switch event.Verb {
	case "UNSET":
		if _, exists := fingerprintState.HeldBeliefs[beliefID]; exists {
			delete(fingerprintState.HeldBeliefs, beliefID)
			changed = true
		}
	case "IDENTIFY_AS":
		if event.Object != "" {
			currentValues := fingerprintState.HeldBeliefs[beliefID]
			found := false
			for _, v := range currentValues {
				if v == event.Object {
					found = true
					break
				}
			}
			if !found {
				fingerprintState.HeldBeliefs[beliefID] = append(currentValues, event.Object)
				changed = true
			}
		}
	default:
		currentValues := fingerprintState.HeldBeliefs[beliefID]
		found := false
		for _, v := range currentValues {
			if v == event.Verb {
				found = true
				break
			}
		}
		if !found {
			fingerprintState.HeldBeliefs[beliefID] = append(currentValues, event.Verb)
			changed = true
		}
	}

	if changed {
		cacheManager.SetFingerprintState(tenantCtx.TenantID, fingerprintState)

		actionID := ulid.MustNew(ulid.Timestamp(time.Now()), ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0))
		query := `INSERT INTO actions (id, object_id, object_type, verb, visit_id, fingerprint_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err := tenantCtx.Database.Conn.Exec(query, actionID.String(), beliefID, event.Type, event.Verb, sessionData.VisitID, sessionData.FingerprintID, time.Now().UTC())
		if err != nil {
			log.Printf("ERROR: EventProcessingService - failed to insert action: %v", err)
		}
	}

	return changed, nil
}
