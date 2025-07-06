// Package events provides belief-specific event processing
package events

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// BeliefProcessor handles belief-specific events and state changes
type BeliefProcessor struct {
	tenantID     string
	sessionID    string
	ctx          *tenant.Context
	cacheManager *cache.Manager
}

// NewBeliefProcessor creates a new belief processor
func NewBeliefProcessor(tenantID, sessionID string, ctx *tenant.Context, cacheManager *cache.Manager) *BeliefProcessor {
	return &BeliefProcessor{
		tenantID:     tenantID,
		sessionID:    sessionID,
		ctx:          ctx,
		cacheManager: cacheManager,
	}
}

// ProcessBelief processes a belief event, updating cache and database
func (bp *BeliefProcessor) ProcessBelief(event models.Event) (bool, error) {
	// 1. Resolve belief slug → database ID using cache-first lookup
	beliefID, exists := bp.cacheManager.GetBeliefIDBySlug(bp.tenantID, event.ID)
	if !exists {
		log.Printf("Belief slug not found in cache: %s", event.ID)
		return false, fmt.Errorf("belief not found: %s", event.ID)
	}

	// 2. Get session data to find fingerprint and visit
	sessionData, exists := bp.cacheManager.GetSession(bp.tenantID, bp.sessionID)
	if !exists {
		return false, fmt.Errorf("session not found: %s", bp.sessionID)
	}

	// 3. Handle UNSET with LINKED-BELIEFS cascade
	if event.Verb == "UNSET" {
		return bp.processUnsetBelief(event.ID, sessionData.FingerprintID)
	}

	// 4. Update cache first (drives UX)
	changed := bp.updateFingerprintCache(event, sessionData.FingerprintID)

	// 5. Queue database persistence (eventually consistent)
	go bp.persistBelief(beliefID, sessionData.FingerprintID, sessionData.VisitID, event)

	return changed, nil
}

// processUnsetBelief handles UNSET events with LINKED-BELIEFS cascade logic
func (bp *BeliefProcessor) processUnsetBelief(beliefSlug, fingerprintID string) (bool, error) {
	// Get current fingerprint state
	fpState, exists := bp.cacheManager.GetFingerprintState(bp.tenantID, fingerprintID)
	if !exists {
		return false, nil // No state to unset
	}

	// Find all beliefs that have this belief in their LINKED-BELIEFS
	var beliefsToUnset []string

	// Check if any beliefs have LINKED-BELIEFS containing our target
	for slug := range fpState.HeldBeliefs {
		if slug == "LINKED-BELIEFS" {
			continue
		}
		// For each belief, check if LINKED-BELIEFS contains our target belief
		if linkedBeliefs, exists := fpState.HeldBeliefs["LINKED-BELIEFS"]; exists {
			for _, linked := range linkedBeliefs {
				if linked == beliefSlug {
					beliefsToUnset = append(beliefsToUnset, slug)
					break
				}
			}
		}
	}

	// Remove the target belief and all linked parents
	beliefsToUnset = append(beliefsToUnset, beliefSlug)
	changed := false

	for _, slug := range beliefsToUnset {
		if _, exists := fpState.HeldBeliefs[slug]; exists {
			delete(fpState.HeldBeliefs, slug)
			changed = true

			// Queue database deletion
			go bp.deleteBelief(slug, fingerprintID)
		}
	}

	if changed {
		fpState.UpdateActivity()
		bp.cacheManager.SetFingerprintState(bp.tenantID, fpState)
	}

	return changed, nil
}

// updateFingerprintCache updates belief state in cache
func (bp *BeliefProcessor) updateFingerprintCache(event models.Event, fingerprintID string) bool {
	fpState, exists := bp.cacheManager.GetFingerprintState(bp.tenantID, fingerprintID)
	if !exists {
		fpState = &models.FingerprintState{
			FingerprintID: fingerprintID,
			HeldBeliefs:   make(map[string][]string),
			HeldBadges:    make(map[string]string),
			LastActivity:  time.Now(),
		}
	}

	beliefSlug := event.ID
	previousValues := fpState.HeldBeliefs[beliefSlug]

	// Update belief value based on event type
	switch event.Verb {
	case "IDENTIFY_AS":
		fpState.HeldBeliefs[beliefSlug] = []string{event.Object}
	default:
		// Scale or toggle events
		fpState.HeldBeliefs[beliefSlug] = []string{event.Verb}
	}

	// Check if values actually changed
	changed := !slicesEqual(previousValues, fpState.HeldBeliefs[beliefSlug])

	if changed {
		fpState.UpdateActivity()
		bp.cacheManager.SetFingerprintState(bp.tenantID, fpState)
	}

	return changed
}

// persistBelief persists belief data to database (actions and heldbeliefs tables)
func (bp *BeliefProcessor) persistBelief(beliefID, fingerprintID, visitID string, event models.Event) error {
	// Record action for all belief events (including UNSET for analytics)
	actionQuery := `INSERT INTO actions 
		(id, object_id, object_type, visit_id, fingerprint_id, verb, created_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := bp.ctx.Database.Conn.Exec(actionQuery,
		utils.GenerateULID(), beliefID, event.Type, visitID, fingerprintID, event.Verb, time.Now())
	if err != nil {
		return fmt.Errorf("failed to insert action: %w", err)
	}

	// Skip heldbeliefs update for UNSET (handled by deleteBelief)
	if event.Verb == "UNSET" {
		return nil
	}

	// Update or insert belief state
	checkQuery := `SELECT verb FROM heldbeliefs WHERE belief_id = ? AND fingerprint_id = ?`
	var existingVerb string
	err = bp.ctx.Database.Conn.QueryRow(checkQuery, beliefID, fingerprintID).Scan(&existingVerb)

	if err == sql.ErrNoRows {
		// Insert new belief
		insertQuery := `INSERT INTO heldbeliefs (id, belief_id, fingerprint_id, verb, object, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`
		_, err = bp.ctx.Database.Conn.Exec(insertQuery,
			utils.GenerateULID(), beliefID, fingerprintID, event.Verb, event.Object, time.Now())
	} else if err == nil {
		// Update existing belief
		updateQuery := `UPDATE heldbeliefs 
			SET verb = ?, object = ?, updated_at = ? 
			WHERE belief_id = ? AND fingerprint_id = ?`
		_, err = bp.ctx.Database.Conn.Exec(updateQuery,
			event.Verb, event.Object, time.Now(), beliefID, fingerprintID)
	}

	return err
}

// deleteBelief removes belief from database (for UNSET events)
func (bp *BeliefProcessor) deleteBelief(beliefSlug, fingerprintID string) error {
	// Get belief ID for database operation
	beliefID, exists := bp.cacheManager.GetBeliefIDBySlug(bp.tenantID, beliefSlug)
	if !exists {
		return fmt.Errorf("belief not found for deletion: %s", beliefSlug)
	}

	// Delete from heldbeliefs (no actions record for UNSET)
	deleteQuery := `DELETE FROM heldbeliefs WHERE belief_id = ? AND fingerprint_id = ?`
	_, err := bp.ctx.Database.Conn.Exec(deleteQuery, beliefID, fingerprintID)
	return err
}

// slicesEqual compares two string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
