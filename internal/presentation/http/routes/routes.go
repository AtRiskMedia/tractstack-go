// Package routes provides HTTP route configuration for the presentation layer.
package routes

import (
	"github.com/AtRiskMedia/tractstack-go/internal/application/container"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/handlers"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all HTTP routes and middleware with dependency injection.
func SetupRoutes(container *container.Container) *gin.Engine {
	r := gin.Default()

	r.Use(middleware.CORSMiddleware())

	// Serve static SysOp dashboard files from the /sysop URL.
	r.Static("/sysop", "web/sysop")
	r.StaticFile("/favicon.ico", "web/sysop/favicon.ico")

	// Initialize handlers
	menuHandlers := handlers.NewMenuHandlers(container.MenuService, container.Logger, container.PerfTracker)
	paneHandlers := handlers.NewPaneHandlers(container.PaneService, container.Logger, container.PerfTracker)
	resourceHandlers := handlers.NewResourceHandlers(container.ResourceService, container.Logger, container.PerfTracker)
	storyFragmentHandlers := handlers.NewStoryFragmentHandlers(container.StoryFragmentService, container.Logger, container.PerfTracker)
	tractStackHandlers := handlers.NewTractStackHandlers(container.TractStackService, container.Logger, container.PerfTracker)
	beliefHandlers := handlers.NewBeliefHandlers(container.BeliefService, container.Logger, container.PerfTracker)
	imageFileHandlers := handlers.NewImageFileHandlers(container.ImageFileService, container.Logger, container.PerfTracker)
	epinetHandlers := handlers.NewEpinetHandlers(container.EpinetService, container.Logger, container.PerfTracker)
	contentMapHandlers := handlers.NewContentMapHandlers(container.ContentMapService, container.Logger, container.PerfTracker)
	orphanHandlers := handlers.NewOrphanAnalysisHandlers(container.OrphanAnalysisService, container.Logger, container.PerfTracker)
	configHandlers := handlers.NewConfigHandlers(container.ConfigService, container.Logger, container.PerfTracker)
	fragmentHandlers := handlers.NewFragmentHandlers(container.FragmentService, container.Logger, container.PerfTracker)
	analyticsHandlers := handlers.NewAnalyticsHandlers(
		container.AnalyticsService,
		container.DashboardAnalyticsService,
		container.EpinetAnalyticsService,
		container.LeadAnalyticsService,
		container.ContentAnalyticsService,
		container.WarmingService,
		container.TenantManager,
		container.Logger,
		container.PerfTracker,
	)
	authHandlers := handlers.NewAuthHandlers(container.AuthService, container.Logger, container.PerfTracker)
	visitHandlers := handlers.NewVisitHandlers(container.SessionService, container.AuthService, container.Broadcaster, container.Logger, container.PerfTracker)
	stateHandlers := handlers.NewStateHandlers(container.EventProcessingService, container.Broadcaster, container.Logger, container.PerfTracker)
	dbHandlers := handlers.NewDBHandlers(container.DBService, container.Logger, container.PerfTracker)
	sysopHandlers := handlers.NewSysOpHandlers(container)
	multiTenantHandlers := handlers.NewMultiTenantHandlers(container.MultiTenantService, container.Logger, container.PerfTracker)

	// SysOp API endpoints moved to /api/sysop to avoid conflict with static file serving
	sysopAPI := r.Group("/api/sysop")
	{
		sysopAPI.GET("/auth", sysopHandlers.AuthCheck)
		sysopAPI.POST("/login", sysopHandlers.Login)

		// SysOp Authenticated endpoints
		sysopAPI.Use(sysopHandlers.SysOpAuthMiddleware())
		{
			sysopAPI.GET("/tenants", sysopHandlers.GetTenants)
			sysopAPI.GET("/activity", sysopHandlers.GetActivityMetrics)
			sysopAPI.POST("/tenant-token", sysopHandlers.GetTenantToken)
			sysopAPI.GET("/logs/levels", sysopHandlers.GetLogLevels)
			sysopAPI.POST("/logs/levels", sysopHandlers.SetLogLevel)
			sysopAPI.GET("/orphan-analysis", sysopHandlers.GetOrphanAnalysis)
		}
	}

	// Log streaming is a special case and can remain at top level
	r.GET("/sysop-logs/stream", sysopHandlers.StreamLogs)

	// Public, non-tenant-specific admin routes for provisioning (conditional).
	if config.EnableMultiTenant {
		tenantAPI := r.Group("/api/v1/tenant")
		{
			tenantAPI.POST("/provision", multiTenantHandlers.HandleProvisionTenant)
			tenantAPI.POST("/activation", multiTenantHandlers.HandleActivateTenant)
			tenantAPI.GET("/capacity", multiTenantHandlers.HandleGetCapacity)
		}
	}

	// API routes with tenant middleware
	api := r.Group("/api/v1")
	api.Use(middleware.TenantMiddleware(container.TenantManager, container.PerfTracker))
	api.Use(middleware.DomainValidationMiddleware(container.TenantManager))
	{
		// Config endpoints
		configGroup := api.Group("/config")
		{
			// Public brand config endpoint
			configGroup.GET("/brand", configHandlers.GetBrandConfig)

			// Protected config endpoints
			configGroup.Use(authHandlers.AuthMiddleware())
			configGroup.PUT("/brand", configHandlers.UpdateBrandConfig)
			configGroup.GET("/advanced", configHandlers.GetAdvancedConfig)
			configGroup.PUT("/advanced", authHandlers.AdminOnlyMiddleware(), configHandlers.UpdateAdvancedConfig)
		}

		// Authentication and system routes
		auth := api.Group("/auth")
		{
			auth.POST("/visit", visitHandlers.PostVisit)
			auth.GET("/sse", visitHandlers.GetSSE)
			auth.GET("/profile/decode", authHandlers.GetDecodeProfile)
			auth.POST("/profile", visitHandlers.PostProfile)
			auth.POST("/login", authHandlers.PostLogin)
			auth.POST("/logout", authHandlers.PostLogout)
			auth.GET("/status", authHandlers.GetAuthStatus)
			auth.POST("/refresh", authHandlers.PostRefreshToken)
		}

		// State management (separate from auth)
		api.POST("/state", stateHandlers.PostState)

		// Database status
		api.GET("/db/status", dbHandlers.GetDatabaseStatus)

		// General health endpoint (legacy format)
		api.GET("/health", dbHandlers.GetGeneralHealth)

		// Analytics endpoints
		analytics := api.Group("/analytics")
		if !config.ExposeAnalytics {
			analytics.Use(authHandlers.AuthMiddleware())
		}
		{
			analytics.GET("/dashboard", analyticsHandlers.HandleDashboardAnalytics)
			analytics.GET("/epinet/:id", analyticsHandlers.HandleEpinetSankey)
			analytics.GET("/storyfragments", analyticsHandlers.HandleStoryfragmentAnalytics)
			analytics.GET("/leads", analyticsHandlers.HandleLeadMetrics)
			analytics.GET("/all", analyticsHandlers.HandleAllAnalytics)
		}

		// Content endpoints
		api.GET("/content/full-map", contentMapHandlers.GetContentMap)

		// Admin endpoints
		admin := api.Group("/admin")
		admin.Use(authHandlers.AuthMiddleware())
		{
			admin.GET("/orphan-analysis", orphanHandlers.GetOrphanAnalysis)
		}

		// Fragment rendering endpoints - PUBLIC (needed for frontend)
		fragments := api.Group("/fragments")
		{
			fragments.GET("/panes/:id", fragmentHandlers.GetPaneFragment)
			fragments.POST("/panes", fragmentHandlers.GetPaneFragmentBatch)
		}

		// Content nodes with public/protected split
		nodes := api.Group("/nodes")
		{
			// PUBLIC ROUTES - Frontend website access (no auth required)
			// Story fragments
			nodes.GET("/storyfragments/slug/:slug", storyFragmentHandlers.GetStoryFragmentBySlug)
			nodes.GET("/storyfragments/home", storyFragmentHandlers.GetHomeStoryFragment)

			// Panes by slug for context pages
			nodes.GET("/panes/slug/:slug", paneHandlers.GetPaneBySlug)

			// Resources - public access needed for frontend
			nodes.GET("/resources/:id", resourceHandlers.GetResourceByID)
			nodes.GET("/resources/slug/:slug", resourceHandlers.GetResourceBySlug)

			// PROTECTED ROUTES - Admin/Editor access only (StoryKeep interface)
			protected := nodes.Group("/")
			protected.Use(authHandlers.AuthMiddleware())
			{
				// Menu endpoints
				protected.GET("/menus", menuHandlers.GetAllMenuIDs)
				protected.POST("/menus", menuHandlers.GetMenusByIDs)
				protected.GET("/menus/:id", menuHandlers.GetMenuByID)

				// Pane endpoints (except slug which is public)
				protected.GET("/panes", paneHandlers.GetAllPaneIDs)
				protected.POST("/panes", paneHandlers.GetPanesByIDs)
				protected.GET("/panes/:id", paneHandlers.GetPaneByID)
				protected.GET("/panes/context", paneHandlers.GetContextPanes)

				// Resource endpoints (except individual gets which are public)
				protected.GET("/resources", resourceHandlers.GetAllResourceIDs)
				protected.POST("/resources", resourceHandlers.GetResourcesByIDs)

				// Story fragment endpoints (except slug and home which are public)
				protected.GET("/storyfragments", storyFragmentHandlers.GetAllStoryFragmentIDs)
				protected.POST("/storyfragments", storyFragmentHandlers.GetStoryFragmentsByIDs)
				protected.GET("/storyfragments/:id", storyFragmentHandlers.GetStoryFragmentByID)

				// TractStack endpoints
				protected.GET("/tractstacks", tractStackHandlers.GetAllTractStackIDs)
				protected.POST("/tractstacks", tractStackHandlers.GetTractStacksByIDs)
				protected.GET("/tractstacks/:id", tractStackHandlers.GetTractStackByID)
				protected.GET("/tractstacks/slug/:slug", tractStackHandlers.GetTractStackBySlug)

				// Belief endpoints
				protected.GET("/beliefs", beliefHandlers.GetAllBeliefIDs)
				protected.POST("/beliefs", beliefHandlers.GetBeliefsByIDs)
				protected.GET("/beliefs/:id", beliefHandlers.GetBeliefByID)
				protected.GET("/beliefs/slug/:slug", beliefHandlers.GetBeliefBySlug)

				// File endpoints
				protected.POST("/files", imageFileHandlers.GetFilesByIDs)
				protected.GET("/files", imageFileHandlers.GetAllFileIDs)
				protected.GET("/files/:id", imageFileHandlers.GetFileByID)

				// Epinet endpoints
				protected.GET("/epinets", epinetHandlers.GetAllEpinetIDs)
				protected.POST("/epinets", epinetHandlers.GetEpinetsByIDs)
				protected.GET("/epinets/:id", epinetHandlers.GetEpinetByID)

				// Mutation Routes (Create/Update/Delete)
				mutations := protected.Group("/")
				{
					mutations.GET("/storyfragments/slug/:slug/full-payload", storyFragmentHandlers.GetStoryFragmentFullPayloadBySlug)
					mutations.POST("/menus/create", menuHandlers.CreateMenu)
					mutations.PUT("/menus/:id", menuHandlers.UpdateMenu)
					mutations.DELETE("/menus/:id", menuHandlers.DeleteMenu)
					mutations.POST("/panes/create", paneHandlers.CreatePane)
					mutations.PUT("/panes/:id", paneHandlers.UpdatePane)
					mutations.DELETE("/panes/:id", paneHandlers.DeletePane)
					mutations.POST("/resources/create", resourceHandlers.CreateResource)
					mutations.PUT("/resources/:id", resourceHandlers.UpdateResource)
					mutations.DELETE("/resources/:id", resourceHandlers.DeleteResource)
					mutations.POST("/storyfragments/create", storyFragmentHandlers.CreateStoryFragment)
					mutations.PUT("/storyfragments/:id", storyFragmentHandlers.UpdateStoryFragment)
					mutations.DELETE("/storyfragments/:id", storyFragmentHandlers.DeleteStoryFragment)
					mutations.POST("/tractstacks/create", tractStackHandlers.CreateTractStack)
					mutations.PUT("/tractstacks/:id", tractStackHandlers.UpdateTractStack)
					mutations.DELETE("/tractstacks/:id", tractStackHandlers.DeleteTractStack)
					mutations.POST("/beliefs/create", beliefHandlers.CreateBelief)
					mutations.PUT("/beliefs/:id", beliefHandlers.UpdateBelief)
					mutations.DELETE("/beliefs/:id", beliefHandlers.DeleteBelief)
					mutations.POST("/files/create", imageFileHandlers.CreateFile)
					mutations.PUT("/files/:id", imageFileHandlers.UpdateFile)
					mutations.DELETE("/files/:id", imageFileHandlers.DeleteFile)
					mutations.POST("/epinets/create", epinetHandlers.CreateEpinet)
					mutations.PUT("/epinets/:id", epinetHandlers.UpdateEpinet)
					mutations.DELETE("/epinets/:id", epinetHandlers.DeleteEpinet)
				}
			}
		}
	}

	return r
}
