// Package routes provides HTTP route configuration for the presentation layer.
package routes

import (
	tenantpkg "github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/handlers"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all HTTP routes and middleware
func SetupRoutes(tenantManager *tenantpkg.Manager) *gin.Engine {
	// Create Gin router
	r := gin.Default()

	// Configure CORS
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"*"} // In production, configure specific origins
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Tenant-ID"}
	config.AllowCredentials = true
	r.Use(cors.New(config))

	// API routes with tenant middleware
	api := r.Group("/api/v1")
	api.Use(middleware.TenantMiddleware(tenantManager))
	{
		// Config endpoints
		api.GET("/config/brand", handlers.GetBrandConfigHandler)

		// Health check endpoint
		api.GET("/health", func(c *gin.Context) {
			tenantCtx, _ := middleware.GetTenantContext(c)
			c.JSON(200, gin.H{
				"status":   "ok",
				"tenantId": tenantCtx.TenantID,
			})
		})

		// Content endpoints
		api.GET("/content/full-map", handlers.GetContentMapHandler)

		// Content nodes
		nodes := api.Group("/nodes")
		{
			// Pane endpoints
			nodes.GET("/panes", handlers.GetAllPaneIDsHandler)
			nodes.POST("/panes", handlers.GetPanesByIDsHandler) // Bulk load panes
			nodes.GET("/panes/:id", handlers.GetPaneByIDHandler)
			nodes.GET("/panes/slug/:slug", handlers.GetPaneBySlugHandler)
			nodes.GET("/panes/context", handlers.GetContextPanesHandler)

			// TractStack endpoints
			nodes.GET("/tractstacks", handlers.GetAllTractStackIDsHandler)
			nodes.POST("/tractstacks", handlers.GetTractStacksByIDsHandler) // Bulk load tractstacks
			nodes.GET("/tractstacks/:id", handlers.GetTractStackByIDHandler)
			nodes.GET("/tractstacks/slug/:slug", handlers.GetTractStackBySlugHandler)

			// StoryFragment endpoints
			nodes.GET("/storyfragments", handlers.GetAllStoryFragmentIDsHandler)
			nodes.POST("/storyfragments", handlers.GetStoryFragmentsByIDsHandler) // Bulk load storyfragments
			nodes.GET("/storyfragments/:id", handlers.GetStoryFragmentByIDHandler)
			nodes.GET("/storyfragments/slug/:slug", handlers.GetStoryFragmentBySlugHandler)
			nodes.GET("/storyfragments/slug/:slug/full-payload", handlers.GetStoryFragmentFullPayloadBySlugHandler)
			nodes.GET("/storyfragments/home", handlers.GetHomeStoryFragmentHandler)

			// Menu endpoints
			nodes.GET("/menus", handlers.GetAllMenuIDsHandler)
			nodes.POST("/menus", handlers.GetMenusByIDsHandler) // Bulk load menus
			nodes.GET("/menus/:id", handlers.GetMenuByIDHandler)
			nodes.POST("/menus/create", handlers.CreateMenuHandler) // TODO: must protect
			nodes.PUT("/menus/:id", handlers.UpdateMenuHandler)     // TODO: must protect
			nodes.DELETE("/menus/:id", handlers.DeleteMenuHandler)  // TODO: must protect

			// Resource endpoints
			nodes.GET("/resources", handlers.GetAllResourceIDsHandler)
			nodes.POST("/resources", handlers.GetResourcesByIDsHandler) // Bulk load resources with filtering
			nodes.GET("/resources/:id", handlers.GetResourceByIDHandler)
			nodes.GET("/resources/slug/:slug", handlers.GetResourceBySlugHandler)
			nodes.POST("/resources/create", handlers.CreateResourceHandler) // TODO: must protect
			nodes.PUT("/resources/:id", handlers.UpdateResourceHandler)     // TODO: must protect
			nodes.DELETE("/resources/:id", handlers.DeleteResourceHandler)  // TODO: must protect

			// Belief endpoints
			nodes.GET("/beliefs", handlers.GetAllBeliefIDsHandler)
			nodes.POST("/beliefs", handlers.GetBeliefsByIDsHandler) // Bulk load beliefs
			nodes.GET("/beliefs/:id", handlers.GetBeliefByIDHandler)
			nodes.GET("/beliefs/slug/:slug", handlers.GetBeliefBySlugHandler)
			nodes.POST("/beliefs/create", handlers.CreateBeliefHandler) // TODO: must protect
			nodes.PUT("/beliefs/:id", handlers.UpdateBeliefHandler)     // TODO: must protect
			nodes.DELETE("/beliefs/:id", handlers.DeleteBeliefHandler)  // TODO: must protect

			// Epinet endpoints
			nodes.GET("/epinets", handlers.GetAllEpinetIDsHandler)
			nodes.POST("/epinets", handlers.GetEpinetsByIDsHandler) // Bulk load epinets
			nodes.GET("/epinets/:id", handlers.GetEpinetByIDHandler)

			// ImageFile endpoints
			nodes.GET("/files", handlers.GetAllFileIDsHandler)
			nodes.POST("/files", handlers.GetFilesByIDsHandler) // Bulk load files
			nodes.GET("/files/:id", handlers.GetFileByIDHandler)
		}

		// Fragment rendering endpoints
		//fragments := api.Group("/fragments")
		//{
		//	fragments.GET("/panes/:id", handlers.GetPaneFragmentHandler)    // TODO: implement
		//	fragments.POST("/panes", handlers.GetPaneFragmentsBatchHandler) // TODO: implement
		//}

		// Admin endpoints
		admin := api.Group("/admin")
		{
			admin.GET("/orphan-analysis", handlers.GetOrphanAnalysisHandler) // TODO: must protect
		}
	}

	return r
}
