package middleware

import (
	"net/url"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORSMiddleware provides enhanced CORS configuration with port range support
func CORSMiddleware(tenantManager *tenant.Manager) gin.HandlerFunc {
	config := cors.Config{
		AllowOriginFunc: func(origin string) bool {
			// Parse the origin URL
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}

			hostname := u.Hostname()

			// Allow localhost and IPv6 development origins
			if strings.HasPrefix(hostname, "localhost") ||
				strings.HasPrefix(hostname, "127.0.0.1") ||
				strings.HasPrefix(hostname, "[::1]") {
				return true
			}

			// For production domains, validate against tenant registry
			// Try default tenant first (most common case)
			detector := tenantManager.GetDetector()
			if detector.ValidateDomain("default", hostname) {
				return true
			}

			// If default fails, check all registered tenants
			registry := detector.GetRegistry()
			for tenantID := range registry.Tenants {
				if detector.ValidateDomain(tenantID, hostname) {
					return true
				}
			}

			return false
		},
		AllowMethods: []string{
			"GET", "POST", "PUT", "DELETE", "OPTIONS",
		},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			"X-Hydration-Token",
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
	}

	return cors.New(config)
}
