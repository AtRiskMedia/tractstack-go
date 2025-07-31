// Package services provides startup warming orchestration
package services

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/cleanup"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/bulk"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content" // Import for concrete repos
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// WarmingService orchestrates startup cache warming for all tenants
type WarmingService struct {
	cache             interfaces.Cache
	bulkRepo          bulk.BulkQueryRepository
	contentMapSvc     *ContentMapService
	beliefRegistrySvc *BeliefRegistryService
	beliefRepo        *content.BeliefRepository // MODIFIED: Use the concrete type, not the interface
	reporter          *cleanup.Reporter
}

// NewWarmingService creates a new warming service
func NewWarmingService(
	cache interfaces.Cache,
	bulkRepo bulk.BulkQueryRepository,
	contentMapSvc *ContentMapService,
	beliefRegistrySvc *BeliefRegistryService,
	beliefRepo *content.BeliefRepository, // MODIFIED: Expect the concrete type
	reporter *cleanup.Reporter,
) *WarmingService {
	return &WarmingService{
		cache:             cache,
		bulkRepo:          bulkRepo,
		contentMapSvc:     contentMapSvc,
		beliefRegistrySvc: beliefRegistrySvc,
		beliefRepo:        beliefRepo,
		reporter:          reporter,
	}
}

// WarmAllTenants performs startup warming for all active tenants
func (ws *WarmingService) WarmAllTenants() error {
	start := time.Now()

	tenants, err := ws.getActiveTenants()
	if err != nil {
		return fmt.Errorf("failed to get active tenants: %w", err)
	}

	ws.reporter.LogHeader(fmt.Sprintf("Cache Warming for %d Tenants", len(tenants)))

	var successCount int
	for _, tenantID := range tenants {
		if err := ws.WarmTenant(tenantID); err != nil {
			ws.reporter.LogError(fmt.Sprintf("Failed to warm tenant %s", tenantID), err)
		} else {
			successCount++
		}
	}

	duration := time.Since(start)
	ws.reporter.LogSubHeader(fmt.Sprintf("Cache Warming Completed in %v", duration))
	ws.reporter.LogSuccess("%d/%d tenants warmed successfully", successCount, len(tenants))

	if successCount < len(tenants) {
		return fmt.Errorf("warming failed for %d tenants", len(tenants)-successCount)
	}

	return nil
}

// WarmTenant performs complete warming sequence for a single tenant
func (ws *WarmingService) WarmTenant(tenantID string) error {
	start := time.Now()
	ws.reporter.LogSubHeader(fmt.Sprintf("Warming Tenant: %s", tenantID))

	ctx, err := ws.getTenantContext(tenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant context: %w", err)
	}
	defer ctx.Database.Close() // Close the connection when this tenant's warming is done

	ws.reporter.LogStage("Warming Content Map")
	if err := ws.warmContentMap(tenantID); err != nil {
		return fmt.Errorf("content map warming failed: %w", err)
	}
	ws.reporter.LogSuccess("Content Map Warmed")

	ws.reporter.LogStage("Warming All Beliefs")
	if err := ws.warmAllBeliefs(tenantID, ctx); err != nil {
		return fmt.Errorf("beliefs warming failed: %w", err)
	}
	ws.reporter.LogSuccess("Beliefs Warmed")

	ws.reporter.LogStage("Warming Home StoryFragment and Dependencies")
	if err := ws.warmHomeStoryfragment(tenantID, ctx); err != nil {
		return fmt.Errorf("home storyfragment warming failed: %w", err)
	}
	ws.reporter.LogSuccess("Home StoryFragment Warmed")

	duration := time.Since(start)
	ws.reporter.LogSuccess("Tenant %s warmed in %v", tenantID, duration)

	return nil
}

// warmContentMap builds and caches the full content map
func (ws *WarmingService) warmContentMap(tenantID string) error {
	// Pass the global cache manager to the warming method
	err := ws.contentMapSvc.WarmContentMap(tenantID, ws.cache)
	if err != nil {
		return fmt.Errorf("failed to warm content map: %w", err)
	}
	return nil
}

// warmAllBeliefs loads all beliefs into cache (foundation for belief registry)
func (ws *WarmingService) warmAllBeliefs(tenantID string, ctx *tenant.Context) error {
	// The call is now valid because ws.beliefRepo is the concrete type
	_, err := ws.beliefRepo.FindAll(tenantID)
	if err != nil {
		return err
	}
	return nil
}

// warmHomeStoryfragment loads home storyfragment and all its dependencies
func (ws *WarmingService) warmHomeStoryfragment(tenantID string, ctx *tenant.Context) error {
	homeSlug := ws.getHomeSlug(ctx)
	ws.reporter.LogInfo("Home slug identified as '%s'", homeSlug)
	ws.reporter.LogWarning("Home storyfragment warming not yet implemented")
	return nil
}

// getHomeSlug extracts home slug from tenant context (defaults to "hello")
func (ws *WarmingService) getHomeSlug(ctx *tenant.Context) string {
	if ctx.Config != nil && ctx.Config.BrandConfig != nil && ctx.Config.BrandConfig.HomeSlug != "" {
		return ctx.Config.BrandConfig.HomeSlug
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

// getTenantContext creates tenant context using existing patterns
func (ws *WarmingService) getTenantContext(tenantID string) (*tenant.Context, error) {
	config, err := tenant.LoadTenantConfig(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	database, err := tenant.NewDatabase(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	ctx := &tenant.Context{
		TenantID: tenantID,
		Config:   config,
		Database: database,
		Status:   "active",
	}

	return ctx, nil
}
