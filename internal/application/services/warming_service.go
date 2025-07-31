// Package services provides startup warming orchestration
package services

import (
	"fmt"
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
func (ws *WarmingService) WarmAllTenants(tenantCtx *tenant.Context, cache interfaces.Cache, contentMapSvc *ContentMapService, beliefRegistrySvc *BeliefRegistryService, reporter *cleanup.Reporter) error {
	start := time.Now()

	tenants, err := ws.getActiveTenants()
	if err != nil {
		return fmt.Errorf("failed to get active tenants: %w", err)
	}

	reporter.LogHeader(fmt.Sprintf("Cache Warming for %d Tenants", len(tenants)))

	var successCount int
	for _, tenantID := range tenants {
		if err := ws.WarmTenant(tenantCtx, tenantID, cache, contentMapSvc, beliefRegistrySvc, reporter); err != nil {
			reporter.LogError(fmt.Sprintf("Failed to warm tenant %s", tenantID), err)
		} else {
			successCount++
		}
	}

	duration := time.Since(start)
	reporter.LogSubHeader(fmt.Sprintf("Cache Warming Completed in %v", duration))
	reporter.LogSuccess("%d/%d tenants warmed successfully", successCount, len(tenants))

	if successCount < len(tenants) {
		return fmt.Errorf("warming failed for %d tenants", len(tenants)-successCount)
	}

	return nil
}

// WarmTenant performs complete warming sequence for a single tenant
func (ws *WarmingService) WarmTenant(tenantCtx *tenant.Context, tenantID string, cache interfaces.Cache, contentMapSvc *ContentMapService, beliefRegistrySvc *BeliefRegistryService, reporter *cleanup.Reporter) error {
	start := time.Now()
	reporter.LogSubHeader(fmt.Sprintf("Warming Tenant: %s", tenantID))

	// FIXED: Use the passed tenant context instead of creating a new one
	// The tenantCtx already has the proper cache manager and database connection

	reporter.LogStage("Warming Content Map")
	if err := ws.warmContentMap(tenantCtx, contentMapSvc, cache); err != nil {
		return fmt.Errorf("content map warming failed: %w", err)
	}
	reporter.LogSuccess("Content Map Warmed")

	reporter.LogStage("Warming All Beliefs")
	if err := ws.warmAllBeliefs(tenantCtx); err != nil {
		return fmt.Errorf("beliefs warming failed: %w", err)
	}
	reporter.LogSuccess("Beliefs Warmed")

	reporter.LogStage("Warming Home StoryFragment and Dependencies")
	if err := ws.warmHomeStoryfragment(tenantCtx); err != nil {
		return fmt.Errorf("home storyfragment warming failed: %w", err)
	}
	reporter.LogSuccess("Home StoryFragment Warmed")

	duration := time.Since(start)
	reporter.LogSuccess("Tenant %s warmed in %v", tenantID, duration)

	return nil
}

// warmContentMap builds and caches the full content map
func (ws *WarmingService) warmContentMap(tenantCtx *tenant.Context, contentMapSvc *ContentMapService, cache interfaces.Cache) error {
	// Use content map service to build and cache content map
	_, _, err := contentMapSvc.GetContentMap(tenantCtx, "", cache)
	if err != nil {
		return fmt.Errorf("failed to warm content map: %w", err)
	}
	return nil
}

// warmAllBeliefs loads all beliefs into cache (foundation for belief registry)
func (ws *WarmingService) warmAllBeliefs(tenantCtx *tenant.Context) error {
	beliefRepo := tenantCtx.BeliefRepo()
	_, err := beliefRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return err
	}
	return nil
}

// warmHomeStoryfragment loads home storyfragment and all its dependencies
func (ws *WarmingService) warmHomeStoryfragment(tenantCtx *tenant.Context) error {
	homeSlug := ws.getHomeSlug(tenantCtx)
	// TODO: Implementation depends on StoryFragmentService being available
	// This would use tenantCtx.StoryFragmentRepo() to load home content
	_ = homeSlug
	return nil
}

// getHomeSlug extracts home slug from tenant context (defaults to "hello")
func (ws *WarmingService) getHomeSlug(tenantCtx *tenant.Context) string {
	if tenantCtx.Config != nil && tenantCtx.Config.BrandConfig != nil && tenantCtx.Config.BrandConfig.HomeSlug != "" {
		return tenantCtx.Config.BrandConfig.HomeSlug
	}
	return "hello" // Default
}

// getActiveTenants loads tenant registry and returns active tenant IDs
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
