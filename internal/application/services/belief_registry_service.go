// Package services provides belief registry management
package services

import (
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// BeliefRegistryService handles storyfragment belief registry operations.
// It is responsible for scanning storyfragments and their panes to build a comprehensive
// map of all belief-based visibility rules and widget dependencies.
type BeliefRegistryService struct {
	// No stored dependencies - all passed via tenant context
}

// NewBeliefRegistryService creates a new belief registry service singleton.
func NewBeliefRegistryService() *BeliefRegistryService {
	return &BeliefRegistryService{}
}

// BuildRegistryFromLoadedPanes constructs a belief registry using already-loaded pane nodes.
// This is the primary entry point, designed to be called after a storyfragment's panes have been fetched.
// It avoids redundant database calls and populates the cache with the resulting registry.
func (brs *BeliefRegistryService) BuildRegistryFromLoadedPanes(tenantCtx *tenant.Context, storyfragmentID string, loadedPanes []*content.PaneNode) (*types.StoryfragmentBeliefRegistry, error) {
	// First, check if a valid registry already exists in the cache.
	if registry, found := tenantCtx.CacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID); found {
		return registry, nil
	}

	// Create a new, empty registry structure.
	registry := &types.StoryfragmentBeliefRegistry{
		StoryfragmentID:    storyfragmentID,
		PaneBeliefPayloads: make(map[string]types.PaneBeliefData),
		RequiredBeliefs:    make(map[string]bool),
		RequiredBadges:     []string{},
		PaneWidgetBeliefs:  make(map[string][]string),
		AllWidgetBeliefs:   make(map[string]bool),
		LastUpdated:        time.Now().UTC(),
	}

	// Iterate through each pane that belongs to the storyfragment.
	for _, paneNode := range loadedPanes {
		if paneNode == nil {
			continue // Skip nil panes
		}

		paneID := paneNode.ID

		// Task 1: Extract pane-level belief visibility rules (held/withheld).
		paneBeliefData := brs.extractPaneBeliefData(paneNode)
		if !brs.isEmpty(paneBeliefData) {
			registry.PaneBeliefPayloads[paneID] = paneBeliefData
			// Add these beliefs to the flat lookup map for quick checks.
			brs.addToRequiredBeliefs(registry.RequiredBeliefs, paneBeliefData)
		}

		// Task 2: Scan the pane's node structure for interactive belief widgets.
		// This is the critical logic that was previously missing.
		widgetBeliefs := brs.extractBeliefWidgetsFromPane(paneNode)
		if len(widgetBeliefs) > 0 {
			registry.PaneWidgetBeliefs[paneID] = widgetBeliefs
			for _, beliefSlug := range widgetBeliefs {
				registry.AllWidgetBeliefs[beliefSlug] = true
			}
		}
	}

	// Cache the newly built registry for future requests.
	tenantCtx.CacheManager.SetStoryfragmentBeliefRegistry(tenantCtx.TenantID, registry)

	return registry, nil
}

// extractPaneBeliefData translates the belief rules from a PaneNode's OptionsPayload
// into the structured PaneBeliefData format used by the registry.
// LOGIC SOURCE: Replicates the parsing logic from `models/content_panes.go#deserializeRowData`.
func (brs *BeliefRegistryService) extractPaneBeliefData(paneNode *content.PaneNode) types.PaneBeliefData {
	data := types.PaneBeliefData{
		HeldBeliefs:     make(map[string][]string),
		WithheldBeliefs: make(map[string][]string),
		MatchAcross:     []string{},
		LinkedBeliefs:   []string{},
		HeldBadges:      []string{},
	}

	if paneNode.HeldBeliefs != nil {
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
	}

	if paneNode.WithheldBeliefs != nil {
		for key, values := range paneNode.WithheldBeliefs {
			data.WithheldBeliefs[key] = values
		}
	}

	return data
}

// extractBeliefWidgetsFromPane is the core of the fix. It scans the `nodes` array within a pane's
// OptionsPayload to find all belief-related widgets and the belief slugs they control.
// LOGIC SOURCE: This combines the node parsing from `html/node_parser.go` with the widget identification
// logic from the legacy `services/belief_registry.go`.
func (brs *BeliefRegistryService) extractBeliefWidgetsFromPane(paneNode *content.PaneNode) []string {
	var widgetBeliefs []string

	if paneNode.OptionsPayload == nil {
		return widgetBeliefs
	}

	// The `nodes` key in the payload contains the array of all elements inside the pane.
	if nodes, ok := paneNode.OptionsPayload["nodes"].([]any); ok {
		for _, nodeInterface := range nodes {
			// Recursively scan each node and its children.
			brs.scanNodeRecursive(nodeInterface, &widgetBeliefs)
		}
	}

	return widgetBeliefs
}

// scanNodeRecursive traverses the node tree within a pane's payload to find belief widgets.
func (brs *BeliefRegistryService) scanNodeRecursive(nodeData any, foundBeliefs *[]string) {
	nodeMap, ok := nodeData.(map[string]any)
	if !ok {
		return
	}

	// A widget is defined as a `code` tag with specific parameters.
	if tagName, ok := nodeMap["tagName"].(string); ok && tagName == "code" {
		var copyText string
		if copy, ok := nodeMap["copy"].(string); ok {
			copyText = copy
		}

		// The widget type (e.g., "belief", "toggle") is encoded in the `copy` field.
		widgetType := extractWidgetTypeFromCopy(copyText)

		// Check if it's a widget that controls a belief.
		if widgetType == "belief" || widgetType == "toggle" || widgetType == "identifyAs" {
			// The belief slug is passed as the first parameter in `codeHookParams`.
			if params, ok := nodeMap["codeHookParams"].([]any); ok && len(params) > 0 {
				if beliefSlug, ok := params[0].(string); ok && beliefSlug != "" {
					*foundBeliefs = append(*foundBeliefs, beliefSlug)
				}
			}
		}
	}

	// Although the legacy structure was flat, a robust implementation should check for nested nodes.
	if children, ok := nodeMap["children"].([]any); ok {
		for _, child := range children {
			brs.scanNodeRecursive(child, foundBeliefs)
		}
	}
}

// extractWidgetTypeFromCopy parses the widget type from the node's `copy` field (e.g., "belief(...)" -> "belief").
// LOGIC SOURCE: Replicates the helper function from the legacy `services/belief_registry.go`.
func extractWidgetTypeFromCopy(copyText string) string {
	if copyText == "" {
		return ""
	}
	if parenIndex := strings.Index(copyText, "("); parenIndex != -1 {
		return copyText[:parenIndex]
	}
	return ""
}

// addToRequiredBeliefs populates the flat lookup map of all beliefs required by a pane.
func (brs *BeliefRegistryService) addToRequiredBeliefs(required map[string]bool, data types.PaneBeliefData) {
	for beliefSlug := range data.HeldBeliefs {
		required[beliefSlug] = true
	}
	for beliefSlug := range data.WithheldBeliefs {
		required[beliefSlug] = true
	}
	for _, beliefSlug := range data.MatchAcross {
		required[beliefSlug] = true
	}
	for _, beliefSlug := range data.LinkedBeliefs {
		required[beliefSlug] = true
	}
}

// isEmpty checks if a PaneBeliefData structure contains any actual rules.
func (brs *BeliefRegistryService) isEmpty(data types.PaneBeliefData) bool {
	return len(data.HeldBeliefs) == 0 &&
		len(data.WithheldBeliefs) == 0 &&
		len(data.MatchAcross) == 0 &&
		len(data.LinkedBeliefs) == 0 &&
		len(data.HeldBadges) == 0
}
