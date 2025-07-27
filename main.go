package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/api"
	"github.com/AtRiskMedia/tractstack-go/cache"
	defaults "github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/warming"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var GlobalCacheManager *cache.Manager

// isMultiTenantEnabled checks if multi-tenant mode is enabled via environment variable
func isMultiTenantEnabled() bool {
	enableMultiTenant := os.Getenv("ENABLE_MULTI_TENANT")
	if enableMultiTenant == "" {
		return false
	}

	enabled, err := strconv.ParseBool(enableMultiTenant)
	if err != nil {
		log.Printf("Invalid ENABLE_MULTI_TENANT value: %s, defaulting to false", enableMultiTenant)
		return false
	}

	return enabled
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found -- config defaults will be used")
	}

	// Set Gin mode based on environment
	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize global cache manager
	GlobalCacheManager = cache.NewManager()
	if GlobalCacheManager == nil {
		log.Fatal("Failed to create cache manager")
	}

	cache.SetGlobalManager(GlobalCacheManager)
	log.Println("Global cache manager initialized")

	// Start cleanup routine
	cache.StartCleanupRoutine(GlobalCacheManager)

	// Initialize tenant manager
	tenantManager, err := tenant.NewManager()
	if err != nil {
		log.Fatalf("Failed to initialize tenant manager: %v", err)
	}

	log.Println("Starting tenant pre-activation...")
	if err := tenant.PreActivateAllTenants(tenantManager); err != nil {
		log.Fatalf("Tenant pre-activation failed: %v", err)
	}

	// Validate all tenants are active
	if err := tenant.ValidatePreActivation(); err != nil {
		log.Fatalf("Tenant pre-activation validation failed: %v", err)
	}
	log.Println("All tenants pre-activated successfully!")

	if err := warming.WarmAllTenants(tenantManager); err != nil {
		log.Printf("Warning: Content warming failed: %v", err)
	} else {
		log.Println("Critical content warmed successfully!")
	}

	r := gin.Default()
	r.SetTrustedProxies([]string{"127.0.0.1", "::1"}) // Add IPv6 support

	// Configure CORS to allow localhost origins (including IPv6)
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"http://localhost:3000",
			"http://localhost:4321", // Astro dev server
			"http://127.0.0.1:3000",
			"http://127.0.0.1:4321",
			"http://[::1]:3000", // IPv6 localhost
			"http://[::1]:4321", // IPv6 Astro dev server
		},
		AllowMethods: []string{
			"GET", "POST", "PUT", "DELETE", "OPTIONS",
		},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			"X-Tenant-ID", "X-Requested-With", "X-TractStack-Session-ID", "X-StoryFragment-ID",
			"hx-current-url", "hx-request", "hx-target", "hx-trigger", "hx-boosted",
			"Cache-Control",
			"hx-trigger-name",
			"hx-active-element",
			"hx-active-element-name",
			"hx-active-element-value",
		},
		AllowCredentials: true,
		ExposeHeaders: []string{
			"Content-Type", "Cache-Control", "Connection",
		},
	}))

	r.Use(func(c *gin.Context) {
		tenantCtx, err := tenantManager.GetContext(c)
		if err != nil {
			log.Printf("Tenant context error for %s from %s: %v", c.Request.URL.Path, c.ClientIP(), err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context failed: " + err.Error()})
			c.Abort()
			return
		}
		defer tenantCtx.Close()

		// Allow reserved tenants ONLY for activation endpoint
		if tenantCtx.Status == "reserved" && c.Request.URL.Path != "/api/v1/tenant/activation" {
			c.JSON(500, gin.H{"error": "tenant not activated"})
			c.Abort()
			return
		}

		// Store tenant context and manager for handlers
		c.Set("tenant", tenantCtx)
		c.Set("tenantManager", tenantManager)
		c.Next()
	})

	// Domain whitelist middleware (after tenant context)
	r.Use(func(c *gin.Context) {
		// Skip domain validation for OPTIONS requests (CORS preflight)
		if c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		host := c.Request.Host

		// Allow localhost by default for development (including IPv6)
		if strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:") ||
			strings.HasPrefix(origin, "http://[::1]:") || // IPv6 localhost
			strings.HasPrefix(host, "localhost:") ||
			strings.HasPrefix(host, "127.0.0.1:") ||
			strings.HasPrefix(host, "[::1]:") || // IPv6 localhost
			host == "localhost:"+defaults.Port || // Direct host access
			host == "127.0.0.1:"+defaults.Port ||
			host == "[::1]:"+defaults.Port { // IPv6 direct access
			c.Next()
			return
		}

		// Get tenant context for domain validation
		tenantCtx, exists := c.Get("tenant")
		if !exists {
			log.Printf("Tenant context missing for domain validation")
			c.JSON(http.StatusForbidden, gin.H{"error": "tenant context required"})
			c.Abort()
			return
		}

		// Get tenant manager for domain validation
		manager, managerExists := c.Get("tenantManager")
		if !managerExists {
			log.Printf("Tenant manager missing for domain validation")
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
			log.Printf("Domain validation failed for %s (tenant: %s)", domain, ctx.TenantID)
			c.JSON(http.StatusForbidden, gin.H{"error": "domain not allowed for tenant"})
			c.Abort()
			return
		}

		c.Next()
	})

	// API routes
	v1 := r.Group("/api/v1")
	{
		// Health checks
		v1.GET("/health", api.HealthHandler)

		// Content map endpoint
		v1.GET("/content/full-map", api.GetFullContentMapHandler)

		// Authentication and system routes
		auth := v1.Group("/auth")
		{
			auth.POST("/visit", api.VisitHandler)
			auth.GET("/sse", api.SseHandler)
			auth.GET("/profile/decode", api.DecodeProfileHandler)
			auth.POST("/profile", api.ProfileHandler)
			auth.POST("/login", api.LoginHandler)
		}

		// analytics endpoints
		analytics := v1.Group("/analytics")
		{
			analytics.GET("/storyfragments", api.HandleStoryfragmentAnalytics)
			analytics.GET("/dashboard", api.HandleDashboardAnalytics)
			analytics.GET("/epinet/:id", api.HandleEpinetSankey)
			analytics.GET("/leads", api.HandleLeadMetrics)
			analytics.GET("/all", api.HandleAllAnalytics)
		}

		// State management (separate from auth)
		v1.POST("/state", api.StateHandler)

		// Database status
		v1.GET("/db/status", api.DBStatusHandler)

		// Config per tenant
		v1.GET("/config/brand", api.GetBrandConfigHandler)
		v1.PUT("/config/brand", api.UpdateBrandConfigHandler)
		v1.GET("/config/advanced", api.GetAdvancedConfigStatusHandler)
		v1.PUT("/config/advanced", api.UpdateAdvancedConfigHandler)

		// Admin routes
		adminRoutes := v1.Group("/admin")
		{
			adminRoutes.GET("/orphan-analysis", api.GetOrphanAnalysisHandler)
		}

		// Multi-tenant routes (conditional)
		if isMultiTenantEnabled() {
			log.Println("SYSTEM MODE: Multi-tenant enabled - registering tenant endpoints")
			v1.POST("/tenant/activation", api.ActivateTenantHandler)
			v1.GET("/tenant/capacity", api.GetTenantCapacityHandler)
			v1.POST("/tenant/provision", api.ProvisionTenantHandler)
		} else {
			log.Println("SYSTEM MODE: Multi-tenant disabled - tenant endpoints not available")
		}

		// Content nodes
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
			nodes.GET("/storyfragments/slug/:slug/full-payload", api.GetStoryFragmentFullPayloadBySlugHandler)
			nodes.GET("/storyfragments/home", api.GetHomeStoryFragmentHandler)

			// Menu endpoints
			nodes.GET("/menus", api.GetAllMenuIDsHandler)
			nodes.POST("/menus", api.GetMenusByIDsHandler) // Bulk load menus
			nodes.GET("/menus/:id", api.GetMenuByIDHandler)
			nodes.POST("/menus/create", api.CreateMenuHandler)
			nodes.PUT("/menus/:id", api.UpdateMenuHandler)
			nodes.DELETE("/menus/:id", api.DeleteMenuHandler)

			// Resource endpoints
			nodes.GET("/resources", api.GetAllResourceIDsHandler)
			nodes.POST("/resources", api.GetResourcesByIDsHandler) // Bulk load resources
			nodes.GET("/resources/:id", api.GetResourceByIDHandler)
			nodes.GET("/resources/slug/:slug", api.GetResourceBySlugHandler)
			nodes.POST("/resources/create", api.CreateResourceHandler)
			nodes.PUT("/resources/:id", api.UpdateResourceHandler)
			nodes.DELETE("/resources/:id", api.DeleteResourceHandler)

			// Belief endpoints
			nodes.GET("/beliefs", api.GetAllBeliefIDsHandler)
			nodes.POST("/beliefs", api.GetBeliefsByIDsHandler) // Bulk load beliefs
			nodes.GET("/beliefs/:id", api.GetBeliefByIDHandler)
			nodes.GET("/beliefs/slug/:slug", api.GetBeliefBySlugHandler)
			nodes.POST("/beliefs/create", api.CreateBeliefHandler)
			nodes.PUT("/beliefs/:id", api.UpdateBeliefHandler)
			nodes.DELETE("/beliefs/:id", api.DeleteBeliefHandler)

			// Epinet routes
			nodes.GET("/epinets", api.GetAllEpinetIDsHandler)
			nodes.POST("/epinets", api.GetEpinetsByIDsHandler)
			nodes.GET("/epinets/:id", api.GetEpinetByIDHandler)

			// ImageFile endpoints
			nodes.GET("/files", api.GetAllFileIDsHandler)
			nodes.POST("/files", api.GetFilesByIDsHandler) // Bulk load files
			nodes.GET("/files/:id", api.GetFileByIDHandler)
		}

		// Fragment rendering endpoints
		fragments := v1.Group("/fragments")
		{
			fragments.GET("/panes/:id", api.GetPaneFragmentHandler)
			fragments.POST("/panes", api.GetPaneFragmentsBatchHandler)
		}
	}

	serverAddress := ":" + defaults.Port
	log.Printf("Starting server on %s", serverAddress)
	if err := r.Run(serverAddress); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
