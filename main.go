package main

import (
	"log"

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
	r.Use(cors.Default())

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

	// Routes with tenant context
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
			nodes.GET("/panes/:id", api.GetPaneByIDHandler)
			nodes.GET("/panes/slug/:slug", api.GetPaneBySlugHandler)
			nodes.GET("/panes/context", api.GetContextPanesHandler)
		}
	}

	r.Run(":8080")
}
