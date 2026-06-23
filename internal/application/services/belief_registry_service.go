// Package services provides belief registry management
package services

import (
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// BeliefRegistryService handles storyfragment belief registry operations.
// It is responsible for scanning storyfragments and their panes to build a comprehensive
// map of all belief-based visibility rules and widget dependencies.
type BeliefRegistryService struct {
	logger *logging.ChanneledLogger
}

// NewBeliefRegistryService creates a new belief registry service singleton.
func NewBeliefRegistryService(logger *logging.ChanneledLogger) *BeliefRegistryService {
	return &BeliefRegistryService{
		logger: logger,
	}
}

// BuildRegistryFromLoadedPanes constructs a belief registry using already-loaded pane nodes.
// This is the primary entry point, designed to be called after a storyfragment's panes have been fetched.
// It avoids redundant database calls and populates the cache with the resulting registry.
func (brs *BeliefRegistryService) BuildRegistryFromLoadedPanes(tenantCtx *tenant.Context, storyfragmentID string, loadedPanes []*content.PaneNode) (*types.StoryfragmentBeliefRegistry, error) {
	start := time.Now()
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

	brs.logger.Content().Info("Successfully built belief registry from loaded panes", "tenantId", tenantCtx.TenantID, "storyfragmentId", storyfragmentID, "paneCount", len(loadedPanes), "duration", time.Since(start))

	return registry, nil
}

// RebuildForStoryFragment invalidates and synchronously rebuilds the belief registry
// for a storyfragment from its current pane IDs.
func (brs *BeliefRegistryService) RebuildForStoryFragment(tenantCtx *tenant.Context, storyFragmentID string, paneIDs []string) error {
	tenantCtx.CacheManager.InvalidateStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyFragmentID)

	var loadedPanes []*content.PaneNode
	if len(paneIDs) > 0 {
		panes, err := tenantCtx.PaneRepo().FindByIDs(tenantCtx.TenantID, paneIDs)
		if err != nil {
			return err
		}
		loadedPanes = panes
	}

	_, err := brs.BuildRegistryFromLoadedPanes(tenantCtx, storyFragmentID, loadedPanes)
	return err
}

// ComputeCodeHookVisibility returns initial codehook pane visibility for page load or SSE.
// storyFragment must carry enriched CodeHookTargets; paneFilter limits to affected panes (SSE).
func (brs *BeliefRegistryService) ComputeCodeHookVisibility(
	tenantCtx *tenant.Context,
	sessionID string,
	storyFragment *content.StoryFragmentNode,
	paneFilter []string,
) map[string]any {
	codeHookVisibility := make(map[string]any)
	if storyFragment == nil {
		return codeHookVisibility
	}

	codeHookTargets := brs.resolveCodeHookTargets(tenantCtx, storyFragment)
	if len(codeHookTargets) == 0 {
		return codeHookVisibility
	}

	storyFragmentWithTargets := *storyFragment
	storyFragmentWithTargets.CodeHookTargets = codeHookTargets

	codeHookPaneIDs := brs.codeHookPaneIDs(&storyFragmentWithTargets, paneFilter)
	if len(codeHookPaneIDs) == 0 {
		return codeHookVisibility
	}

	if _, found := tenantCtx.CacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyFragment.ID); !found {
		if err := brs.RebuildForStoryFragment(tenantCtx, storyFragment.ID, storyFragment.PaneIDs); err != nil {
			brs.logger.Content().Error("Failed to rebuild belief registry for codehook visibility",
				"error", err, "storyFragmentId", storyFragment.ID, "tenantId", tenantCtx.TenantID)
		}
	}

	beliefRegistry, found := tenantCtx.CacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyFragment.ID)
	userBeliefs := brs.loadUserBeliefs(tenantCtx, sessionID)
	beliefEngine := NewBeliefEvaluationService()

	for _, paneID := range codeHookPaneIDs {
		if !found {
			codeHookVisibility[paneID] = true
			continue
		}
		if paneBeliefs, exists := beliefRegistry.PaneBeliefPayloads[paneID]; exists {
			codeHookVisibility[paneID] = beliefEngine.CalculateCodeHookVisibilityState(paneBeliefs, userBeliefs)
		} else {
			codeHookVisibility[paneID] = true
		}
	}

	return codeHookVisibility
}

func (brs *BeliefRegistryService) resolveCodeHookTargets(tenantCtx *tenant.Context, storyFragment *content.StoryFragmentNode) map[string]string {
	if storyFragment.CodeHookTargets != nil && len(storyFragment.CodeHookTargets) > 0 {
		return storyFragment.CodeHookTargets
	}

	targets := make(map[string]string)
	if len(storyFragment.PaneIDs) == 0 {
		return targets
	}

	panes, err := tenantCtx.PaneRepo().FindByIDs(tenantCtx.TenantID, storyFragment.PaneIDs)
	if err != nil {
		brs.logger.Content().Debug("Failed to load panes for codehook target resolution", "error", err, "storyFragmentId", storyFragment.ID)
		return targets
	}

	for _, pane := range panes {
		if pane == nil || pane.CodeHookTarget == nil || *pane.CodeHookTarget == "" {
			continue
		}
		targets[pane.ID] = *pane.CodeHookTarget
		if pane.CodeHookPayload != nil {
			if optionsStr, ok := pane.CodeHookPayload["options"]; ok {
				targets[pane.ID+"-"+*pane.CodeHookTarget] = optionsStr
			}
		}
	}

	return targets
}

func (brs *BeliefRegistryService) codeHookPaneIDs(storyFragment *content.StoryFragmentNode, paneFilter []string) []string {
	candidates := storyFragment.PaneIDs
	if len(paneFilter) > 0 {
		candidates = paneFilter
	}

	var paneIDs []string
	for _, paneID := range candidates {
		if hook, ok := storyFragment.CodeHookTargets[paneID]; ok && hook != "" {
			paneIDs = append(paneIDs, paneID)
		}
	}
	return paneIDs
}

func (brs *BeliefRegistryService) loadUserBeliefs(tenantCtx *tenant.Context, sessionID string) map[string][]string {
	var userBeliefs map[string][]string
	sessionData, sessionExists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, sessionID)
	if sessionExists {
		fpState, fpExists := tenantCtx.CacheManager.GetFingerprintState(tenantCtx.TenantID, sessionData.FingerprintID)
		if fpExists && fpState.HeldBeliefs != nil {
			userBeliefs = fpState.HeldBeliefs
		}
	}
	if userBeliefs == nil {
		userBeliefs = make(map[string][]string)
	}
	return userBeliefs
}

// extractPaneBeliefData translates the belief rules from a PaneNode's OptionsPayload
// into the structured PaneBeliefData format used by the registry.
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
		if copyVal, ok := nodeMap["copy"].(string); ok {
			copyText = copyVal
		}

		// The widget type (e.g., "belief", "toggle") is encoded in the `copy` field.
		widgetType := extractWidgetTypeFromCopy(copyText)

		// Check if it's a widget that controls a belief.
		if widgetType == "belief" || widgetType == "toggle" || widgetType == "identifyAs" || widgetType == "interactiveDisclosure" {
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
