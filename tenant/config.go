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

// Config holds tenant-specific configuration
type Config struct {
	TenantID      string `json:"tenantId"`
	TursoDatabase string `json:"TURSO_DATABASE_URL"`
	TursoToken    string `json:"TURSO_AUTH_TOKEN"`
	JWTSecret     string `json:"JWT_SECRET"`
	AESKey        string `json:"AES_KEY"`
	SQLitePath    string `json:"-"` // computed, not from JSON
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
		config.AESKey = generateRandomKey(32) // 32 chars for AES-256
		needsSave = true
	}

	// Save config if we generated new keys
	if needsSave {
		if err := saveConfig(config, configPath); err != nil {
			return nil, fmt.Errorf("failed to save generated config: %w", err)
		}
	}

	config.TenantID = tenantID
	config.SQLitePath = filepath.Join(os.Getenv("HOME"), "t8k-go-server", "db", tenantID, "tractstack.db")

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
