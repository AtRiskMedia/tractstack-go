// Package tenant provides multi-tenant configuration and management.
package tenant

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BrandConfig holds tenant-specific branding configuration
type BrandConfig struct {
	SiteInit           bool   `json:"SITE_INIT"`
	WordmarkMode       string `json:"WORDMARK_MODE"`
	OpenDemo           bool   `json:"OPEN_DEMO"`
	HomeSlug           string `json:"HOME_SLUG"`
	TractStackHomeSlug string `json:"TRACTSTACK_HOME_SLUG"`
	Theme              string `json:"THEME"`
	BrandColours       string `json:"BRAND_COLOURS"`
	Socials            string `json:"SOCIALS"`
	SiteURL            string `json:"SITE_URL"`
	Slogan             string `json:"SLOGAN"`
	Footer             string `json:"FOOTER"`
	OGTitle            string `json:"OGTITLE"`
	OGAuthor           string `json:"OGAUTHOR"`
	OGDesc             string `json:"OGDESC"`
	Gtag               string `json:"GTAG"`
	StylesVer          int64  `json:"STYLES_VER"`
	Logo               string `json:"LOGO"`
	Wordmark           string `json:"WORDMARK"`
	Favicon            string `json:"FAVICON"`
	OG                 string `json:"OG"`
	OGLogo             string `json:"OGLOGO"`
	LogoBase64         string `json:"LOGO_BASE64,omitempty"`
	WordmarkBase64     string `json:"WORDMARK_BASE64,omitempty"`
	OGBase64           string `json:"OG_BASE64,omitempty"`
	OGLogoBase64       string `json:"OGLOGO_BASE64,omitempty"`
	FaviconBase64      string `json:"FAVICON_BASE64,omitempty"`
}

// Config holds tenant-specific configuration
type Config struct {
	TenantID           string       `json:"tenantId"`
	TursoDatabase      string       `json:"TURSO_DATABASE_URL"`
	TursoToken         string       `json:"TURSO_AUTH_TOKEN"`
	JWTSecret          string       `json:"JWT_SECRET"`
	AESKey             string       `json:"AES_KEY"`
	AdminPassword      string       `json:"ADMIN_PASSWORD,omitempty"`
	EditorPassword     string       `json:"EDITOR_PASSWORD,omitempty"`
	HomeSlug           string       `json:"HOME_SLUG,omitempty"`
	TractStackHomeSlug string       `json:"TRACTSTACK_HOME_SLUG,omitempty"`
	SQLitePath         string       `json:"-"` // computed, not from JSON
	BrandConfig        *BrandConfig `json:"-"`
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
	registryPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", "t8k", "tenants.json")

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

// LoadBrandConfig loads brand configuration for a specific tenant
func LoadBrandConfig(tenantID string) (*BrandConfig, error) {
	brandPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantID, "brand.json")

	// Return defaults if file doesn't exist
	if _, err := os.Stat(brandPath); os.IsNotExist(err) {
		return &BrandConfig{
			Logo:         "",
			Wordmark:     "",
			WordmarkMode: "",
			Footer:       "",
			Socials:      "",
			SiteURL:      "",
			Slogan:       "",
			Gtag:         "",
			OGAuthor:     "",
			OGTitle:      "",
			OGDesc:       "",
			OG:           "",
			OGLogo:       "",
			Favicon:      "",
		}, nil
	}

	data, err := os.ReadFile(brandPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read brand config: %w", err)
	}

	var brand BrandConfig
	if err := json.Unmarshal(data, &brand); err != nil {
		return nil, fmt.Errorf("failed to parse brand config: %w", err)
	}

	return &brand, nil
}

// LoadTenantConfig loads configuration for a specific tenant and ensures all required fields exist
func LoadTenantConfig(tenantID string) (*Config, error) {
	configPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantID, "env.json")

	config := &Config{
		TenantID:   tenantID,
		SQLitePath: filepath.Join(os.Getenv("HOME"), "t8k-go-server", "db", tenantID, "tractstack.db"),
	}

	// Load existing config if it exists
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read tenant config: %w", err)
		}

		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse tenant config: %w", err)
		}
	}

	// Ensure required fields exist
	needsSave := false

	if config.JWTSecret == "" {
		config.JWTSecret = generateRandomKey(64) // 64 chars for JWT
		needsSave = true
	}

	if config.AESKey == "" {
		config.AESKey = generateRandomKey(64) // 64 chars (32 bytes) for AES-256
		needsSave = true
	}

	// Set default values for HOME_SLUG and TRACTSTACK_HOME_SLUG if not present
	if config.HomeSlug == "" {
		config.HomeSlug = "hello"
		needsSave = true
	}

	if config.TractStackHomeSlug == "" {
		config.TractStackHomeSlug = "HELLO"
		needsSave = true
	}

	// Save config if we generated new keys or set defaults
	if needsSave {
		if err := saveConfig(config, configPath); err != nil {
			return nil, fmt.Errorf("failed to save generated config: %w", err)
		}
	}

	config.TenantID = tenantID
	config.SQLitePath = filepath.Join(os.Getenv("HOME"), "t8k-go-server", "db", tenantID, "tractstack.db")

	// Load brand configuration
	brandConfig, err := LoadBrandConfig(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to load brand config: %w", err)
	}
	config.BrandConfig = brandConfig

	return config, nil
}

// generateRandomKey creates a random hex string of specified length
func generateRandomKey(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("failed to generate random key: %v", err))
	}
	return hex.EncodeToString(bytes)
}

// saveConfig saves the config to the specified path
func saveConfig(config *Config, configPath string) error {
	// Ensure directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create config data (exclude computed fields)
	configData := map[string]string{
		"JWT_SECRET": config.JWTSecret,
		"AES_KEY":    config.AESKey,
	}

	// Include Turso credentials if present
	if config.TursoDatabase != "" {
		configData["TURSO_DATABASE_URL"] = config.TursoDatabase
	}
	if config.TursoToken != "" {
		configData["TURSO_AUTH_TOKEN"] = config.TursoToken
	}

	// Include admin/editor passwords if present
	if config.AdminPassword != "" {
		configData["ADMIN_PASSWORD"] = config.AdminPassword
	}
	if config.EditorPassword != "" {
		configData["EDITOR_PASSWORD"] = config.EditorPassword
	}

	// Include HOME_SLUG and TRACTSTACK_HOME_SLUG
	if config.HomeSlug != "" {
		configData["HOME_SLUG"] = config.HomeSlug
	}
	if config.TractStackHomeSlug != "" {
		configData["TRACTSTACK_HOME_SLUG"] = config.TractStackHomeSlug
	}

	data, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil { // 0600 for security
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// RegisterTenant adds a new tenant to the registry
func RegisterTenant(tenantID string) error {
	registryPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", "t8k", "tenants.json")

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
