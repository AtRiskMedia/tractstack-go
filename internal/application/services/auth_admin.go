package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	domainUser "github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	adminPrincipalCacheTTL = 30 * time.Second
	passwordResetTTL       = 60 * time.Minute
	minPasswordLength      = 8
)

// AdminPrincipal is the authenticated Storykeep staff principal.
type AdminPrincipal struct {
	Sub                string
	Username           string
	Email              string
	Name               string
	Role               string
	IsMain             bool
	MustChangePassword bool
	TokenVersion       string
}

// AuthResultExtended adds principal fields to AuthResult.
type AuthResultExtended struct {
	AuthResult
	Principal *AdminPrincipal
}

func reservedAdminUsername(username string) bool {
	return strings.EqualFold(strings.TrimSpace(username), "admin")
}

func mainPasswordTV(adminPasswordHash string) string {
	sum := sha256.Sum256([]byte(adminPasswordHash))
	return hex.EncodeToString(sum[:])[:16]
}

func dummyBcrypt() {
	_, _ = bcrypt.GenerateFromPassword([]byte("timing-equalization"), bcrypt.DefaultCost)
}

// AuthenticateAdminUsername validates username+password and mints a JWT.
func (a *AuthService) AuthenticateAdminUsername(username, password string, tenantCtx *tenant.Context) *AuthResultExtended {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		dummyBcrypt()
		return &AuthResultExtended{AuthResult: AuthResult{Success: false, Error: "Invalid credentials"}}
	}

	adminRepo := tenantCtx.AdminRepo()
	admin, err := adminRepo.FindByUsername(username)
	if err != nil || admin == nil {
		dummyBcrypt()
		return &AuthResultExtended{AuthResult: AuthResult{Success: false, Error: "Invalid credentials"}}
	}

	var ok bool
	if admin.IsMain || reservedAdminUsername(admin.Username) {
		if tenantCtx.Config.AdminPasswordHash == "" {
			dummyBcrypt()
			return &AuthResultExtended{AuthResult: AuthResult{Success: false, Error: "Invalid credentials"}}
		}
		ok = bcrypt.CompareHashAndPassword([]byte(tenantCtx.Config.AdminPasswordHash), []byte(password)) == nil
	} else {
		ok = bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)) == nil
	}
	if !ok {
		dummyBcrypt()
		return &AuthResultExtended{AuthResult: AuthResult{Success: false, Error: "Invalid credentials"}}
	}

	_ = adminRepo.TouchLastLogin(admin.ID)

	principal := principalFromAdmin(admin, tenantCtx.Config.AdminPasswordHash)
	token, err := a.mintAdminToken(principal, tenantCtx)
	if err != nil {
		return &AuthResultExtended{AuthResult: AuthResult{Success: false, Error: "Token generation failed"}}
	}

	return &AuthResultExtended{
		AuthResult: AuthResult{Token: token, Role: principal.Role, Success: true},
		Principal:  principal,
	}
}

func principalFromAdmin(admin *domainUser.Admin, envHash string) *AdminPrincipal {
	tv := fmt.Sprintf("%d", admin.TokenVersion)
	if admin.IsMain {
		tv = mainPasswordTV(envHash)
	}
	return &AdminPrincipal{
		Sub:                admin.ID,
		Username:           admin.Username,
		Email:              admin.Email,
		Name:               admin.DisplayName,
		Role:               admin.Role,
		IsMain:             admin.IsMain,
		MustChangePassword: admin.MustChangePassword,
		TokenVersion:       tv,
	}
}

func (a *AuthService) mintAdminToken(p *AdminPrincipal, tenantCtx *tenant.Context) (string, error) {
	claims := jwt.MapClaims{
		"sub":      p.Sub,
		"username": p.Username,
		"email":    p.Email,
		"name":     p.Name,
		"role":     p.Role,
		"main":     p.IsMain,
		"mcp":      p.MustChangePassword,
		"tv":       p.TokenVersion,
		"tenantId": tenantCtx.Config.TenantID,
		"type":     "admin_auth",
		"iat":      time.Now().UTC().Unix(),
		"exp":      time.Now().UTC().Add(24 * time.Hour).Unix(),
	}
	return a.GenerateJWT(claims, tenantCtx.Config.JWTSecret)
}

// ValidateTokenWithRolesPrincipal validates JWT and returns the principal.
func (a *AuthService) ValidateTokenWithRolesPrincipal(tokenString string, tenantCtx *tenant.Context, allowedRoles []string) (*AdminPrincipal, bool) {
	if tokenString == "" {
		return nil, false
	}
	token := strings.TrimPrefix(tokenString, "Bearer ")
	claims, err := security.ValidateJWT(token, tenantCtx.Config.JWTSecret)
	if err != nil {
		return nil, false
	}
	tokenType, _ := claims["type"].(string)
	if tokenType != "admin_auth" {
		return nil, false
	}
	tokenTenantID, _ := claims["tenantId"].(string)
	if tokenTenantID != tenantCtx.TenantID {
		return nil, false
	}
	tokenRole, _ := claims["role"].(string)
	allowed := false
	for _, r := range allowedRoles {
		if r == tokenRole {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, false
	}

	sub, _ := claims["sub"].(string)
	if sub == "sysop" {
		return &AdminPrincipal{
			Sub:      "sysop",
			Username: "admin",
			Role:     "admin",
			IsMain:   true,
		}, true
	}

	// Pre-cutover cookies: no sub
	if sub == "" {
		if tokenRole == "editor" {
			return nil, false
		}
		if tokenRole == "admin" {
			main, err := tenantCtx.AdminRepo().FindMain()
			if err != nil || main == nil {
				return &AdminPrincipal{Role: "admin", IsMain: true, Username: "admin"}, true
			}
			return principalFromAdmin(main, tenantCtx.Config.AdminPasswordHash), true
		}
		return nil, false
	}

	admin := a.loadAdminCached(tenantCtx, sub)
	if admin == nil {
		return nil, false
	}
	if admin.Role != tokenRole {
		return nil, false
	}
	p := principalFromAdmin(admin, tenantCtx.Config.AdminPasswordHash)
	claimTV, _ := claims["tv"].(string)
	if claimTV != "" && claimTV != p.TokenVersion {
		return nil, false
	}
	return p, true
}

func (a *AuthService) loadAdminCached(tenantCtx *tenant.Context, id string) *domainUser.Admin {
	cacheKey := "admin_principal:" + id
	if tenantCtx.CacheManager != nil {
		if v, ok := tenantCtx.CacheManager.GetGeneric(tenantCtx.TenantID, cacheKey); ok {
			if admin, ok := v.(*domainUser.Admin); ok {
				return admin
			}
		}
	}
	admin, err := tenantCtx.AdminRepo().FindByID(id)
	if err != nil || admin == nil {
		return nil
	}
	if tenantCtx.CacheManager != nil {
		tenantCtx.CacheManager.SetGenericWithTTL(tenantCtx.TenantID, cacheKey, admin, adminPrincipalCacheTTL)
	}
	return admin
}

// InvalidateAdminPrincipalCache clears cached principal for an admin id.
func (a *AuthService) InvalidateAdminPrincipalCache(tenantCtx *tenant.Context, adminID string) {
	if tenantCtx.CacheManager == nil {
		return
	}
	// Overwrite with expired empty by setting zero TTL entry via short TTL nil — use Set with past by storing sentinel
	tenantCtx.CacheManager.SetGenericWithTTL(tenantCtx.TenantID, "admin_principal:"+adminID, (*domainUser.Admin)(nil), time.Nanosecond)
}

// MintTokenForPrincipal re-mints a JWT for an existing principal (refresh / after password change).
func (a *AuthService) MintTokenForPrincipal(p *AdminPrincipal, tenantCtx *tenant.Context) (string, error) {
	// Reload row for fresh mcp/tv when sub is a real admin id
	if p.Sub != "" && p.Sub != "sysop" {
		admin, err := tenantCtx.AdminRepo().FindByID(p.Sub)
		if err == nil && admin != nil {
			p = principalFromAdmin(admin, tenantCtx.Config.AdminPasswordHash)
		}
	}
	return a.mintAdminToken(p, tenantCtx)
}

// ChangeOwnPassword changes the caller's password.
func (a *AuthService) ChangeOwnPassword(p *AdminPrincipal, current, next string, tenantCtx *tenant.Context) (string, error) {
	if len(next) < minPasswordLength {
		return "", fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	admin, err := tenantCtx.AdminRepo().FindByID(p.Sub)
	if err != nil || admin == nil {
		return "", fmt.Errorf("account not found")
	}

	if admin.IsMain {
		if bcrypt.CompareHashAndPassword([]byte(tenantCtx.Config.AdminPasswordHash), []byte(current)) != nil {
			return "", fmt.Errorf("current password is incorrect")
		}
		if a.configService == nil {
			return "", fmt.Errorf("config service unavailable")
		}
		req := &AdvancedConfigUpdateRequest{AdminPassword: next}
		if err := a.configService.ProcessAdvancedConfigUpdate(req, tenantCtx); err != nil {
			return "", err
		}
		if err := a.configService.SaveAdvancedConfig(tenantCtx); err != nil {
			return "", err
		}
		a.InvalidateAdminPrincipalCache(tenantCtx, admin.ID)
		np := principalFromAdmin(admin, tenantCtx.Config.AdminPasswordHash)
		np.MustChangePassword = false
		return a.mintAdminToken(np, tenantCtx)
	}

	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(current)) != nil {
		return "", fmt.Errorf("current password is incorrect")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	admin.PasswordHash = string(hashed)
	admin.MustChangePassword = false
	admin.TokenVersion++
	if err := tenantCtx.AdminRepo().Update(admin); err != nil {
		return "", err
	}
	a.InvalidateAdminPrincipalCache(tenantCtx, admin.ID)
	return a.mintAdminToken(principalFromAdmin(admin, tenantCtx.Config.AdminPasswordHash), tenantCtx)
}

// RequestPasswordReset always succeeds from the caller's perspective.
func (a *AuthService) RequestPasswordReset(email string, tenantCtx *tenant.Context, hasResend bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !hasResend || a.emailWorker == nil {
		return
	}
	admin, err := tenantCtx.AdminRepo().FindByEmail(email)
	if err != nil || admin == nil {
		return
	}
	raw, err := security.GenerateSecureToken(32)
	if err != nil {
		return
	}
	sum := sha256.Sum256([]byte(raw))
	tokenHash := hex.EncodeToString(sum[:])
	_ = tenantCtx.AdminTokenRepo().InvalidateFor(admin.ID, "reset")
	_ = tenantCtx.AdminTokenRepo().PruneExpired()
	tok := &domainUser.AdminToken{
		ID:        security.GenerateULID(),
		AdminID:   admin.ID,
		Purpose:   "reset",
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(passwordResetTTL),
		CreatedAt: time.Now().UTC(),
	}
	if err := tenantCtx.AdminTokenRepo().Store(tok); err != nil {
		return
	}
	base := resetLinkBase(tenantCtx)
	if base == "" {
		return
	}
	resetURL := fmt.Sprintf("%s/storykeep/reset?token=%s", strings.TrimRight(base, "/"), raw)
	_ = a.emailWorker.Enqueue(EmailJob{
		TenantID:     tenantCtx.TenantID,
		To:           []string{admin.Email},
		Category:     "system",
		TemplateName: "admin-password-reset",
		Data: map[string]any{
			"DisplayName":    admin.DisplayName,
			"ResetURL":       resetURL,
			"ExpiresMinutes": int(passwordResetTTL.Minutes()),
		},
	})
}

func resetLinkBase(tenantCtx *tenant.Context) string {
	if tenantCtx.Config.BrandConfig != nil && tenantCtx.Config.BrandConfig.SiteURL != "" {
		return tenantCtx.Config.BrandConfig.SiteURL
	}
	for _, d := range tenantCtx.Domains {
		if d != "" && d != "*" && !strings.HasPrefix(d, "localhost") && !strings.HasPrefix(d, "127.0.0.1") {
			if strings.HasPrefix(d, "http://") || strings.HasPrefix(d, "https://") {
				return d
			}
			return "https://" + d
		}
	}
	return ""
}

// PasswordTokenStatus returns whether a raw reset token is valid.
func (a *AuthService) PasswordTokenStatus(raw string, tenantCtx *tenant.Context) (valid bool, purpose string) {
	sum := sha256.Sum256([]byte(raw))
	tok, err := tenantCtx.AdminTokenRepo().FindValidByHash(hex.EncodeToString(sum[:]))
	if err != nil || tok == nil {
		return false, ""
	}
	return true, tok.Purpose
}

// ConsumePasswordReset sets a new password from a reset token. No session is issued.
func (a *AuthService) ConsumePasswordReset(raw, newPassword string, tenantCtx *tenant.Context) error {
	if len(newPassword) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	sum := sha256.Sum256([]byte(raw))
	tok, err := tenantCtx.AdminTokenRepo().FindValidByHash(hex.EncodeToString(sum[:]))
	if err != nil || tok == nil {
		return fmt.Errorf("invalid or expired token")
	}
	admin, err := tenantCtx.AdminRepo().FindByID(tok.AdminID)
	if err != nil || admin == nil {
		return fmt.Errorf("account not found")
	}

	if admin.IsMain {
		if a.configService == nil {
			return fmt.Errorf("config service unavailable")
		}
		req := &AdvancedConfigUpdateRequest{AdminPassword: newPassword}
		if err := a.configService.ProcessAdvancedConfigUpdate(req, tenantCtx); err != nil {
			return err
		}
		if err := a.configService.SaveAdvancedConfig(tenantCtx); err != nil {
			return err
		}
	} else {
		hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		admin.PasswordHash = string(hashed)
		admin.MustChangePassword = false
		admin.TokenVersion++
		if err := tenantCtx.AdminRepo().Update(admin); err != nil {
			return err
		}
	}

	_ = tenantCtx.AdminTokenRepo().MarkUsed(tok.ID)
	_ = tenantCtx.AdminTokenRepo().InvalidateFor(admin.ID, "reset")
	a.InvalidateAdminPrincipalCache(tenantCtx, admin.ID)
	return nil
}
