// Package services provides belief broadcasting service for SSE integration
package services

import (
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/messaging"
)

// BeliefBroadcastService handles tenant-scoped targeted broadcasting when beliefs change.
type BeliefBroadcastService struct {
	cacheManager interfaces.Cache
}

// NewBeliefBroadcastService creates a new belief broadcast service.
func NewBeliefBroadcastService(cacheManager interfaces.Cache) *BeliefBroadcastService {
	return &BeliefBroadcastService{cacheManager: cacheManager}
}

// StoryfragmentUpdate represents an update for a single storyfragment
type StoryfragmentUpdate struct {
	StoryfragmentID string   `json:"storyfragmentId"`
	AffectedPanes   []string `json:"affectedPanes"`
	GotoPaneID      *string  `json:"gotoPaneId,omitempty"`
}

// BatchUpdate represents a batch of storyfragment updates
type BatchUpdate struct {
	Updates []StoryfragmentUpdate `json:"updates"`
}

// CalculateBeliefDiff determines which panes change visibility between two belief states.
func (b *BeliefBroadcastService) CalculateBeliefDiff(tenantID, storyfragmentID string, beforeBeliefs, afterBeliefs map[string][]string) []string {
	// Get the storyfragment belief registry - same as PAGEVIEWED logic
	beliefRegistry, registryExists := b.cacheManager.GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
	if !registryExists {
		return nil
	}

	var affectedPanes []string

	// Create belief evaluator
	beliefEngine := NewBeliefEvaluationService()

	for paneID, paneBeliefs := range beliefRegistry.PaneBeliefPayloads {
		beforeVisibility := beliefEngine.EvaluatePaneVisibility(paneBeliefs, beforeBeliefs)
		beforeVisible := (beforeVisibility == "visible")

		afterVisibility := beliefEngine.EvaluatePaneVisibility(paneBeliefs, afterBeliefs)
		afterVisible := (afterVisibility == "visible")

		if beforeVisible != afterVisible {
			affectedPanes = append(affectedPanes, paneID)
		}
	}

	return affectedPanes
}

func (b *BeliefBroadcastService) computeScrollTarget(
	tenantID, sessionID, storyfragmentID string,
	beforeSnapshot map[string]bool,
	affectedPanes []string,
) *string {
	sessionContext, exists := b.cacheManager.GetSessionBeliefContext(tenantID, sessionID, storyfragmentID)
	if !exists {
		return nil
	}
	registry, exists := b.cacheManager.GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
	if !exists {
		return nil
	}
	var newlyRevealed []string

	beliefEngine := NewBeliefEvaluationService()

	for _, paneID := range affectedPanes {
		wasVisible := beforeSnapshot[paneID]
		if paneBeliefs, exists := registry.PaneBeliefPayloads[paneID]; exists {
			visibilityResult := beliefEngine.EvaluatePaneVisibility(paneBeliefs, sessionContext.UserBeliefs)
			isVisible := (visibilityResult == "visible")
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

func (b *BeliefBroadcastService) BroadcastBeliefChange(tenantID, sessionID, storyfragmentID string, changedBeliefs []string, visibilitySnapshot map[string]map[string]bool, currentPaneID, gotoPaneID string, broadcaster messaging.Broadcaster) {
	// Get session data to find fingerprint
	sessionData, exists := b.cacheManager.GetSession(tenantID, sessionID)
	if !exists {
		return
	}

	// Find ALL sessions using this fingerprint (cross-browser sync!)
	allSessionIDs := b.cacheManager.GetSessionsByFingerprint(tenantID, sessionData.FingerprintID)

	// Find ALL storyfragments affected by these belief changes
	affectedStoryfragments := b.FindAffectedStoryfragments(tenantID, changedBeliefs)

	if len(affectedStoryfragments) == 0 || len(allSessionIDs) == 0 {
		return
	}

	// Broadcast to ALL sessions using this fingerprint
	for _, targetSessionID := range allSessionIDs {
		// For each affected storyfragment, send individual broadcasts
		for affectedStoryfragmentID, affectedPanes := range affectedStoryfragments {
			var scrollTarget *string

			// Only add scroll target for the original triggering session and storyfragment
			if targetSessionID == sessionID && affectedStoryfragmentID == storyfragmentID {
				if visibilitySnapshot != nil && currentPaneID != "" && gotoPaneID == "" {
					scrollTarget = b.computeScrollTarget(tenantID, sessionID, storyfragmentID, visibilitySnapshot[storyfragmentID], affectedPanes)
				} else if gotoPaneID != "" {
					scrollTarget = &gotoPaneID
				}
			}

			broadcaster.BroadcastToSpecificSession(tenantID, targetSessionID, affectedStoryfragmentID, affectedPanes, scrollTarget)

			// Invalidate belief context AFTER broadcasting (so computeScrollTarget can use it)
			b.cacheManager.InvalidateSessionBeliefContext(tenantID, targetSessionID, affectedStoryfragmentID)
		}
	}
}

func (b *BeliefBroadcastService) FindAffectedStoryfragments(tenantID string, changedBeliefs []string) map[string][]string {
	result := make(map[string][]string)
	beliefSet := make(map[string]bool)
	for _, belief := range changedBeliefs {
		beliefSet[belief] = true
	}
	storyfragmentIDs := b.cacheManager.GetAllStoryfragmentBeliefRegistryIDs(tenantID)
	for _, storyfragmentID := range storyfragmentIDs {
		if registry, exists := b.cacheManager.GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID); exists {
			var affectedPanes []string
			for paneID, paneBeliefData := range registry.PaneBeliefPayloads {
				if b.paneUsesChangedBeliefs(paneBeliefData, beliefSet) {
					affectedPanes = append(affectedPanes, paneID)
				}
			}
			if len(affectedPanes) > 0 {
				result[storyfragmentID] = affectedPanes
			}
		}
	}
	return result
}

func (b *BeliefBroadcastService) paneUsesChangedBeliefs(paneData types.PaneBeliefData, changedBeliefs map[string]bool) bool {
	for beliefSlug := range paneData.HeldBeliefs {
		if changedBeliefs[beliefSlug] {
			return true
		}
	}
	for beliefSlug := range paneData.WithheldBeliefs {
		if changedBeliefs[beliefSlug] {
			return true
		}
	}
	for _, beliefSlug := range paneData.MatchAcross {
		if changedBeliefs[beliefSlug] {
			return true
		}
	}
	for _, beliefSlug := range paneData.LinkedBeliefs {
		if changedBeliefs[beliefSlug] {
			return true
		}
	}
	return false
}
