// Package services provides belief broadcasting service for Stage 3 SSE integration
package services

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
)

// BeliefBroadcastService handles tenant-scoped targeted broadcasting when beliefs change
type BeliefBroadcastService struct {
	cacheManager *cache.Manager
	sessionID    string
}

// NewBeliefBroadcastService creates a new belief broadcast service
func NewBeliefBroadcastService(cacheManager *cache.Manager, sessionID string) *BeliefBroadcastService {
	return &BeliefBroadcastService{cacheManager: cacheManager, sessionID: sessionID}
}

// BroadcastBeliefChange identifies affected storyfragments and broadcasts updates within tenant
func (bbs *BeliefBroadcastService) BroadcastBeliefChange(tenantID, sessionID string, changedBeliefs []string, visibilitySnapshot map[string]map[string]bool, currentPaneID string, gotoPaneID string) {
	// log.Printf("DEBUG: BroadcastBeliefChange received gotoPaneID: '%s'", gotoPaneID)
	// Find storyfragments using any of the changed beliefs within this tenant
	affectedStoryfragments := bbs.FindAffectedStoryfragments(tenantID, changedBeliefs)

	if len(affectedStoryfragments) == 0 {
		// log.Printf("No affected storyfragments found for belief changes: %v", changedBeliefs)
		return
	}

	for storyfragmentID, affectedPanes := range affectedStoryfragments {
		// Only broadcast if sessions are viewing this storyfragment within this tenant
		if models.Broadcaster.HasViewingSessions(tenantID, storyfragmentID) {
			// log.Printf("Broadcasting belief change to tenant %s storyfragment %s, affected panes: %v",
			//	tenantID, storyfragmentID, affectedPanes)

			var scrollTarget *string
			if visibilitySnapshot != nil && currentPaneID != "" && gotoPaneID == "" {
				scrollTarget = bbs.computeScrollTarget(tenantID, storyfragmentID, currentPaneID, visibilitySnapshot[storyfragmentID], affectedPanes)
				//if scrollTarget != nil {
				//	log.Printf("DEBUG: Scroll target computed: %s", *scrollTarget)
				//} else {
				//	log.Printf("DEBUG: No scroll target found")
				//}
			} else {
				scrollTarget = &gotoPaneID
			}

			models.Broadcaster.BroadcastToSpecificSession(tenantID, sessionID, storyfragmentID, affectedPanes, scrollTarget)
			//} else {
			// log.Printf("No sessions viewing storyfragment %s - skipping broadcast", storyfragmentID)
		}
	}
}

// FindAffectedStoryfragments scans belief registries for panes using changed beliefs within tenant
func (bbs *BeliefBroadcastService) FindAffectedStoryfragments(tenantID string, changedBeliefs []string) map[string][]string {
	result := make(map[string][]string)

	// Create a set of changed beliefs for faster lookup
	beliefSet := make(map[string]bool)
	for _, belief := range changedBeliefs {
		beliefSet[belief] = true
	}

	// Scan all tenant-specific storyfragment belief registries
	// Note: This requires iterating through cached registries - we'll need to extend cache interface
	storyfragmentIDs := bbs.getAllStoryfragmentIDs(tenantID)

	for _, storyfragmentID := range storyfragmentIDs {
		if registry, exists := bbs.cacheManager.GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID); exists {
			affectedPanes := bbs.findAffectedPanesInRegistry(registry, beliefSet)
			if len(affectedPanes) > 0 {
				result[storyfragmentID] = affectedPanes
			}
		}
	}

	return result
}

// getAllStoryfragmentIDs gets all storyfragment IDs for tenant
func (bbs *BeliefBroadcastService) getAllStoryfragmentIDs(tenantID string) []string {
	return bbs.cacheManager.GetAllStoryfragmentBeliefRegistryIDs(tenantID)
}

// findAffectedPanesInRegistry checks which panes in a registry use any of the changed beliefs
func (bbs *BeliefBroadcastService) findAffectedPanesInRegistry(registry *models.StoryfragmentBeliefRegistry, changedBeliefs map[string]bool) []string {
	var affectedPanes []string

	for paneID, paneBeliefData := range registry.PaneBeliefPayloads {
		if bbs.paneUsesChangedBeliefs(paneBeliefData, changedBeliefs) {
			affectedPanes = append(affectedPanes, paneID)
		}
	}

	widgetAffectedPanes := bbs.checkWidgetBeliefs(registry, changedBeliefs)
	affectedPanes = append(affectedPanes, widgetAffectedPanes...)

	return affectedPanes
}

// paneUsesChangedBeliefs checks if a pane uses any of the changed beliefs
func (bbs *BeliefBroadcastService) paneUsesChangedBeliefs(paneData models.PaneBeliefData, changedBeliefs map[string]bool) bool {
	// Check held beliefs
	for beliefSlug := range paneData.HeldBeliefs {
		if changedBeliefs[beliefSlug] {
			return true
		}
	}

	// Check withheld beliefs
	for beliefSlug := range paneData.WithheldBeliefs {
		if changedBeliefs[beliefSlug] {
			return true
		}
	}

	// Check match-across beliefs
	for _, beliefSlug := range paneData.MatchAcross {
		if changedBeliefs[beliefSlug] {
			return true
		}
	}

	// Check linked beliefs
	for _, beliefSlug := range paneData.LinkedBeliefs {
		if changedBeliefs[beliefSlug] {
			return true
		}
	}

	return false
}

// checkWidgetBeliefs checks if any changed beliefs affect widget beliefs in the registry
func (bbs *BeliefBroadcastService) checkWidgetBeliefs(registry *models.StoryfragmentBeliefRegistry, changedBeliefs map[string]bool) []string {
	var affectedPanes []string

	// Check if any changed beliefs are widget beliefs
	for beliefSlug := range changedBeliefs {
		if registry.AllWidgetBeliefs[beliefSlug] {
			// Find all panes that have widgets using this belief
			for paneID, widgetBeliefs := range registry.PaneWidgetBeliefs {
				for _, widgetBelief := range widgetBeliefs {
					if widgetBelief == beliefSlug {
						affectedPanes = append(affectedPanes, paneID)
						break // Found this pane, move to next
					}
				}
			}
		}
	}

	return affectedPanes
}

// computeScrollTarget determines which pane to scroll to
// Simplified version: returns first newly revealed pane
func (bbs *BeliefBroadcastService) computeScrollTarget(tenantID, storyfragmentID, currentPaneID string, beforeSnapshot map[string]bool, affectedPanes []string) *string {
	// Get current session belief context to evaluate new visibility
	sessionData, exists := bbs.cacheManager.GetSession(tenantID, bbs.sessionID)
	if !exists {
		return nil
	}

	// Force session belief context creation if it doesn't exist
	sessionContext, exists := bbs.cacheManager.GetSessionBeliefContext(tenantID, bbs.sessionID, storyfragmentID)
	if !exists {
		// Try to get the fingerprint data to build session context
		fingerprintData, fingerprintExists := bbs.cacheManager.GetFingerprintState(tenantID, sessionData.FingerprintID)
		if !fingerprintExists {
			return nil
		}

		// Create a temporary session belief context using current fingerprint beliefs
		sessionContext = &models.SessionBeliefContext{
			TenantID:        tenantID,
			SessionID:       bbs.sessionID,
			StoryfragmentID: storyfragmentID,
			UserBeliefs:     fingerprintData.HeldBeliefs,
			LastEvaluation:  time.Now(),
		}

	}

	// Get belief registry to evaluate new visibility
	registry, exists := bbs.cacheManager.GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
	if !exists {
		return nil
	}

	// Find newly revealed panes
	var newlyRevealed []string
	beliefEngine := NewBeliefEvaluationEngine()

	for _, paneID := range affectedPanes {
		wasVisible := beforeSnapshot[paneID] // defaults to false if missing

		// Evaluate current visibility
		if paneBeliefs, exists := registry.PaneBeliefPayloads[paneID]; exists {
			visibilityResult := beliefEngine.EvaluatePaneVisibility(paneBeliefs, sessionContext.UserBeliefs)
			isVisible := (visibilityResult == "visible" || visibilityResult == "true")

			if !wasVisible && isVisible {
				newlyRevealed = append(newlyRevealed, paneID)
			}
		}
	}

	if len(newlyRevealed) == 0 {
		return nil
	}

	firstRevealed := newlyRevealed[0]
	return &firstRevealed
}
