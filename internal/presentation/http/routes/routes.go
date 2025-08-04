// Package routes provides HTTP route configuration for the presentation layer.
package routes

import (
	"github.com/AtRiskMedia/tractstack-go/internal/application/container"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/handlers"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all HTTP routes and middleware with dependency injection
func SetupRoutes(container *container.Container) *gin.Engine {
	// Create Gin router
	r := gin.Default()

	// Configure CORS
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"*"} // In production, configure specific origins
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Tenant-ID"}
	config.AllowCredentials = true
	r.Use(cors.New(config))

	// Initialize all handler structs with injected singleton services and logger
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
	configHandlers := handlers.NewConfigHandlers(container.Logger, container.PerfTracker)
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

	// SysOp Dashboard
	sysopHandlers := handlers.NewSysOpHandlers(container)

	// Serve static SysOp dashboard
	r.Static("/sysop", "./web/sysop")

	// SysOp endpoints (no WebSocket, direct service access)
	r.GET("/sysop-auth", sysopHandlers.AuthCheck)
	r.POST("/sysop-login", sysopHandlers.Login)
	r.GET("/sysop-dashboard", sysopHandlers.GetDashboard)
	r.GET("/sysop-tenants", sysopHandlers.GetTenants)
	r.POST("/sysop-reload", sysopHandlers.ForceReload)

	// API routes with tenant middleware
	api := r.Group("/api/v1")
	api.Use(middleware.TenantMiddleware(container.TenantManager))
	{
		// Config endpoints
		api.GET("/config/brand", configHandlers.GetBrandConfig)

		// Health check endpoint
		api.GET("/health", func(c *gin.Context) {
			tenantCtx, _ := middleware.GetTenantContext(c)
			c.JSON(200, gin.H{
				"status":   "ok",
				"tenantId": tenantCtx.TenantID,
			})
		})

		// Analytics endpoints
		analytics := api.Group("/analytics")
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
		{
			admin.GET("/orphan-analysis", orphanHandlers.GetOrphanAnalysis)
		}

		// Fragment rendering endpoints (MATCHES LEGACY main.go EXACTLY)
		fragments := api.Group("/fragments")
		{
			fragments.GET("/panes/:id", fragmentHandlers.GetPaneFragment)
			fragments.POST("/panes", fragmentHandlers.GetPaneFragmentBatch)
		}

		// Content nodes
		nodes := api.Group("/nodes")
		{
			// Menu endpoints
			nodes.GET("/menus", menuHandlers.GetAllMenuIDs)
			nodes.POST("/menus", menuHandlers.GetMenusByIDs)
			nodes.GET("/menus/:id", menuHandlers.GetMenuByID)
			nodes.POST("/menus/create", menuHandlers.CreateMenu)
			nodes.PUT("/menus/:id", menuHandlers.UpdateMenu)
			nodes.DELETE("/menus/:id", menuHandlers.DeleteMenu)

			// Pane endpoints
			nodes.GET("/panes", paneHandlers.GetAllPaneIDs)
			nodes.POST("/panes", paneHandlers.GetPanesByIDs)
			nodes.GET("/panes/:id", paneHandlers.GetPaneByID)
			nodes.GET("/panes/slug/:slug", paneHandlers.GetPaneBySlug)
			nodes.GET("/panes/context", paneHandlers.GetContextPanes)
			nodes.POST("/panes/create", paneHandlers.CreatePane)
			nodes.PUT("/panes/:id", paneHandlers.UpdatePane)
			nodes.DELETE("/panes/:id", paneHandlers.DeletePane)

			// Resource endpoints
			nodes.GET("/resources", resourceHandlers.GetAllResourceIDs)
			nodes.POST("/resources", resourceHandlers.GetResourcesByIDs)
			nodes.GET("/resources/:id", resourceHandlers.GetResourceByID)
			nodes.GET("/resources/slug/:slug", resourceHandlers.GetResourceBySlug)
			nodes.POST("/resources/create", resourceHandlers.CreateResource)
			nodes.PUT("/resources/:id", resourceHandlers.UpdateResource)
			nodes.DELETE("/resources/:id", resourceHandlers.DeleteResource)

			// StoryFragment endpoints
			nodes.GET("/storyfragments", storyFragmentHandlers.GetAllStoryFragmentIDs)
			nodes.POST("/storyfragments", storyFragmentHandlers.GetStoryFragmentsByIDs)
			nodes.GET("/storyfragments/:id", storyFragmentHandlers.GetStoryFragmentByID)
			nodes.GET("/storyfragments/slug/:slug", storyFragmentHandlers.GetStoryFragmentBySlug)
			nodes.GET("/storyfragments/slug/:slug/full-payload", storyFragmentHandlers.GetStoryFragmentFullPayloadBySlug)
			nodes.GET("/storyfragments/home", storyFragmentHandlers.GetHomeStoryFragment)
			nodes.POST("/storyfragments/create", storyFragmentHandlers.CreateStoryFragment)
			nodes.PUT("/storyfragments/:id", storyFragmentHandlers.UpdateStoryFragment)
			nodes.DELETE("/storyfragments/:id", storyFragmentHandlers.DeleteStoryFragment)

			// TractStack endpoints
			nodes.GET("/tractstacks", tractStackHandlers.GetAllTractStackIDs)
			nodes.POST("/tractstacks", tractStackHandlers.GetTractStacksByIDs)
			nodes.GET("/tractstacks/:id", tractStackHandlers.GetTractStackByID)
			nodes.GET("/tractstacks/slug/:slug", tractStackHandlers.GetTractStackBySlug)
			nodes.POST("/tractstacks/create", tractStackHandlers.CreateTractStack)
			nodes.PUT("/tractstacks/:id", tractStackHandlers.UpdateTractStack)
			nodes.DELETE("/tractstacks/:id", tractStackHandlers.DeleteTractStack)

			// Belief endpoints
			nodes.GET("/beliefs", beliefHandlers.GetAllBeliefIDs)
			nodes.POST("/beliefs", beliefHandlers.GetBeliefsByIDs)
			nodes.GET("/beliefs/:id", beliefHandlers.GetBeliefByID)
			nodes.GET("/beliefs/slug/:slug", beliefHandlers.GetBeliefBySlug)
			nodes.POST("/beliefs/create", beliefHandlers.CreateBelief)
			nodes.PUT("/beliefs/:id", beliefHandlers.UpdateBelief)
			nodes.DELETE("/beliefs/:id", beliefHandlers.DeleteBelief)

			// ImageFile endpoints
			nodes.GET("/files", imageFileHandlers.GetAllFileIDs)
			nodes.POST("/files", imageFileHandlers.GetFilesByIDs)
			nodes.GET("/files/:id", imageFileHandlers.GetFileByID)
			nodes.POST("/files/create", imageFileHandlers.CreateFile)
			nodes.PUT("/files/:id", imageFileHandlers.UpdateFile)
			nodes.DELETE("/files/:id", imageFileHandlers.DeleteFile)

			// Epinet endpoints
			nodes.GET("/epinets", epinetHandlers.GetAllEpinetIDs)
			nodes.POST("/epinets", epinetHandlers.GetEpinetsByIDs)
			nodes.GET("/epinets/:id", epinetHandlers.GetEpinetByID)
			nodes.POST("/epinets/create", epinetHandlers.CreateEpinet)
			nodes.PUT("/epinets/:id", epinetHandlers.UpdateEpinet)
			nodes.DELETE("/epinets/:id", epinetHandlers.DeleteEpinet)
		}
	}

	return r
}
