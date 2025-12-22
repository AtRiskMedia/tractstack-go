package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/gin-gonic/gin"
)

// DomainValidationMiddleware validates requests against tenant allowed domains
func DomainValidationMiddleware(tenantManager *tenant.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip OPTIONS requests (CORS preflight)
		if c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		host := c.Request.Host

		hostDomain := strings.Split(host, ":")[0]

		// Allow localhost and IPv6 development origins
		if hostDomain == "localhost" || hostDomain == "127.0.0.1" || hostDomain == "::1" {
			c.Next()
			return
		}

		// Check for tenant ID in headers or query (Primary for Multi-tenant)
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = c.Query("tenantId")
		}

		// If we have a tenant ID, validate the stripped hostDomain immediately
		if tenantID != "" {
			detector := tenantManager.GetDetector()
			if detector.ValidateDomain(tenantID, hostDomain) {
				c.Next()
				return
			}
		}

		// Get tenant context (Required fallback for Dedicated instances)
		tenantCtx, exists := GetTenantContext(c)
		if !exists {
			// If context doesn't exist, we try one last validation against the 'default' tenant
			detector := tenantManager.GetDetector()
			if detector.ValidateDomain("default", hostDomain) {
				c.Next()
				return
			}

			c.JSON(http.StatusForbidden, gin.H{"error": "tenant context required"})
			c.Abort()
			return
		}

		// Extract domain from origin or host for final verification
		var finalDomain string
		if origin != "" {
			if originURL, err := url.Parse(origin); err == nil {
				finalDomain = originURL.Hostname()
			}
		} else {
			finalDomain = hostDomain
		}

		// Final validation against the resolved tenant context
		if !tenantManager.GetDetector().ValidateDomain(tenantCtx.TenantID, finalDomain) {
			c.JSON(http.StatusForbidden, gin.H{"error": "domain not allowed for tenant"})
			c.Abort()
			return
		}

		c.Next()
	}
}
