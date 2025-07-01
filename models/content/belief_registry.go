package content

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// BeliefRegistryService handles storyfragment belief registry operations
type BeliefRegistryService struct {
	ctx *tenant.Context
}

// NewBeliefRegistryService creates a new belief registry service
func NewBeliefRegistryService(ctx *tenant.Context) *BeliefRegistryService {
	return &BeliefRegistryService{ctx: ctx}
}

// ExtractAndCacheBeliefRegistry extracts beliefs from storyfragment panes and caches the registry
func (brs *BeliefRegistryService) ExtractAndCacheBeliefRegistry(storyfragmentID string, paneIDs []string) (*models.StoryfragmentBeliefRegistry, error) {
	// Check if registry already exists in cache
	if registry, found := brs.getFromCache(storyfragmentID); found {
		return registry, nil
	}

	// Extract beliefs from all panes
	registry, err := brs.buildRegistryFromPanes(storyfragmentID, paneIDs)
	if err != nil {
		return nil, err
	}

	// Cache the registry
	brs.setInCache(storyfragmentID, registry)

	return registry, nil
}

// buildRegistryFromPanes constructs belief registry by examining all panes
func (brs *BeliefRegistryService) buildRegistryFromPanes(storyfragmentID string, paneIDs []string) (*models.StoryfragmentBeliefRegistry, error) {
	paneService := NewPaneService(brs.ctx, nil)

	registry := &models.StoryfragmentBeliefRegistry{
		StoryfragmentID:    storyfragmentID,
		PaneBeliefPayloads: make(map[string]models.PaneBeliefData),
		RequiredBeliefs:    make(map[string]bool),
		RequiredBadges:     []string{},
		LastUpdated:        time.Now(),
	}

	for _, paneID := range paneIDs {
		paneNode, err := paneService.GetByID(paneID)
		if err != nil {
			return nil, err
		}
		if paneNode == nil {
			continue // Skip missing panes
		}

		// Extract belief data from this pane
		paneBeliefData := brs.extractPaneBeliefData(paneNode)

		// Only store if pane has belief requirements
		if brs.hasBeliefRequirements(paneBeliefData) {
			registry.PaneBeliefPayloads[paneID] = paneBeliefData

			// Add to flat required beliefs list
			brs.addToRequiredBeliefs(registry.RequiredBeliefs, paneBeliefData)
		}
	}

	return registry, nil
}

// extractPaneBeliefData converts PaneNode beliefs to PaneBeliefData format
func (brs *BeliefRegistryService) extractPaneBeliefData(paneNode *models.PaneNode) models.PaneBeliefData {
	data := models.PaneBeliefData{
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
func (brs *BeliefRegistryService) hasBeliefRequirements(data models.PaneBeliefData) bool {
	return len(data.HeldBeliefs) > 0 ||
		len(data.WithheldBeliefs) > 0 ||
		len(data.MatchAcross) > 0 ||
		len(data.LinkedBeliefs) > 0 ||
		len(data.HeldBadges) > 0
}

// addToRequiredBeliefs adds belief slugs to the flat required list
func (brs *BeliefRegistryService) addToRequiredBeliefs(required map[string]bool, data models.PaneBeliefData) {
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

// Cache operations
func (brs *BeliefRegistryService) getFromCache(storyfragmentID string) (*models.StoryfragmentBeliefRegistry, bool) {
	return cache.GetGlobalManager().GetStoryfragmentBeliefRegistry(brs.ctx.TenantID, storyfragmentID)
}

func (brs *BeliefRegistryService) setInCache(storyfragmentID string, registry *models.StoryfragmentBeliefRegistry) {
	cache.GetGlobalManager().SetStoryfragmentBeliefRegistry(brs.ctx.TenantID, registry)
}
