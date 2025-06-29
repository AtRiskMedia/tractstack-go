package api

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

// AdminRole represents the user's admin role
type AdminRole string

const (
	RoleAdmin  AdminRole = "admin"
	RoleEditor AdminRole = "editor"
	RoleNone   AdminRole = "none"
)

func LoginHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req models.LoginRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	role := validateAdminLogin(req.TenantID, req.Password, ctx)
	if role == RoleNone {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Generate JWT token with role claims
	token, err := generateAdminToken(string(role), req.TenantID, ctx.Config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Set httpOnly cookie with JWT
	c.SetCookie("auth_token", token, 24*3600, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "role": string(role)})
}

func DecodeProfileHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check Authorization header for JWT instead of cookies
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || len(authHeader) <= 7 || authHeader[:7] != "Bearer " {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	token := authHeader[7:]
	claims, err := utils.ValidateJWT(token, ctx.Config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	profile := claims["profile"]
	c.JSON(http.StatusOK, gin.H{"profile": profile})
}

// validateAdminLogin checks password against config and returns role
func validateAdminLogin(tenantID, password string, ctx *tenant.Context) AdminRole {
	if tenantID != ctx.TenantID {
		return RoleNone
	}

	// Check admin password first
	if ctx.Config.AdminPassword != "" && password == ctx.Config.AdminPassword {
		return RoleAdmin
	}

	// Check editor password
	if ctx.Config.EditorPassword != "" && password == ctx.Config.EditorPassword {
		return RoleEditor
	}

	return RoleNone
}

// generateAdminToken creates a JWT token with admin/editor role claims
func generateAdminToken(role, tenantID, jwtSecret string) (string, error) {
	claims := jwt.MapClaims{
		"role":     role,
		"tenantId": tenantID,
		"type":     "admin_auth",
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

// validateEncryptedCredentials validates profile credentials
func validateEncryptedCredentials(email, code string, ctx *tenant.Context) *models.Profile {
	// Decrypt credentials
	decryptedEmail, err := utils.Decrypt(email, ctx.Config.AESKey)
	if err != nil {
		return nil
	}

	decryptedCode, err := utils.Decrypt(code, ctx.Config.AESKey)
	if err != nil {
		return nil
	}

	// Validate against database
	lead, err := ValidateLeadCredentials(decryptedEmail, decryptedCode, ctx)
	if err != nil || lead == nil {
		return nil
	}

	// Convert lead to profile
	return &models.Profile{
		LeadID:         lead.ID,
		Firstname:      lead.FirstName,
		Email:          lead.Email,
		ContactPersona: lead.ContactPersona,
		ShortBio:       lead.ShortBio,
	}
}

// getProfileFromLeadID gets profile by lead ID
func getProfileFromLeadID(leadID string, ctx *tenant.Context) *models.Profile {
	lead, err := GetLeadByID(leadID, ctx)
	if err != nil || lead == nil {
		return nil
	}

	return &models.Profile{
		LeadID:         lead.ID,
		Firstname:      lead.FirstName,
		Email:          lead.Email,
		ContactPersona: lead.ContactPersona,
		ShortBio:       lead.ShortBio,
	}
}
