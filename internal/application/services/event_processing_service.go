package services

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	domainEvents "github.com/AtRiskMedia/tractstack-go/internal/domain/events"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/oklog/ulid/v2"
)

// generateULID is a helper function that now lives within the service package.
func generateULID() string {
	entropy := ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// EventProcessingService contains the refactored business logic for handling events.
type EventProcessingService struct{}

// NewEventProcessingService creates a new event processing service.
func NewEventProcessingService() *EventProcessingService {
	return &EventProcessingService{}
}

// ProcessEvents is the main entry point for processing a list of events.
func (s *EventProcessingService) ProcessEvents(
	tenantCtx *tenant.Context,
	sessionID string,
	events []domainEvents.Event,
	currentPaneID string,
	gotoPaneID string,
) error {
	for _, event := range events {
		if event.Type == "Belief" {
			_, err := s.processBelief(tenantCtx, sessionID, event)
			if err != nil {
				log.Printf("ERROR: EventProcessingService - error processing belief event %+v: %v", event, err)
				continue // Continue processing other events
			}
		} else {
			log.Printf("WARNING: EventProcessingService - unknown event type: %s", event.Type)
		}
	}
	return nil
}

// processBelief contains the refactored logic from your legacy BeliefProcessor.
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

	return s.updateFingerprintCache(cacheManager, tenantCtx.TenantID, event, sessionData.FingerprintID), nil
}

func (s *EventProcessingService) updateFingerprintCache(cacheManager *manager.Manager, tenantID string, event domainEvents.Event, fingerprintID string) bool {
	fpState, exists := cacheManager.GetFingerprintState(tenantID, fingerprintID)
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
		cacheManager.SetFingerprintState(tenantID, fpState)
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
	switch err {
	case sql.ErrNoRows:
		_, err = tenantCtx.Database.Conn.Exec(`INSERT INTO heldbeliefs (id, belief_id, fingerprint_id, verb, object, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			generateULID(), beliefID, fingerprintID, event.Verb, event.Object, time.Now().UTC())
	case nil:
		_, err = tenantCtx.Database.Conn.Exec(`UPDATE heldbeliefs SET verb = ?, object = ?, updated_at = ? WHERE id = ?`,
			event.Verb, event.Object, time.Now().UTC(), existingID)
	}
	if err != nil && err != sql.ErrNoRows {
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
