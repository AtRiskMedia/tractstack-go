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

	r.Use(middleware.CORSMiddleware(container.TenantManager))

	// Serve static SysOp dashboard files from the /sysop URL.
	r.Static("/sysop", "web/sysop")
	r.StaticFile("/favicon.ico", "web/sysop/favicon.ico")

	// Initialize handlers
	menuHandlers := handlers.NewMenuHandlers(container.MenuService, container.Logger, container.PerfTracker)
	paneHandlers := handlers.NewPaneHandlers(container.PaneService, container.Logger, container.PerfTracker)
	resourceHandlers := handlers.NewResourceHandlers(container.ResourceService, container.ImageFileService, container.Logger, container.PerfTracker)
	storyFragmentHandlers := handlers.NewStoryFragmentHandlers(container.StoryFragmentService, container.FragmentService, container.Logger, container.PerfTracker)
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
	dbHandlers := handlers.NewDBHandlers(container.DBService, container.Logger, container.PerfTracker, container.TenantManager)
	sysopHandlers := handlers.NewSysOpHandlers(container)
	multiTenantHandlers := handlers.NewMultiTenantHandlers(container.MultiTenantService, container.Logger, container.PerfTracker)
	aaiHandlers := handlers.NewAAIHandlers(container.AAIService, container.Logger, container.PerfTracker)
	tailwindHandlers := handlers.NewTailwindHandlers(container.TailwindService, container.Logger, container.PerfTracker)
	searchHandlers := handlers.NewSearchHandlers(container.SearchService, container.Logger, container.PerfTracker)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "alive"})
	})

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
			sysopAPI.GET("/ws/session-map", sysopHandlers.HandleSessionMapStream)
			sysopAPI.GET("/graph", sysopHandlers.GetActivityGraph)
		}
	}
	r.GET("/sysop-logs/stream", sysopHandlers.StreamLogs)

	// Multi-tenant provisioning routes (conditional)
	if config.EnableMultiTenant {
		tenantAPI := r.Group("/api/v1/tenant")
		{
			tenantAPI.GET("/capacity", multiTenantHandlers.HandleGetCapacity)
		}
	}

	setupAPI := r.Group("/api/v1/setup")
	setupAPI.Use(middleware.CORSMiddleware(container.TenantManager))
	{
		setupAPI.POST("/initialize", multiTenantHandlers.HandleSetupInitialize)
	}

	// Public domain resolution endpoint (no tenant context required)
	r.GET("/api/v1/resolve-domain", multiTenantHandlers.HandleResolveDomain)

	// API routes with tenant middleware
	api := r.Group("/api/v1")
	api.Use(middleware.TenantMiddleware(container.TenantManager, container.PerfTracker))
	api.Use(middleware.DomainValidationMiddleware(container.TenantManager))
	{
		api.GET("/setup/suitcase", multiTenantHandlers.HandleFetchSuitcase)
		api.POST("/setup/complete", multiTenantHandlers.HandleSetupComplete)

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

		// Tailwind
		tailwindGroup := api.Group("/tailwind")
		{
			tailwindGroup.POST("/classes", authHandlers.AuthMiddleware(), tailwindHandlers.GetTailwindClasses)
			tailwindGroup.POST("/update", authHandlers.AuthMiddleware(), tailwindHandlers.UpdateTailwindCSS)
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

		// General health endpoint
		api.GET("/health", dbHandlers.GetGeneralHealth)

		// Analytics endpoints
		analytics := api.Group("/analytics")
		if !config.ExposeAnalytics {
			analytics.Use(authHandlers.AuthMiddleware())
		}
		{
			analytics.GET("/dashboard", analyticsHandlers.HandleDashboardAnalytics)
			analytics.GET("/content-summary", analyticsHandlers.HandleContentSummary)
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
			admin.GET("/leads/download", analyticsHandlers.HandleLeadsDownload)
			api.POST("/aai/askLemur", authHandlers.AskLemurAuthMiddleware(), aaiHandlers.PostAskLemur)
		}

		// Fragment rendering endpoints
		fragments := api.Group("/fragments")
		{
			fragments.GET("/panes/:id", fragmentHandlers.GetPaneFragment)
			fragments.GET("/panes/:id/static", fragmentHandlers.GetPaneFragmentStatic)
			fragments.POST("/panes", fragmentHandlers.GetPaneFragmentBatch)
			fragments.POST("/preview", fragmentHandlers.GeneratePreviewFromPayload)
		}

		// Search endpoints
		search := api.Group("/search")
		{
			search.GET("/discover", searchHandlers.HandleDiscovery)
			search.GET("/retrieve", searchHandlers.HandleRetrieval)
		}

		// Content nodes - Read operations (Public)
		nodes := api.Group("/nodes")
		{
			// Menu endpoints - Read
			nodes.GET("/menus", menuHandlers.GetAllMenuIDs)
			nodes.POST("/menus", menuHandlers.GetMenusByIDs)
			nodes.GET("/menus/:id", menuHandlers.GetMenuByID)

			// Pane endpoints - Read
			nodes.GET("/panes", paneHandlers.GetAllPaneIDs)
			nodes.POST("/panes", paneHandlers.GetPanesByIDs)
			nodes.GET("/panes/:id", paneHandlers.GetPaneByID)
			nodes.GET("/panes/:id/template", paneHandlers.GetPaneTemplate)
			nodes.GET("/panes/slug/:slug", paneHandlers.GetPaneBySlug)
			nodes.GET("/panes/context", paneHandlers.GetContextPanes)
			nodes.GET("/panes/slug/:slug/full-payload", paneHandlers.GetContextPaneFullPayload)

			// Resource endpoints - Read
			nodes.GET("/resources", resourceHandlers.GetAllResourceIDs)
			nodes.POST("/resources", resourceHandlers.GetResourcesByIDs)
			nodes.GET("/resources/:id", resourceHandlers.GetResourceByID)
			nodes.GET("/resources/slug/:slug", resourceHandlers.GetResourceBySlug)

			// Story fragment endpoints - Read
			nodes.GET("/storyfragments", storyFragmentHandlers.GetAllStoryFragmentIDs)
			nodes.GET("/storyfragments/slug/:slug/full-payload", storyFragmentHandlers.GetStoryFragmentFullPayloadBySlug)
			nodes.GET("/storyfragments/slug/:slug/personalized-payload", storyFragmentHandlers.GetStoryFragmentPersonalizedPayloadBySlug)
			nodes.GET("/storyfragments/home/personalized-payload", storyFragmentHandlers.GetStoryFragmentPersonalizedPayloadBySlug)
			nodes.POST("/storyfragments", storyFragmentHandlers.GetStoryFragmentsByIDs)
			nodes.GET("/storyfragments/:id", storyFragmentHandlers.GetStoryFragmentByID)
			nodes.GET("/storyfragments/slug/:slug", storyFragmentHandlers.GetStoryFragmentBySlug)
			nodes.GET("/storyfragments/home", storyFragmentHandlers.GetHomeStoryFragment)

			// TractStack endpoints - Read
			nodes.GET("/tractstacks", tractStackHandlers.GetAllTractStackIDs)
			nodes.POST("/tractstacks", tractStackHandlers.GetTractStacksByIDs)
			nodes.GET("/tractstacks/:id", tractStackHandlers.GetTractStackByID)
			nodes.GET("/tractstacks/slug/:slug", tractStackHandlers.GetTractStackBySlug)

			// Belief endpoints - Read
			nodes.GET("/beliefs", beliefHandlers.GetAllBeliefIDs)
			nodes.POST("/beliefs", beliefHandlers.GetBeliefsByIDs)
			nodes.GET("/beliefs/:id", beliefHandlers.GetBeliefByID)
			nodes.GET("/beliefs/slug/:slug", beliefHandlers.GetBeliefBySlug)

			// File endpoints - Read
			nodes.GET("/files", imageFileHandlers.GetAllFileIDs)
			nodes.POST("/files", imageFileHandlers.GetFilesByIDs)
			nodes.GET("/files/:id", imageFileHandlers.GetFileByID)

			// Epinet endpoints - Read
			nodes.GET("/epinets", epinetHandlers.GetAllEpinetIDs)
			nodes.POST("/epinets", epinetHandlers.GetEpinetsByIDs)
			nodes.GET("/epinets/:id", epinetHandlers.GetEpinetByID)

			// Protected Nodes - Write operations (Authenticated)
			protectedNodes := nodes.Group("")
			protectedNodes.Use(authHandlers.AuthMiddleware())
			{
				// Menu - Write
				protectedNodes.POST("/menus/create", menuHandlers.CreateMenu)
				protectedNodes.PUT("/menus/:id", menuHandlers.UpdateMenu)
				protectedNodes.DELETE("/menus/:id", menuHandlers.DeleteMenu)

				// Pane - Write
				protectedNodes.POST("/panes/bulk", paneHandlers.BulkProcessPanes)
				protectedNodes.POST("/panes/create", paneHandlers.CreatePane)
				protectedNodes.PUT("/panes/:id", paneHandlers.UpdatePane)
				protectedNodes.DELETE("/panes/:id", paneHandlers.DeletePane)
				protectedNodes.POST("/panes/files/bulk", paneHandlers.BulkUpdateFilePaneRelationships)

				// Resource - Write
				protectedNodes.POST("/resources/create", resourceHandlers.CreateResource)
				protectedNodes.PUT("/resources/:id", resourceHandlers.UpdateResource)
				protectedNodes.DELETE("/resources/:id", resourceHandlers.DeleteResource)

				// Story fragment - Write
				protectedNodes.POST("/storyfragments/create", storyFragmentHandlers.CreateStoryFragment)
				protectedNodes.PUT("/storyfragments/:id", storyFragmentHandlers.UpdateStoryFragment)
				protectedNodes.DELETE("/storyfragments/:id", storyFragmentHandlers.DeleteStoryFragment)
				protectedNodes.PUT("/storyfragments/:id/complete", storyFragmentHandlers.UpdateStoryFragmentComplete)

				// TractStack - Write
				protectedNodes.POST("/tractstacks/create", tractStackHandlers.CreateTractStack)
				protectedNodes.PUT("/tractstacks/:id", tractStackHandlers.UpdateTractStack)
				protectedNodes.DELETE("/tractstacks/:id", tractStackHandlers.DeleteTractStack)

				// Belief - Write
				protectedNodes.POST("/beliefs/create", beliefHandlers.CreateBelief)
				protectedNodes.PUT("/beliefs/:id", beliefHandlers.UpdateBelief)
				protectedNodes.DELETE("/beliefs/:id", beliefHandlers.DeleteBelief)

				// File - Write
				protectedNodes.POST("/files/create", imageFileHandlers.CreateFile)
				protectedNodes.PUT("/files/:id", imageFileHandlers.UpdateFile)
				protectedNodes.DELETE("/files/:id", imageFileHandlers.DeleteFile)

				// OG Images - Write
				protectedNodes.POST("/images/og", imageFileHandlers.UploadOGImage)
				protectedNodes.DELETE("/images/og", imageFileHandlers.DeleteOGImage)

				// Epinet - Write
				protectedNodes.POST("/epinets/create", epinetHandlers.CreateEpinet)
				protectedNodes.PUT("/epinets/:id", epinetHandlers.UpdateEpinet)
				protectedNodes.DELETE("/epinets/:id", epinetHandlers.DeleteEpinet)
			}
		}
	}

	// Serve index.html for unmatched routes
	r.NoRoute(func(c *gin.Context) {
		c.File("web/index.html")
	})

	return r
}
