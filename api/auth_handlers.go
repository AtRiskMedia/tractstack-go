package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

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
	var loginReq struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&loginReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// TODO: Implement actual admin/editor authentication
	// For now, this is a placeholder
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Admin authentication not implemented"})
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
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(24 * time.Hour).Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}
