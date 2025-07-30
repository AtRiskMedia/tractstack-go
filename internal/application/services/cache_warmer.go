// Package services contains the application-level services that orchestrate
// business logic and data access.
package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/analytics"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// CacheWarmerService is responsible for pre-populating the cache with analytics data.
type CacheWarmerService struct {
	cache             interfaces.Cache
	eventRepo         analytics.EventRepository
	epinetRepo        repositories.EpinetRepository
	paneRepo          repositories.PaneRepository
	storyFragmentRepo repositories.StoryFragmentRepository
}

// NewCacheWarmerService creates a new instance of the CacheWarmerService.
func NewCacheWarmerService(
	cache interfaces.Cache,
	eventRepo analytics.EventRepository,
	epinetRepo repositories.EpinetRepository,
	paneRepo repositories.PaneRepository,
	storyFragmentRepo repositories.StoryFragmentRepository,
) *CacheWarmerService {
	return &CacheWarmerService{
		cache:             cache,
		eventRepo:         eventRepo,
		epinetRepo:        epinetRepo,
		paneRepo:          paneRepo,
		storyFragmentRepo: storyFragmentRepo,
	}
}

// WarmCache performs the primary cache warming logic using repository interfaces
func (s *CacheWarmerService) WarmCache(ctx context.Context, tenantID string) error {
	start := time.Now()
	log.Printf("Starting cache warming for tenant: %s", tenantID)

	// Get tenant context
	tenantCtx, err := s.getTenantContext(tenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant context: %w", err)
	}
	defer tenantCtx.Close()

	// Warm analytics data using repository interfaces
	if err := s.warmAnalyticsData(tenantCtx); err != nil {
		log.Printf("Warning: Analytics warming failed for tenant %s: %v", tenantID, err)
		// Continue with other warming - analytics is not critical for basic operation
	}

	// Warm content using repository interfaces
	if err := s.warmContentData(tenantCtx); err != nil {
		return fmt.Errorf("content warming failed: %w", err)
	}

	duration := time.Since(start)
	log.Printf("Cache warming completed for tenant %s in %v", tenantID, duration)
	return nil
}

// warmAnalyticsData warms analytics cache using repository interfaces
func (s *CacheWarmerService) warmAnalyticsData(ctx *tenant.Context) error {
	log.Printf("  Warming analytics data for tenant: %s", ctx.TenantID)

	// TODO: Implement analytics warming when repository interfaces are complete
	// This should use s.eventRepo to load recent analytics events
	// and populate hourly bins in the analytics cache

	log.Printf("  TODO: Analytics warming not yet implemented - repository interfaces needed")
	log.Printf("  HEY THIS SHOULD HAPPEN --> Load recent analytics events using repository interfaces")
	log.Printf("  HEY THIS SHOULD HAPPEN --> Process events into hourly bins and cache them")

	return nil
}

// warmContentData warms content cache using repository interfaces
func (s *CacheWarmerService) warmContentData(ctx *tenant.Context) error {
	log.Printf("  Warming content data for tenant: %s", ctx.TenantID)

	// TODO: Implement content warming when repository interfaces are complete
	// This should:
	// 1. Use s.epinetRepo to load all epinets
	// 2. Use s.paneRepo to load frequently accessed panes
	// 3. Use s.storyFragmentRepo to load key story fragments
	// 4. Populate content cache through cache interface

	log.Printf("  TODO: Content warming not yet implemented - repository interfaces needed")
	log.Printf("  HEY THIS SHOULD HAPPEN --> Load epinets using s.epinetRepo.FindAll()")
	log.Printf("  HEY THIS SHOULD HAPPEN --> Load key panes using s.paneRepo.FindRecent()")
	log.Printf("  HEY THIS SHOULD HAPPEN --> Load story fragments using s.storyFragmentRepo.FindActive()")
	log.Printf("  HEY THIS SHOULD HAPPEN --> Cache all loaded content using s.cache interface methods")

	return nil
}

// getTenantContext creates tenant context for cache warming
func (s *CacheWarmerService) getTenantContext(tenantID string) (*tenant.Context, error) {
	// Load tenant config
	config, err := tenant.LoadTenantConfig(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Create database connection
	database, err := tenant.NewDatabase(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	// Create context
	ctx := &tenant.Context{
		TenantID: tenantID,
		Config:   config,
		Database: database,
		Status:   "active",
	}

	return ctx, nil
}

// WarmAllTenants performs cache warming for all active tenants
func (s *CacheWarmerService) WarmAllTenants(ctx context.Context) error {
	// Get all active tenants from registry
	tenants, err := s.getActiveTenants()
	if err != nil {
		return fmt.Errorf("failed to get active tenants: %w", err)
	}

	log.Printf("Starting cache warming for %d active tenants", len(tenants))

	var failedTenants []string
	for _, tenantID := range tenants {
		if err := s.WarmCache(ctx, tenantID); err != nil {
			log.Printf("Cache warming failed for tenant %s: %v", tenantID, err)
			failedTenants = append(failedTenants, tenantID)
			continue
		}
	}

	if len(failedTenants) > 0 {
		return fmt.Errorf("cache warming failed for %d tenants: %v", len(failedTenants), failedTenants)
	}

	log.Printf("Successfully warmed cache for all %d tenants", len(tenants))
	return nil
}

// getActiveTenants loads tenant registry and returns active tenant IDs
func (s *CacheWarmerService) getActiveTenants() ([]string, error) {
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
