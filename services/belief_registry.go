// Package services
package services

import (
	"log"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/html"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
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

func (brs *BeliefRegistryService) scanPaneForWidgetBeliefs(paneID string, paneNode *models.PaneNode) ([]string, error) {
	// Use html package to extract nodes from pane
	nodesData, _, err := html.ExtractNodesFromPane(paneNode)
	if err != nil {
		return nil, err
	}

	var widgetBeliefs []string

	// Scan all nodes for code widgets
	for _, nodeData := range nodesData {
		// Look for TagElement nodes with code tag
		if nodeData.NodeType == "TagElement" && nodeData.TagName != nil && *nodeData.TagName == "code" {
			// Check if node has codeHookParams AND copy field
			if nodeData.CustomData != nil {
				if params, exists := nodeData.CustomData["codeHookParams"]; exists {
					if paramsSlice, ok := params.([]string); ok && len(paramsSlice) > 0 {
						// Extract widget type from copy field (before the parentheses)
						copyText := getNodeCopy(nodeData)
						widgetType := extractWidgetTypeFromCopy(copyText)

						// Check if it's a belief widget type
						if widgetType == "belief" || widgetType == "toggle" || widgetType == "identifyAs" {
							// Belief slug is the first parameter
							beliefSlug := paramsSlice[0]
							if beliefSlug != "" {
								widgetBeliefs = append(widgetBeliefs, beliefSlug)
							}
						}
					}
				}
			}
		}
	}

	return widgetBeliefs, nil
}

// Helper to get copy from nodeData
func getNodeCopy(nodeData *models.NodeRenderData) string {
	if nodeData.Copy != nil {
		return *nodeData.Copy
	}
	return ""
}

// Extract widget type from copy field (e.g., "belief(..." -> "belief")
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

// BuildRegistryFromLoadedPanes constructs belief registry using already-loaded pane nodes
// This optimizes the process by avoiding duplicate database calls and JSON parsing
func (brs *BeliefRegistryService) BuildRegistryFromLoadedPanes(storyfragmentID string, loadedPanes []*models.PaneNode) (*models.StoryfragmentBeliefRegistry, error) {
	// Check if registry already exists in cache
	if registry, found := brs.getFromCache(storyfragmentID); found {
		// log.Printf("CACHE HIT: Registry for storyfragment %s found in cache, skipping rebuild", storyfragmentID)
		return registry, nil
	}

	// log.Printf("CACHE MISS: Building registry for storyfragment %s with %d loaded panes", storyfragmentID, len(loadedPanes))

	registry := &models.StoryfragmentBeliefRegistry{
		StoryfragmentID:    storyfragmentID,
		PaneBeliefPayloads: make(map[string]models.PaneBeliefData),
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

			// log.Printf("Found %d belief widgets in pane %s: %v", len(widgetBeliefs), paneID, widgetBeliefs)

			// Load widget beliefs into cache
			for _, beliefSlug := range widgetBeliefs {
				beliefService := content.NewBeliefService(brs.ctx, nil)
				_, err := beliefService.GetBySlug(beliefSlug)
				if err != nil {
					log.Printf("Failed to load widget belief into cache: %s - %v", beliefSlug, err)
					//} else {
					//	log.Printf("Loaded widget belief into cache: %s", beliefSlug)
				}
			}
		}
	}

	// Cache the registry
	brs.setInCache(storyfragmentID, registry)
	// log.Printf("CACHE SET: Registry for storyfragment %s cached successfully", storyfragmentID)

	return registry, nil
}
