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
			return false, fmt.Errorf("belief not found by slug %s: %w", event.ID, err)
		}
		beliefID = foundID
	}
	sessionData, exists := cacheManager.GetSession(tenantCtx.TenantID, sessionID)
	if !exists {
		return false, fmt.Errorf("session not found: %s", sessionID)
	}
	go s.persistBelief(tenantCtx, beliefID, sessionData.FingerprintID, sessionData.VisitID, event)
	return s.updateFingerprintCache(tenantCtx, event, sessionData.FingerprintID), nil
}

func (s *EventProcessingService) updateFingerprintCache(tenantCtx *tenant.Context, event domainEvents.Event, fingerprintID string) bool {
	cacheManager := tenantCtx.CacheManager
	fpState, exists := cacheManager.GetFingerprintState(tenantCtx.TenantID, fingerprintID)
	if !exists {
		return false
	}
	beliefSlug := event.ID
	previousValues := fpState.HeldBeliefs[beliefSlug]
	var newValues []string
	switch event.Verb {
	case "UNSET":
		delete(fpState.HeldBeliefs, beliefSlug)
	case "IDENTIFY_AS":
		newValues = []string{event.Object}
		fpState.HeldBeliefs[beliefSlug] = newValues
	default:
		newValues = []string{event.Verb}
		fpState.HeldBeliefs[beliefSlug] = newValues
	}
	changed := !slicesEqual(previousValues, newValues)
	if changed {
		fpState.LastActivity = time.Now().UTC()
		cacheManager.SetFingerprintState(tenantCtx.TenantID, fpState)
	}
	return changed
}

func (s *EventProcessingService) persistBelief(tenantCtx *tenant.Context, beliefID, fingerprintID, visitID string, event domainEvents.Event) {
	actionQuery := `INSERT INTO actions (id, object_id, object_type, visit_id, fingerprint_id, verb, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := tenantCtx.Database.Conn.Exec(actionQuery, generateULID(), beliefID, event.Type, visitID, fingerprintID, event.Verb, time.Now().UTC())
	if err != nil {
		log.Printf("ERROR: Failed to insert action: %v", err)
	}
	if event.Verb == "UNSET" {
		deleteQuery := `DELETE FROM heldbeliefs WHERE belief_id = ? AND fingerprint_id = ?`
		_, err = tenantCtx.Database.Conn.Exec(deleteQuery, beliefID, fingerprintID)
		if err != nil {
			log.Printf("ERROR: Failed to delete held belief: %v", err)
		}
		return
	}
	var existingID string
	err = tenantCtx.Database.Conn.QueryRow(`SELECT id FROM heldbeliefs WHERE belief_id = ? AND fingerprint_id = ?`, beliefID, fingerprintID).Scan(&existingID)
	switch {
	case err == sql.ErrNoRows:
		_, err = tenantCtx.Database.Conn.Exec(`INSERT INTO heldbeliefs (id, belief_id, fingerprint_id, verb, object, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, generateULID(), beliefID, fingerprintID, event.Verb, event.Object, time.Now().UTC())
	case err == nil:
		_, err = tenantCtx.Database.Conn.Exec(`UPDATE heldbeliefs SET verb = ?, object = ?, updated_at = ? WHERE id = ?`, event.Verb, event.Object, time.Now().UTC(), existingID)
	}
	if err != nil {
		log.Printf("ERROR: Failed to upsert held belief: %v", err)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func generateULID() string {
	entropy := ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
