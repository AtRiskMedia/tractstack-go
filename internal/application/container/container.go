// Package container provides dependency injection for all singleton services
package container

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
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
	BeliefRegistryService *services.BeliefRegistryService
	WarmingService        *services.WarmingService

	// Fragment Services
	SessionBeliefService *services.SessionBeliefService
	WidgetContextService *services.WidgetContextService
	FragmentService      *services.FragmentService

	// Analytics Services (stateless singletons)
	AnalyticsService          *services.AnalyticsService
	DashboardAnalyticsService *services.DashboardAnalyticsService
	EpinetAnalyticsService    *services.EpinetAnalyticsService
	LeadAnalyticsService      *services.LeadAnalyticsService
	ContentAnalyticsService   *services.ContentAnalyticsService

	// Infrastructure Dependencies
	TenantManager *tenant.Manager
	CacheManager  *manager.Manager
	Logger        *logging.ChanneledLogger // ADD THIS LINE
}

// NewContainer creates and wires all singleton services
func NewContainer(tenantManager *tenant.Manager, cacheManager *manager.Manager) *Container {
	// Initialize observability infrastructure
	perfTracker := performance.NewTracker(performance.DefaultTrackerConfig())

	// Use existing log directory structure
	loggerConfig := logging.DefaultLoggerConfig()
	loggerConfig.LogDirectory = filepath.Join(os.Getenv("HOME"), "t8k-go-server", "log")

	// Wire LogVerbosity config to actual logger level
	switch strings.ToUpper(config.LogVerbosity) {
	case "TRACE":
		loggerConfig.DefaultLevel = slog.LevelDebug - 4 // Trace level
	case "DEBUG":
		loggerConfig.DefaultLevel = slog.LevelDebug
	case "INFO":
		loggerConfig.DefaultLevel = slog.LevelInfo
	case "WARN":
		loggerConfig.DefaultLevel = slog.LevelWarn
	case "ERROR":
		loggerConfig.DefaultLevel = slog.LevelError
	default:
		loggerConfig.DefaultLevel = slog.LevelInfo
	}

	// Clear channel-specific levels to ensure DefaultLevel is respected
	loggerConfig.ChannelLevels = make(map[logging.Channel]slog.Level)

	logger, err := logging.NewChanneledLogger(loggerConfig)
	if err != nil {
		// In startup context, we can't return error gracefully
		panic("Failed to initialize logger: " + err.Error())
	}

	// LOG THE LOGGER INITIALIZATION
	logger.Startup().Info("Channeled logger initialized successfully",
		"logDirectory", loggerConfig.LogDirectory,
		"channels", []string{
			"system", "startup", "shutdown", "auth", "content",
			"analytics", "cache", "database", "tenant", "sse",
			"performance", "slow-query", "memory", "alert", "debug", "trace",
		})

	// Initialize fragment services with proper dependency injection including observability
	sessionBeliefService := services.NewSessionBeliefService()
	widgetContextService := services.NewWidgetContextService(sessionBeliefService)
	fragmentService := services.NewFragmentService(
		widgetContextService,
		sessionBeliefService,
		perfTracker,
		logger,
	)

	logger.Startup().Info("Dependency injection container services initialized")

	return &Container{
		// Content Services (stateless singletons)
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
		BeliefRegistryService: services.NewBeliefRegistryService(),
		WarmingService:        services.NewWarmingService(logger),

		// Fragment Services
		SessionBeliefService: sessionBeliefService,
		WidgetContextService: widgetContextService,
		FragmentService:      fragmentService,

		// Analytics Services (stateless singletons)
		AnalyticsService:          services.NewAnalyticsService(),
		DashboardAnalyticsService: services.NewDashboardAnalyticsService(),
		EpinetAnalyticsService:    services.NewEpinetAnalyticsService(),
		LeadAnalyticsService:      services.NewLeadAnalyticsService(),
		ContentAnalyticsService:   services.NewContentAnalyticsService(),

		// Infrastructure
		TenantManager: tenantManager,
		CacheManager:  cacheManager,
		Logger:        logger,
	}
}
