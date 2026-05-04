// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// GoogleHandlers manages oauth lifecycle endpoints for Google integration.
type GoogleHandlers struct {
	oauthService  *services.GoogleOAuthService
	configService *services.ConfigService
	logger        *logging.ChanneledLogger
	perfTracker   *performance.Tracker
}

// NewGoogleHandlers creates google oauth handlers.
func NewGoogleHandlers(
	oauthService *services.GoogleOAuthService,
	configService *services.ConfigService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *GoogleHandlers {
	return &GoogleHandlers{
		oauthService:  oauthService,
		configService: configService,
		logger:        logger,
		perfTracker:   perfTracker,
	}
}

func deriveBackendCallbackURI(c *gin.Context) string {
	proto := c.Request.Header.Get("X-Forwarded-Proto")
	if strings.Contains(proto, ",") {
		proto = strings.TrimSpace(strings.Split(proto, ",")[0])
	}
	if proto == "" {
		if c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	host := c.Request.Header.Get("X-Forwarded-Host")
	if strings.Contains(host, ",") {
		host = strings.TrimSpace(strings.Split(host, ",")[0])
	}
	if host == "" {
		host = c.Request.Host
	}

	return fmt.Sprintf("%s://%s/api/google/oauth/callback", proto, host)
}

func deriveStoryKeepRedirectURL(c *gin.Context, tenantCtx any, googleStatus string) string {
	type brandConfigCarrier struct {
		BrandConfig interface {
			GetSiteURL() string
		}
	}

	siteURL := ""
	if typed, ok := tenantCtx.(interface {
		GetBrandSiteURL() string
	}); ok {
		siteURL = typed.GetBrandSiteURL()
	}

	// Fallback path: use concrete tenant context structure fields when available.
	if siteURL == "" {
		if ctx, ok := tenantCtx.(interface {
			BrandSiteURL() string
		}); ok {
			siteURL = ctx.BrandSiteURL()
		}
	}

	// Direct access path for current tenant context type.
	if siteURL == "" {
		if ctx, ok := tenantCtx.(interface {
			GetConfigSiteURL() string
		}); ok {
			siteURL = ctx.GetConfigSiteURL()
		}
	}

	// Current implementation path: access the expected struct fields.
	if siteURL == "" {
		if ctx, ok := tenantCtx.(*middlewareTenantContextShim); ok {
			siteURL = ctx.siteURL
		}
	}

	siteURL = strings.TrimSpace(siteURL)
	if siteURL != "" {
		if !strings.HasPrefix(siteURL, "http://") && !strings.HasPrefix(siteURL, "https://") {
			siteURL = "https://" + siteURL
		}
		base, err := url.Parse(siteURL)
		if err == nil {
			base.Path = "/storykeep"
			base.RawQuery = url.Values{"google": []string{googleStatus}}.Encode()
			base.Fragment = ""
			return base.String()
		}
	}

	// Safe fallback for environments without configured SITE_URL.
	return "/storykeep?google=" + url.QueryEscape(googleStatus)
}

type middlewareTenantContextShim struct {
	siteURL string
}

// HandleOAuthStart creates state and returns oauth consent URL.
func (h *GoogleHandlers) HandleOAuthStart(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	state, err := h.oauthService.GenerateState(tenantCtx.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize oauth state"})
		return
	}

	callbackURI := deriveBackendCallbackURI(c)
	authURL, err := h.oauthService.BuildAuthorizeURL(tenantCtx, callbackURI, state)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"authorization": authURL,
		"callbackUri":  callbackURI,
	})
}

// HandleOAuthCallback validates state and exchanges code for tokens.
func (h *GoogleHandlers) HandleOAuthCallback(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.String(http.StatusInternalServerError, "tenant context not found")
		return
	}

	redirectTo := func(status string) string {
		siteURL := ""
		if tenantCtx.Config != nil && tenantCtx.Config.BrandConfig != nil {
			siteURL = tenantCtx.Config.BrandConfig.SiteURL
		}
		return deriveStoryKeepRedirectURL(c, &middlewareTenantContextShim{siteURL: siteURL}, status)
	}

	if oauthError := c.Query("error"); oauthError != "" {
		h.logger.System().Warn("Google OAuth callback returned error", "tenantId", tenantCtx.TenantID, "error", oauthError)
		c.Redirect(http.StatusFound, redirectTo("oauth_error"))
		return
	}

	state := c.Query("state")
	code := c.Query("code")
	if state == "" || code == "" {
		c.Redirect(http.StatusFound, redirectTo("invalid_callback"))
		return
	}
	if err := h.oauthService.ValidateAndConsumeState(tenantCtx.TenantID, state); err != nil {
		h.logger.System().Warn("Google OAuth state validation failed", "tenantId", tenantCtx.TenantID, "error", err)
		c.Redirect(http.StatusFound, redirectTo("invalid_state"))
		return
	}

	callbackURI := deriveBackendCallbackURI(c)
	payload, err := h.oauthService.ExchangeCode(c.Request.Context(), tenantCtx, code, callbackURI)
	if err != nil {
		h.logger.System().Error("Google OAuth code exchange failed", "tenantId", tenantCtx.TenantID, "error", err)
		c.Redirect(http.StatusFound, redirectTo("token_exchange_failed"))
		return
	}

	tenantCtx.Config.GoogleAccessToken = payload.AccessToken
	tenantCtx.Config.GoogleRefreshToken = payload.RefreshToken
	tenantCtx.Config.GoogleTokenExpiry = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	if err := h.configService.SaveAdvancedConfig(tenantCtx); err != nil {
		h.logger.System().Error("Failed to persist Google OAuth tokens", "tenantId", tenantCtx.TenantID, "error", err)
		c.Redirect(http.StatusFound, redirectTo("persist_failed"))
		return
	}

	c.Redirect(http.StatusFound, redirectTo("connected"))
}

// HandleOAuthStatus returns google connection status for StoryKeep.
func (h *GoogleHandlers) HandleOAuthStatus(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	hasGoogleSync := tenantCtx.Config.GoogleOAuthClientID != "" &&
		tenantCtx.Config.GoogleOAuthClientSecret != "" &&
		tenantCtx.Config.GoogleCalendarID != "" &&
		tenantCtx.Config.GoogleRefreshToken != "" &&
		tenantCtx.Config.GoogleTokenExpiry != ""

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"hasGoogleSync":            hasGoogleSync,
			"googleOauthClientIdSet":   tenantCtx.Config.GoogleOAuthClientID != "",
			"googleOauthClientSecretSet": tenantCtx.Config.GoogleOAuthClientSecret != "",
			"googleCalendarIdSet":      tenantCtx.Config.GoogleCalendarID != "",
			"googleAccessTokenSet":     tenantCtx.Config.GoogleAccessToken != "",
			"googleRefreshTokenSet":    tenantCtx.Config.GoogleRefreshToken != "",
			"googleTokenExpirySet":     tenantCtx.Config.GoogleTokenExpiry != "",
		},
	})
}

// HandleOAuthDisconnect clears oauth token state for tenant.
func (h *GoogleHandlers) HandleOAuthDisconnect(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	tenantCtx.Config.GoogleAccessToken = ""
	tenantCtx.Config.GoogleRefreshToken = ""
	tenantCtx.Config.GoogleTokenExpiry = ""
	if err := h.configService.SaveAdvancedConfig(tenantCtx); err != nil {
		h.logger.System().Error("Failed to disconnect google oauth", "tenantId", tenantCtx.TenantID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disconnect google integration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Google integration disconnected",
	})
}

func sanitizeOAuthScope(scope string) string {
	return strings.TrimSpace(scope)
}
