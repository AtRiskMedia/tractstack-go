// Package warming provides comprehensive cache warming functionality for multi-tenant content management.
// This package handles the pre-loading and caching of critical content structures during application startup
// to ensure optimal performance for first-time visitors.
package warming

import (
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/api"
	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/html"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/services"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// WarmAllTenants warms critical content for all active tenants after pre-activation.
// This function ensures that home pages load quickly for first-time visitors by pre-loading
// and caching all necessary content structures, HTML fragments, and belief registries.
func WarmAllTenants(tenantManager *tenant.Manager) error {
	start := time.Now().UTC()

	// Get list of active tenants
	activeTenants, err := getActiveTenantList()
	if err != nil {
		return fmt.Errorf("failed to get active tenant list: %w", err)
	}

	if len(activeTenants) == 0 {
		log.Println("No active tenants found - skipping content warming")
		return nil
	}

	// Track warming results
	warmedCount := 0
	failedTenants := make([]string, 0)

	// Warm critical content for each active tenant
	for _, tenantID := range activeTenants {
		log.Printf("Warming content for tenant: '%s'", tenantID)

		if err := warmTenantContent(tenantID); err != nil {
			log.Printf("ERROR: Failed to warm content for tenant '%s': %v", tenantID, err)
			failedTenants = append(failedTenants, tenantID)
			continue
		}

		warmedCount++
		log.Printf("✓ Successfully warmed content for tenant: '%s'", tenantID)
	}

	// Report results
	elapsed := time.Since(start)
	log.Printf("=== Content warming complete in %v ===", elapsed)

	if len(failedTenants) > 0 {
		log.Printf("Failed tenants: %v", failedTenants)
		// Non-blocking: return error but don't fail startup
		return fmt.Errorf("content warming failed for %d tenants: %v", len(failedTenants), failedTenants)
	}

	return nil
}

// warmTenantContent performs comprehensive warming for a single tenant, including:
// - HOME storyfragment loading and caching
// - All associated panes bulk loading
// - Menu loading if present
// - Belief registry generation and caching
// - HTML fragment pre-generation and caching
// - Full content map generation using existing API function
func warmTenantContent(tenantID string) error {
	tenantStart := time.Now().UTC()

	// Load tenant configuration
	config, err := tenant.LoadTenantConfig(tenantID)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create database connection
	database, err := tenant.NewDatabase(config)
	if err != nil {
		return fmt.Errorf("failed to create database connection: %w", err)
	}
	defer database.Close()

	// Create tenant context for all services
	ctx := &tenant.Context{
		TenantID: tenantID,
		Config:   config,
		Database: database,
		Status:   "active",
	}

	// Initialize cache manager
	cacheManager := cache.GetGlobalManager()

	// === Phase 1: Warm HOME StoryFragment ===
	storyFragmentService := content.NewStoryFragmentService(ctx, cacheManager)

	homeSlug := config.HomeSlug
	if homeSlug == "" {
		homeSlug = "hello" // Default fallback
	}

	storyFragment, err := storyFragmentService.GetBySlug(homeSlug)
	if err != nil {
		return fmt.Errorf("failed to warm HOME storyfragment '%s': %w", homeSlug, err)
	}

	if storyFragment == nil {
		log.Printf("  - Warning: HOME storyfragment '%s' not found for tenant '%s'", homeSlug, tenantID)
		return nil
	}

	// === Phase 2: Warm All Associated Panes ===
	var loadedPanes []*models.PaneNode
	if len(storyFragment.PaneIDs) > 0 {
		paneService := content.NewPaneService(ctx, cacheManager)
		loadedPanes, err = paneService.GetByIDs(storyFragment.PaneIDs)
		if err != nil {
			return fmt.Errorf("failed to load panes: %w", err)
		}
	}

	// === Phase 3: Warm Menu and Attach to StoryFragment ===
	if storyFragment.MenuID != nil {
		menuService := content.NewMenuService(ctx, cacheManager)
		menuData, err := menuService.GetByID(*storyFragment.MenuID)
		if err != nil {
			log.Printf("  - Warning: Failed to load menu %s: %v", *storyFragment.MenuID, err)
		} else if menuData != nil {
			// ✅ NEW: Attach menu to storyfragment
			storyFragment.Menu = menuData
		}
	}

	// === Phase 4: Warm Belief Registry ===
	if len(loadedPanes) > 0 {
		beliefRegistryService := services.NewBeliefRegistryService(ctx)
		_, err := beliefRegistryService.BuildRegistryFromLoadedPanes(storyFragment.ID, loadedPanes)
		if err != nil {
			log.Printf("  - Warning: Failed to build belief registry: %v", err)
		} // else {
		// Verify it was actually cached
		//if _, found := cacheManager.GetStoryfragmentBeliefRegistry(tenantID, storyFragment.ID); found {
		//	log.Printf("  - ✅ Belief registry successfully cached and verified for %s", storyFragment.ID)
		//} else {
		//	log.Printf("  - ❌ ERROR: Belief registry failed to cache for %s", storyFragment.ID)
		//}
		//}
	}

	// === Phase 5: Generate and Cache HTML Fragments ===
	if len(loadedPanes) > 0 {
		fragmentsGenerated := 0
		for _, paneNode := range loadedPanes {
			if paneNode == nil {
				continue
			}

			// Extract and parse nodes from optionsPayload
			nodesData, parentChildMap, err := html.ExtractNodesFromPane(paneNode)
			if err != nil {
				log.Printf("  - Warning: Failed to parse pane %s nodes: %v", paneNode.ID, err)
				continue
			}

			// Add the pane itself to the nodes data structure
			paneNodeData := &models.NodeRenderData{
				ID:       paneNode.ID,
				NodeType: "Pane",
				PaneData: &models.PaneRenderData{
					Title:           paneNode.Title,
					Slug:            paneNode.Slug,
					IsDecorative:    paneNode.IsDecorative,
					BgColour:        extractBgColour(paneNode),
					HeldBeliefs:     paneNode.HeldBeliefs,
					WithheldBeliefs: paneNode.WithheldBeliefs,
					CodeHookTarget:  paneNode.CodeHookTarget,
					CodeHookPayload: paneNode.CodeHookPayload,
				},
			}
			nodesData[paneNode.ID] = paneNodeData

			// Create render context for warming (no session context needed)
			renderCtx := &models.RenderContext{
				AllNodes:         nodesData,
				ParentNodes:      parentChildMap,
				TenantID:         ctx.TenantID,
				SessionID:        "", // No session during warming
				StoryfragmentID:  storyFragment.ID,
				ContainingPaneID: paneNode.ID,
			}

			// Create HTML generator and generate fragment
			generator := html.NewGenerator(renderCtx)
			htmlContent := generator.Render(paneNode.ID)

			// Cache the generated HTML fragment
			variant := models.PaneVariantDefault
			cacheManager.SetHTMLChunk(tenantID, paneNode.ID, variant, htmlContent, []string{paneNode.ID})

			fragmentsGenerated++
		}
	}

	// === Phase 5.5: Extract and Attach CodeHook Targets ===
	if len(loadedPanes) > 0 {
		codeHookTargets := make(map[string]string)
		extractedCount := 0

		for _, paneNode := range loadedPanes {
			if paneNode != nil && paneNode.CodeHookTarget != nil && *paneNode.CodeHookTarget != "" {
				codeHookTargets[paneNode.ID] = *paneNode.CodeHookTarget
				extractedCount++
			}
		}
		storyFragment.CodeHookTargets = codeHookTargets
	}

	// === Phase 5.7: Re-cache the Enriched StoryFragment ===
	cacheManager.SetStoryFragment(tenantID, storyFragment)

	// === Phase 6: Warm TRACTSTACK_HOME_SLUG if Different ===
	if config.TractStackHomeSlug != "" && config.TractStackHomeSlug != homeSlug {
		_, err := storyFragmentService.GetBySlug(config.TractStackHomeSlug)
		if err != nil {
			log.Printf("  - Warning: Failed to warm TRACTSTACK HOME storyfragment '%s': %v",
				config.TractStackHomeSlug, err)
			//} else if tractStackStoryFragment != nil {
			//	log.Printf("  - TRACTSTACK HOME storyfragment '%s' warmed (ID: %s, %d panes)",
			//		config.TractStackHomeSlug, tractStackStoryFragment.ID, len(tractStackStoryFragment.PaneIDs))
		}
	}

	// === Phase 7: Warm Full Content Map using existing API function ===
	contentMap, err := api.BuildFullContentMapFromDB(ctx)
	if err != nil {
		log.Printf("  - Warning: Failed to build content map from database: %v", err)
	} else {
		// Cache the complete content map
		cacheManager.SetFullContentMap(tenantID, contentMap)
	}

	elapsed := time.Since(tenantStart)
	log.Printf("  - Tenant '%s' content warmed in %v", tenantID, elapsed)
	return nil
}

// extractBgColour extracts background colour from pane options payload.
func extractBgColour(paneNode *models.PaneNode) *string {
	if paneNode.OptionsPayload != nil {
		if bgColour, ok := paneNode.OptionsPayload["bgColour"]; ok {
			if bgColourStr, ok := bgColour.(string); ok {
				return &bgColourStr
			}
		}
	}
	return nil
}

// getActiveTenantList returns a list of all active tenant IDs from the registry.
func getActiveTenantList() ([]string, error) {
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load tenant registry: %w", err)
	}

	activeTenants := make([]string, 0)
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeTenants = append(activeTenants, tenantID)
		}
	}

	return activeTenants, nil
}
