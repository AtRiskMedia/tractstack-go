// Package middleware provides HTTP middleware for the presentation layer.
package middleware

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/gin-gonic/gin"
)

// TenantMiddleware creates middleware that extracts tenant information and creates full tenant context
func TenantMiddleware(tenantManager *tenant.Manager) gin.HandlerFunc {
	// TODO: Add tenant operation logging once manager has logger access
	return func(c *gin.Context) {
		// Extract tenant ID from X-Tenant-ID header
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "X-Tenant-ID header is required"})
			c.Abort()
			return
		}

		// Create tenant context using the manager
		tenantCtx, err := tenantManager.NewContextFromID(tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
			c.Abort()
			return
		}

		// Store tenant context in gin context for handlers to access
		c.Set("tenant", tenantCtx)

		// Clean up context after request
		defer func() {
			if tenantCtx != nil {
				tenantCtx.Close()
			}
		}()

		c.Next()
	}
}

// GetTenantContext retrieves the tenant context from gin context
func GetTenantContext(c *gin.Context) (*tenant.Context, bool) {
	tenantCtx, exists := c.Get("tenant")
	if !exists {
		return nil, false
	}

	ctx, ok := tenantCtx.(*tenant.Context)
	return ctx, ok
}
