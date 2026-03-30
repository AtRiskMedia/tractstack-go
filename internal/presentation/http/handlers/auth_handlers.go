// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"github.com/gin-gonic/gin"
)

// LookupLeadRequest contains the email for the frictionless lead lookup
type LookupLeadRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// AuthHandlers contains all authentication-related HTTP handlers
type AuthHandlers struct {
	authService *services.AuthService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// VerifyLeadRequest contains auth fields for lead verification request
type VerifyLeadRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Codeword string `json:"codeword" binding:"required"`
}

// LoginRequest represents the structure for login requests
type LoginRequest struct {
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents the response structure for login requests
type LoginResponse struct {
	Success bool   `json:"success"`
	Role    string `json:"role,omitempty"`
	Token   string `json:"token,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// AuthStatusResponse represents the response structure for auth status requests
type AuthStatusResponse struct {
	Authenticated bool      `json:"authenticated"`
	Method        string    `json:"method,omitempty"`
	Role          string    `json:"role,omitempty"`
	TenantID      string    `json:"tenantId,omitempty"`
	ExpiresAt     time.Time `json:"expiresAt,omitempty"`
}

// RefreshTokenResponse represents the response structure for token refresh requests
type RefreshTokenResponse struct {
	Success bool   `json:"success"`
	Role    string `json:"role,omitempty"`
	Token   string `json:"token,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// NewAuthHandlers creates auth handlers with injected dependencies
func NewAuthHandlers(authService *services.AuthService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *AuthHandlers {
	return &AuthHandlers{
		authService: authService,
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// GetDecodeProfile handles GET /api/v1/auth/profile/decode - decodes and validates profile JWT tokens
func (h *AuthHandlers) GetDecodeProfile(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_decode_profile_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Auth().Debug("Received decode profile request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Get Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		h.logger.Auth().Debug("Decode profile request with no valid authorization header", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	token := authHeader[7:]

	// Use auth service to decode the profile token
	result := h.authService.DecodeProfileToken(token, tenantCtx)

	if !result.Valid {
		h.logger.Auth().Debug("Profile token decode failed or invalid", "tenantId", tenantCtx.TenantID, "duration", time.Since(start))
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	h.logger.Auth().Info("Profile token decoded successfully", "tenantId", tenantCtx.TenantID, "hasProfile", result.Profile != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetDecodeProfile request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{"profile": result.Profile})
}

// GetAuthStatus handles GET /api/v1/auth/status - checks current authentication status
func (h *AuthHandlers) GetAuthStatus(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_auth_status_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Auth().Debug("Received auth status request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Check Authorization header for bearer token
	authHeader := c.GetHeader("Authorization")
	var tokenInfo *services.TokenInfo
	var authenticated bool
	var authMethod string

	if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token := authHeader[7:]
		tokenInfo = h.authService.GetTokenInfo(token, tenantCtx)
		if tokenInfo.Valid {
			authenticated = true
			authMethod = "bearer"
		}
	}

	// If no bearer token, check cookies
	if !authenticated {
		adminCookie, err := c.Cookie("admin_auth")
		if err == nil && adminCookie != "" {
			tokenInfo = h.authService.GetTokenInfo(adminCookie, tenantCtx)
			if tokenInfo.Valid {
				authenticated = true
				authMethod = "cookie"
			}
		}

		if !authenticated {
			editorCookie, err := c.Cookie("editor_auth")
			if err == nil && editorCookie != "" {
				tokenInfo = h.authService.GetTokenInfo(editorCookie, tenantCtx)
				if tokenInfo.Valid {
					authenticated = true
					authMethod = "cookie"
				}
			}
		}
	}

	response := gin.H{
		"authenticated": authenticated,
		"method":        authMethod,
	}

	if authenticated && tokenInfo != nil {
		response["role"] = tokenInfo.Role
		response["tenantId"] = tokenInfo.TenantID
		response["expiresAt"] = tokenInfo.ExpiresAt
	}

	h.logger.Auth().Info("Auth status check completed", "tenantId", tenantCtx.TenantID, "authenticated", authenticated, "method", authMethod, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetAuthStatus request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, response)
}

// AuthMiddleware provides general authentication middleware for admin or editor
func (h *AuthHandlers) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantCtx, exists := middleware.GetTenantContext(c)
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
			c.Abort()
			return
		}

		// Check for valid authentication
		authHeader := c.GetHeader("Authorization")
		authenticated := false

		if authHeader != "" {
			if h.authService.ValidateAdminOrEditorToken(authHeader, tenantCtx) {
				authenticated = true
			}
		} else {
			// Check cookies WITHOUT adding "Bearer " prefix
			if adminCookie, err := c.Cookie("admin_auth"); err == nil {
				if h.authService.ValidateAdminOrEditorToken(adminCookie, tenantCtx) {
					authenticated = true
				}
			} else if editorCookie, err := c.Cookie("editor_auth"); err == nil {
				if h.authService.ValidateAdminOrEditorToken(editorCookie, tenantCtx) {
					authenticated = true
				}
			}
		}

		if !authenticated {
			h.logger.Auth().Warn("Unauthorized access attempt", "tenantId", tenantCtx.TenantID, "path", c.Request.URL.Path)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// AdminOnlyMiddleware provides admin-only authentication middleware
func (h *AuthHandlers) AdminOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantCtx, exists := middleware.GetTenantContext(c)
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
			c.Abort()
			return
		}

		// Check for valid admin authentication
		authHeader := c.GetHeader("Authorization")
		authenticated := false

		if authHeader != "" {
			if h.authService.ValidateAdminToken(authHeader, tenantCtx) {
				authenticated = true
			}
		} else {
			if adminCookie, err := c.Cookie("admin_auth"); err == nil {
				if h.authService.ValidateAdminToken(adminCookie, tenantCtx) {
					authenticated = true
				}
			}
		}

		if !authenticated {
			h.logger.Auth().Warn("Unauthorized admin access attempt", "tenantId", tenantCtx.TenantID, "path", c.Request.URL.Path)
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// AaiAuthMiddleware provides specific authentication for the aai endpoint.
// It allows access for EITHER a standard admin/editor user OR a request
// from the sandbox proxy authenticated with a shared secret.
func (h *AuthHandlers) AaiAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantCtx, exists := middleware.GetTenantContext(c)
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
			c.Abort()
			return
		}

		// Path 1: Check for standard admin/editor JWT cookie.
		// This authenticates a real user logged into the editor.
		if adminCookie, err := c.Cookie("admin_auth"); err == nil && adminCookie != "" {
			if h.authService.ValidateAdminOrEditorToken(adminCookie, tenantCtx) {
				c.Next()
				return
			}
		} else if editorCookie, err := c.Cookie("editor_auth"); err == nil && editorCookie != "" {
			if h.authService.ValidateAdminOrEditorToken(editorCookie, tenantCtx) {
				c.Next()
				return
			}
		}

		// Path 2: Check for the sandbox proxy's shared secret in the Authorization header.
		// This authenticates the server-to-server request from the Astro BFF.
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) > 7 && strings.HasPrefix(authHeader, "Bearer ") {
			token := authHeader[7:]
			// The secret must be configured on the server and must match the token.
			if config.SandboxSecret != "" && token == config.SandboxSecret {
				c.Next()
				return
			}
		}

		// If neither authentication path succeeded, deny access.
		h.logger.Auth().Warn("Unauthorized aai access attempt", "tenantId", tenantCtx.TenantID, "path", c.Request.URL.Path)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		c.Abort()
	}
}

// PostLogin handles POST /api/v1/auth/login - admin/editor authentication
func (h *AuthHandlers) PostLogin(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_login_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Auth().Debug("Received login request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	var loginReq struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&loginReq); err != nil {
		h.logger.Auth().Error("Login request JSON binding failed", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	result := h.authService.AuthenticateAdmin(loginReq.Password, tenantCtx)

	if !result.Success {
		h.logger.Auth().Warn("Login attempt failed", "tenantId", tenantCtx.TenantID, "error", result.Error, "duration", time.Since(start))
		marker.SetSuccess(false)
		h.logger.Perf().Info("Performance for PostLogin request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", false)

		c.JSON(http.StatusUnauthorized, gin.H{"error": result.Error})
		return
	}

	cookieName := "admin_auth"
	if result.Role == "editor" {
		cookieName = "editor_auth"
	}

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

	c.SetCookie(
		cookieName,
		result.Token,
		86400,
		"/",
		cookieDomain,
		false,
		true,
	)

	h.logger.Auth().Info("Login successful", "tenantId", tenantCtx.TenantID, "role", result.Role, "domain", cookieDomain, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostLogin request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"role":   result.Role,
		"token":  result.Token,
	})
}

// PostLogout handles POST /api/v1/auth/logout - clears authentication cookies
func (h *AuthHandlers) PostLogout(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_logout_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Auth().Debug("Received logout request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

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

	c.SetCookie("admin_auth", "", -1, "/", cookieDomain, false, true)
	c.SetCookie("editor_auth", "", -1, "/", cookieDomain, false, true)

	h.logger.Auth().Info("Logout completed", "tenantId", tenantCtx.TenantID, "domain", cookieDomain, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostLogout request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logout successful",
	})
}

// PostRefreshToken handles POST /api/v1/auth/refresh - refreshes authentication tokens
func (h *AuthHandlers) PostRefreshToken(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_refresh_token_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Auth().Debug("Received refresh token request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	var currentToken string
	var tokenSource string

	authHeader := c.GetHeader("Authorization")
	if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		currentToken = authHeader[7:]
		tokenSource = "bearer"
	} else {
		if adminCookie, err := c.Cookie("admin_auth"); err == nil && adminCookie != "" {
			currentToken = adminCookie
			tokenSource = "admin_cookie"
		} else if editorCookie, err := c.Cookie("editor_auth"); err == nil && editorCookie != "" {
			currentToken = editorCookie
			tokenSource = "editor_cookie"
		}
	}

	if currentToken == "" {
		h.logger.Auth().Warn("Refresh token request with no current token", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No valid token found"})
		return
	}

	tokenInfo := h.authService.GetTokenInfo(currentToken, tenantCtx)
	if !tokenInfo.Valid {
		h.logger.Auth().Warn("Refresh token request with invalid current token", "tenantId", tenantCtx.TenantID, "source", tokenSource)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	newResult := h.authService.AuthenticateAdmin("", tenantCtx)
	if !newResult.Success {
		h.logger.Auth().Error("Token refresh failed", "tenantId", tenantCtx.TenantID, "error", newResult.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token refresh failed"})
		return
	}

	if tokenSource == "admin_cookie" || tokenSource == "editor_cookie" {
		cookieName := "admin_auth"
		if tokenInfo.Role == "editor" {
			cookieName = "editor_auth"
		}

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

		c.SetCookie(cookieName, newResult.Token, 86400, "/", cookieDomain, false, true)
	}

	h.logger.Auth().Info("Token refresh successful", "tenantId", tenantCtx.TenantID, "role", tokenInfo.Role, "source", tokenSource, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostRefreshToken request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"role":    tokenInfo.Role,
		"token":   newResult.Token,
		"message": "Token refreshed successfully",
	})
}

// HandleVerifyLead authenticates a lead using their email and codeword
func (h *AuthHandlers) HandleVerifyLead(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req VerifyLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	leadID, err := h.authService.VerifyLeadIdentity(tenantCtx, req.Email, req.Codeword)
	if err != nil {
		h.logger.System().Warn("Lead verification failed", "email", req.Email, "error", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"leadId": leadID})
}

// HandleLookupLead checks if a lead exists by email and returns their ID if so.
func (h *AuthHandlers) HandleLookupLead(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req LookupLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	leadRepo := tenantCtx.LeadRepo()
	lead, err := leadRepo.FindByEmail(req.Email)
	if err != nil {
		h.logger.System().Warn("Database error during lead lookup", "error", err, "email", req.Email)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}

	// Email not found
	if lead == nil {
		c.JSON(http.StatusOK, gin.H{"exists": false})
		return
	}

	// Email found, return the linking ID for the booking handshake
	c.JSON(http.StatusOK, gin.H{
		"exists": true,
		"leadId": lead.ID,
	})
}
