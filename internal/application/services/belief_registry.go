// Package services provides belief registry management
package services

import (
	"log"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// BeliefRegistryService handles storyfragment belief registry operations
type BeliefRegistryService struct {
	cache interfaces.UserStateCache
	ctx   *tenant.Context
}

// NewBeliefRegistryService creates a new belief registry service
func NewBeliefRegistryService(cache interfaces.UserStateCache, ctx *tenant.Context) *BeliefRegistryService {
	return &BeliefRegistryService{
		cache: cache,
		ctx:   ctx,
	}
}

// BuildRegistryFromLoadedPanes constructs belief registry using already-loaded pane nodes
// This optimizes the process by avoiding duplicate database calls and JSON parsing
func (brs *BeliefRegistryService) BuildRegistryFromLoadedPanes(storyfragmentID string, loadedPanes []*content.PaneNode) (*types.StoryfragmentBeliefRegistry, error) {
	// Check if registry already exists in cache
	if registry, found := brs.cache.GetStoryfragmentBeliefRegistry(brs.ctx.TenantID, storyfragmentID); found {
		return registry, nil
	}

	registry := &types.StoryfragmentBeliefRegistry{
		StoryfragmentID:    storyfragmentID,
		PaneBeliefPayloads: make(map[string]types.PaneBeliefData),
		RequiredBeliefs:    make(map[string]bool),
		RequiredBadges:     []string{},
		PaneWidgetBeliefs:  make(map[string][]string),
		AllWidgetBeliefs:   make(map[string]bool),
		LastUpdated:        time.Now().UTC(),
	}

	for _, paneNode := range loadedPanes {
		if paneNode == nil {
			continue // Skip nil panes
		}

		paneID := paneNode.ID

		// Extract belief data from this pane
		paneBeliefData := brs.extractPaneBeliefData(paneNode)

		// Only store if pane has belief requirements
		if brs.hasBeliefRequirements(paneBeliefData) {
			registry.PaneBeliefPayloads[paneID] = paneBeliefData

			// Add to flat required beliefs list
			brs.addToRequiredBeliefs(registry.RequiredBeliefs, paneBeliefData)
		}

		// Always scan for widget beliefs (even if pane has no traditional belief requirements)
		widgetBeliefs, err := brs.scanPaneForWidgetBeliefs(paneID, paneNode)
		if err != nil {
			log.Printf("Error scanning pane %s for widgets: %v", paneID, err)
		} else if len(widgetBeliefs) > 0 {
			// Store widget beliefs for this pane
			registry.PaneWidgetBeliefs[paneID] = widgetBeliefs

			// Add to flat widget beliefs lookup
			for _, beliefSlug := range widgetBeliefs {
				registry.AllWidgetBeliefs[beliefSlug] = true
			}

			// TODO: Load widget beliefs into cache when content repositories are ready
			// for _, beliefSlug := range widgetBeliefs {
			//     beliefRepo := content.NewBeliefRepository(brs.ctx.Database)
			//     belief, err := beliefRepo.FindBySlug(brs.ctx.TenantID, beliefSlug)
			//     if err != nil {
			//         log.Printf("Failed to load widget belief: %s - %v", beliefSlug, err)
			//     } else {
			//         brs.cache.SetBelief(brs.ctx.TenantID, belief)
			//     }
			// }
		}
	}

	// Cache the registry
	brs.cache.SetStoryfragmentBeliefRegistry(brs.ctx.TenantID, registry)
	return registry, nil
}

// extractPaneBeliefData converts PaneNode beliefs to PaneBeliefData format
func (brs *BeliefRegistryService) extractPaneBeliefData(paneNode *content.PaneNode) types.PaneBeliefData {
	data := types.PaneBeliefData{
		HeldBeliefs:     make(map[string][]string),
		WithheldBeliefs: make(map[string][]string),
		MatchAcross:     []string{},
		LinkedBeliefs:   []string{},
		HeldBadges:      []string{},
	}

	// Process heldBeliefs, separating special keys
	for key, values := range paneNode.HeldBeliefs {
		switch key {
		case "MATCH-ACROSS":
			data.MatchAcross = values
		case "LINKED-BELIEFS":
			data.LinkedBeliefs = values
		default:
			data.HeldBeliefs[key] = values
		}
	}

	// Process withheldBeliefs
	for key, values := range paneNode.WithheldBeliefs {
		// No special keys in withheldBeliefs
		data.WithheldBeliefs[key] = values
	}

	// TODO: Process heldBadges when implemented

	return data
}

// hasBeliefRequirements checks if pane has any belief/badge requirements
func (brs *BeliefRegistryService) hasBeliefRequirements(data types.PaneBeliefData) bool {
	return len(data.HeldBeliefs) > 0 ||
		len(data.WithheldBeliefs) > 0 ||
		len(data.MatchAcross) > 0 ||
		len(data.LinkedBeliefs) > 0 ||
		len(data.HeldBadges) > 0
}

// addToRequiredBeliefs adds belief slugs to the flat required list
func (brs *BeliefRegistryService) addToRequiredBeliefs(required map[string]bool, data types.PaneBeliefData) {
	// Add standard held beliefs
	for beliefSlug := range data.HeldBeliefs {
		required[beliefSlug] = true
	}

	// Add withheld beliefs
	for beliefSlug := range data.WithheldBeliefs {
		required[beliefSlug] = true
	}

	// Add match-across beliefs
	for _, beliefSlug := range data.MatchAcross {
		required[beliefSlug] = true
	}

	// Add linked beliefs
	for _, beliefSlug := range data.LinkedBeliefs {
		required[beliefSlug] = true
	}
}

// scanPaneForWidgetBeliefs scans pane nodes for belief widgets
func (brs *BeliefRegistryService) scanPaneForWidgetBeliefs(paneID string, paneNode *content.PaneNode) ([]string, error) {
	// TODO: Implement widget belief scanning when HTML extraction is available
	// This should:
	// 1. Extract nodes from pane using html.ExtractNodesFromPane
	// 2. Scan for TagElement nodes with code tag
	// 3. Check for codeHookParams AND copy field
	// 4. Extract widget type from copy field (before the parentheses)
	// 5. Check if it's a belief widget type (belief, toggle, identifyAs)
	// 6. Extract belief slug from first parameter

	log.Printf("TODO: Widget belief scanning not yet implemented for pane %s", paneID)
	log.Printf("HEY THIS SHOULD HAPPEN --> Extract nodes from pane and scan for belief widgets")

	// For now, return empty list
	return []string{}, nil
}

// GetFromCache retrieves cached belief registry
func (brs *BeliefRegistryService) GetFromCache(storyfragmentID string) (*types.StoryfragmentBeliefRegistry, bool) {
	return brs.cache.GetStoryfragmentBeliefRegistry(brs.ctx.TenantID, storyfragmentID)
}

// InvalidateRegistry removes belief registry from cache
func (brs *BeliefRegistryService) InvalidateRegistry(storyfragmentID string) {
	brs.cache.InvalidateStoryfragmentBeliefRegistry(brs.ctx.TenantID, storyfragmentID)
}

// extractWidgetTypeFromCopy extracts widget type from copy field (e.g., "belief(...)" -> "belief")
func extractWidgetTypeFromCopy(copyText string) string {
	if copyText == "" {
		return ""
	}

	// Find the first opening parenthesis
	parenIndex := strings.Index(copyText, "(")
	if parenIndex == -1 {
		return ""
	}

	// Return everything before the parenthesis
	return copyText[:parenIndex]
}
