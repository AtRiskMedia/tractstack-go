// Package container provides dependency injection for all singleton services
package container

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/email"
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

	// System & State Services
	AuthService        *services.AuthService
	SessionService     *services.SessionService
	StateService       *services.StateService
	DBService          *services.DBService
	ConfigService      *services.ConfigService
	MultiTenantService *services.MultiTenantService
	LogBroadcaster     *logging.LogBroadcaster

	// Infrastructure Dependencies
	TenantManager *tenant.Manager
	CacheManager  *manager.Manager
	Logger        *logging.ChanneledLogger
	PerfTracker   *performance.Tracker
	EmailService  email.Service
}

// NewContainer creates and wires all singleton services
func NewContainer(tenantManager *tenant.Manager, cacheManager *manager.Manager) *Container {
	// Initialize observability infrastructure
	perfTracker := performance.NewTracker(performance.DefaultTrackerConfig())
	emailService, err := email.NewService()
	if err != nil {
		panic("Failed to initialize email service: " + err.Error())
	}

	loggerConfig := logging.DefaultLoggerConfig()
	loggerConfig.LogDirectory = filepath.Join(os.Getenv("HOME"), "t8k-go-server", "log")

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

	// Initialize fragment services
	sessionBeliefService := services.NewSessionBeliefService()
	widgetContextService := services.NewWidgetContextService(sessionBeliefService)
	fragmentService := services.NewFragmentService(
		widgetContextService,
		sessionBeliefService,
		perfTracker,
		logger,
	)

	// Create contentMapService before other content services
	contentMapService := services.NewContentMapService(logger, perfTracker)

	// Initialize auth, session, state, and system services with correct dependencies
	authService := services.NewAuthService(logger, perfTracker)
	sessionService := services.NewSessionService(logger, perfTracker)
	stateService := services.NewStateService(logger, perfTracker)
	dbService := services.NewDBService(logger, perfTracker)
	configService := services.NewConfigService(logger, perfTracker)
	// Initialize the new MultiTenantService
	multiTenantService := services.NewMultiTenantService(tenantManager, emailService, logger, perfTracker)
	logBroadcaster := logging.GetBroadcaster()

	logger.Startup().Info("Dependency injection container services initialized")

	return &Container{
		// Content Services
		MenuService:           services.NewMenuService(logger, perfTracker, contentMapService),
		PaneService:           services.NewPaneService(logger, perfTracker, contentMapService),
		ResourceService:       services.NewResourceService(logger, perfTracker, contentMapService),
		StoryFragmentService:  services.NewStoryFragmentService(logger, perfTracker, contentMapService),
		TractStackService:     services.NewTractStackService(logger, perfTracker, contentMapService),
		BeliefService:         services.NewBeliefService(logger, perfTracker, contentMapService),
		ImageFileService:      services.NewImageFileService(logger, perfTracker, contentMapService),
		EpinetService:         services.NewEpinetService(logger, perfTracker, contentMapService),
		ContentMapService:     contentMapService,
		OrphanAnalysisService: services.NewOrphanAnalysisService(logger),
		BeliefRegistryService: services.NewBeliefRegistryService(logger),
		WarmingService:        services.NewWarmingService(logger, perfTracker),

		// Fragment Services
		SessionBeliefService: sessionBeliefService,
		WidgetContextService: widgetContextService,
		FragmentService:      fragmentService,

		// Analytics Services
		AnalyticsService:          services.NewAnalyticsService(logger, perfTracker),
		DashboardAnalyticsService: services.NewDashboardAnalyticsService(logger, perfTracker),
		EpinetAnalyticsService:    services.NewEpinetAnalyticsService(logger, perfTracker),
		LeadAnalyticsService:      services.NewLeadAnalyticsService(logger, perfTracker),
		ContentAnalyticsService:   services.NewContentAnalyticsService(logger, perfTracker),

		// System & State Services
		AuthService:        authService,
		SessionService:     sessionService,
		StateService:       stateService,
		DBService:          dbService,
		ConfigService:      configService,
		MultiTenantService: multiTenantService,
		LogBroadcaster:     logBroadcaster,

		// Infrastructure
		TenantManager: tenantManager,
		CacheManager:  cacheManager,
		Logger:        logger,
		PerfTracker:   perfTracker,
		EmailService:  emailService,
	}
}
