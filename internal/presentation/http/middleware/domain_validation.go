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

		// Also check tenant registry domains directly (for setup mode)
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = c.Query("tenantId")
		}

		detector := tenantManager.GetDetector()

		// Primary check: if tenant ID is provided, validate hostDomain
		if tenantID != "" {
			if detector.ValidateDomain(tenantID, hostDomain) {
				c.Next()
				return
			}
		}

		// Fallback check for dedicated sites: validate hostDomain against default tenant
		if detector.ValidateDomain("default", hostDomain) {
			c.Next()
			return
		}

		// Origin validation path
		if origin != "" {
			if originURL, err := url.Parse(origin); err == nil {
				originHost := originURL.Hostname()
				if detector.ValidateDomain("default", originHost) {
					c.Next()
					return
				}
				if tenantID != "" && detector.ValidateDomain(tenantID, originHost) {
					c.Next()
					return
				}
			}
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
			domain = hostDomain
		}

		// Validate domain against tenant's allowed domains
		if !detector.ValidateDomain(tenantCtx.TenantID, domain) {
			c.JSON(http.StatusForbidden, gin.H{"error": "domain not allowed for tenant"})
			c.Abort()
			return
		}

		c.Next()
	}
}
