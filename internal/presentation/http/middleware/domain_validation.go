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

		// Allow localhost and IPv6 development origins
		if strings.HasPrefix(host, "localhost:") ||
			strings.HasPrefix(host, "127.0.0.1:") ||
			strings.HasPrefix(host, "[::1]:") {
			c.Next()
			return
		}

		// Get tenant context
		tenantCtx, exists := GetTenantContext(c)
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "tenant context required"})
			c.Abort()
			return
		}

		// Extract domain from origin or host
		var domain string
		if origin != "" {
			if originURL, err := url.Parse(origin); err == nil {
				domain = originURL.Hostname()
			}
		} else {
			domain = host
		}

		// Validate domain against tenant's allowed domains
		if !tenantManager.GetDetector().ValidateDomain(tenantCtx.TenantID, domain) {
			c.JSON(http.StatusForbidden, gin.H{"error": "domain not allowed for tenant"})
			c.Abort()
			return
		}

		c.Next()
	}
}
