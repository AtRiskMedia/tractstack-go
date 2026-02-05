// Package container provides dependency injection for all singleton services
package container

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/email"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/fts"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/messaging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/templates"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// Container holds all singleton services and infrastructure dependencies
type Container struct {
	// Content Services
	MenuService                 *services.MenuService
	PaneService                 *services.PaneService
	ResourceService             *services.ResourceService
	StoryFragmentService        *services.StoryFragmentService
	TractStackService           *services.TractStackService
	BeliefService               *services.BeliefService
	ImageFileService            *services.ImageFileService
	EpinetService               *services.EpinetService
	ContentMapService           *services.ContentMapService
	OrphanAnalysisService       *services.OrphanAnalysisService
	BeliefRegistryService       *services.BeliefRegistryService
	WarmingService              *services.WarmingService
	RegistryRebuildOrchestrator *services.RegistryRebuildOrchestrator
	SearchService               *services.SearchService

	// Fragment Services
	SessionBeliefService *services.SessionBeliefService
	WidgetContextService *services.WidgetContextService
	FragmentService      *services.FragmentService
	ScrollTargetService  *services.ScrollTargetService
	UnsetButtonRenderer  *templates.UnsetButtonRenderer

	// Analytics Services
	AnalyticsService          *services.AnalyticsService
	DashboardAnalyticsService *services.DashboardAnalyticsService
	EpinetAnalyticsService    *services.EpinetAnalyticsService
	LeadAnalyticsService      *services.LeadAnalyticsService
	ContentAnalyticsService   *services.ContentAnalyticsService

	// System & State Services
	AuthService            *services.AuthService
	SessionService         *services.SessionService
	EventProcessingService *services.EventProcessingService
	DBService              *services.DBService
	ConfigService          *services.ConfigService
	TailwindService        *services.TailwindService
	MultiTenantService     *services.MultiTenantService
	AAIService             *services.AAIService
	LogBroadcaster         *logging.LogBroadcaster
	Broadcaster            messaging.Broadcaster
	SysOpBroadcaster       *messaging.SysOpBroadcaster
	SysOpService           *services.SysOpService
	ShopifyService         *services.ShopifyService
	Service                *fts.Service

	// Infrastructure Dependencies
	TenantManager  *tenant.Manager
	CacheManager   *manager.Manager
	Logger         *logging.ChanneledLogger
	PerfTracker    *performance.Tracker
	EmailService   email.Service
	LeadRepository user.LeadRepository
}

// NewContainer creates and wires all singleton services
func NewContainer(tenantManager *tenant.Manager, cacheManager *manager.Manager) *Container {
	// Initialize observability infrastructure
	perfTracker := performance.NewTracker(performance.DefaultTrackerConfig())

	loggerConfig := logging.DefaultLoggerConfig()
	loggerConfig.LogDirectory = filepath.Join(config.BackendPath, "log")

	switch strings.ToUpper(config.LogVerbosity) {
	case "TRACE":
		loggerConfig.DefaultLevel = slog.LevelDebug - 4
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
	loggerConfig.ChannelLevels = make(map[logging.Channel]slog.Level)

	logger, err := logging.NewChanneledLogger(loggerConfig)
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	logger.Startup().Info("Channeled logger initialized successfully", "logDirectory", loggerConfig.LogDirectory)

	var emailService email.Service
	if apiKey := os.Getenv("RESEND_API_KEY"); apiKey != "" {
		emailService = email.NewService()
		logger.Startup().Info("Email service initialized successfully with Resend API")
	} else {
		emailService = nil
		logger.Startup().Warn("Email service disabled - RESEND_API_KEY not configured")
	}

	shopifyService := services.NewShopifyService(logger, tenantManager)
	aaiService := services.NewAAIService(logger, perfTracker)
	ftsService := fts.NewService(logger)
	beliefEvaluationService := services.NewBeliefEvaluationService()
	beliefBroadcastService := services.NewBeliefBroadcastService(cacheManager)
	eventProcessingService := services.NewEventProcessingService(beliefBroadcastService, beliefEvaluationService, logger)
	sessionBeliefService := services.NewSessionBeliefService()
	widgetContextService := services.NewWidgetContextService(sessionBeliefService)
	scrollTargetService := services.NewScrollTargetService()
	unsetButtonRenderer := templates.NewUnsetButtonRenderer()
	fragmentService := services.NewFragmentService(
		widgetContextService,
		sessionBeliefService,
		beliefEvaluationService,
		perfTracker,
		logger,
		unsetButtonRenderer,
		scrollTargetService,
	)
	contentMapService := services.NewContentMapService(logger, perfTracker)
	authService := services.NewAuthService(logger, perfTracker)
	sessionService := services.NewSessionService(beliefBroadcastService, logger, perfTracker)
	dbService := services.NewDBService(logger, perfTracker)
	configService := services.NewConfigService(logger, perfTracker)
	beliefRegistryService := services.NewBeliefRegistryService(logger)
	imageFileService := services.NewImageFileService(logger, perfTracker, contentMapService)
	resourceService := services.NewResourceService(logger, perfTracker, contentMapService, imageFileService)

	// Create the orchestrator first, omitting the PaneService to break the dependency cycle.
	registryRebuildOrchestrator := services.NewRegistryRebuildOrchestrator(
		logger,
		tenantManager,
		beliefRegistryService,
	)

	// Create PaneService, injecting the orchestrator and other dependencies.
	paneService := services.NewPaneService(
		logger,
		perfTracker,
		contentMapService,
		registryRebuildOrchestrator,
		aaiService,
		resourceService,
	)

	// Now that PaneService is created, inject it back into the orchestrator.
	registryRebuildOrchestrator.SetPaneService(paneService)

	storyFragmentService := services.NewStoryFragmentService(logger, perfTracker, contentMapService, sessionBeliefService)
	tractStackService := services.NewTractStackService(logger, perfTracker, contentMapService)
	menuService := services.NewMenuService(logger, perfTracker, contentMapService)
	beliefService := services.NewBeliefService(logger, perfTracker, contentMapService)
	epinetService := services.NewEpinetService(logger, perfTracker, contentMapService)
	searchService := services.NewSearchService(paneService, storyFragmentService, resourceService, contentMapService)

	// Create WarmingService, now injecting all its required content service dependencies.
	warmingService := services.NewWarmingService(
		logger,
		perfTracker,
		beliefEvaluationService,
		sessionBeliefService,
		tractStackService,
		storyFragmentService,
		paneService,
		menuService,
		resourceService,
		beliefService,
		epinetService,
		imageFileService,
		contentMapService,
	)

	tailwindService := services.NewTailwindService(paneService, configService, logger, perfTracker)

	multiTenantService := services.NewMultiTenantService(tenantManager, emailService, logger, perfTracker)
	logBroadcaster := logging.GetBroadcaster()
	broadcaster := messaging.NewSSEBroadcaster(logger)
	sysOpService := services.NewSysOpService(
		cacheManager,
		tenantManager,
		contentMapService,
		logger,
		perfTracker,
	)
	sysOpBroadcaster := messaging.NewSysOpBroadcaster(tenantManager, cacheManager)
	go sysOpBroadcaster.Run()

	logger.Startup().Info("Dependency injection container services initialized")

	return &Container{
		// Content Services
		MenuService:                 menuService,
		PaneService:                 paneService,
		ResourceService:             resourceService,
		StoryFragmentService:        storyFragmentService,
		TractStackService:           tractStackService,
		BeliefService:               beliefService,
		ImageFileService:            imageFileService,
		EpinetService:               epinetService,
		ContentMapService:           contentMapService,
		OrphanAnalysisService:       services.NewOrphanAnalysisService(logger),
		BeliefRegistryService:       beliefRegistryService,
		WarmingService:              warmingService,
		RegistryRebuildOrchestrator: registryRebuildOrchestrator,
		SearchService:               searchService,

		// Fragment Services
		SessionBeliefService: sessionBeliefService,
		WidgetContextService: widgetContextService,
		FragmentService:      fragmentService,
		ScrollTargetService:  scrollTargetService,
		UnsetButtonRenderer:  unsetButtonRenderer,

		// Analytics Services
		AnalyticsService:          services.NewAnalyticsService(broadcaster, logger, perfTracker),
		DashboardAnalyticsService: services.NewDashboardAnalyticsService(logger, perfTracker),
		EpinetAnalyticsService:    services.NewEpinetAnalyticsService(logger, perfTracker),
		LeadAnalyticsService:      services.NewLeadAnalyticsService(logger, perfTracker),
		ContentAnalyticsService:   services.NewContentAnalyticsService(logger, perfTracker),

		// System & State Services
		AuthService:            authService,
		SessionService:         sessionService,
		EventProcessingService: eventProcessingService,
		DBService:              dbService,
		ConfigService:          configService,
		TailwindService:        tailwindService,
		MultiTenantService:     multiTenantService,
		AAIService:             aaiService,
		LogBroadcaster:         logBroadcaster,
		Broadcaster:            broadcaster,
		SysOpService:           sysOpService,
		SysOpBroadcaster:       sysOpBroadcaster,
		ShopifyService:         shopifyService,
		Service:                ftsService,

		// Infrastructure
		TenantManager: tenantManager,
		CacheManager:  cacheManager,
		Logger:        logger,
		PerfTracker:   perfTracker,
		EmailService:  emailService,
	}
}
