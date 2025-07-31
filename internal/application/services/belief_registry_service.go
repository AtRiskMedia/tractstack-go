// Package services provides belief registry management
package services

import (
	"log"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// BeliefRegistryService handles storyfragment belief registry operations
type BeliefRegistryService struct {
	// No stored dependencies - all passed via tenant context
}

// NewBeliefRegistryService creates a new belief registry service singleton
func NewBeliefRegistryService() *BeliefRegistryService {
	return &BeliefRegistryService{}
}

// BuildRegistryFromLoadedPanes constructs belief registry using already-loaded pane nodes
// This optimizes the process by avoiding duplicate database calls and JSON parsing
func (brs *BeliefRegistryService) BuildRegistryFromLoadedPanes(tenantCtx *tenant.Context, storyfragmentID string, loadedPanes []*content.PaneNode) (*types.StoryfragmentBeliefRegistry, error) {
	// FIXED: Use manager interface method instead of calling map as function
	if registry, found := tenantCtx.CacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID); found {
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

		// Extract belief widgets from pane (placeholder implementation)
		widgetBeliefs := brs.extractBeliefWidgetsFromPane(paneID)
		if len(widgetBeliefs) > 0 {
			registry.PaneWidgetBeliefs[paneID] = widgetBeliefs
			for _, beliefSlug := range widgetBeliefs {
				registry.AllWidgetBeliefs[beliefSlug] = true
			}
		}

		// Extract belief requirements from pane options
		beliefData := brs.extractPaneBeliefData(paneNode)
		if !brs.isEmpty(beliefData) {
			registry.PaneBeliefPayloads[paneID] = beliefData

			// Update flat lookup maps
			for beliefSlug := range beliefData.HeldBeliefs {
				registry.RequiredBeliefs[beliefSlug] = true
			}
			for beliefSlug := range beliefData.WithheldBeliefs {
				registry.RequiredBeliefs[beliefSlug] = true
			}
			for _, beliefSlug := range beliefData.MatchAcross {
				registry.RequiredBeliefs[beliefSlug] = true
			}
		}
	}

	// Cache the built registry using manager interface
	tenantCtx.CacheManager.SetStoryfragmentBeliefRegistry(tenantCtx.TenantID, registry)

	return registry, nil
}

// extractBeliefWidgetsFromPane extracts belief widget slugs from pane content
func (brs *BeliefRegistryService) extractBeliefWidgetsFromPane(paneID string) []string {
	// TODO: Implement widget belief scanning
	// 1. Extract nodes from pane using html.ExtractNodesFromPane
	// 2. Scan for TagElement nodes with code tag
	// 3. Check for codeHookParams AND copy field
	// 4. Extract widget type from copy field (before the parentheses)
	// 5. Check if it's a belief widget type (belief, toggle, identifyAs)
	// 6. Extract belief slug from first parameter

	log.Printf("TODO: Widget belief scanning not yet implemented for pane %s", paneID)
	log.Printf("HEY THIS SHOULD HAPPEN --> Extract nodes from pane and scan for belief widgets")

	// For now, return empty list
	return []string{}
}

// GetFromCache retrieves cached belief registry
func (brs *BeliefRegistryService) GetFromCache(tenantCtx *tenant.Context, storyfragmentID string) (*types.StoryfragmentBeliefRegistry, bool) {
	// FIXED: Use manager interface method instead of calling map as function
	return tenantCtx.CacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID)
}

// InvalidateRegistry removes belief registry from cache
func (brs *BeliefRegistryService) InvalidateRegistry(tenantCtx *tenant.Context, storyfragmentID string) {
	// FIXED: Use manager interface method instead of calling map as function
	tenantCtx.CacheManager.InvalidateStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID)
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

// extractPaneBeliefData extracts belief requirements from pane options
func (brs *BeliefRegistryService) extractPaneBeliefData(paneNode *content.PaneNode) types.PaneBeliefData {
	// TODO: Implement extraction from pane.OptionsPayload
	// This would parse the JSON options and extract belief requirements
	return types.PaneBeliefData{
		HeldBeliefs:     make(map[string][]string),
		WithheldBeliefs: make(map[string][]string),
		MatchAcross:     []string{},
		LinkedBeliefs:   []string{},
		HeldBadges:      []string{},
	}
}

// isEmpty checks if belief data has any requirements
func (brs *BeliefRegistryService) isEmpty(data types.PaneBeliefData) bool {
	return len(data.HeldBeliefs) == 0 &&
		len(data.WithheldBeliefs) == 0 &&
		len(data.MatchAcross) == 0 &&
		len(data.LinkedBeliefs) == 0 &&
		len(data.HeldBadges) == 0
}
