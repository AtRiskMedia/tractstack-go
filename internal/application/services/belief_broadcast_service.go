package services

import (
	"slices"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/messaging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// BeliefBroadcastService handles tenant-scoped targeted broadcasting when beliefs change.
type BeliefBroadcastService struct {
	cacheManager          interfaces.Cache
	beliefRegistryService *BeliefRegistryService
	tenantManager         *tenant.Manager
}

// NewBeliefBroadcastService creates a new belief broadcast service.
func NewBeliefBroadcastService(
	cacheManager interfaces.Cache,
	beliefRegistryService *BeliefRegistryService,
	tenantManager *tenant.Manager,
) *BeliefBroadcastService {
	return &BeliefBroadcastService{
		cacheManager:          cacheManager,
		beliefRegistryService: beliefRegistryService,
		tenantManager:         tenantManager,
	}
}

// StoryfragmentUpdate represents an update for a single storyfragment
type StoryfragmentUpdate struct {
	StoryfragmentID    string   `json:"storyfragmentId"`
	AffectedPanes      []string `json:"affectedPanes"`
	GotoPaneID         *string  `json:"gotoPaneId,omitempty"`
	CodeHookVisibility map[string]bool
}

// BatchUpdate represents a batch of storyfragment updates
type BatchUpdate struct {
	Updates []StoryfragmentUpdate `json:"updates"`
}

// CalculateBeliefDiff determines which panes change visibility between two belief states.
func (b *BeliefBroadcastService) CalculateBeliefDiff(tenantID, storyfragmentID string, beforeBeliefs, afterBeliefs map[string][]string) []string {
	beliefRegistry, registryExists := b.cacheManager.GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
	if !registryExists {
		return nil
	}

	var affectedPanes []string
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
		paneBeliefs, exists := registry.PaneBeliefPayloads[paneID]
		if !exists {
			continue
		}

		visibilityResult := beliefEngine.EvaluatePaneVisibility(paneBeliefs, sessionContext.UserBeliefs)
		isVisible := (visibilityResult == "visible")

		if !wasVisible && isVisible {
			newlyRevealed = append(newlyRevealed, paneID)
		}
	}

	if len(newlyRevealed) == 0 {
		return nil
	}

	firstRevealed := newlyRevealed[0]
	return &firstRevealed
}

// BroadcastBeliefChange notifies relevant listeners about changes in belief state.
func (b *BeliefBroadcastService) BroadcastBeliefChange(tenantID, sessionID, storyfragmentID string, changedBeliefs []string, visibilitySnapshot map[string]map[string]bool, currentPaneID, gotoPaneID string, broadcaster messaging.Broadcaster) {
	sessionData, exists := b.cacheManager.GetSession(tenantID, sessionID)
	if !exists {
		return
	}

	allSessionIDs := b.cacheManager.GetSessionsByFingerprint(tenantID, sessionData.FingerprintID)
	affectedStoryfragments := b.FindAffectedStoryfragments(tenantID, changedBeliefs)

	if currentPaneID != "" && storyfragmentID != "" {
		if panes, exists := affectedStoryfragments[storyfragmentID]; exists {
			if !slices.Contains(panes, currentPaneID) {
				affectedStoryfragments[storyfragmentID] = append(panes, currentPaneID)
			}
		} else {
			affectedStoryfragments[storyfragmentID] = []string{currentPaneID}
		}
	}

	if len(affectedStoryfragments) == 0 || len(allSessionIDs) == 0 {
		return
	}

	for _, targetSessionID := range allSessionIDs {
		for affectedStoryfragmentID, affectedPanes := range affectedStoryfragments {
			var scrollTarget *string

			// Calculate scroll target BEFORE invalidating the context.
			if targetSessionID == sessionID && affectedStoryfragmentID == storyfragmentID {
				if visibilitySnapshot != nil && currentPaneID != "" && gotoPaneID == "" {
					scrollTarget = b.computeScrollTarget(tenantID, sessionID, storyfragmentID, visibilitySnapshot[storyfragmentID], affectedPanes)
				} else if gotoPaneID != "" {
					scrollTarget = &gotoPaneID
				}
			}

			codeHookVisibility := b.calculateSSECodeHookVisibility(tenantID, targetSessionID, affectedStoryfragmentID, affectedPanes)
			broadcaster.BroadcastToSpecificSession(tenantID, targetSessionID, affectedStoryfragmentID, affectedPanes, scrollTarget, codeHookVisibility)
		}
	}

	var invalidationTargets []types.SessionBeliefTarget
	for _, targetSessionID := range allSessionIDs {
		for affectedStoryfragmentID := range affectedStoryfragments {
			invalidationTargets = append(invalidationTargets, types.SessionBeliefTarget{
				SessionID:       targetSessionID,
				StoryfragmentID: affectedStoryfragmentID,
			})
		}
	}
	b.cacheManager.BatchInvalidateSessionBeliefContexts(tenantID, invalidationTargets)
}

func (b *BeliefBroadcastService) calculateSSECodeHookVisibility(tenantID, sessionID, storyfragmentID string, affectedPanes []string) map[string]any {
	storyFragment, exists := b.cacheManager.GetStoryFragment(tenantID, storyfragmentID)
	if !exists || storyFragment == nil {
		return map[string]any{}
	}

	tenantCtx, err := b.tenantManager.NewContextFromID(tenantID)
	if err != nil {
		return map[string]any{}
	}
	defer func() {
		if closeErr := tenantCtx.Close(); closeErr != nil {
			// Best-effort close during SSE visibility compute
			_ = closeErr
		}
	}()

	return b.beliefRegistryService.ComputeCodeHookVisibility(tenantCtx, sessionID, storyFragment, affectedPanes)
}

// FindAffectedStoryfragments identifies story fragments affected by the given belief changes.
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

			for paneID, widgetBeliefs := range registry.PaneWidgetBeliefs {
				paneHasChangedWidget := false
				for _, widgetBelief := range widgetBeliefs {
					if beliefSet[widgetBelief] {
						paneHasChangedWidget = true
						break
					}
				}

				if paneHasChangedWidget && !slices.Contains(affectedPanes, paneID) {
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
