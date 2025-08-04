// Package services provides application-level orchestration services
package services

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"slices"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/golang-jwt/jwt/v4"
)

// AuthService handles authentication workflows and JWT operations
type AuthService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewAuthService creates a new authentication service
func NewAuthService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *AuthService {
	return &AuthService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// AuthResult holds authentication result data
type AuthResult struct {
	Token   string `json:"token"`
	Role    string `json:"role"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ProfileDecodeResult holds profile decode result data
type ProfileDecodeResult struct {
	Profile any  `json:"profile"`
	Valid   bool `json:"valid"`
}

// DecodeProfileToken validates and decodes a JWT profile token
func (a *AuthService) DecodeProfileToken(tokenString string, tenantCtx *tenant.Context) *ProfileDecodeResult {
	if tokenString == "" {
		return &ProfileDecodeResult{Profile: nil, Valid: false}
	}

	// Validate JWT token
	claims, err := security.ValidateJWT(tokenString, tenantCtx.Config.JWTSecret)
	if err != nil {
		return &ProfileDecodeResult{Profile: nil, Valid: false}
	}

	// Extract profile from claims
	profile := security.GetProfileFromClaims(claims)
	if profile == nil {
		return &ProfileDecodeResult{Profile: nil, Valid: false}
	}

	return &ProfileDecodeResult{Profile: profile, Valid: true}
}

// AuthenticateAdmin validates admin or editor credentials and generates JWT
func (a *AuthService) AuthenticateAdmin(password string, tenantCtx *tenant.Context) *AuthResult {
	var role string

	// Check against admin password
	if tenantCtx.Config.AdminPassword != "" && password == tenantCtx.Config.AdminPassword {
		role = "admin"
	} else if tenantCtx.Config.EditorPassword != "" && password == tenantCtx.Config.EditorPassword {
		role = "editor"
	} else {
		return &AuthResult{
			Success: false,
			Error:   "Invalid credentials",
		}
	}

	// Generate JWT token
	claims := jwt.MapClaims{
		"role":     role,
		"tenantId": tenantCtx.Config.TenantID,
		"type":     "admin_auth",
		"exp":      time.Now().Add(24 * time.Hour).Unix(), // 24 hour expiry
		"iat":      time.Now().Unix(),
	}

	token, err := a.GenerateJWT(claims, tenantCtx.Config.JWTSecret)
	if err != nil {
		return &AuthResult{
			Success: false,
			Error:   "Token generation failed",
		}
	}

	return &AuthResult{
		Token:   token,
		Role:    role,
		Success: true,
	}
}

// GenerateJWT creates a JWT token with the given claims
func (a *AuthService) GenerateJWT(claims jwt.MapClaims, secret string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("JWT secret not configured")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateAdminRole checks if the request has valid admin authentication
func (a *AuthService) ValidateAdminRole(authHeader string, tenantCtx *tenant.Context) bool {
	return a.validateRole(authHeader, tenantCtx, []string{"admin"})
}

// ValidateAdminOrEditorRole checks if the request has valid admin or editor authentication
func (a *AuthService) ValidateAdminOrEditorRole(authHeader string, tenantCtx *tenant.Context) bool {
	return a.validateRole(authHeader, tenantCtx, []string{"admin", "editor"})
}

// validateRole validates JWT token and checks role membership
func (a *AuthService) validateRole(authHeader string, tenantCtx *tenant.Context, allowedRoles []string) bool {
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		return false
	}

	tokenString := authHeader[7:]

	// Validate JWT token
	claims, err := security.ValidateJWT(tokenString, tenantCtx.Config.JWTSecret)
	if err != nil {
		return false
	}

	// Check token type
	tokenType, ok := claims["type"].(string)
	if !ok || tokenType != "admin_auth" {
		return false
	}

	// Check tenant ID matches
	tokenTenantID, ok := claims["tenantId"].(string)
	if !ok || tokenTenantID != tenantCtx.TenantID {
		return false
	}

	// Check role
	tokenRole, ok := claims["role"].(string)
	if !ok {
		return false
	}

	// Verify role is in allowed list
	return slices.Contains(allowedRoles, tokenRole)
}

// GenerateSecureToken generates a cryptographically secure random token
func (a *AuthService) GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// TokenInfo holds information about a decoded token
type TokenInfo struct {
	Valid     bool           `json:"valid"`
	Claims    map[string]any `json:"claims,omitempty"`
	Role      string         `json:"role,omitempty"`
	TenantID  string         `json:"tenantId,omitempty"`
	ExpiresAt time.Time      `json:"expiresAt,omitempty"`
}

// GetTokenInfo extracts information from a JWT token without validating permissions
func (a *AuthService) GetTokenInfo(tokenString string, tenantCtx *tenant.Context) *TokenInfo {
	if tokenString == "" {
		return &TokenInfo{Valid: false}
	}

	claims, err := security.ValidateJWT(tokenString, tenantCtx.Config.JWTSecret)
	if err != nil {
		return &TokenInfo{Valid: false}
	}

	info := &TokenInfo{
		Valid:  true,
		Claims: claims,
	}

	// Extract common fields
	if role, ok := claims["role"].(string); ok {
		info.Role = role
	}

	if tenantID, ok := claims["tenantId"].(string); ok {
		info.TenantID = tenantID
	}

	if exp, ok := claims["exp"].(float64); ok {
		info.ExpiresAt = time.Unix(int64(exp), 0)
	}

	return info
}
