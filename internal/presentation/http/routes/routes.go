// Package routes provides HTTP route configuration for the presentation layer.
package routes

import (
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/handlers"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all HTTP routes and middleware
func SetupRoutes() *gin.Engine {
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
	api.Use(middleware.TenantMiddleware())
	{
		// Config endpoints
		api.GET("/config/brand", handlers.GetBrandConfigHandler)

		// Health check endpoint (for testing)
		api.GET("/health", func(c *gin.Context) {
			tenantCtx, _ := middleware.GetTenantContext(c)
			c.JSON(200, gin.H{
				"status":   "ok",
				"tenantId": tenantCtx.TenantID,
			})
		})
	}

	return r
}
