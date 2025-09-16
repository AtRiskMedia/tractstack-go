// Package services provides the multi-tenant service for tenant lifecycle management.
package services

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/database"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/email"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	pkgconfig "github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// MultiTenantService orchestrates tenant lifecycle operations.
type MultiTenantService struct {
	tenantManager *tenant.Manager
	emailService  email.Service
	logger        *logging.ChanneledLogger
	perfTracker   *performance.Tracker
}

// NewMultiTenantService creates a new MultiTenantService.
func NewMultiTenantService(
	tenantManager *tenant.Manager,
	emailService email.Service,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *MultiTenantService {
	return &MultiTenantService{
		tenantManager: tenantManager,
		emailService:  emailService,
		logger:        logger,
		perfTracker:   perfTracker,
	}
}

// ProvisionRequest defines the input for creating a new tenant.
type ProvisionRequest struct {
	TenantID         string   `json:"tenantId"`
	AdminEmail       string   `json:"adminEmail"`
	AdminPassword    string   `json:"adminPassword"`
	Domains          []string `json:"domains"`
	TursoDatabaseURL string   `json:"tursoDatabaseURL"`
	TursoAuthToken   string   `json:"tursoAuthToken"`
}

// ActivationRequest defines the input for activating a tenant.
type ActivationRequest struct {
	Token string `json:"token"`
}

// CapacityResult defines the output for the capacity check.
type CapacityResult struct {
	Available      bool `json:"available"`
	CurrentTenants int  `json:"currentTenants"`
	MaxTenants     int  `json:"maxTenants"`
	AvailableSlots int  `json:"availableSlots"`
}

// ProvisionTenant handles the creation of a new, reserved tenant.
func (s *MultiTenantService) ProvisionTenant(req ProvisionRequest) (string, error) {
	marker := s.perfTracker.StartOperation("service_provision_tenant", req.TenantID)
	defer marker.Complete()

	// Auto-populate domains for sandbox if not provided
	if len(req.Domains) == 0 {
		req.Domains = []string{"*"}
	}

	// 1. Input Validation
	if err := s.validateProvisionRequest(req); err != nil {
		marker.SetError(err)
		return "", err
	}

	// 2. Generate Secrets
	jwtSecret, _ := security.GenerateSecureKey(64)
	aesKey, _ := security.GenerateSecureKey(64)
	activationToken, _ := security.GenerateSecureToken(32)

	// 3. Create Tenant Configuration
	newConfig := &tenant.Config{
		TenantID:        req.TenantID,
		TursoDatabase:   req.TursoDatabaseURL,
		TursoToken:      req.TursoAuthToken,
		JWTSecret:       jwtSecret,
		AESKey:          aesKey,
		TursoEnabled:    req.TursoDatabaseURL != "" && req.TursoAuthToken != "",
		AdminPassword:   req.AdminPassword,
		ActivationToken: activationToken,
	}

	// 4. Persist Configuration
	if err := s.saveTenantConfig(newConfig); err != nil {
		marker.SetError(err)
		return "", err
	}

	if err := s.copyDefaultStyles(req.TenantID); err != nil {
		marker.SetError(err)
		return "", err
	}

	if err := s.copyDefaultFonts(req.TenantID); err != nil {
		marker.SetError(err)
		return "", err
	}

	if err := s.updateTenantRegistry(req.TenantID, "reserved", req.Domains); err != nil {
		marker.SetError(err)
		return "", err
	}

	// 5. Send Activation Email (if avail)
	if s.emailService != nil {
		activationURL := fmt.Sprintf("https://%s/activate?token=%s", req.Domains[0], activationToken)
		if err := s.emailService.SendTenantActivationEmail(req.AdminEmail, req.TenantID, activationURL); err != nil {
			marker.SetError(err)
			s.logger.System().Error("Failed to send activation email", "error", err, "tenantId", req.TenantID)
		} else {
			s.logger.System().Info("Activation email sent successfully", "tenantId", req.TenantID, "adminEmail", req.AdminEmail)
		}
	} else {
		s.logger.System().Warn("Activation email not sent - email service not configured",
			"tenantId", req.TenantID, "adminEmail", req.AdminEmail)
	}

	marker.SetSuccess(true)
	s.logger.Tenant().Info("Tenant successfully provisioned", "tenantId", req.TenantID)
	return activationToken, nil
}

func (s *MultiTenantService) validateProvisionRequest(req ProvisionRequest) error {
	re := regexp.MustCompile(`^[a-z0-9-]{3,12}$`)
	if !re.MatchString(req.TenantID) {
		return fmt.Errorf("invalid tenant ID format: must be 3-12 lowercase alphanumeric characters or hyphens")
	}
	if len(req.AdminPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(req.Domains) == 0 || req.Domains[0] == "" {
		return fmt.Errorf("at least one domain is required")
	}

	// Use detector's in-memory registry instead of reading filesystem
	detector := s.tenantManager.GetDetector()
	registry := detector.GetRegistry()

	if _, exists := registry.Tenants[req.TenantID]; exists {
		// Special case: allow provisioning default tenant if it's inactive (fresh install)
		if req.TenantID == "default" {
			tenantInfo := registry.Tenants[req.TenantID]
			if tenantInfo.Status == "inactive" {
				// Allow provisioning - this is fresh install setup
				return nil
			}
		}
		return fmt.Errorf("tenant ID '%s' already exists", req.TenantID)
	}
	return nil
}

// ActivateTenant finalizes tenant setup by creating the database schema.
func (s *MultiTenantService) ActivateTenant(token string) error {
	marker := s.perfTracker.StartOperation("service_activate_tenant", "unknown")
	defer marker.Complete()

	// 1. Find Tenant by Activation Token
	tenantID, err := s.findTenantByActivationToken(token)
	if err != nil {
		marker.SetError(err)
		return err
	}
	marker.TenantID = tenantID // Update marker with found tenant

	// 2. Create Tenant Context to establish DB connection
	ctx, err := s.tenantManager.NewContextFromID(tenantID)
	if err != nil {
		marker.SetError(err)
		return fmt.Errorf("failed to create context for activation: %w", err)
	}
	defer ctx.Close()

	// 3. Create Database Schema
	tableCreator := database.NewTableCreator()
	if err := tableCreator.CreateSchema(ctx.Database.Conn); err != nil {
		marker.SetError(err)
		return fmt.Errorf("database schema creation failed: %w", err)
	}
	if err := tableCreator.SeedInitialContent(ctx.Database.Conn); err != nil {
		marker.SetError(err)
		return fmt.Errorf("database seeding failed: %w", err)
	}

	// 4. Update Status
	if err := s.updateTenantRegistry(tenantID, "active", nil); err != nil {
		marker.SetError(err)
		return err
	}

	// Refresh detector registry to sync with updated file
	detector := s.tenantManager.GetDetector()
	if err := detector.RefreshRegistry(); err != nil {
		marker.SetError(err)
		return fmt.Errorf("failed to refresh tenant registry: %w", err)
	}
	// Invalidate cached tenant context to force recreation with new status
	s.tenantManager.InvalidateTenantContext(tenantID)

	// 5. Clear Activation Token
	ctx.Config.ActivationToken = ""
	if err := s.saveTenantConfig(ctx.Config); err != nil {
		s.logger.Tenant().Warn("Failed to clear activation token after activation", "error", err, "tenantId", tenantID)
	}

	marker.SetSuccess(true)
	s.logger.Tenant().Info("Tenant successfully activated", "tenantId", tenantID)
	return nil
}

// GetCapacity checks the system's capacity for new tenants.
func (s *MultiTenantService) GetCapacity() (*CapacityResult, error) {
	// Use detector's in-memory registry instead of reading filesystem
	detector := s.tenantManager.GetDetector()
	registry := detector.GetRegistry()

	currentTenants := len(registry.Tenants)
	maxTenants := pkgconfig.MaxTenants
	availableSlots := maxTenants - currentTenants
	availableSlots = max(0, availableSlots)

	return &CapacityResult{
		Available:      availableSlots > 0,
		CurrentTenants: currentTenants,
		MaxTenants:     maxTenants,
		AvailableSlots: availableSlots,
	}, nil
}

func (s *MultiTenantService) saveTenantConfig(config *tenant.Config) error {
	configPath := filepath.Join(pkgconfig.BackendPath, "config", config.TenantID, "env.json")
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, configData, 0o600)
}

func (s *MultiTenantService) updateTenantRegistry(tenantID, status string, domains []string) error {
	registryPath := filepath.Join(pkgconfig.BackendPath, "config", "t8k", "tenants.json")

	// Use detector's in-memory registry as base instead of reading filesystem
	detector := s.tenantManager.GetDetector()
	registry := detector.GetRegistry()

	// Make a copy to avoid modifying the detector's registry directly
	registryCopy := &tenant.TenantRegistry{
		Tenants: make(map[string]tenant.TenantInfo),
	}
	for k, v := range registry.Tenants {
		registryCopy.Tenants[k] = v
	}

	info, exists := registryCopy.Tenants[tenantID]
	if !exists {
		info = tenant.TenantInfo{TenantID: tenantID}
	}
	info.Status = status
	if domains != nil {
		info.Domains = domains
	}
	registryCopy.Tenants[tenantID] = info

	registryData, err := json.MarshalIndent(registryCopy, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	// Write to filesystem
	if err := os.WriteFile(registryPath, registryData, 0o644); err != nil {
		return fmt.Errorf("failed to write registry: %w", err)
	}

	// Refresh detector's in-memory cache to sync with file
	return detector.RefreshRegistry()
}

func (s *MultiTenantService) findTenantByActivationToken(token string) (string, error) {
	// Use detector's in-memory registry instead of reading filesystem
	detector := s.tenantManager.GetDetector()
	registry := detector.GetRegistry()

	for tenantID, info := range registry.Tenants {
		if info.Status == "reserved" {
			config, err := tenant.LoadTenantConfig(tenantID, s.logger)
			if err != nil {
				s.logger.System().Warn("Could not load config for reserved tenant during activation check", "tenantId", tenantID)
				continue
			}
			if config.ActivationToken == token {
				return tenantID, nil
			}
		}
	}

	return "", fmt.Errorf("invalid or expired activation token")
}

// GetTenantManager returns the tenant manager instance
func (s *MultiTenantService) GetTenantManager() *tenant.Manager {
	return s.tenantManager
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Sync to ensure the file is written to disk
	return destFile.Sync()
}

func (s *MultiTenantService) copyDefaultStyles(tenantID string) error {
	sourceDir := filepath.Join("pkg", "styles")
	targetDir := filepath.Join(pkgconfig.BackendPath, "config", tenantID, "media", "css")

	// Create target directory
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create CSS directory: %w", err)
	}

	// Copy frontend.css
	if err := copyFile(
		filepath.Join(sourceDir, "frontend.css"),
		filepath.Join(targetDir, "frontend.css"),
	); err != nil {
		return fmt.Errorf("failed to copy frontend.css: %w", err)
	}

	// Copy custom.css
	if err := copyFile(
		filepath.Join(sourceDir, "custom.css"),
		filepath.Join(targetDir, "custom.css"),
	); err != nil {
		return fmt.Errorf("failed to copy custom.css: %w", err)
	}

	// Copy storykeep.css
	if err := copyFile(
		filepath.Join(sourceDir, "storykeep.css"),
		filepath.Join(targetDir, "storykeep.css"),
	); err != nil {
		return fmt.Errorf("failed to copy storykeep.css: %w", err)
	}

	return nil
}

func (s *MultiTenantService) copyDefaultFonts(tenantID string) error {
	sourceDir := filepath.Join("pkg", "fonts")
	targetDir := filepath.Join(pkgconfig.BackendPath, "config", tenantID, "media", "fonts")

	// Create target directory
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create fonts directory: %w", err)
	}

	// Copy Inter-Regular.woff2
	if err := copyFile(
		filepath.Join(sourceDir, "Inter-Regular.woff2"),
		filepath.Join(targetDir, "Inter-Regular.woff2"),
	); err != nil {
		return fmt.Errorf("failed to copy Inter-Regular.woff2: %w", err)
	}

	// Copy Inter-Bold.woff2
	if err := copyFile(
		filepath.Join(sourceDir, "Inter-Bold.woff2"),
		filepath.Join(targetDir, "Inter-Bold.woff2"),
	); err != nil {
		return fmt.Errorf("failed to copy Inter-Bold.woff2: %w", err)
	}

	// Copy Inter-Black.woff2
	if err := copyFile(
		filepath.Join(sourceDir, "Inter-Black.woff2"),
		filepath.Join(targetDir, "Inter-Black.woff2"),
	); err != nil {
		return fmt.Errorf("failed to copy Inter-Black.woff2: %w", err)
	}

	return nil
}

// HasEmailService returns whether email functionality is available
func (s *MultiTenantService) HasEmailService() bool {
	return s.emailService != nil
}
