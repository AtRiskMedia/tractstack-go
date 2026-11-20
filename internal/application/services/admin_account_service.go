package services

import (
	"fmt"
	"strings"
	"time"

	domainUser "github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"golang.org/x/crypto/bcrypt"
)

// AdminAccountService manages Storykeep staff accounts (not the caller's own password).
type AdminAccountService struct {
	logger      *logging.ChanneledLogger
	authService *AuthService
}

// NewAdminAccountService creates an AdminAccountService.
func NewAdminAccountService(logger *logging.ChanneledLogger, authService *AuthService) *AdminAccountService {
	return &AdminAccountService{logger: logger, authService: authService}
}

// CreateAdminInput is the create payload.
type CreateAdminInput struct {
	Username           string
	Email              string
	DisplayName        string
	Role               string
	Password           string
	MustChangePassword bool
}

// UpdateAdminInput is the update payload (nil pointers = leave unchanged).
type UpdateAdminInput struct {
	Username    *string
	Email       *string
	DisplayName *string
	Role        *string
	Password    *string
}

func (s *AdminAccountService) logAuth(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Auth().Info(msg, args...)
	}
}

func (s *AdminAccountService) List(p *AdminPrincipal, tenantCtx *tenant.Context) ([]*domainUser.Admin, error) {
	if p == nil || p.Role != "admin" {
		return nil, fmt.Errorf("forbidden")
	}
	return tenantCtx.AdminRepo().List()
}

func (s *AdminAccountService) Create(p *AdminPrincipal, in CreateAdminInput, tenantCtx *tenant.Context) (*domainUser.Admin, error) {
	if p == nil || p.Role != "admin" {
		return nil, fmt.Errorf("forbidden")
	}
	role := strings.TrimSpace(in.Role)
	if role != "admin" && role != "editor" {
		return nil, fmt.Errorf("invalid role")
	}
	if role == "admin" && !p.IsMain {
		return nil, fmt.Errorf("only the main admin can create admin accounts")
	}
	username := strings.TrimSpace(in.Username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if reservedAdminUsername(username) {
		return nil, fmt.Errorf("username is reserved")
	}
	if len(in.Password) < minPasswordLength {
		return nil, fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	repo := tenantCtx.AdminRepo()
	if existing, _ := repo.FindByUsername(username); existing != nil {
		return nil, fmt.Errorf("username already exists")
	}
	if existing, _ := repo.FindByEmail(email); existing != nil {
		return nil, fmt.Errorf("email already exists")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	createdBy := p.Sub
	admin := &domainUser.Admin{
		ID:                 security.GenerateULID(),
		Username:           username,
		Email:              email,
		DisplayName:        strings.TrimSpace(in.DisplayName),
		Role:               role,
		IsMain:             false,
		PasswordHash:       string(hashed),
		MustChangePassword: in.MustChangePassword,
		TokenVersion:       0,
		CreatedBy:          &createdBy,
		CreatedAt:          time.Now().UTC(),
	}
	if err := repo.Store(admin); err != nil {
		return nil, err
	}
	s.logAuth("Admin account created", "actor", p.Sub, "newId", admin.ID, "role", role, "tenantId", tenantCtx.TenantID)
	return admin, nil
}

func (s *AdminAccountService) Update(p *AdminPrincipal, id string, in UpdateAdminInput, tenantCtx *tenant.Context) (*domainUser.Admin, error) {
	if p == nil || p.Role != "admin" {
		return nil, fmt.Errorf("forbidden")
	}
	repo := tenantCtx.AdminRepo()
	target, err := repo.FindByID(id)
	if err != nil || target == nil {
		return nil, fmt.Errorf("account not found")
	}
	if target.IsMain {
		return nil, fmt.Errorf("cannot edit main admin via this endpoint")
	}
	if target.Role == "admin" && !p.IsMain {
		return nil, fmt.Errorf("only the main admin can edit admin accounts")
	}
	if p.Sub == target.ID {
		return nil, fmt.Errorf("use account self-service for your own profile")
	}

	if in.Username != nil {
		u := strings.TrimSpace(*in.Username)
		if reservedAdminUsername(u) {
			return nil, fmt.Errorf("username is reserved")
		}
		if other, _ := repo.FindByUsername(u); other != nil && other.ID != target.ID {
			return nil, fmt.Errorf("username already exists")
		}
		target.Username = u
	}
	if in.Email != nil {
		e := strings.ToLower(strings.TrimSpace(*in.Email))
		if other, _ := repo.FindByEmail(e); other != nil && other.ID != target.ID {
			return nil, fmt.Errorf("email already exists")
		}
		target.Email = e
		target.TokenVersion++
	}
	if in.DisplayName != nil {
		target.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	if in.Role != nil {
		if !p.IsMain {
			return nil, fmt.Errorf("only the main admin can change roles")
		}
		r := strings.TrimSpace(*in.Role)
		if r != "admin" && r != "editor" {
			return nil, fmt.Errorf("invalid role")
		}
		target.Role = r
		target.TokenVersion++
	}
	if in.Password != nil && *in.Password != "" {
		if len(*in.Password) < minPasswordLength {
			return nil, fmt.Errorf("password must be at least %d characters", minPasswordLength)
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(*in.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		target.PasswordHash = string(hashed)
		target.TokenVersion++
	}

	if err := repo.Update(target); err != nil {
		return nil, err
	}
	if s.authService != nil {
		s.authService.InvalidateAdminPrincipalCache(tenantCtx, target.ID)
	}
	s.logAuth("Admin account updated", "actor", p.Sub, "targetId", target.ID, "tenantId", tenantCtx.TenantID)
	return target, nil
}

func (s *AdminAccountService) Delete(p *AdminPrincipal, id string, tenantCtx *tenant.Context) error {
	if p == nil || p.Role != "admin" {
		return fmt.Errorf("forbidden")
	}
	repo := tenantCtx.AdminRepo()
	target, err := repo.FindByID(id)
	if err != nil || target == nil {
		return fmt.Errorf("account not found")
	}
	if target.IsMain {
		return fmt.Errorf("cannot delete main admin")
	}
	if p.Sub == target.ID {
		return fmt.Errorf("cannot delete yourself via this endpoint")
	}
	if target.Role == "admin" && !p.IsMain {
		return fmt.Errorf("only the main admin can delete admin accounts")
	}
	if err := repo.Delete(id); err != nil {
		return err
	}
	if s.authService != nil {
		s.authService.InvalidateAdminPrincipalCache(tenantCtx, id)
	}
	s.logAuth("Admin account deleted", "actor", p.Sub, "targetId", id, "tenantId", tenantCtx.TenantID)
	return nil
}

// UpdateMe updates the caller's own profile (not username for anyone; email/display for self).
func (s *AdminAccountService) UpdateMe(p *AdminPrincipal, displayName *string, email *string, tenantCtx *tenant.Context) (*domainUser.Admin, error) {
	if p == nil || p.Sub == "" || p.Sub == "sysop" {
		return nil, fmt.Errorf("forbidden")
	}
	repo := tenantCtx.AdminRepo()
	admin, err := repo.FindByID(p.Sub)
	if err != nil || admin == nil {
		return nil, fmt.Errorf("account not found")
	}
	if displayName != nil {
		admin.DisplayName = strings.TrimSpace(*displayName)
	}
	if email != nil {
		e := strings.ToLower(strings.TrimSpace(*email))
		if other, _ := repo.FindByEmail(e); other != nil && other.ID != admin.ID {
			return nil, fmt.Errorf("email already exists")
		}
		admin.Email = e
		admin.TokenVersion++
	}
	if err := repo.Update(admin); err != nil {
		return nil, err
	}
	if s.authService != nil {
		s.authService.InvalidateAdminPrincipalCache(tenantCtx, admin.ID)
	}
	return admin, nil
}

// DeleteSelf allows non-main users to delete their own account.
func (s *AdminAccountService) DeleteSelf(p *AdminPrincipal, tenantCtx *tenant.Context) error {
	if p == nil || p.IsMain || p.Sub == "" || p.Sub == "sysop" {
		return fmt.Errorf("forbidden")
	}
	if err := tenantCtx.AdminRepo().Delete(p.Sub); err != nil {
		return err
	}
	if s.authService != nil {
		s.authService.InvalidateAdminPrincipalCache(tenantCtx, p.Sub)
	}
	s.logAuth("Admin self-deleted", "actor", p.Sub, "tenantId", tenantCtx.TenantID)
	return nil
}
