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

	// CORRECTED: Call the correct constructor
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

func (b *BeliefBroadcastService) BroadcastBeliefChange(tenantID, sessionID string, changedBeliefs []string, visibilitySnapshot map[string]map[string]bool, currentPaneID, gotoPaneID string, broadcaster messaging.Broadcaster) {
	affectedStoryfragments := b.FindAffectedStoryfragments(tenantID, changedBeliefs)
	if len(affectedStoryfragments) == 0 {
		return
	}
	for storyfragmentID, affectedPanes := range affectedStoryfragments {
		if broadcaster.HasViewingSessions(tenantID, storyfragmentID) {
			var scrollTarget *string
			if visibilitySnapshot != nil && currentPaneID != "" && gotoPaneID == "" {
				scrollTarget = b.computeScrollTarget(tenantID, sessionID, storyfragmentID, visibilitySnapshot[storyfragmentID], affectedPanes)
			} else {
				scrollTarget = &gotoPaneID
			}
			broadcaster.BroadcastToSpecificSession(tenantID, sessionID, storyfragmentID, affectedPanes, scrollTarget)
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
