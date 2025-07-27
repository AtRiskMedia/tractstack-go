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
func WarmAllTenants(tenantManager *tenant.Manager) error {
	start := time.Now().UTC()
	activeTenants, err := getActiveTenantList()
	if err != nil {
		return fmt.Errorf("failed to get active tenant list: %w", err)
	}
	if len(activeTenants) == 0 {
		log.Println("No active tenants found - skipping content warming")
		return nil
	}

	for _, tenantID := range activeTenants {
		log.Printf("Warming content for tenant: '%s'", tenantID)
		if err := warmTenantContent(tenantID); err != nil {
			log.Printf("ERROR: Failed to warm content for tenant '%s': %v", tenantID, err)
			continue // Continue to next tenant even if one fails
		}
		log.Printf("✓ Successfully warmed content for tenant: '%s'", tenantID)
	}

	log.Printf("=== Content warming complete in %v ===", time.Since(start))
	return nil
}

// warmTenantContent performs comprehensive, discovery-first warming for a single tenant.
func warmTenantContent(tenantID string) error {
	tenantStart := time.Now().UTC()
	cacheManager := cache.GetGlobalManager()
	if cacheManager == nil {
		return fmt.Errorf("cache manager not available")
	}

	// Phase 0: Initialize Cache and DB Connection
	log.Printf("  - Initializing cache structures for tenant '%s'", tenantID)
	cacheManager.InitializeTenant(tenantID)
	log.Printf("  - ✅ Cache structures initialized for tenant '%s'", tenantID)

	config, err := tenant.LoadTenantConfig(tenantID)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	database, err := tenant.NewDatabase(config)
	if err != nil {
		return fmt.Errorf("failed to create database connection: %w", err)
	}
	defer database.Close()

	// Phase 1: Verify & Repair Database State
	log.Printf("  - Verifying and repairing initial database content for tenant '%s'", tenantID)
	if err := tenant.EnsureInitialContent(database); err != nil {
		return fmt.Errorf("failed to ensure initial db content: %w", err)
	}
	log.Printf("  - ✅ Initial database content verified for tenant '%s'", tenantID)

	ctx := &tenant.Context{
		TenantID: tenantID,
		Config:   config,
		Database: database,
		Status:   "active",
	}

	// Phase 2: Authoritative Content Discovery
	log.Printf("  - Building full content map for tenant '%s'", tenantID)
	fullContentMap, err := api.BuildFullContentMapFromDB(ctx)
	if err != nil {
		return fmt.Errorf("failed to build content map from database: %w", err)
	}
	cacheManager.SetFullContentMap(tenantID, fullContentMap)
	log.Printf("  - ✅ Successfully built and cached content map with %d items.", len(fullContentMap))

	// Phase 3: Load All Discovered Content into Cache
	log.Printf("  - Loading all discovered content into cache for tenant '%s'", tenantID)
	storyFragmentService := content.NewStoryFragmentService(ctx, cacheManager)
	paneService := content.NewPaneService(ctx, cacheManager)
	epinetService := content.NewEpinetService(ctx, cacheManager)
	menuService := content.NewMenuService(ctx, cacheManager)

	var allStoryFragmentIDs, allPaneIDs, allEpinetIDs, allMenuIDs []string
	for _, item := range fullContentMap {
		switch item.Type {
		case "StoryFragment":
			allStoryFragmentIDs = append(allStoryFragmentIDs, item.ID)
		case "Pane":
			allPaneIDs = append(allPaneIDs, item.ID)
		case "Epinet":
			allEpinetIDs = append(allEpinetIDs, item.ID)
		case "Menu":
			allMenuIDs = append(allMenuIDs, item.ID)
		}
	}

	// Bulk load all content types to populate the cache
	if len(allStoryFragmentIDs) > 0 {
		if _, err := storyFragmentService.GetByIDs(allStoryFragmentIDs); err != nil {
			log.Printf("  - Warning: failed to bulk warm StoryFragments: %v", err)
		}
	}
	if len(allPaneIDs) > 0 {
		if _, err := paneService.GetByIDs(allPaneIDs); err != nil {
			log.Printf("  - Warning: failed to bulk warm Panes: %v", err)
		}
	}
	if len(allEpinetIDs) > 0 {
		if _, err := epinetService.GetByIDs(allEpinetIDs); err != nil {
			log.Printf("  - Warning: failed to bulk warm Epinets: %v", err)
		}
	}
	if len(allMenuIDs) > 0 {
		if _, err := menuService.GetByIDs(allMenuIDs); err != nil {
			log.Printf("  - Warning: failed to bulk warm Menus: %v", err)
		}
	}
	log.Printf("  - ✅ All content items loaded into cache.")

	// Phase 4: Enrich StoryFragments with Computed Data
	// This is the critical logic that was previously deleted.
	log.Printf("  - Enriching cached StoryFragments with computed data...")
	for _, sfID := range allStoryFragmentIDs {
		storyFragment, found := cacheManager.GetStoryFragment(tenantID, sfID)
		if !found || storyFragment == nil {
			continue
		}

		// Enrich with Menu
		if storyFragment.MenuID != nil {
			if menu, found := cacheManager.GetMenu(tenantID, *storyFragment.MenuID); found {
				storyFragment.Menu = menu
			}
		}

		// Enrich with CodeHookTargets
		panes, _ := paneService.GetByIDs(storyFragment.PaneIDs)
		if len(panes) > 0 {
			codeHookTargets := make(map[string]string)
			for _, paneNode := range panes {
				if paneNode != nil && paneNode.CodeHookTarget != nil && *paneNode.CodeHookTarget != "" {
					codeHookTargets[paneNode.ID] = *paneNode.CodeHookTarget
				}
			}
			storyFragment.CodeHookTargets = codeHookTargets
		}

		// Re-cache the now fully enriched story fragment object.
		cacheManager.SetStoryFragment(tenantID, storyFragment)
	}
	log.Printf("  - ✅ StoryFragments enriched.")

	// Phase 5: Warm High-Priority Dependencies (HOME page HTML)
	homeSlug := config.HomeSlug
	if homeSlug == "" {
		homeSlug = "hello"
	}
	homeStoryFragment, _ := storyFragmentService.GetBySlug(homeSlug)
	if homeStoryFragment != nil {
		homePanes, _ := paneService.GetByIDs(homeStoryFragment.PaneIDs)
		log.Printf("  - ✅ Warmed HOME storyfragment '%s' and its %d panes.", homeSlug, len(homePanes))

		// Build and cache belief registry for home page
		if len(homePanes) > 0 {
			beliefRegistryService := services.NewBeliefRegistryService(ctx)
			if _, err := beliefRegistryService.BuildRegistryFromLoadedPanes(homeStoryFragment.ID, homePanes); err != nil {
				log.Printf("  - Warning: Failed to build belief registry for HOME: %v", err)
			}
		}

		// Generate HTML fragments for home page
		for _, paneNode := range homePanes {
			if paneNode != nil {
				warmHTMLFragmentForPane(ctx, paneNode, homeStoryFragment.ID)
			}
		}
	}

	log.Printf("  - Tenant '%s' content warmed in %v", tenantID, time.Since(tenantStart))
	return nil
}

// warmHTMLFragmentForPane is a helper to generate and cache HTML for a single pane.
func warmHTMLFragmentForPane(ctx *tenant.Context, paneNode *models.PaneNode, storyFragmentID string) {
	cacheManager := cache.GetGlobalManager()

	nodesData, parentChildMap, err := html.ExtractNodesFromPane(paneNode)
	if err != nil {
		log.Printf("  - Warning: Failed to parse pane %s nodes: %v", paneNode.ID, err)
		return
	}

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

	renderCtx := &models.RenderContext{
		AllNodes:         nodesData,
		ParentNodes:      parentChildMap,
		TenantID:         ctx.TenantID,
		SessionID:        "", // No session during warming
		StoryfragmentID:  storyFragmentID,
		ContainingPaneID: paneNode.ID,
	}

	generator := html.NewGenerator(renderCtx)
	htmlContent := generator.Render(paneNode.ID)

	variant := models.PaneVariantDefault
	cacheManager.SetHTMLChunk(ctx.TenantID, paneNode.ID, variant, htmlContent, []string{paneNode.ID})
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

	var activeTenants []string
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeTenants = append(activeTenants, tenantID)
		}
	}
	return activeTenants, nil
}
