package handlers

import (
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/gin-gonic/gin"
)

func cookieDomainFor(tenantCtx *tenant.Context) string {
	cookieDomain := ""
	if len(tenantCtx.Domains) > 0 {
		shortest := tenantCtx.Domains[0]
		for _, d := range tenantCtx.Domains {
			if len(d) < len(shortest) {
				shortest = d
			}
		}
		if !strings.HasPrefix(shortest, "localhost") && !strings.HasPrefix(shortest, "127.0.0.1") {
			cookieDomain = shortest
		}
	}
	return cookieDomain
}

func cookieSecure(c *gin.Context) bool {
	return c.Request.TLS != nil
}

func extractAuthToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	if adminCookie, err := c.Cookie("admin_auth"); err == nil && adminCookie != "" {
		return adminCookie
	}
	if editorCookie, err := c.Cookie("editor_auth"); err == nil && editorCookie != "" {
		return editorCookie
	}
	return ""
}

func setPrincipal(c *gin.Context, p *services.AdminPrincipal) {
	c.Set("principal", p)
}

func getPrincipal(c *gin.Context) *services.AdminPrincipal {
	v, ok := c.Get("principal")
	if !ok {
		return nil
	}
	p, _ := v.(*services.AdminPrincipal)
	return p
}

func resolvePrincipal(h *AuthHandlers, c *gin.Context, tenantCtx *tenant.Context, roles []string) (*services.AdminPrincipal, bool) {
	token := extractAuthToken(c)
	return h.authService.ValidateTokenWithRolesPrincipal(token, tenantCtx, roles)
}
