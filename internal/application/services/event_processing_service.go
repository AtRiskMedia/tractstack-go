// Package services provides event processing orchestration
package services

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	domainEvents "github.com/AtRiskMedia/tractstack-go/internal/domain/events"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/messaging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/oklog/ulid/v2"
)

// EventProcessingService contains the refactored business logic for handling events.
type EventProcessingService struct {
	beliefBroadcaster *BeliefBroadcastService
	beliefEvaluator   *BeliefEvaluationService
	logger            *logging.ChanneledLogger
}

// NewEventProcessingService creates a new event processing service with its dependencies.
func NewEventProcessingService(broadcaster *BeliefBroadcastService, evaluator *BeliefEvaluationService, logger *logging.ChanneledLogger) *EventProcessingService {
	return &EventProcessingService{
		beliefBroadcaster: broadcaster,
		beliefEvaluator:   evaluator,
		logger:            logger,
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
				s.logger.Content().Error("Error processing belief event",
					"error", err.Error(),
					"tenantId", tenantCtx.TenantID,
					"sessionId", sessionID,
					"event", event,
				)
				continue
			}
			if changed {
				changedBeliefs = append(changedBeliefs, event.ID)
			}
		} else if event.Type == "StoryFragment" && event.Verb == "PAGEVIEWED" {
			storyfragmentID := event.ID

			var currentAuthoritativeUserBeliefs map[string][]string
			if sessionData, sessionExists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, sessionID); sessionExists {
				if fpState, fpExists := tenantCtx.CacheManager.GetFingerprintState(tenantCtx.TenantID, sessionData.FingerprintID); fpExists {
					currentAuthoritativeUserBeliefs = fpState.HeldBeliefs
				}
			}

			if len(currentAuthoritativeUserBeliefs) > 0 {
				s.logger.Content().Debug("PAGEVIEWED: User has beliefs, proceeding to check visibility.", "storyfragmentId", storyfragmentID)
				if beliefRegistry, registryExists := tenantCtx.CacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID); registryExists {

					emptyBeliefs := make(map[string][]string)
					var affectedPanes []string

					for paneID, paneBeliefs := range beliefRegistry.PaneBeliefPayloads {
						beforeVisibility := s.beliefEvaluator.EvaluatePaneVisibility(paneBeliefs, emptyBeliefs)
						beforeVisible := (beforeVisibility == "visible")

						afterVisibility := s.beliefEvaluator.EvaluatePaneVisibility(paneBeliefs, currentAuthoritativeUserBeliefs)
						afterVisible := (afterVisibility == "visible")

						s.logger.Content().Debug("🔍 PAGEVIEWED Visibility Check",
							"paneId", paneID,
							"storyfragmentId", storyfragmentID,
							"paneHasHeldRequirements", len(paneBeliefs.HeldBeliefs) > 0,
							"paneHeldRequirements", paneBeliefs.HeldBeliefs,
							"userBeliefs", currentAuthoritativeUserBeliefs,
							"beforeVisibility", beforeVisibility,
							"afterVisibility", afterVisibility,
							"visibilityDidChange", beforeVisible != afterVisible,
						)

						if beforeVisible != afterVisible {
							affectedPanes = append(affectedPanes, paneID)
						}
					}

					if len(affectedPanes) > 0 {
						s.logger.Content().Debug("PAGEVIEWED: Visibility changes detected, broadcasting SSE.", "affectedPanes", affectedPanes)
						broadcaster.BroadcastToSpecificSession(tenantCtx.TenantID, sessionID, storyfragmentID, affectedPanes, nil)
					} else {
						s.logger.Content().Debug("PAGEVIEWED: No visibility changes detected, ignoring.", "storyfragmentId", storyfragmentID)
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
	beliefSlug := event.ID
	beliefID, exists := cacheManager.GetContentBySlug(tenantCtx.TenantID, "belief:"+beliefSlug)
	if !exists {
		var foundID string
		err := tenantCtx.Database.Conn.QueryRow("SELECT id FROM beliefs WHERE slug = ?", beliefSlug).Scan(&foundID)
		if err != nil {
			if err == sql.ErrNoRows {
				s.logger.Content().Warn("Belief slug not found in database", "slug", beliefSlug, "tenantId", tenantCtx.TenantID)
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
		if _, exists := fingerprintState.HeldBeliefs[beliefSlug]; exists {
			delete(fingerprintState.HeldBeliefs, beliefSlug)
			changed = true
		}
	case "IDENTIFY_AS":
		if event.Object != "" {
			currentValues := fingerprintState.HeldBeliefs[beliefSlug]
			found := false
			for _, v := range currentValues {
				if v == event.Object {
					found = true
					break
				}
			}
			if !found {
				fingerprintState.HeldBeliefs[beliefSlug] = append(currentValues, event.Object)
				changed = true
			}
		}
	default:
		currentValues := fingerprintState.HeldBeliefs[beliefSlug]
		found := false
		for _, v := range currentValues {
			if v == event.Verb {
				found = true
				break
			}
		}
		if !found {
			fingerprintState.HeldBeliefs[beliefSlug] = append(currentValues, event.Verb)
			changed = true
		}
	}

	if changed {
		cacheManager.SetFingerprintState(tenantCtx.TenantID, fingerprintState)

		actionID := ulid.MustNew(ulid.Timestamp(time.Now()), ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0))
		query := `INSERT INTO actions (id, object_id, object_type, verb, visit_id, fingerprint_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err := tenantCtx.Database.Conn.Exec(query, actionID.String(), beliefID, event.Type, event.Verb, sessionData.VisitID, sessionData.FingerprintID, time.Now().UTC())
		if err != nil {
			s.logger.Database().Error("Failed to insert analytics action for belief change",
				"error", err.Error(),
				"tenantId", tenantCtx.TenantID,
				"beliefId", beliefID,
				"verb", event.Verb,
			)
		}
	}

	return changed, nil
}
