// Package services provides belief broadcasting service for Stage 3 SSE integration
package services

import (
	"log"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
)

// BeliefBroadcastService handles tenant-scoped targeted broadcasting when beliefs change
type BeliefBroadcastService struct {
	cacheManager *cache.Manager
}

// NewBeliefBroadcastService creates a new belief broadcast service
func NewBeliefBroadcastService(cacheManager *cache.Manager) *BeliefBroadcastService {
	return &BeliefBroadcastService{cacheManager: cacheManager}
}

// BroadcastBeliefChange identifies affected storyfragments and broadcasts updates within tenant
func (bbs *BeliefBroadcastService) BroadcastBeliefChange(tenantID, sessionID string, changedBeliefs []string) {
	log.Printf("Processing belief change for tenant %s session %s: changed beliefs %v",
		tenantID, sessionID, changedBeliefs)

	// Find storyfragments using any of the changed beliefs within this tenant
	affectedStoryfragments := bbs.findAffectedStoryfragments(tenantID, changedBeliefs)

	if len(affectedStoryfragments) == 0 {
		log.Printf("No affected storyfragments found for belief changes: %v", changedBeliefs)
		return
	}

	for storyfragmentID, affectedPanes := range affectedStoryfragments {
		// Only broadcast if sessions are viewing this storyfragment within this tenant
		if models.Broadcaster.HasViewingSessions(tenantID, storyfragmentID) {
			log.Printf("Broadcasting belief change to tenant %s storyfragment %s, affected panes: %v",
				tenantID, storyfragmentID, affectedPanes)

			models.Broadcaster.BroadcastToAffectedPanes(tenantID, storyfragmentID, affectedPanes)
		} else {
			log.Printf("No viewing sessions for storyfragment %s, skipping broadcast", storyfragmentID)
		}
	}
}

// findAffectedStoryfragments scans belief registries for panes using changed beliefs within tenant
func (bbs *BeliefBroadcastService) findAffectedStoryfragments(tenantID string, changedBeliefs []string) map[string][]string {
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
