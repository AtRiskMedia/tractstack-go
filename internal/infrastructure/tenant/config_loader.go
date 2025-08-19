// Package tenant handles loading and providing tenant-specific configurations.
package tenant

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
)

// Config represents the structure of a single tenant's configuration
type Config struct {
	TenantID           string             `json:"tenantId"`
	Domains            []string           `json:"domains"`
	Status             string             `json:"status"`
	DatabaseType       string             `json:"databaseType"`
	TursoDatabase      string             `json:"TURSO_DATABASE_URL"`
	TursoToken         string             `json:"TURSO_AUTH_TOKEN"`
	AAIAPIKey          string             `json:"AAI_API_KEY"`
	JWTSecret          string             `json:"JWT_SECRET"`
	AESKey             string             `json:"AES_KEY"`
	TursoEnabled       bool               `json:"TURSO_ENABLED"`
	AdminPassword      string             `json:"ADMIN_PASSWORD,omitempty"`
	EditorPassword     string             `json:"EDITOR_PASSWORD,omitempty"`
	HomeSlug           string             `json:"HOME_SLUG,omitempty"`
	TractStackHomeSlug string             `json:"TRACTSTACK_HOME_SLUG,omitempty"`
	ActivationToken    string             `json:"ACTIVATION_TOKEN,omitempty"`
	SQLitePath         string             `json:"-"`
	BrandConfig        *types.BrandConfig `json:"-"`
}

// LoadTenantConfig loads configuration for a specific tenant from its env.json file.
func LoadTenantConfig(tenantID string, logger *logging.ChanneledLogger) (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not find user home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, "t8k-go-server", "config", tenantID, "env.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("tenant config file not found at %s", configPath)
	}

	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not read tenant config file: %w", err)
	}

	var tenantConfig Config
	if err := json.Unmarshal(configFile, &tenantConfig); err != nil {
		return nil, fmt.Errorf("could not parse tenant config json: %w", err)
	}

	// Set computed fields
	tenantConfig.TenantID = tenantID
	tenantConfig.SQLitePath = filepath.Join(homeDir, "t8k-go-server", "db", tenantID, "tractstack.db")

	// Load brand configuration
	brandConfig, err := LoadBrandConfig(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to load brand config: %w", err)
	}
	tenantConfig.BrandConfig = brandConfig

	// Ensure tailwindWhitelist.json exists
	whitelistPath := filepath.Join(homeDir, "t8k-go-server", "config", tenantID, "tailwindWhitelist.json")
	if _, err := os.Stat(whitelistPath); os.IsNotExist(err) {
		emptyWhitelist := map[string][]string{"safelist": {}}
		whitelistData, err := json.MarshalIndent(emptyWhitelist, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal empty tailwind whitelist: %w", err)
		}

		if err := os.WriteFile(whitelistPath, whitelistData, 0644); err != nil {
			return nil, fmt.Errorf("failed to create tailwind whitelist file: %w", err)
		}
	}

	return &tenantConfig, nil
}

// LoadBrandConfig loads brand configuration for a specific tenant
func LoadBrandConfig(tenantID string) (*types.BrandConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not find user home directory: %w", err)
	}

	brandPath := filepath.Join(homeDir, "t8k-go-server", "config", tenantID, "brand.json")

	// Return defaults if file doesn't exist
	if _, err := os.Stat(brandPath); os.IsNotExist(err) {
		return &types.BrandConfig{
			SiteInit:           false,
			WordmarkMode:       "",
			OpenDemo:           false,
			HomeSlug:           "home",
			TractStackHomeSlug: "tractstack",
			Theme:              "Default",
			BrandColours:       "",
			Socials:            "",
			SiteURL:            "",
			Slogan:             "",
			Footer:             "",
			OGTitle:            "",
			OGAuthor:           "",
			OGDesc:             "",
			Gtag:               "",
			StylesVer:          1,
			Logo:               "",
			Wordmark:           "",
			Favicon:            "",
			OG:                 "",
			OGLogo:             "",
			KnownResources:     &types.KnownResourcesConfig{},
		}, nil
	}

	data, err := os.ReadFile(brandPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read brand config: %w", err)
	}

	var brand types.BrandConfig
	if err := json.Unmarshal(data, &brand); err != nil {
		return nil, fmt.Errorf("failed to parse brand config: %w", err)
	}

	// Load known resources separately
	knownResources, err := LoadKnownResources(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to load known resources: %w", err)
	}
	brand.KnownResources = knownResources

	return &brand, nil
}

// LoadKnownResources loads known resources configuration for a specific tenant
func LoadKnownResources(tenantID string) (*types.KnownResourcesConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not find user home directory: %w", err)
	}

	knownResourcesPath := filepath.Join(homeDir, "t8k-go-server", "config", tenantID, "knownResources.json")

	// Return empty config if file doesn't exist
	if _, err := os.Stat(knownResourcesPath); os.IsNotExist(err) {
		return &types.KnownResourcesConfig{}, nil
	}

	data, err := os.ReadFile(knownResourcesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read known resources config: %w", err)
	}

	var knownResources types.KnownResourcesConfig
	if err := json.Unmarshal(data, &knownResources); err != nil {
		return nil, fmt.Errorf("failed to parse known resources config: %w", err)
	}

	return &knownResources, nil
}

// TenantRegistry holds the global tenant configuration
type TenantRegistry struct {
	Tenants map[string]TenantInfo `json:"tenants"`
}

// TenantInfo holds tenant metadata
type TenantInfo struct {
	TenantID     string   `json:"tenantId"`
	Domains      []string `json:"domains"`
	Status       string   `json:"status"`       // "unknown", "inactive", "active"
	DatabaseType string   `json:"databaseType"` // "turso", "sqlite3"
}

// LoadTenantRegistry loads the global tenant registry
func LoadTenantRegistry() (*TenantRegistry, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not find user home directory: %w", err)
	}

	registryPath := filepath.Join(homeDir, "t8k-go-server", "config", "t8k", "tenants.json")

	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		// Create default registry if it doesn't exist
		defaultRegistry := &TenantRegistry{
			Tenants: map[string]TenantInfo{
				"default": {
					TenantID:     "default",
					Domains:      []string{"*"},
					Status:       "inactive",
					DatabaseType: "",
				},
			},
		}
		return defaultRegistry, nil
	}

	data, err := os.ReadFile(registryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read tenant registry: %w", err)
	}

	var registry TenantRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse tenant registry: %w", err)
	}

	return &registry, nil
}

// RegisterTenant adds a new tenant to the registry
func RegisterTenant(tenantID string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not find user home directory: %w", err)
	}

	registryPath := filepath.Join(homeDir, "t8k-go-server", "config", "t8k", "tenants.json")

	registry, err := LoadTenantRegistry()
	if err != nil {
		return err
	}

	// Add tenant if it doesn't exist
	if _, exists := registry.Tenants[tenantID]; !exists {
		registry.Tenants[tenantID] = TenantInfo{
			TenantID:     tenantID,
			Domains:      []string{"*"},
			Status:       "inactive",
			DatabaseType: "",
		}

		// Ensure directory exists
		registryDir := filepath.Dir(registryPath)
		if err := os.MkdirAll(registryDir, 0755); err != nil {
			return fmt.Errorf("failed to create registry directory: %w", err)
		}

		// Save registry
		data, err := json.MarshalIndent(registry, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal registry: %w", err)
		}

		if err := os.WriteFile(registryPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write registry: %w", err)
		}
	}

	return nil
}
