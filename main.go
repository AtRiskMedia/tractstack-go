package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/api"
	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var GlobalCacheManager *cache.Manager

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize global cache manager
	GlobalCacheManager = cache.NewManager()
	cache.GlobalInstance = GlobalCacheManager // Set the global instance
	log.Println("Global cache manager initialized")

	// Initialize tenant manager
	tenantManager, err := tenant.NewManager()
	if err != nil {
		log.Fatalf("Failed to initialize tenant manager: %v", err)
	}

	r := gin.Default()
	r.SetTrustedProxies([]string{"127.0.0.1"})

	// Configure CORS to allow localhost origins
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"http://localhost:3000",
			"http://localhost:4321", // Astro dev server
			"http://127.0.0.1:3000",
			"http://127.0.0.1:4321",
		},
		AllowMethods: []string{
			"GET", "POST", "PUT", "DELETE", "OPTIONS",
		},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			"X-TractStack-Tenant", "X-Requested-With",
			"hx-current-url", "hx-request", "hx-target", "hx-trigger", "hx-boosted",
		},
		AllowCredentials: true,
	}))

	// Add tenant context middleware
	r.Use(func(c *gin.Context) {
		tenantCtx, err := tenantManager.GetContext(c)
		if err != nil {
			c.JSON(500, gin.H{"error": "tenant context failed: " + err.Error()})
			c.Abort()
			return
		}
		defer tenantCtx.Close()

		// Store tenant context and manager for handlers
		c.Set("tenant", tenantCtx)
		c.Set("tenantManager", tenantManager)
		c.Next()
	})

	// Domain whitelist middleware (after tenant context)
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		host := c.Request.Host

		// Allow localhost by default for development
		if strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:") ||
			strings.HasPrefix(host, "localhost:") ||
			strings.HasPrefix(host, "127.0.0.1:") {
			c.Next()
			return
		}

		// Get tenant context for domain validation
		tenantCtx, exists := c.Get("tenant")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "tenant context required"})
			c.Abort()
			return
		}

		// Get tenant manager for domain validation
		manager, managerExists := c.Get("tenantManager")
		if !managerExists {
			c.JSON(http.StatusForbidden, gin.H{"error": "tenant manager required"})
			c.Abort()
			return
		}

		// Extract domain from origin
		var domain string
		if origin != "" {
			if originURL, err := url.Parse(origin); err == nil {
				domain = originURL.Hostname()
			}
		} else {
			domain = host
		}

		// Validate domain against tenant's allowed domains
		tenantManager := manager.(*tenant.Manager)
		ctx := tenantCtx.(*tenant.Context)

		if !tenantManager.GetDetector().ValidateDomain(ctx.TenantID, domain) {
			c.JSON(http.StatusForbidden, gin.H{"error": "domain not allowed for tenant"})
			c.Abort()
			return
		}

		c.Next()
	})

	// Authentication and system routes
	r.POST("/api/v1/auth/visit", api.VisitHandler)
	r.GET("/api/v1/auth/sse", api.SseHandler)
	r.POST("/api/v1/auth/state", api.StateHandler)
	r.GET("/api/v1/auth/profile/decode", api.DecodeProfileHandler)
	r.POST("/api/v1/auth/login", api.LoginHandler)
	r.GET("/api/v1/db/status", api.DBStatusHandler)

	// Content API routes
	v1 := r.Group("/api/v1")
	{
		nodes := v1.Group("/nodes")
		{
			// Pane endpoints
			nodes.GET("/panes", api.GetAllPaneIDsHandler)
			nodes.POST("/panes", api.GetPanesByIDsHandler) // Bulk load panes
			nodes.GET("/panes/:id", api.GetPaneByIDHandler)
			nodes.GET("/panes/slug/:slug", api.GetPaneBySlugHandler)
			nodes.GET("/panes/context", api.GetContextPanesHandler)

			// TractStack endpoints
			nodes.GET("/tractstacks", api.GetAllTractStackIDsHandler)
			nodes.POST("/tractstacks", api.GetTractStacksByIDsHandler) // Bulk load tractstacks
			nodes.GET("/tractstacks/:id", api.GetTractStackByIDHandler)
			nodes.GET("/tractstacks/slug/:slug", api.GetTractStackBySlugHandler)

			// StoryFragment endpoints
			nodes.GET("/storyfragments", api.GetAllStoryFragmentIDsHandler)
			nodes.POST("/storyfragments", api.GetStoryFragmentsByIDsHandler) // Bulk load storyfragments
			nodes.GET("/storyfragments/:id", api.GetStoryFragmentByIDHandler)
			nodes.GET("/storyfragments/slug/:slug", api.GetStoryFragmentBySlugHandler)

			// Menu endpoints
			nodes.GET("/menus", api.GetAllMenuIDsHandler)
			nodes.POST("/menus", api.GetMenusByIDsHandler) // Bulk load menus
			nodes.GET("/menus/:id", api.GetMenuByIDHandler)

			// Resource endpoints
			nodes.GET("/resources", api.GetAllResourceIDsHandler)
			nodes.POST("/resources", api.GetResourcesByIDsHandler) // Bulk load resources
			nodes.GET("/resources/:id", api.GetResourceByIDHandler)
			nodes.GET("/resources/slug/:slug", api.GetResourceBySlugHandler)

			// Belief endpoints
			nodes.GET("/beliefs", api.GetAllBeliefIDsHandler)
			nodes.POST("/beliefs", api.GetBeliefsByIDsHandler) // Bulk load beliefs
			nodes.GET("/beliefs/:id", api.GetBeliefByIDHandler)
			nodes.GET("/beliefs/slug/:slug", api.GetBeliefBySlugHandler)

			// ImageFile endpoints
			nodes.GET("/files", api.GetAllFileIDsHandler)
			nodes.POST("/files", api.GetFilesByIDsHandler) // Bulk load files
			nodes.GET("/files/:id", api.GetFileByIDHandler)
		}

		// Fragment rendering endpoints
		fragments := v1.Group("/fragments")
		{
			fragments.GET("/panes/:id", api.GetPaneFragmentHandler)
		}
	}

	log.Println("Starting server on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
