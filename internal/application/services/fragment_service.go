package services

import (
	"fmt"
	"log"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/beliefs"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/widgets"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/templates"
)

// FragmentService orchestrates fragment generation with personalization
type FragmentService struct {
	widgetContextService *WidgetContextService
	sessionBeliefService *SessionBeliefService
}

// NewFragmentService creates a new fragment service
func NewFragmentService(
	widgetContextService *WidgetContextService,
	sessionBeliefService *SessionBeliefService,
) *FragmentService {
	return &FragmentService{
		widgetContextService: widgetContextService,
		sessionBeliefService: sessionBeliefService,
	}
}

// GenerateFragment creates HTML fragment for a single pane with caching
func (s *FragmentService) GenerateFragment(
	tenantCtx *tenant.Context,
	paneID, sessionID, storyfragmentID string,
) (string, error) {
	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindByID(tenantCtx.TenantID, paneID)
	if err != nil {
		return "", fmt.Errorf("failed to load pane %s: %w", paneID, err)
	}
	if pane == nil {
		return "", fmt.Errorf("pane %s not found", paneID)
	}

	beliefRegistry, err := s.getBeliefRegistry(tenantCtx, storyfragmentID)
	if err != nil {
		log.Printf("Warning: failed to get belief registry: %v", err)
		beliefRegistry = nil
	}

	shouldBypass := false
	if beliefRegistry != nil && s.widgetContextService != nil {
		shouldBypass, _ = s.widgetContextService.ShouldBypassCacheForWidgets(
			tenantCtx, paneID, sessionID, storyfragmentID, beliefRegistry,
		)
	}

	var htmlContent string
	cacheManager := tenantCtx.CacheManager

	if shouldBypass && sessionID != "" {
		htmlContent, err = s.generatePersonalizedHTML(tenantCtx, pane, sessionID, storyfragmentID, beliefRegistry)
		if err != nil {
			return "", fmt.Errorf("failed to generate personalized HTML: %w", err)
		}
	} else {
		variant := types.PaneVariant{}
		if cachedHTML, exists := cacheManager.GetHTMLChunk(tenantCtx.TenantID, paneID, variant); exists {
			htmlContent = cachedHTML.HTML
		} else {
			htmlContent, err = s.getCachedHTML(tenantCtx, pane)
			if err != nil {
				return "", fmt.Errorf("failed to get cached HTML: %w", err)
			}

			dependencies := s.extractDependencies(pane)
			cacheManager.SetHTMLChunk(tenantCtx.TenantID, paneID, variant, htmlContent, dependencies)
		}
	}

	if beliefRegistry != nil && sessionID != "" {
		htmlContent, err = s.applyBeliefVisibility(tenantCtx, htmlContent, pane, sessionID, storyfragmentID, beliefRegistry)
		if err != nil {
			log.Printf("Warning: failed to apply visibility: %v", err)
		}
	}

	return htmlContent, nil
}

// GenerateFragmentBatch creates HTML fragments for multiple panes
func (s *FragmentService) GenerateFragmentBatch(
	tenantCtx *tenant.Context,
	paneIDs []string,
	sessionID, storyfragmentID string,
) (map[string]string, map[string]string, error) {
	results := make(map[string]string)
	errors := make(map[string]string)

	beliefRegistry, err := s.getBeliefRegistry(tenantCtx, storyfragmentID)
	if err != nil {
		log.Printf("Warning: failed to get belief registry for batch: %v", err)
		beliefRegistry = nil
	}

	var widgetCtx *widgets.WidgetContext
	if beliefRegistry != nil && sessionID != "" && s.widgetContextService != nil {
		widgetCtx, err = s.widgetContextService.PreResolveWidgetData(
			tenantCtx, sessionID, storyfragmentID, paneIDs, beliefRegistry,
		)
		if err != nil {
			log.Printf("Warning: failed to pre-resolve widget data: %v", err)
		}
	}

	for _, paneID := range paneIDs {
		html, err := s.generateSingleFragmentWithContext(
			tenantCtx, paneID, sessionID, storyfragmentID, beliefRegistry, widgetCtx,
		)
		if err != nil {
			errors[paneID] = err.Error()
			continue
		}
		results[paneID] = html
	}

	return results, errors, nil
}

// generateSingleFragmentWithContext generates a fragment with pre-resolved context
func (s *FragmentService) generateSingleFragmentWithContext(
	tenantCtx *tenant.Context,
	paneID, sessionID, storyfragmentID string,
	beliefRegistry *beliefs.BeliefRegistry,
	widgetCtx *widgets.WidgetContext,
) (string, error) {
	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindByID(tenantCtx.TenantID, paneID)
	if err != nil {
		return "", fmt.Errorf("failed to load pane: %w", err)
	}
	if pane == nil {
		return "", fmt.Errorf("pane not found")
	}

	shouldBypass := false
	if beliefRegistry != nil && s.widgetContextService != nil {
		shouldBypass, _ = s.widgetContextService.ShouldBypassCacheForWidgets(
			tenantCtx, paneID, sessionID, storyfragmentID, beliefRegistry,
		)
	}

	var htmlContent string
	cacheManager := tenantCtx.CacheManager

	if shouldBypass && widgetCtx != nil {
		renderCtx := &rendering.RenderContext{
			TenantID:        tenantCtx.TenantID,
			SessionID:       sessionID,
			StoryfragmentID: storyfragmentID,
			WidgetContext:   widgetCtx,
		}

		generator := templates.NewGenerator(renderCtx)
		htmlContent = generator.RenderPaneFragment(pane.ID)
	} else {
		variant := types.PaneVariant{}
		if cachedHTML, exists := cacheManager.GetHTMLChunk(tenantCtx.TenantID, paneID, variant); exists {
			htmlContent = cachedHTML.HTML
		} else {
			htmlContent, err = s.getCachedHTML(tenantCtx, pane)
			if err != nil {
				return "", fmt.Errorf("failed to get cached HTML: %w", err)
			}

			dependencies := s.extractDependencies(pane)
			cacheManager.SetHTMLChunk(tenantCtx.TenantID, paneID, variant, htmlContent, dependencies)
		}
	}

	if beliefRegistry != nil && sessionID != "" {
		htmlContent, err = s.applyBeliefVisibility(tenantCtx, htmlContent, pane, sessionID, storyfragmentID, beliefRegistry)
		if err != nil {
			log.Printf("Warning: failed to apply visibility: %v", err)
		}
	}

	return htmlContent, nil
}

// getCachedHTML generates base HTML content for caching
func (s *FragmentService) getCachedHTML(tenantCtx *tenant.Context, pane *content.PaneNode) (string, error) {
	nodesData, parentChildMap, err := templates.ExtractNodesFromPane(pane)
	if err != nil {
		return "", fmt.Errorf("failed to extract nodes: %w", err)
	}

	paneNodeData := &rendering.NodeRenderData{
		ID:       pane.ID,
		NodeType: "Pane",
		PaneData: &rendering.PaneRenderData{
			Title:           pane.Title,
			Slug:            pane.Slug,
			IsDecorative:    pane.IsDecorative,
			BgColour:        pane.BgColour,
			HeldBeliefs:     s.convertStringMapToInterface(pane.HeldBeliefs),
			WithheldBeliefs: s.convertStringMapToInterface(pane.WithheldBeliefs),
			CodeHookTarget:  pane.CodeHookTarget,
			CodeHookPayload: s.convertStringMapToInterfaceMap(pane.CodeHookPayload),
		},
	}
	nodesData[pane.ID] = paneNodeData

	renderCtx := &rendering.RenderContext{
		AllNodes:         nodesData,
		ParentNodes:      parentChildMap,
		TenantID:         tenantCtx.TenantID,
		SessionID:        "",
		StoryfragmentID:  "",
		ContainingPaneID: pane.ID,
		WidgetContext:    nil,
	}

	generator := templates.NewGenerator(renderCtx)
	return generator.RenderPaneFragment(pane.ID), nil
}

// generatePersonalizedHTML generates fresh HTML with widget personalization
func (s *FragmentService) generatePersonalizedHTML(
	tenantCtx *tenant.Context,
	pane *content.PaneNode,
	sessionID, storyfragmentID string,
	beliefRegistry *beliefs.BeliefRegistry,
) (string, error) {
	nodesData, parentChildMap, err := templates.ExtractNodesFromPane(pane)
	if err != nil {
		return "", fmt.Errorf("failed to extract nodes: %w", err)
	}

	paneNodeData := &rendering.NodeRenderData{
		ID:       pane.ID,
		NodeType: "Pane",
		PaneData: &rendering.PaneRenderData{
			Title:           pane.Title,
			Slug:            pane.Slug,
			IsDecorative:    pane.IsDecorative,
			BgColour:        pane.BgColour,
			HeldBeliefs:     s.convertStringMapToInterface(pane.HeldBeliefs),
			WithheldBeliefs: s.convertStringMapToInterface(pane.WithheldBeliefs),
			CodeHookTarget:  pane.CodeHookTarget,
			CodeHookPayload: s.convertStringMapToInterfaceMap(pane.CodeHookPayload),
		},
	}
	nodesData[pane.ID] = paneNodeData

	var widgetCtx *widgets.WidgetContext
	if s.widgetContextService != nil {
		widgetCtx, err = s.widgetContextService.BuildWidgetContext(
			tenantCtx, sessionID, storyfragmentID, beliefRegistry,
		)
		if err != nil {
			return "", fmt.Errorf("failed to build widget context: %w", err)
		}
	}

	renderCtx := &rendering.RenderContext{
		AllNodes:         nodesData,
		ParentNodes:      parentChildMap,
		TenantID:         tenantCtx.TenantID,
		SessionID:        sessionID,
		StoryfragmentID:  storyfragmentID,
		ContainingPaneID: pane.ID,
		WidgetContext:    widgetCtx,
	}

	generator := templates.NewGenerator(renderCtx)
	return generator.RenderPaneFragment(pane.ID), nil
}

// getBeliefRegistry retrieves or builds belief registry for storyfragment
func (s *FragmentService) getBeliefRegistry(
	tenantCtx *tenant.Context,
	storyfragmentID string,
) (*beliefs.BeliefRegistry, error) {
	if storyfragmentID == "" {
		return nil, nil
	}

	cacheManager := tenantCtx.CacheManager

	if typesRegistry, exists := cacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID); exists {
		registry := s.convertTypesToDomainRegistry(typesRegistry)
		return registry, nil
	}

	storyfragmentRepo := tenantCtx.StoryFragmentRepo()
	storyfragment, err := storyfragmentRepo.FindByID(tenantCtx.TenantID, storyfragmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to load storyfragment: %w", err)
	}
	if storyfragment == nil {
		return nil, fmt.Errorf("storyfragment not found")
	}

	var panes []*content.PaneNode
	if len(storyfragment.PaneIDs) > 0 {
		paneRepo := tenantCtx.PaneRepo()
		panes, err = paneRepo.FindByIDs(tenantCtx.TenantID, storyfragment.PaneIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to load panes: %w", err)
		}
	}

	registry := s.buildBeliefRegistryFromPanes(storyfragmentID, panes)
	typesRegistry := s.convertDomainToTypesRegistry(registry)
	cacheManager.SetStoryfragmentBeliefRegistry(tenantCtx.TenantID, typesRegistry)

	return registry, nil
}

// buildBeliefRegistryFromPanes constructs belief registry from loaded panes
func (s *FragmentService) buildBeliefRegistryFromPanes(
	storyfragmentID string,
	panes []*content.PaneNode,
) *beliefs.BeliefRegistry {
	registry := beliefs.NewBeliefRegistry(storyfragmentID)

	for _, pane := range panes {
		widgetBeliefs := s.extractWidgetBeliefsFromPane(pane)
		if len(widgetBeliefs) > 0 {
			registry.AddPaneWidgetBeliefs(pane.ID, widgetBeliefs)
		}

		beliefData := s.extractPaneBeliefData(pane)
		if beliefData != nil {
			registry.AddPaneBeliefData(pane.ID, beliefData)
		}
	}

	return registry
}

// extractWidgetBeliefsFromPane extracts widget belief dependencies from pane
func (s *FragmentService) extractWidgetBeliefsFromPane(pane *content.PaneNode) []string {
	var beliefKeys []string

	if pane.OptionsPayload == nil {
		return beliefKeys
	}

	if nodes, ok := pane.OptionsPayload["nodes"].(map[string]interface{}); ok {
		for _, nodeData := range nodes {
			if nodeMap, ok := nodeData.(map[string]interface{}); ok {
				if nodeType, ok := nodeMap["type"].(string); ok && nodeType == "code" {
					if beliefKey := s.extractBeliefFromCodeNode(nodeMap); beliefKey != "" {
						beliefKeys = append(beliefKeys, beliefKey)
					}
				}
			}
		}
	}

	return beliefKeys
}

// extractBeliefFromCodeNode extracts belief key from code node
func (s *FragmentService) extractBeliefFromCodeNode(nodeData map[string]interface{}) string {
	if optionsPayload, ok := nodeData["optionsPayload"].(map[string]interface{}); ok {
		if beliefKey, ok := optionsPayload["beliefKey"].(string); ok {
			return beliefKey
		}
		if belief, ok := optionsPayload["belief"].(string); ok {
			return belief
		}
	}
	return ""
}

// extractPaneBeliefData extracts pane-level belief requirements
func (s *FragmentService) extractPaneBeliefData(pane *content.PaneNode) *beliefs.PaneBeliefData {
	if pane.OptionsPayload == nil {
		return nil
	}

	beliefMode := "neutral"
	requiredData := make(map[string]interface{})

	if mode, ok := pane.OptionsPayload["beliefMode"].(string); ok {
		beliefMode = mode
	}

	if requirements, ok := pane.OptionsPayload["beliefRequirements"].(map[string]interface{}); ok {
		requiredData = requirements
	}

	if beliefMode != "neutral" || len(requiredData) > 0 {
		return &beliefs.PaneBeliefData{
			PaneID:       pane.ID,
			BeliefMode:   beliefMode,
			RequiredData: requiredData,
		}
	}

	return nil
}

// applyBeliefVisibility applies belief-based visibility rules to HTML content
func (s *FragmentService) applyBeliefVisibility(
	tenantCtx *tenant.Context,
	htmlContent string,
	pane *content.PaneNode,
	sessionID, storyfragmentID string,
	beliefRegistry *beliefs.BeliefRegistry,
) (string, error) {
	cacheManager := tenantCtx.CacheManager

	if sessionContext, exists := cacheManager.GetSessionBeliefContext(tenantCtx.TenantID, sessionID, storyfragmentID); exists {
		if paneBeliefs, exists := beliefRegistry.GetPaneBeliefData(pane.ID); exists {
			visibility := s.evaluatePaneVisibility(paneBeliefs, sessionContext.UserBeliefs)
			htmlContent = s.applyVisibilityWrapper(htmlContent, visibility)
		}
	} else {
		if s.sessionBeliefService != nil && s.sessionBeliefService.ShouldCreateSessionContext(tenantCtx, sessionID, storyfragmentID) {
			newContext, err := s.sessionBeliefService.CreateSessionBeliefContext(tenantCtx, sessionID, storyfragmentID)
			if err != nil {
				return htmlContent, err
			}

			if paneBeliefs, exists := beliefRegistry.GetPaneBeliefData(pane.ID); exists {
				visibility := s.evaluatePaneVisibility(paneBeliefs, newContext.UserBeliefs)
				htmlContent = s.applyVisibilityWrapper(htmlContent, visibility)
			}
		}
	}

	return htmlContent, nil
}

// evaluatePaneVisibility evaluates whether content should be visible based on beliefs
func (s *FragmentService) evaluatePaneVisibility(
	beliefData *beliefs.PaneBeliefData,
	userBeliefs map[string][]string,
) string {
	switch beliefData.BeliefMode {
	case "held":
		return s.evaluateHeldBeliefs(beliefData.RequiredData, userBeliefs)
	case "withheld":
		return s.evaluateWithheldBeliefs(beliefData.RequiredData, userBeliefs)
	case "match-across":
		return s.evaluateMatchAcrossBeliefs(beliefData.RequiredData, userBeliefs)
	default:
		return "visible"
	}
}

// evaluateHeldBeliefs checks if user holds required beliefs
func (s *FragmentService) evaluateHeldBeliefs(
	requiredData map[string]interface{},
	userBeliefs map[string][]string,
) string {
	if requiredBeliefs, ok := requiredData["beliefs"].([]interface{}); ok {
		for _, beliefInterface := range requiredBeliefs {
			if beliefKey, ok := beliefInterface.(string); ok {
				if _, exists := userBeliefs[beliefKey]; !exists {
					return "hidden"
				}
			}
		}
		return "visible"
	}
	return "visible"
}

// evaluateWithheldBeliefs checks if user withholds required beliefs
func (s *FragmentService) evaluateWithheldBeliefs(
	requiredData map[string]interface{},
	userBeliefs map[string][]string,
) string {
	if requiredBeliefs, ok := requiredData["beliefs"].([]interface{}); ok {
		for _, beliefInterface := range requiredBeliefs {
			if beliefKey, ok := beliefInterface.(string); ok {
				if _, exists := userBeliefs[beliefKey]; exists {
					return "hidden"
				}
			}
		}
		return "visible"
	}
	return "visible"
}

// evaluateMatchAcrossBeliefs checks for belief matching across categories
func (s *FragmentService) evaluateMatchAcrossBeliefs(
	requiredData map[string]interface{},
	userBeliefs map[string][]string,
) string {
	if categories, ok := requiredData["categories"].([]interface{}); ok {
		matchCount := 0
		for _, categoryInterface := range categories {
			if categoryBeliefs, ok := categoryInterface.([]interface{}); ok {
				for _, beliefInterface := range categoryBeliefs {
					if beliefKey, ok := beliefInterface.(string); ok {
						if _, exists := userBeliefs[beliefKey]; exists {
							matchCount++
							break
						}
					}
				}
			}
		}
		if matchCount == len(categories) {
			return "visible"
		}
		return "hidden"
	}
	return "visible"
}

// applyVisibilityWrapper wraps content based on visibility state
func (s *FragmentService) applyVisibilityWrapper(htmlContent, visibility string) string {
	switch visibility {
	case "hidden":
		return fmt.Sprintf(`<div style="display: none;" class="belief-hidden">%s</div>`, htmlContent)
	case "visible":
		return htmlContent
	default:
		return htmlContent
	}
}

// extractDependencies extracts content dependencies for cache invalidation
func (s *FragmentService) extractDependencies(pane *content.PaneNode) []string {
	dependencies := []string{pane.ID}

	if pane.OptionsPayload != nil {
		if deps, ok := pane.OptionsPayload["dependencies"].([]interface{}); ok {
			for _, dep := range deps {
				if depStr, ok := dep.(string); ok {
					dependencies = append(dependencies, depStr)
				}
			}
		}
	}

	return dependencies
}

// convertTypesToDomainRegistry converts cache types to domain entity
func (s *FragmentService) convertTypesToDomainRegistry(typesRegistry *types.StoryfragmentBeliefRegistry) *beliefs.BeliefRegistry {
	registry := beliefs.NewBeliefRegistry(typesRegistry.StoryfragmentID)

	for paneID, typesBeliefData := range typesRegistry.PaneBeliefPayloads {
		beliefData := &beliefs.PaneBeliefData{
			PaneID:       paneID,
			BeliefMode:   "neutral",
			RequiredData: make(map[string]interface{}),
		}

		if len(typesBeliefData.HeldBeliefs) > 0 {
			beliefData.BeliefMode = "held"
			beliefData.RequiredData["beliefs"] = s.convertMapToSlice(typesBeliefData.HeldBeliefs)
		}

		if len(typesBeliefData.WithheldBeliefs) > 0 {
			beliefData.BeliefMode = "withheld"
			beliefData.RequiredData["beliefs"] = s.convertMapToSlice(typesBeliefData.WithheldBeliefs)
		}

		if len(typesBeliefData.MatchAcross) > 0 {
			beliefData.BeliefMode = "match-across"
			beliefData.RequiredData["categories"] = []interface{}{typesBeliefData.MatchAcross}
		}

		registry.AddPaneBeliefData(paneID, beliefData)
	}

	for paneID, beliefs := range typesRegistry.PaneWidgetBeliefs {
		registry.AddPaneWidgetBeliefs(paneID, beliefs)
	}

	return registry
}

// convertDomainToTypesRegistry converts domain entity to cache types
func (s *FragmentService) convertDomainToTypesRegistry(registry *beliefs.BeliefRegistry) *types.StoryfragmentBeliefRegistry {
	typesRegistry := &types.StoryfragmentBeliefRegistry{
		StoryfragmentID:    registry.StoryfragmentID,
		PaneBeliefPayloads: make(map[string]types.PaneBeliefData),
		RequiredBeliefs:    make(map[string]bool),
		RequiredBadges:     []string{},
		PaneWidgetBeliefs:  make(map[string][]string),
		AllWidgetBeliefs:   make(map[string]bool),
		LastUpdated:        registry.LastUpdated,
	}

	for paneID, beliefData := range registry.PaneBeliefPayloads {
		typesBeliefData := types.PaneBeliefData{
			HeldBeliefs:     make(map[string][]string),
			WithheldBeliefs: make(map[string][]string),
			MatchAcross:     []string{},
			LinkedBeliefs:   []string{},
			HeldBadges:      []string{},
		}

		switch beliefData.BeliefMode {
		case "held":
			if beliefs, ok := beliefData.RequiredData["beliefs"].([]interface{}); ok {
				for _, belief := range beliefs {
					if beliefStr, ok := belief.(string); ok {
						typesBeliefData.HeldBeliefs[beliefStr] = []string{beliefStr}
						typesRegistry.RequiredBeliefs[beliefStr] = true
					}
				}
			}
		case "withheld":
			if beliefs, ok := beliefData.RequiredData["beliefs"].([]interface{}); ok {
				for _, belief := range beliefs {
					if beliefStr, ok := belief.(string); ok {
						typesBeliefData.WithheldBeliefs[beliefStr] = []string{beliefStr}
						typesRegistry.RequiredBeliefs[beliefStr] = true
					}
				}
			}
		case "match-across":
			if categories, ok := beliefData.RequiredData["categories"].([]interface{}); ok {
				for _, category := range categories {
					if categorySlice, ok := category.([]string); ok {
						typesBeliefData.MatchAcross = append(typesBeliefData.MatchAcross, categorySlice...)
						for _, belief := range categorySlice {
							typesRegistry.RequiredBeliefs[belief] = true
						}
					}
				}
			}
		}

		typesRegistry.PaneBeliefPayloads[paneID] = typesBeliefData
	}

	for paneID, beliefs := range registry.PaneWidgetBeliefs {
		typesRegistry.PaneWidgetBeliefs[paneID] = beliefs
		for _, belief := range beliefs {
			typesRegistry.AllWidgetBeliefs[belief] = true
		}
	}

	return typesRegistry
}

// convertMapToSlice converts map[string][]string keys to []interface{}
func (s *FragmentService) convertMapToSlice(beliefMap map[string][]string) []interface{} {
	var result []interface{}
	for key := range beliefMap {
		result = append(result, key)
	}
	return result
}

// convertStringMapToInterface converts map[string][]string to map[string]interface{}
func (s *FragmentService) convertStringMapToInterface(input map[string][]string) map[string]interface{} {
	if input == nil {
		return nil
	}
	result := make(map[string]interface{})
	for k, v := range input {
		result[k] = v
	}
	return result
}

// convertStringMapToInterfaceMap converts map[string]string to map[string]interface{}
func (s *FragmentService) convertStringMapToInterfaceMap(input map[string]string) map[string]interface{} {
	if input == nil {
		return nil
	}
	result := make(map[string]interface{})
	for k, v := range input {
		result[k] = v
	}
	return result
}
