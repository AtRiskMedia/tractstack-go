// Package services provides application-level orchestration services
package services

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
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

// CreateLeadResult holds the result of a lead creation operation
type CreateLeadResult struct {
	Success        bool
	Profile        *user.Profile
	Token          string
	EncryptedEmail string
	EncryptedCode  string
	Error          string
}

// DecodeProfileToken validates and decodes a JWT profile token
func (a *AuthService) DecodeProfileToken(tokenString string, tenantCtx *tenant.Context) *ProfileDecodeResult {
	if tokenString == "" {
		return &ProfileDecodeResult{Profile: nil, Valid: false}
	}

	claims, err := security.ValidateJWT(tokenString, tenantCtx.Config.JWTSecret)
	if err != nil {
		return &ProfileDecodeResult{Profile: nil, Valid: false}
	}

	profile := security.GetProfileFromClaims(claims)
	if profile == nil {
		return &ProfileDecodeResult{Profile: nil, Valid: false}
	}

	return &ProfileDecodeResult{Profile: profile, Valid: true}
}

// AuthenticateAdmin validates admin or editor credentials and generates JWT
func (a *AuthService) AuthenticateAdmin(password string, tenantCtx *tenant.Context) *AuthResult {
	var role string

	if tenantCtx.Config.AdminPassword != "" && password == tenantCtx.Config.AdminPassword {
		role = "admin"
	} else if tenantCtx.Config.EditorPassword != "" && password == tenantCtx.Config.EditorPassword {
		role = "editor"
	} else {
		return &AuthResult{Success: false, Error: "Invalid credentials"}
	}

	claims := jwt.MapClaims{
		"role":     role,
		"tenantId": tenantCtx.Config.TenantID,
		"type":     "admin_auth",
		"iat":      time.Now().UTC().Unix(),
		"exp":      time.Now().UTC().Add(24 * time.Hour).Unix(),
	}

	token, err := a.GenerateJWT(claims, tenantCtx.Config.JWTSecret)
	if err != nil {
		return &AuthResult{Success: false, Error: "Token generation failed"}
	}

	return &AuthResult{
		Token:   token,
		Role:    role,
		Success: true,
	}
}

// CreateLead creates a new lead with encrypted credentials
func (a *AuthService) CreateLead(firstName, email, password, contactPersona, shortBio string, tenantCtx *tenant.Context) (*CreateLeadResult, error) {
	leadRepo := tenantCtx.LeadRepo()
	encryptedEmail, err := security.Encrypt(email, tenantCtx.Config.AESKey)
	if err != nil {
		return nil, fmt.Errorf("email encryption failed")
	}

	encryptedCode, err := security.Encrypt(password, tenantCtx.Config.AESKey)
	if err != nil {
		return nil, fmt.Errorf("password encryption failed")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("password hashing failed")
	}

	newLead := &user.Lead{
		ID:             security.GenerateULID(),
		FirstName:      firstName,
		Email:          email,
		PasswordHash:   string(hashedPassword),
		ContactPersona: contactPersona,
		ShortBio:       shortBio,
		EncryptedCode:  encryptedCode,
		EncryptedEmail: encryptedEmail,
		CreatedAt:      time.Now().UTC(),
		Changed:        time.Now().UTC(),
	}

	if err := leadRepo.Store(newLead); err != nil {
		a.logger.Auth().Error("Failed to store new lead", "error", err, "leadId", newLead.ID)
		return nil, fmt.Errorf("failed to create profile")
	}

	profile := &user.Profile{
		LeadID:         newLead.ID,
		Firstname:      newLead.FirstName,
		Email:          newLead.Email,
		ContactPersona: newLead.ContactPersona,
		ShortBio:       newLead.ShortBio,
	}

	token, err := security.GenerateProfileToken(profile, tenantCtx.Config.JWTSecret, tenantCtx.Config.AESKey)
	if err != nil {
		return nil, fmt.Errorf("token generation failed")
	}

	return &CreateLeadResult{
		Success:        true,
		Profile:        profile,
		Token:          token,
		EncryptedEmail: encryptedEmail,
		EncryptedCode:  encryptedCode,
	}, nil
}

// GenerateJWT creates a JWT token with given claims
func (a *AuthService) GenerateJWT(claims jwt.MapClaims, jwtSecret string) (string, error) {
	if _, ok := claims["iat"]; !ok {
		claims["iat"] = time.Now().UTC().Unix()
	}
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().UTC().Add(24 * time.Hour).Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

// ValidateAdminToken checks if a token belongs to an admin user
func (a *AuthService) ValidateAdminToken(tokenString string, tenantCtx *tenant.Context) bool {
	return a.ValidateTokenWithRoles(tokenString, tenantCtx, []string{"admin"})
}

// ValidateAdminOrEditorToken checks if a token belongs to an admin or editor user
func (a *AuthService) ValidateAdminOrEditorToken(tokenString string, tenantCtx *tenant.Context) bool {
	return a.ValidateTokenWithRoles(tokenString, tenantCtx, []string{"admin", "editor"})
}

// ValidateTokenWithRoles validates a token and checks if the role is in the allowed list
func (a *AuthService) ValidateTokenWithRoles(tokenString string, tenantCtx *tenant.Context, allowedRoles []string) bool {
	if tokenString == "" {
		return false
	}

	token := strings.TrimPrefix(tokenString, "Bearer ")

	claims, err := security.ValidateJWT(token, tenantCtx.Config.JWTSecret)
	if err != nil {
		return false
	}

	tokenType, ok := claims["type"].(string)
	if !ok || tokenType != "admin_auth" {
		return false
	}

	tokenTenantID, ok := claims["tenantId"].(string)
	if !ok || tokenTenantID != tenantCtx.TenantID {
		return false
	}

	tokenRole, ok := claims["role"].(string)
	if !ok {
		return false
	}

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

	token := strings.TrimPrefix(tokenString, "Bearer ")

	claims, err := security.ValidateJWT(token, tenantCtx.Config.JWTSecret)
	if err != nil {
		return &TokenInfo{Valid: false}
	}

	info := &TokenInfo{
		Valid:  true,
		Claims: claims,
	}

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

// ValidateEncryptedCredentials validates profile credentials using encrypted data
func (a *AuthService) ValidateEncryptedCredentials(encryptedEmail, encryptedCode string, tenantCtx *tenant.Context) *user.Profile {
	leadRepo := tenantCtx.LeadRepo()
	decryptedEmail, err := security.Decrypt(encryptedEmail, tenantCtx.Config.AESKey)
	if err != nil {
		a.logger.Auth().Warn("Failed to decrypt email for credential validation", "tenantId", tenantCtx.TenantID)
		return nil
	}

	decryptedCode, err := security.Decrypt(encryptedCode, tenantCtx.Config.AESKey)
	if err != nil {
		a.logger.Auth().Warn("Failed to decrypt code for credential validation", "tenantId", tenantCtx.TenantID)
		return nil
	}

	lead, err := leadRepo.FindByEmail(decryptedEmail)
	if err != nil || lead == nil {
		return nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(lead.PasswordHash), []byte(decryptedCode)); err != nil {
		return nil
	}

	return &user.Profile{
		LeadID:         lead.ID,
		Firstname:      lead.FirstName,
		Email:          lead.Email,
		ContactPersona: lead.ContactPersona,
		ShortBio:       lead.ShortBio,
	}
}
