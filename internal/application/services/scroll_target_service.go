package services

import (
	"slices"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// ScrollTargetService handles scroll target calculation for UNSET buttons
type ScrollTargetService struct{}

// NewScrollTargetService creates a new scroll target service
func NewScrollTargetService() *ScrollTargetService {
	return &ScrollTargetService{}
}

// FindScrollTargetPane finds the first pane with widgets for any of the beliefs being unset
func (s *ScrollTargetService) FindScrollTargetPane(
	beliefIDs []string,
	beliefRegistry *types.StoryfragmentBeliefRegistry,
) string {
	if beliefRegistry == nil || len(beliefIDs) == 0 {
		return ""
	}

	// Find pane that has widgets for any of the beliefs being unset
	for paneID, widgetBeliefs := range beliefRegistry.PaneWidgetBeliefs {
		for _, widgetBelief := range widgetBeliefs {
			if slices.Contains(beliefIDs, widgetBelief) {
				return paneID // First match wins
			}
		}
	}

	return "" // No scroll target found
}
