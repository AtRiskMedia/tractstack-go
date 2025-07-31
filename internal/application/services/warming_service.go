// Package services provides startup warming orchestration
package services

import (
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/cleanup"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// WarmingService orchestrates startup cache warming for all tenants
type WarmingService struct {
	// No stored dependencies - all passed via tenant context
}

// NewWarmingService creates a new warming service singleton
func NewWarmingService() *WarmingService {
	return &WarmingService{}
}

// WarmAllTenants performs startup warming for all active tenants
func (ws *WarmingService) WarmAllTenants(tenantManager *tenant.Manager, cache interfaces.Cache, contentMapSvc *ContentMapService, beliefRegistrySvc *BeliefRegistryService, reporter *cleanup.Reporter) error {
	start := time.Now()

	tenants, err := ws.getActiveTenants()
	if err != nil {
		return fmt.Errorf("failed to get active tenants: %w", err)
	}

	reporter.LogHeader(fmt.Sprintf("Strategic Cache Warming for %d Tenants", len(tenants)))

	var successCount int
	for _, tenantID := range tenants {
		tenantCtx, err := tenantManager.NewContextFromID(tenantID)
		if err != nil {
			reporter.LogError(fmt.Sprintf("Failed to create context for tenant %s", tenantID), err)
			continue
		}

		if err := ws.WarmTenant(tenantCtx, tenantID, cache, contentMapSvc, beliefRegistrySvc, reporter); err != nil {
			reporter.LogError(fmt.Sprintf("Failed to warm tenant %s", tenantID), err)
		} else {
			successCount++
		}
		tenantCtx.Close()
	}

	duration := time.Since(start)
	reporter.LogSubHeader(fmt.Sprintf("Strategic Warming Completed in %v", duration))
	reporter.LogSuccess("%d/%d tenants warmed successfully", successCount, len(tenants))

	if successCount < len(tenants) {
		return fmt.Errorf("warming failed for %d tenants", len(tenants)-successCount)
	}

	return nil
}

// WarmTenant performs a focused warming sequence for a single tenant.
func (ws *WarmingService) WarmTenant(tenantCtx *tenant.Context, tenantID string, cache interfaces.Cache, contentMapSvc *ContentMapService, beliefRegistrySvc *BeliefRegistryService, reporter *cleanup.Reporter) error {
	start := time.Now()
	reporter.LogSubHeader(fmt.Sprintf("Warming Tenant: %s", tenantID))

	// Step 1: Warm the content map for content discovery. This is essential.
	reporter.LogStage("Warming Content Map")
	if err := ws.warmContentMap(tenantCtx, contentMapSvc, cache); err != nil {
		return fmt.Errorf("content map warming failed: %w", err)
	}
	reporter.LogSuccess("Content Map Warmed")

	// Step 2: Warm all Beliefs. They are small, foundational, and needed for personalization logic.
	reporter.LogStage("Warming All Beliefs")
	if err := ws.warmAllBeliefs(tenantCtx); err != nil {
		return fmt.Errorf("critical failure: beliefs warming failed: %w", err)
	}
	reporter.LogSuccess("Beliefs Warmed")

	// Step 3: Strategically warm ONLY the home page and its direct dependencies.
	// This ensures the first page load is fast.
	reporter.LogStage("Warming Home StoryFragment and its direct dependencies (Panes, Menu, etc.)")
	if err := ws.warmHomeStoryfragmentAndDeps(tenantCtx); err != nil {
		// This is not a fatal error, as the robust services can lazy-load them later.
		reporter.LogWarning("Home storyfragment dependency warming failed: %v", err)
	} else {
		reporter.LogSuccess("Home StoryFragment and its dependencies Warmed")
	}

	duration := time.Since(start)
	reporter.LogSuccess("Tenant %s strategically warmed in %v", tenantID, duration)

	return nil
}

// warmContentMap builds and caches the full content map.
func (ws *WarmingService) warmContentMap(tenantCtx *tenant.Context, contentMapSvc *ContentMapService, cache interfaces.Cache) error {
	_, _, err := contentMapSvc.GetContentMap(tenantCtx, "", cache)
	if err != nil {
		return fmt.Errorf("failed to warm content map: %w", err)
	}
	return nil
}

// warmAllBeliefs loads all beliefs into the cache.
func (ws *WarmingService) warmAllBeliefs(tenantCtx *tenant.Context) error {
	beliefService := NewBeliefService()
	_, err := beliefService.GetAllIDs(tenantCtx)
	return err
}

// warmHomeStoryfragmentAndDeps finds the home page and uses the GetFullPayload method
// to trigger the cache-first loading of the StoryFragment, its Panes, Menu, and TractStack.
func (ws *WarmingService) warmHomeStoryfragmentAndDeps(tenantCtx *tenant.Context) error {
	storyFragmentService := NewStoryFragmentService()

	// Determine the home slug from the tenant's configuration.
	homeSlug := "hello" // Default fallback
	if tenantCtx.Config != nil && tenantCtx.Config.BrandConfig != nil && tenantCtx.Config.BrandConfig.HomeSlug != "" {
		homeSlug = tenantCtx.Config.BrandConfig.HomeSlug
	}

	// Calling GetFullPayloadBySlug is the most efficient way to warm all dependencies.
	// The service method itself uses the robust, cache-first repositories.
	payload, err := storyFragmentService.GetFullPayloadBySlug(tenantCtx, homeSlug)
	if err != nil {
		return fmt.Errorf("failed to get home storyfragment full payload for warming ('%s'): %w", homeSlug, err)
	}
	if payload == nil || payload.StoryFragment == nil {
		log.Printf("WARN: No home storyfragment found for tenant %s with slug '%s'.", tenantCtx.TenantID, homeSlug)
		return nil
	}

	log.Printf("  - Warmed Home Page ('%s') and its %d panes.", homeSlug, len(payload.Panes))
	return nil
}

// getActiveTenants loads the tenant registry and returns active tenant IDs.
func (ws *WarmingService) getActiveTenants() ([]string, error) {
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return nil, err
	}

	activeTenants := make([]string, 0)
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeTenants = append(activeTenants, tenantID)
		}
	}

	return activeTenants, nil
}
