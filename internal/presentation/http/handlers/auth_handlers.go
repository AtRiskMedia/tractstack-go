// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// AuthHandlers contains all authentication-related HTTP handlers
type AuthHandlers struct {
	authService *services.AuthService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
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

	// Parse login request
	var loginReq struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&loginReq); err != nil {
		h.logger.Auth().Error("Login request JSON binding failed", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Use auth service to authenticate
	result := h.authService.AuthenticateAdmin(loginReq.Password, tenantCtx)

	if !result.Success {
		h.logger.Auth().Warn("Login attempt failed", "tenantId", tenantCtx.TenantID, "error", result.Error, "duration", time.Since(start))
		marker.SetSuccess(false)
		h.logger.Perf().Info("Performance for PostLogin request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", false)

		c.JSON(http.StatusUnauthorized, gin.H{"error": result.Error})
		return
	}

	// Set role-specific HTTP-only cookie
	cookieName := "admin_auth"
	if result.Role == "editor" {
		cookieName = "editor_auth"
	}

	c.SetCookie(
		cookieName,   // name (admin_auth or editor_auth)
		result.Token, // value
		86400,        // maxAge (24 hours in seconds)
		"/",          // path
		"",           // domain (empty for current domain)
		false,        // secure (set to true in production)
		true,         // httpOnly
	)

	h.logger.Auth().Info("Login successful", "tenantId", tenantCtx.TenantID, "role", result.Role, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostLogin request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"role":    result.Role,
		"message": "Login successful",
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

	// Clear both admin and editor auth cookies by setting them to expired
	c.SetCookie("admin_auth", "", -1, "/", "", false, true)
	c.SetCookie("editor_auth", "", -1, "/", "", false, true)

	h.logger.Auth().Info("Logout completed", "tenantId", tenantCtx.TenantID, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostLogout request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logout successful",
	})
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

	// Get current token from Authorization header or cookies
	var currentToken string
	var tokenSource string

	authHeader := c.GetHeader("Authorization")
	if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		currentToken = authHeader[7:]
		tokenSource = "bearer"
	} else {
		// Try cookies
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

	// Validate current token
	tokenInfo := h.authService.GetTokenInfo(currentToken, tenantCtx)
	if !tokenInfo.Valid {
		h.logger.Auth().Warn("Refresh token request with invalid current token", "tenantId", tenantCtx.TenantID, "source", tokenSource)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	// Generate new token with same role
	// TODO:
	h.authService.AuthenticateAdmin("", tenantCtx) // This is a bit of a hack - we should have a refresh method
	// For now, we'll generate a new token with the same role
	claims := map[string]interface{}{
		"role":     tokenInfo.Role,
		"tenantId": tokenInfo.TenantID,
		"type":     "admin_auth",
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	newToken, err := h.authService.GenerateJWT(claims, tenantCtx.Config.JWTSecret)
	if err != nil {
		h.logger.Auth().Error("Token refresh failed", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	// Update cookie if token came from cookie
	if tokenSource == "admin_cookie" {
		c.SetCookie("admin_auth", newToken, 86400, "/", "", false, true)
	} else if tokenSource == "editor_cookie" {
		c.SetCookie("editor_auth", newToken, 86400, "/", "", false, true)
	}

	h.logger.Auth().Info("Token refresh successful", "tenantId", tenantCtx.TenantID, "role", tokenInfo.Role, "source", tokenSource, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostRefreshToken request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	response := gin.H{
		"success": true,
		"role":    tokenInfo.Role,
		"message": "Token refreshed successfully",
	}

	// Include new token in response if it came from bearer header
	if tokenSource == "bearer" {
		response["token"] = newToken
	}

	c.JSON(http.StatusOK, response)
}

// AuthMiddleware provides authentication middleware functions
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
			if h.authService.ValidateAdminOrEditorRole(authHeader, tenantCtx) {
				authenticated = true
			}
		} else {
			// Check cookies
			if adminCookie, err := c.Cookie("admin_auth"); err == nil {
				if h.authService.ValidateAdminOrEditorRole("Bearer "+adminCookie, tenantCtx) {
					authenticated = true
				}
			}

			if !authenticated {
				if editorCookie, err := c.Cookie("editor_auth"); err == nil {
					if h.authService.ValidateAdminOrEditorRole("Bearer "+editorCookie, tenantCtx) {
						authenticated = true
					}
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
			if h.authService.ValidateAdminRole(authHeader, tenantCtx) {
				authenticated = true
			}
		} else {
			// Check admin cookie only
			if adminCookie, err := c.Cookie("admin_auth"); err == nil {
				if h.authService.ValidateAdminRole("Bearer "+adminCookie, tenantCtx) {
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
