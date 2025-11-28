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
	TenantID          string   `json:"tenantId"`
	AdminEmail        string   `json:"adminEmail"`
	AdminPassword     string   `json:"adminPassword"`
	AdminPasswordHash string   `json:"adminPasswordHash"`
	Domains           []string `json:"domains"`
	HydrationToken    string   `json:"hydrationToken"`
	TursoDatabaseURL  string   `json:"tursoDatabaseURL"`
	TursoAuthToken    string   `json:"tursoAuthToken"`
	AAIAPIKey         string   `json:"aaiApiKey"`
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

	if len(req.Domains) == 0 {
		req.Domains = []string{"*"}
	}

	if err := s.validateProvisionRequest(req); err != nil {
		marker.SetError(err)
		return err
	}

	jwtSecret, _ := security.GenerateSecureKey(64)
	aesKey, _ := security.GenerateSecureKey(64)

	var finalPasswordHash string
	if req.AdminPasswordHash != "" {
		finalPasswordHash = req.AdminPasswordHash
	} else {
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.AdminPassword), bcrypt.DefaultCost)
		if err != nil {
			marker.SetError(err)
			return fmt.Errorf("failed to hash password: %w", err)
		}
		finalPasswordHash = string(hashedBytes)
	}

	newConfig := &tenant.Config{
		TenantID:          req.TenantID,
		TursoDatabase:     req.TursoDatabaseURL,
		TursoToken:        req.TursoAuthToken,
		JWTSecret:         jwtSecret,
		AESKey:            aesKey,
		TursoEnabled:      req.TursoDatabaseURL != "" && req.TursoAuthToken != "",
		HydrationToken:    req.HydrationToken,
		AdminPasswordHash: finalPasswordHash,
		AAIAPIKey:         req.AAIAPIKey,
	}

	if err := s.saveTenantConfig(newConfig); err != nil {
		marker.SetError(err)
		return err
	}

	// Create initial brand.json with SITE_URL from the first domain in the list
	if err := s.saveInitialBrandConfig(req.TenantID, req.Domains[0]); err != nil {
		marker.SetError(err)
		return err
	}

	if err := s.copyDefaultStyles(req.TenantID); err != nil {
		marker.SetError(err)
		return err
	}

	if err := s.copyDefaultFonts(req.TenantID); err != nil {
		marker.SetError(err)
		return err
	}

	if err := s.copyDefaultDesigns(req.TenantID); err != nil {
		marker.SetError(err)
		return err
	}

	if err := s.updateTenantRegistry(req.TenantID, "reserved", req.Domains); err != nil {
		marker.SetError(err)
		return err
	}

	marker.SetSuccess(true)
	s.logger.Tenant().Info("Tenant successfully provisioned", "tenantId", req.TenantID)
	return nil
}

func (s *MultiTenantService) validateProvisionRequest(req ProvisionRequest) error {
	re := regexp.MustCompile(`^[a-z0-9-]{3,12}$`)
	if !re.MatchString(req.TenantID) {
		return fmt.Errorf("invalid tenant ID format: must be 3-12 lowercase alphanumeric characters or hyphens")
	}
	if req.AdminPasswordHash == "" && len(req.AdminPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(req.Domains) == 0 || req.Domains[0] == "" {
		return fmt.Errorf("at least one domain is required")
	}

	detector := s.tenantManager.GetDetector()
	registry := detector.GetRegistry()

	if _, exists := registry.Tenants[req.TenantID]; exists {
		if req.TenantID == "default" {
			tenantInfo := registry.Tenants[req.TenantID]
			if tenantInfo.Status == "inactive" {
				return nil
			}
		}
		return fmt.Errorf("tenant ID '%s' already exists", req.TenantID)
	}
	return nil
}

// ActivateTenant finalizes tenant setup by creating the database schema.
func (s *MultiTenantService) ActivateTenant(tenantID string) error {
	marker := s.perfTracker.StartOperation("service_activate_tenant", tenantID)
	defer marker.Complete()

	ctx, err := s.tenantManager.NewContextFromID(tenantID)
	if err != nil {
		marker.SetError(err)
		return fmt.Errorf("failed to create context for activation: %w", err)
	}
	defer func() {
		if closeErr := ctx.Close(); closeErr != nil {
			s.logger.System().Warn("Failed to close activation context", "error", closeErr)
		}
	}()

	tableCreator := database.NewTableCreator()
	if err := tableCreator.CreateSchema(ctx.Database.Conn); err != nil {
		marker.SetError(err)
		return fmt.Errorf("database schema creation failed: %w", err)
	}
	if err := tableCreator.SeedInitialContent(ctx.Database.Conn); err != nil {
		marker.SetError(err)
		return fmt.Errorf("database seeding failed: %w", err)
	}

	if err := s.updateTenantRegistry(tenantID, "active", nil); err != nil {
		marker.SetError(err)
		return err
	}

	detector := s.tenantManager.GetDetector()
	if err := detector.RefreshRegistry(); err != nil {
		marker.SetError(err)
		return fmt.Errorf("failed to refresh tenant registry: %w", err)
	}
	s.tenantManager.InvalidateTenantContext(tenantID)

	marker.SetSuccess(true)
	s.logger.Tenant().Info("Tenant successfully activated", "tenantId", tenantID)
	return nil
}

// GetCapacity checks the system's capacity for new tenants.
func (s *MultiTenantService) GetCapacity() (*CapacityResult, error) {
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

	detector := s.tenantManager.GetDetector()
	registry := detector.GetRegistry()

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

	if err := os.WriteFile(registryPath, registryData, 0o644); err != nil {
		return fmt.Errorf("failed to write registry: %w", err)
	}

	return detector.RefreshRegistry()
}

// GetTenantManager returns the tenant manager instance
func (s *MultiTenantService) GetTenantManager() *tenant.Manager {
	return s.tenantManager
}

func copyFile(src, dst string) (err error) {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := sourceFile.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := destFile.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}

func (s *MultiTenantService) copyDefaultStyles(tenantID string) error {
	sourceDir := filepath.Join("pkg", "styles")
	targetDir := filepath.Join(pkgconfig.BackendPath, "config", tenantID, "media", "css")

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create CSS directory: %w", err)
	}

	if err := copyFile(
		filepath.Join(sourceDir, "frontend.css"),
		filepath.Join(targetDir, "frontend.css"),
	); err != nil {
		return fmt.Errorf("failed to copy frontend.css: %w", err)
	}

	if err := copyFile(
		filepath.Join(sourceDir, "custom.css"),
		filepath.Join(targetDir, "custom.css"),
	); err != nil {
		return fmt.Errorf("failed to copy custom.css: %w", err)
	}

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

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create fonts directory: %w", err)
	}

	if err := copyFile(
		filepath.Join(sourceDir, "Inter-Regular.woff2"),
		filepath.Join(targetDir, "Inter-Regular.woff2"),
	); err != nil {
		return fmt.Errorf("failed to copy Inter-Regular.woff2: %w", err)
	}

	if err := copyFile(
		filepath.Join(sourceDir, "Inter-Bold.woff2"),
		filepath.Join(targetDir, "Inter-Bold.woff2"),
	); err != nil {
		return fmt.Errorf("failed to copy Inter-Bold.woff2: %w", err)
	}

	if err := copyFile(
		filepath.Join(sourceDir, "Inter-Black.woff2"),
		filepath.Join(targetDir, "Inter-Black.woff2"),
	); err != nil {
		return fmt.Errorf("failed to copy Inter-Black.woff2: %w", err)
	}

	return nil
}

func (s *MultiTenantService) HasEmailService() bool {
	return s.emailService != nil
}

func (s *MultiTenantService) copyDefaultDesigns(tenantID string) error {
	sourcePath := filepath.Join("pkg", "designs", "designLibrary.json")
	targetPath := filepath.Join(pkgconfig.BackendPath, "config", tenantID, "designLibrary.json")
	return copyFile(sourcePath, targetPath)
}

func (s *MultiTenantService) saveInitialBrandConfig(tenantID, domain string) error {
	siteURL := ""
	if domain != "*" {
		siteURL = fmt.Sprintf("https://%s", domain)
	}

	brandConfig := map[string]any{
		"SITE_URL":             siteURL,
		"HOME_SLUG":            "hello",
		"TRACTSTACK_HOME_SLUG": "HELLO",
		"THEME":                "light-bold",
	}

	configPath := filepath.Join(pkgconfig.BackendPath, "config", tenantID, "brand.json")

	data, err := json.MarshalIndent(brandConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal initial brand config: %w", err)
	}

	return os.WriteFile(configPath, data, 0o644)
}

func (s *MultiTenantService) CompleteSetup(tenantID, hydrationToken string) error {
	marker := s.perfTracker.StartOperation("service_complete_setup", tenantID)
	defer marker.Complete()

	// 1. Clear from env.json (The Source of Truth)
	configPath := filepath.Join(pkgconfig.BackendPath, "config", tenantID, "env.json")

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read tenant config: %w", err)
	}

	var cfg tenant.Config
	if err := json.Unmarshal(configData, &cfg); err != nil {
		return fmt.Errorf("failed to parse tenant config: %w", err)
	}

	// Validation: If a token was passed (internal lookup), match it.
	if cfg.HydrationToken != "" && hydrationToken != "" && cfg.HydrationToken != hydrationToken {
		return fmt.Errorf("invalid hydration token provided")
	}

	// Clear it
	cfg.HydrationToken = ""

	newData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	if err := os.WriteFile(configPath, newData, 0o600); err != nil {
		return fmt.Errorf("failed to save updated config: %w", err)
	}

	// 2. Refresh Registry (to ensure status/domains are current, though token is gone)
	detector := s.tenantManager.GetDetector()
	if err := detector.RefreshRegistry(); err != nil {
		return fmt.Errorf("failed to refresh registry: %w", err)
	}

	marker.SetSuccess(true)
	s.logger.Tenant().Info("Setup completed and hydration token cleared", "tenantId", tenantID)
	return nil
}
