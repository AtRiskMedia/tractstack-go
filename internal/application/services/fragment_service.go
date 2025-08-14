// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/beliefs"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/widgets"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/templates"
)

// FragmentService orchestrates fragment generation with personalization
type FragmentService struct {
	widgetContextService    *WidgetContextService
	sessionBeliefService    *SessionBeliefService
	beliefEvaluationService *BeliefEvaluationService
	perfTracker             *performance.Tracker
	logger                  *logging.ChanneledLogger
	buttonRenderer          *templates.UnsetButtonRenderer
	scrollTargetSvc         *ScrollTargetService
}

// NewFragmentService creates a new fragment service
func NewFragmentService(
	widgetContextService *WidgetContextService,
	sessionBeliefService *SessionBeliefService,
	beliefEvaluationService *BeliefEvaluationService,
	perfTracker *performance.Tracker,
	logger *logging.ChanneledLogger,
	buttonRenderer *templates.UnsetButtonRenderer,
	scrollTargetSvc *ScrollTargetService,
) *FragmentService {
	return &FragmentService{
		widgetContextService:    widgetContextService,
		sessionBeliefService:    sessionBeliefService,
		beliefEvaluationService: beliefEvaluationService,
		perfTracker:             perfTracker,
		logger:                  logger,
		buttonRenderer:          buttonRenderer,
		scrollTargetSvc:         scrollTargetSvc,
	}
}

func (s *FragmentService) GenerateFragment(
	tenantCtx *tenant.Context,
	paneID, sessionID, storyfragmentID string,
) (string, error) {
	s.logger.Content().Debug("🔍 GENERATE FRAGMENT START", "paneId", paneID, "sessionId", sessionID)

	// Step 1: Load the essential static data for the pane.
	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindByID(tenantCtx.TenantID, paneID)
	if err != nil || pane == nil {
		return "", fmt.Errorf("pane %s not found or failed to load: %w", paneID, err)
	}

	cacheManager := tenantCtx.CacheManager
	beliefRegistry, hasRegistry := cacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID)

	// Step 2: Get the user's current beliefs. This is our personalization trigger.
	userBeliefs, _ := s.sessionBeliefService.GetUserBeliefs(tenantCtx, sessionID)
	hasBeliefs := len(userBeliefs) > 0

	// Step 3: Determine if this specific pane has ANY belief-related logic.
	_, hasPaneBeliefs := beliefRegistry.PaneBeliefPayloads[paneID]

	var htmlContent string

	// Step 4: The definitive decision.
	// Regenerate HTML if the user has beliefs AND this pane uses beliefs in any way.
	if hasBeliefs && hasPaneBeliefs {
		s.logger.Content().Debug("Personalization required: User has beliefs and pane has belief logic. Generating fresh HTML.", "paneId", paneID)
		htmlContent, err = s.generateFreshHTML(tenantCtx, pane, sessionID, storyfragmentID, beliefRegistry)
		if err != nil {
			return "", fmt.Errorf("failed to generate fresh HTML for session: %w", err)
		}
	} else {
		// Otherwise, it's safe to use the fast, cached, non-personalized version.
		s.logger.Content().Debug("No personalization required. Using cached base HTML.", "paneId", paneID)
		htmlContent, err = s.getCachedOrGenerateHTML(tenantCtx, pane)
		if err != nil {
			return "", fmt.Errorf("failed to get cached HTML: %w", err)
		}
	}

	// Finally, apply the visibility wrapper (using the already-corrected logic).
	if hasRegistry {
		htmlContent = s.applyBeliefVisibility(tenantCtx, htmlContent, paneID, sessionID, storyfragmentID, beliefRegistry)
	}

	return htmlContent, nil
}

// GenerateFragmentBatch creates HTML fragments for multiple panes efficiently
func (s *FragmentService) GenerateFragmentBatch(
	tenantCtx *tenant.Context,
	paneIDs []string,
	sessionID, storyfragmentID string,
) (map[string]string, map[string]string, error) {
	s.logger.Content().Debug("🔍 BATCH GENERATE START", "paneCount", len(paneIDs), "sessionId", sessionID, "storyfragmentId", storyfragmentID)

	results := make(map[string]string)
	errors := make(map[string]string)

	// Get belief registry once for all panes
	cacheManager := tenantCtx.CacheManager
	beliefRegistry, hasRegistry := cacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID)

	s.logger.Content().Debug("🔍 BATCH BELIEF REGISTRY",
		"hasRegistry", hasRegistry,
		"registryPaneCount", func() int {
			if hasRegistry {
				return len(beliefRegistry.PaneBeliefPayloads)
			}
			return 0
		}())

	// Pre-resolve widget context for batch efficiency
	var widgetCtx *widgets.WidgetContext
	if hasRegistry && sessionID != "" && s.widgetContextService != nil {
		domainRegistry := s.buildDomainRegistry(beliefRegistry)
		widgetCtx, _ = s.widgetContextService.PreResolveWidgetData(
			tenantCtx, sessionID, storyfragmentID, paneIDs, domainRegistry,
		)
		s.logger.Content().Debug("🔍 BATCH WIDGET CONTEXT RESOLVED", "sessionId", sessionID)
	}

	// Process each pane
	for _, paneID := range paneIDs {
		html, err := s.generateSingleFragment(
			tenantCtx, paneID, sessionID, storyfragmentID, beliefRegistry, widgetCtx,
		)
		if err != nil {
			errors[paneID] = err.Error()
			continue
		}
		results[paneID] = html
	}

	s.logger.Content().Debug("🔍 BATCH GENERATE COMPLETE", "successCount", len(results), "errorCount", len(errors))
	return results, errors, nil
}

// generateSingleFragment handles individual pane generation within batch
func (s *FragmentService) generateSingleFragment(
	tenantCtx *tenant.Context,
	paneID, sessionID, storyfragmentID string,
	beliefRegistry *types.StoryfragmentBeliefRegistry,
	widgetCtx *widgets.WidgetContext,
) (string, error) {
	// Load pane
	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindByID(tenantCtx.TenantID, paneID)
	if err != nil {
		return "", fmt.Errorf("failed to load pane: %w", err)
	}
	if pane == nil {
		return "", fmt.Errorf("pane not found")
	}

	var htmlContent string

	// Check if we should use pre-resolved widget context
	if widgetCtx != nil && s.shouldBypassCacheForWidgets(tenantCtx, paneID, sessionID, beliefRegistry) {
		s.logger.Content().Debug("🔍 BATCH USING FRESH HTML", "paneId", paneID)
		// Generate fresh HTML with widgets
		htmlContent = s.generateFreshHTMLWithWidgets(tenantCtx, pane, sessionID, storyfragmentID, widgetCtx)
	} else {
		s.logger.Content().Debug("🔍 BATCH USING CACHED HTML", "paneId", paneID)
		// Use cached approach
		htmlContent, err = s.getCachedOrGenerateHTML(tenantCtx, pane)
		if err != nil {
			return "", fmt.Errorf("failed to get cached HTML: %w", err)
		}
	}

	// Apply belief visibility wrapper
	if beliefRegistry != nil {
		htmlContent = s.applyBeliefVisibility(tenantCtx, htmlContent, paneID, sessionID, storyfragmentID, beliefRegistry)
	}

	return htmlContent, nil
}

// shouldBypassCacheForWidgets determines if cache should be bypassed for widget personalization
func (s *FragmentService) shouldBypassCacheForWidgets(
	tenantCtx *tenant.Context,
	paneID, sessionID string,
	beliefRegistry *types.StoryfragmentBeliefRegistry,
) bool {
	s.logger.Content().Debug("🔍 CACHE BYPASS CHECK", "paneId", paneID, "sessionId", sessionID)

	if sessionID == "" || beliefRegistry == nil {
		s.logger.Content().Debug("🔍 CACHE BYPASS = FALSE", "reason", "no session or registry", "paneId", paneID)
		return false
	}

	// Check if pane has widgets
	widgetBeliefs, hasWidgets := beliefRegistry.PaneWidgetBeliefs[paneID]
	if !hasWidgets || len(widgetBeliefs) == 0 {
		s.logger.Content().Debug("🔍 CACHE BYPASS = FALSE", "reason", "no widgets", "paneId", paneID)
		return false
	}

	s.logger.Content().Debug("🔍 PANE HAS WIDGETS", "paneId", paneID, "widgetBeliefs", widgetBeliefs)

	// Check if user has beliefs that intersect with widgets
	if s.sessionBeliefService != nil {
		userBeliefs, _ := s.sessionBeliefService.GetUserBeliefs(tenantCtx, sessionID)
		s.logger.Content().Debug("🔍 USER BELIEFS FOR WIDGET CHECK",
			"sessionId", sessionID,
			"userBeliefsCount", len(userBeliefs),
			"userBeliefs", userBeliefs)

		if userBeliefs != nil {
			for _, widgetBelief := range widgetBeliefs {
				if _, hasUserBelief := userBeliefs[widgetBelief]; hasUserBelief {
					s.logger.Content().Debug("🔍 CACHE BYPASS = TRUE", "reason", "user has widget belief", "belief", widgetBelief, "paneId", paneID)
					return true
				}
			}
		}
	}

	s.logger.Content().Debug("🔍 CACHE BYPASS = FALSE", "reason", "no belief intersection", "paneId", paneID)
	return false
}

// getCachedOrGenerateHTML handles cache-first HTML generation
func (s *FragmentService) getCachedOrGenerateHTML(tenantCtx *tenant.Context, pane *content.PaneNode) (string, error) {
	cacheManager := tenantCtx.CacheManager
	variant := types.PaneVariant{
		BeliefMode:      "default",
		HeldBeliefs:     []string{},
		WithheldBeliefs: []string{},
	}

	s.logger.Content().Debug("🔍 CACHE LOOKUP", "paneId", pane.ID, "variant", "default")

	// Try cache first
	if cachedHTML, exists := cacheManager.GetHTMLChunk(tenantCtx.TenantID, pane.ID, variant); exists {
		s.logger.Content().Debug("🔍 CACHE HIT", "paneId", pane.ID, "htmlLength", len(cachedHTML.HTML))
		return cachedHTML.HTML, nil
	}

	s.logger.Content().Debug("🔍 CACHE MISS - GENERATING", "paneId", pane.ID)

	// Generate fresh HTML and cache it
	htmlContent, err := s.generateBaseHTML(tenantCtx, pane)
	if err != nil {
		return "", err
	}

	s.logger.Content().Debug("🔍 BASE HTML GENERATED", "paneId", pane.ID, "htmlLength", len(htmlContent))

	// Cache the generated HTML
	dependencies := []string{pane.ID}
	if pane.OptionsPayload != nil {
		if deps, ok := pane.OptionsPayload["dependencies"].([]any); ok {
			for _, dep := range deps {
				if depStr, ok := dep.(string); ok {
					dependencies = append(dependencies, depStr)
				}
			}
		}
	}

	cacheManager.SetHTMLChunk(tenantCtx.TenantID, pane.ID, variant, htmlContent, dependencies)
	s.logger.Content().Debug("🔍 HTML CACHED", "paneId", pane.ID, "dependencies", dependencies)

	return htmlContent, nil
}

// generateFreshHTML generates HTML with widget personalization context
func (s *FragmentService) generateFreshHTML(
	tenantCtx *tenant.Context,
	pane *content.PaneNode,
	sessionID, storyfragmentID string,
	beliefRegistry *types.StoryfragmentBeliefRegistry,
) (string, error) {
	s.logger.Content().Debug("🔍 GENERATING FRESH HTML WITH WIDGETS", "paneId", pane.ID)

	// Build widget context for this specific pane
	var widgetCtx *widgets.WidgetContext
	if s.widgetContextService != nil {
		domainRegistry := s.buildDomainRegistry(beliefRegistry)
		var err error
		widgetCtx, err = s.widgetContextService.BuildWidgetContext(
			tenantCtx, sessionID, storyfragmentID, domainRegistry,
		)
		if err != nil {
			return "", fmt.Errorf("failed to build widget context: %w", err)
		}
	}

	return s.generateFreshHTMLWithWidgets(tenantCtx, pane, sessionID, storyfragmentID, widgetCtx), nil
}

// generateFreshHTMLWithWidgets generates HTML with pre-built widget context
func (s *FragmentService) generateFreshHTMLWithWidgets(
	tenantCtx *tenant.Context,
	pane *content.PaneNode,
	sessionID, storyfragmentID string,
	widgetCtx *widgets.WidgetContext,
) string {
	nodesData, parentChildMap, err := templates.ExtractNodesFromPane(pane)
	if err != nil {
		s.logger.Content().Error("Failed to extract nodes", "error", err.Error(), "paneId", pane.ID)
		return fmt.Sprintf(`<div>Error extracting nodes for pane %s</div>`, pane.ID)
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
		SessionID:        sessionID,
		StoryfragmentID:  storyfragmentID,
		ContainingPaneID: pane.ID,
		WidgetContext:    widgetCtx,
	}

	generator := templates.NewGenerator(renderCtx)
	return generator.RenderPaneFragment(pane.ID)
}

// generateBaseHTML creates non-personalized HTML for caching
func (s *FragmentService) generateBaseHTML(tenantCtx *tenant.Context, pane *content.PaneNode) (string, error) {
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

// applyBeliefVisibility applies belief-based visibility wrapper to HTML content
func (s *FragmentService) applyBeliefVisibility(
	tenantCtx *tenant.Context,
	htmlContent string,
	paneID, sessionID, storyfragmentID string,
	beliefRegistry *types.StoryfragmentBeliefRegistry,
) string {
	paneBeliefs, hasPaneBeliefs := beliefRegistry.PaneBeliefPayloads[paneID]
	if !hasPaneBeliefs {
		return htmlContent // No belief requirements = always visible
	}

	// Use a safe default of empty beliefs
	userBeliefs := make(map[string][]string)

	// If a session exists, create a fresh context to get the latest beliefs.
	if sessionID != "" {
		newContext, err := s.sessionBeliefService.CreateSessionBeliefContext(tenantCtx, sessionID, storyfragmentID)
		if err != nil {
			s.logger.Content().Error("Failed to create fresh session context for visibility", "error", err)
		} else if newContext != nil {
			userBeliefs = newContext.UserBeliefs
		}
	}

	visibility := s.beliefEvaluationService.EvaluatePaneVisibility(paneBeliefs, userBeliefs)
	result := s.applyVisibilityWrapper(htmlContent, visibility)

	// ... (rest of the function for the 'unset button' logic remains the same) ...

	if visibility == "visible" && len(userBeliefs) > 0 {
		hasRequirements := len(paneBeliefs.HeldBeliefs) > 0 ||
			len(paneBeliefs.WithheldBeliefs) > 0

		if hasRequirements {
			effectiveFilter := s.beliefEvaluationService.CalculateEffectiveFilter(paneBeliefs, userBeliefs)
			if len(effectiveFilter) > 0 {
				beliefsToUnset := s.beliefEvaluationService.ExtractBeliefsToUnset(effectiveFilter)
				if len(beliefsToUnset) > 0 {
					gotoPaneID := s.scrollTargetSvc.FindScrollTargetPane(beliefsToUnset, beliefRegistry)
					buttonHTML := s.buttonRenderer.RenderUnsetButton(paneID, beliefsToUnset, gotoPaneID)
					result = s.buttonRenderer.InjectButtonIntoHTML(result, buttonHTML)
				}
			}
		}
	}

	return result
}

// applyVisibilityWrapper wraps content based on visibility state
func (s *FragmentService) applyVisibilityWrapper(htmlContent, visibility string) string {
	switch visibility {
	case "visible":
		return htmlContent
	case "hidden":
		// Use legacy-compatible wrapper with !important specificity
		return fmt.Sprintf(`<div style="display:none !important;">%s</div>`, htmlContent)
	case "empty":
		// Support for future heldBadges feature
		return `<div style="display:none !important;"></div>`
	default:
		return htmlContent
	}
}

// buildDomainRegistry converts types registry to domain entity for widget service compatibility
func (s *FragmentService) buildDomainRegistry(typesRegistry *types.StoryfragmentBeliefRegistry) *beliefs.BeliefRegistry {
	if typesRegistry == nil {
		return nil
	}

	registry := beliefs.NewBeliefRegistry(typesRegistry.StoryfragmentID)

	// Add widget beliefs (this is all we need for widget context)
	for paneID, widgetBeliefs := range typesRegistry.PaneWidgetBeliefs {
		registry.AddPaneWidgetBeliefs(paneID, widgetBeliefs)
	}

	return registry
}

// convertStringMapToInterface converts map[string][]string to map[string]any
func (s *FragmentService) convertStringMapToInterface(input map[string][]string) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range input {
		result[k] = v
	}
	return result
}

// convertStringMapToInterfaceMap converts map[string]string to map[string]any
func (s *FragmentService) convertStringMapToInterfaceMap(input map[string]string) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range input {
		result[k] = v
	}
	return result
}

// GenerateHTMLFromPayload generates HTML directly from OptionsPayload without database persistence
// This bypasses all belief-based visibility and personalization for preview purposes
func (s *FragmentService) GenerateHTMLFromPayload(tenantCtx *tenant.Context, paneID string, optionsPayload map[string]any) (string, error) {
	// Extract pane-level fields from optionsPayload
	var bgColour *string
	var isDecorative bool

	if bg, ok := optionsPayload["bgColour"].(string); ok {
		bgColour = &bg
	}

	if deco, ok := optionsPayload["isDecorative"].(bool); ok {
		isDecorative = deco
	}

	// Create temporary pane node from payload using the CORRECT pane ID
	tempPane := &content.PaneNode{
		ID:              paneID, // <-- Use the actual pane ID from the request!
		Title:           "Preview Pane",
		Slug:            "preview-temp",
		BgColour:        bgColour,
		IsDecorative:    isDecorative,
		HeldBeliefs:     make(map[string][]string), // Empty - no belief filtering
		WithheldBeliefs: make(map[string][]string), // Empty - no belief filtering
		OptionsPayload:  optionsPayload,
	}

	// Generate HTML directly without personalization or caching
	return s.generateBaseHTML(tenantCtx, tempPane)
}
