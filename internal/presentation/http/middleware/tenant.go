// Package middleware provides HTTP middleware for the presentation layer.
package middleware

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/gin-gonic/gin"
)

// TenantContext represents the tenant information for a request
type TenantContext struct {
	TenantID string
	Config   *tenant.Config
}

// TenantMiddleware extracts tenant information from X-Tenant-ID header and loads tenant config
func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract tenant ID from X-Tenant-ID header
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "X-Tenant-ID header is required"})
			c.Abort()
			return
		}

		// Load tenant configuration
		tenantConfig, err := tenant.LoadTenantConfig(tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
			c.Abort()
			return
		}

		// Create tenant context
		tenantCtx := &TenantContext{
			TenantID: tenantID,
			Config:   tenantConfig,
		}

		// Store in gin context for handlers to access
		c.Set("tenant", tenantCtx)

		c.Next()
	}
}

// GetTenantContext retrieves the tenant context from gin context
func GetTenantContext(c *gin.Context) (*TenantContext, bool) {
	tenantCtx, exists := c.Get("tenant")
	if !exists {
		return nil, false
	}

	ctx, ok := tenantCtx.(*TenantContext)
	return ctx, ok
}
