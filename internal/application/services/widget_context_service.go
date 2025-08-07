package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/beliefs"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/widgets"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// WidgetContextService manages widget context resolution and pre-processing
type WidgetContextService struct {
	sessionBeliefService *SessionBeliefService
}

// NewWidgetContextService creates a new widget context service
func NewWidgetContextService(sessionBeliefService *SessionBeliefService) *WidgetContextService {
	return &WidgetContextService{
		sessionBeliefService: sessionBeliefService,
	}
}

// BuildWidgetContext creates pre-resolved widget context for template rendering
func (s *WidgetContextService) BuildWidgetContext(
	tenantCtx *tenant.Context,
	sessionID, storyfragmentID string,
	beliefRegistry *beliefs.BeliefRegistry,
) (*widgets.WidgetContext, error) {
	// Create base widget context
	widgetCtx := widgets.NewWidgetContext(sessionID, storyfragmentID)

	// Get session belief context
	sessionBeliefCtx, err := s.sessionBeliefService.GetSessionBeliefContext(tenantCtx, sessionID, storyfragmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session belief context: %w", err)
	}

	// If no session context exists, create one
	if sessionBeliefCtx == nil && sessionID != "" {
		sessionBeliefCtx, err = s.sessionBeliefService.CreateSessionBeliefContext(tenantCtx, sessionID, storyfragmentID)
		if err != nil {
			return nil, fmt.Errorf("failed to create session belief context: %w", err)
		}
	}

	// Update widget context from session
	if sessionBeliefCtx != nil {
		widgetCtx.UpdateFromSessionContext(sessionBeliefCtx)
	}

	// Build widget states from belief registry
	if beliefRegistry != nil {
		s.buildWidgetStatesFromRegistry(widgetCtx, beliefRegistry)
	}

	return widgetCtx, nil
}

// buildWidgetStatesFromRegistry populates widget states based on registry
func (s *WidgetContextService) buildWidgetStatesFromRegistry(
	widgetCtx *widgets.WidgetContext,
	beliefRegistry *beliefs.BeliefRegistry,
) {
	// Process pane widget mappings
	for paneID, beliefKeys := range beliefRegistry.PaneWidgetBeliefs {
		widgetCtx.AddPaneWidgetMapping(paneID, beliefKeys)

		// Create widget states for each belief key
		for _, beliefKey := range beliefKeys {
			widgetID := fmt.Sprintf("%s_%s", paneID, beliefKey)
			widgetState := widgets.NewWidgetState(widgetID, "belief", beliefKey)

			// Set current value if user has this belief
			if beliefValues, exists := widgetCtx.UserBeliefs[beliefKey]; exists {
				widgetState.UpdateCurrentValue(beliefValues)
				widgetState.SetVisibility("visible")
			} else {
				widgetState.SetVisibility("default")
			}

			widgetCtx.AddWidgetState(widgetState)
		}
	}
}

// ShouldBypassCacheForWidgets determines if cache should be bypassed for widget personalization
func (s *WidgetContextService) ShouldBypassCacheForWidgets(
	tenantCtx *tenant.Context,
	paneID, sessionID, storyfragmentID string,
	beliefRegistry *beliefs.BeliefRegistry,
) (bool, error) {
	// No bypass needed if no session
	if sessionID == "" || storyfragmentID == "" {
		return false, nil
	}

	// No bypass needed if pane has no widgets
	if beliefRegistry == nil {
		return false, nil
	}

	widgetBeliefs, hasWidgets := beliefRegistry.GetPaneWidgetBeliefs(paneID)
	if !hasWidgets || len(widgetBeliefs) == 0 {
		return false, nil
	}

	// Get user beliefs
	userBeliefs, err := s.sessionBeliefService.GetUserBeliefs(tenantCtx, sessionID)
	if err != nil {
		return false, fmt.Errorf("failed to get user beliefs: %w", err)
	}

	// Check for intersection between user beliefs and widget requirements
	hasIntersection := s.sessionBeliefService.HasBeliefIntersection(userBeliefs, widgetBeliefs)

	return hasIntersection, nil
}

// PreResolveWidgetData pre-resolves all widget data to eliminate template cache calls
func (s *WidgetContextService) PreResolveWidgetData(
	tenantCtx *tenant.Context,
	sessionID, storyfragmentID string,
	paneIDs []string,
	beliefRegistry *beliefs.BeliefRegistry,
) (*widgets.WidgetContext, error) {
	// Build complete widget context
	widgetCtx, err := s.BuildWidgetContext(tenantCtx, sessionID, storyfragmentID, beliefRegistry)
	if err != nil {
		return nil, fmt.Errorf("failed to build widget context: %w", err)
	}

	// Pre-resolve data for specific panes
	for _, paneID := range paneIDs {
		s.preResolvePaneWidgets(widgetCtx, paneID, beliefRegistry)
	}

	return widgetCtx, nil
}

// preResolvePaneWidgets pre-resolves widget data for a specific pane
func (s *WidgetContextService) preResolvePaneWidgets(
	widgetCtx *widgets.WidgetContext,
	paneID string,
	beliefRegistry *beliefs.BeliefRegistry,
) {
	if beliefRegistry == nil {
		return
	}

	// Get widget beliefs for this pane
	widgetBeliefs, exists := beliefRegistry.GetPaneWidgetBeliefs(paneID)
	if !exists {
		return
	}

	// Ensure widget context has mappings for this pane
	widgetCtx.AddPaneWidgetMapping(paneID, widgetBeliefs)

	// Pre-resolve each widget's data
	for _, beliefKey := range widgetBeliefs {
		widgetID := fmt.Sprintf("%s_%s", paneID, beliefKey)

		// Get or create widget state
		widgetState, exists := widgetCtx.GetWidgetState(widgetID)
		if !exists {
			widgetState = widgets.NewWidgetState(widgetID, "belief", beliefKey)
			widgetCtx.AddWidgetState(widgetState)
		}

		// Update with current belief values
		if beliefValues := widgetCtx.GetBeliefValues(beliefKey); len(beliefValues) > 0 {
			widgetState.UpdateCurrentValue(beliefValues)
			widgetState.SetVisibility("visible")
		} else {
			widgetState.SetVisibility("default")
		}
	}
}
