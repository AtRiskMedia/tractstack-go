// Package services provides the multi-tenant service for tenant lifecycle management.
package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/database"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/email"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"golang.org/x/crypto/bcrypt"
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
func (s *MultiTenantService) ProvisionTenant(req ProvisionRequest) error {
	marker := s.perfTracker.StartOperation("service_provision_tenant", req.TenantID)
	defer marker.Complete()

	// 1. Input Validation
	if err := s.validateProvisionRequest(req); err != nil {
		marker.SetError(err)
		return err
	}

	// 2. Generate Secrets
	jwtSecret, _ := security.GenerateSecureKey(64)
	aesKey, _ := security.GenerateSecureKey(64)
	activationToken, _ := security.GenerateSecureToken(32)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		marker.SetError(err)
		s.logger.System().Error("Failed to hash admin password during provisioning", "error", err, "tenantId", req.TenantID)
		return fmt.Errorf("password hashing failed")
	}

	// 3. Create Tenant Configuration
	newConfig := &tenant.Config{
		TenantID:        req.TenantID,
		TursoDatabase:   req.TursoDatabaseURL,
		TursoToken:      req.TursoAuthToken,
		JWTSecret:       jwtSecret,
		AESKey:          aesKey,
		TursoEnabled:    req.TursoDatabaseURL != "" && req.TursoAuthToken != "",
		AdminPassword:   string(hashedPassword),
		HomeSlug:        "hello",
		ActivationToken: activationToken,
	}

	// 4. Persist Configuration
	if err := s.saveTenantConfig(newConfig); err != nil {
		marker.SetError(err)
		return err
	}

	if err := s.updateTenantRegistry(req.TenantID, "reserved", req.Domains); err != nil {
		marker.SetError(err)
		return err
	}

	// 5. Send Activation Email
	activationURL := fmt.Sprintf("https://%s/activate?token=%s", req.Domains[0], activationToken)
	if err := s.emailService.SendTenantActivationEmail(req.AdminEmail, req.TenantID, activationURL); err != nil {
		marker.SetError(err)
		s.logger.System().Error("Failed to send activation email", "error", err, "tenantId", req.TenantID)
		// Do not fail the entire operation, but log it as a critical issue.
	}

	marker.SetSuccess(true)
	s.logger.Tenant().Info("Tenant successfully provisioned", "tenantId", req.TenantID)
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
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return nil, fmt.Errorf("could not load tenant registry: %w", err)
	}

	currentTenants := len(registry.Tenants)
	maxTenants := config.MaxTenants
	availableSlots := maxTenants - currentTenants
	if availableSlots < 0 {
		availableSlots = 0
	}

	return &CapacityResult{
		Available:      availableSlots > 0,
		CurrentTenants: currentTenants,
		MaxTenants:     maxTenants,
		AvailableSlots: availableSlots,
	}, nil
}

// --- Private Helper Methods ---

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
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("could not load tenant registry for validation")
	}
	if _, exists := registry.Tenants[req.TenantID]; exists {
		return fmt.Errorf("tenant ID '%s' already exists", req.TenantID)
	}
	return nil
}

func (s *MultiTenantService) saveTenantConfig(config *tenant.Config) error {
	configPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", config.TenantID, "env.json")
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, configData, 0600)
}

func (s *MultiTenantService) updateTenantRegistry(tenantID, status string, domains []string) error {
	registryPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", "t8k", "tenants.json")
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry to update: %w", err)
	}

	info, exists := registry.Tenants[tenantID]
	if !exists {
		info = tenant.TenantInfo{TenantID: tenantID}
	}
	info.Status = status
	if domains != nil {
		info.Domains = domains
	}
	registry.Tenants[tenantID] = info

	registryData, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}
	return os.WriteFile(registryPath, registryData, 0644)
}

func (s *MultiTenantService) findTenantByActivationToken(token string) (string, error) {
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return "", err
	}

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
