// Package container provides dependency injection for all singleton services
package container

import (
	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// Container holds all singleton services and infrastructure dependencies
type Container struct {
	// Content Services (stateless singletons)
	MenuService           *services.MenuService
	PaneService           *services.PaneService
	ResourceService       *services.ResourceService
	StoryFragmentService  *services.StoryFragmentService
	TractStackService     *services.TractStackService
	BeliefService         *services.BeliefService
	ImageFileService      *services.ImageFileService
	EpinetService         *services.EpinetService
	ContentMapService     *services.ContentMapService
	OrphanAnalysisService *services.OrphanAnalysisService
	CacheWarmerService    *services.CacheWarmerService
	BeliefRegistryService *services.BeliefRegistryService
	WarmingService        *services.WarmingService

	// REMOVED: Analytics services (not yet migrated to clean architecture)
	// TODO: Add analytics services when they're migrated from legacy /services to /internal/application/services
	// DashboardAnalyticsService    *services.DashboardAnalyticsService
	// EpinetAnalyticsService       *services.EpinetAnalyticsService
	// LeadAnalyticsService         *services.LeadAnalyticsService
	// ContentAnalyticsService      *services.ContentAnalyticsService
	// AnalyticsOrchestratorService *services.AnalyticsOrchestratorService

	// Infrastructure Dependencies
	TenantManager *tenant.Manager
	CacheManager  *manager.Manager
}

// NewContainer creates and wires all singleton services
func NewContainer(tenantManager *tenant.Manager, cacheManager *manager.Manager) *Container {
	return &Container{
		// Content Services (stateless singletons - no repository dependencies stored)
		MenuService:           services.NewMenuService(),
		PaneService:           services.NewPaneService(),
		ResourceService:       services.NewResourceService(),
		StoryFragmentService:  services.NewStoryFragmentService(),
		TractStackService:     services.NewTractStackService(),
		BeliefService:         services.NewBeliefService(),
		ImageFileService:      services.NewImageFileService(),
		EpinetService:         services.NewEpinetService(),
		ContentMapService:     services.NewContentMapService(),
		OrphanAnalysisService: services.NewOrphanAnalysisService(),
		CacheWarmerService:    services.NewCacheWarmerService(),
		BeliefRegistryService: services.NewBeliefRegistryService(),
		WarmingService:        services.NewWarmingService(),

		// REMOVED: Analytics service constructors (not yet migrated)
		// TODO: Add when analytics services are migrated to clean architecture
		// DashboardAnalyticsService:    services.NewDashboardAnalyticsService(cacheManager),
		// EpinetAnalyticsService:       services.NewEpinetAnalyticsService(cacheManager),
		// LeadAnalyticsService:         services.NewLeadAnalyticsService(cacheManager),
		// ContentAnalyticsService:      services.NewContentAnalyticsService(cacheManager),
		// AnalyticsOrchestratorService: services.NewAnalyticsOrchestratorService(),

		// Infrastructure
		TenantManager: tenantManager,
		CacheManager:  cacheManager,
	}
}
