package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

// DecodeProfileHandler handles profile token decoding
func DecodeProfileHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	token := authHeader[7:]

	// Validate JWT token
	claims, err := utils.ValidateJWT(token, ctx.Config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	// Extract profile from claims
	profile := utils.GetProfileFromClaims(claims)
	if profile == nil {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"profile": profile})
}

// LoginHandler handles admin/editor authentication
func LoginHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var loginReq struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&loginReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	var role string

	// Check against admin password
	if ctx.Config.AdminPassword != "" && loginReq.Password == ctx.Config.AdminPassword {
		role = "admin"
	} else if ctx.Config.EditorPassword != "" && loginReq.Password == ctx.Config.EditorPassword {
		role = "editor"
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Generate JWT token
	claims := jwt.MapClaims{
		"role":     role,
		"tenantId": ctx.Config.TenantID,
		"type":     "admin_auth",
	}

	token, err := GenerateJWT(claims, ctx.Config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	// Set role-specific HTTP-only cookie
	cookieName := "admin_auth"
	if role == "editor" {
		cookieName = "editor_auth"
	}
	c.SetCookie(
		cookieName, // name (admin_auth or editor_auth)
		token,      // value
		86400,      // maxAge (24 hours in seconds)
		"/",        // path
		"",         // domain (empty for current domain)
		false,      // secure (set to true in production with HTTPS)
		true,       // httpOnly
	)

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"role":   role,
		"token":  token,
	})
}

// GenerateSecureToken generates a cryptographically secure random token
func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateJWT creates a JWT token with given claims
func GenerateJWT(claims jwt.MapClaims, jwtSecret string) (string, error) {
	// Set standard claims
	claims["iat"] = time.Now().UTC().Unix()
	claims["exp"] = time.Now().UTC().Add(24 * time.Hour).Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

// validateAdmin checks for admin authentication only
func validateAdmin(c *gin.Context, ctx *tenant.Context) bool {
	adminCookie, err := c.Cookie("admin_auth")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Admin authentication required"})
		return false
	}

	claims, err := utils.ValidateJWT(adminCookie, ctx.Config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return false
	}

	role, ok := claims["role"].(string)
	if !ok || role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return false
	}

	return true
}

// validateEditor checks for editor authentication only
func validateEditor(c *gin.Context, ctx *tenant.Context) bool {
	editorCookie, err := c.Cookie("editor_auth")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Editor authentication required"})
		return false
	}

	claims, err := utils.ValidateJWT(editorCookie, ctx.Config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return false
	}

	role, ok := claims["role"].(string)
	if !ok || role != "editor" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Editor access required"})
		return false
	}

	return true
}

// validateAdminOrEditor checks for admin OR editor authentication
func validateAdminOrEditor(c *gin.Context, ctx *tenant.Context) bool {
	// Try admin first (don't send response on failure)
	if adminCookie, err := c.Cookie("admin_auth"); err == nil {
		if claims, err := utils.ValidateJWT(adminCookie, ctx.Config.JWTSecret); err == nil {
			if role, ok := claims["role"].(string); ok && role == "admin" {
				return true
			}
		}
	}

	// Try editor (don't send response on failure)
	if editorCookie, err := c.Cookie("editor_auth"); err == nil {
		if claims, err := utils.ValidateJWT(editorCookie, ctx.Config.JWTSecret); err == nil {
			if role, ok := claims["role"].(string); ok && role == "editor" {
				return true
			}
		}
	}

	// Only send error response if both failed
	c.JSON(http.StatusUnauthorized, gin.H{"error": "Admin or Editor authentication required"})
	return false
}
