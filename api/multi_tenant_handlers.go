// Package api provides multi-tenant handlers
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	defaults "github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/email"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// GetTenantCapacityHandler returns tenant capacity info and list of existing tenants
func GetTenantCapacityHandler(c *gin.Context) {
	// Load tenant registry
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tenant registry"})
		return
	}

	// Get max tenants from config
	maxTenants := defaults.MaxTenants

	// Build tenant list
	existingTenants := make([]string, 0, len(registry.Tenants))
	for tenantID := range registry.Tenants {
		existingTenants = append(existingTenants, tenantID)
	}

	// Build response
	response := models.TenantCapacityResponse{
		CurrentCount:    len(registry.Tenants),
		MaxTenants:      maxTenants,
		Available:       maxTenants - len(registry.Tenants),
		ExistingTenants: existingTenants,
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// ProvisionTenantHandler creates a new tenant with "reserved" status
func ProvisionTenantHandler(c *gin.Context) {
	// Parse request
	var request models.TenantProvisioningRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Validate tenant ID
	if err := models.ValidateTenantID(request.TenantID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check capacity
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tenant registry"})
		return
	}

	maxTenants := defaults.MaxTenants
	if len(registry.Tenants) >= maxTenants {
		c.JSON(http.StatusConflict, gin.H{"error": "Maximum tenant capacity reached"})
		return
	}

	// Check for duplicate tenant ID
	if _, exists := registry.Tenants[request.TenantID]; exists {
		c.JSON(http.StatusConflict, gin.H{"error": "Tenant ID already exists"})
		return
	}

	// Validate Turso credentials if provided
	if request.TursoEnabled {
		if request.TursoDatabaseURL == "" || request.TursoAuthToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Both Turso Database URL and Auth Token must be provided when Turso is enabled"})
			return
		}

		// Test Turso connection using shared helper
		if err := TestTursoConnection(request.TursoDatabaseURL, request.TursoAuthToken); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Turso connection test failed: %v", err)})
			return
		}
	}

	// Register tenant with "reserved" status
	if err := tenant.RegisterTenantWithStatus(request.TenantID, "reserved"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to register tenant: %v", err)})
		return
	}

	// Create tenant configuration
	config := &tenant.Config{
		TenantID:           request.TenantID,
		AdminPassword:      request.AdminPassword,
		TursoEnabled:       request.TursoEnabled,
		TursoDatabase:      request.TursoDatabaseURL,
		TursoToken:         request.TursoAuthToken,
		ActivationToken:    ulid.Make().String(),
		JWTSecret:          generateRandomKey(64),
		AESKey:             generateRandomKey(64),
		HomeSlug:           "hello",
		TractStackHomeSlug: "HELLO",
	}

	// Save tenant configuration using existing saveConfig pattern
	configPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", request.TenantID, "env.json")
	if err := saveConfig(config, configPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save tenant config: %v", err)})
		return
	}

	// Send activation email
	emailClient, err := email.NewClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to initialize email client: %v", err)})
		return
	}

	// Build activation URL
	activationURL := fmt.Sprintf("https://%s.sandbox.freewebpress.com/sandbox/activate?token=%s", request.TenantID, config.ActivationToken)

	if err := emailClient.SendTenantActivationEmail(request.TenantID, request.Name, request.Email, activationURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to send activation email: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Tenant provisioned successfully. Activation email sent.",
		"token":   config.ActivationToken,
	})
}

// ActivateTenantHandler activates a tenant using the activation token
func ActivateTenantHandler(c *gin.Context) {
	// Parse request
	var request models.ActivationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Load tenant registry to find tenant with this token
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tenant registry"})
		return
	}

	var foundTenantID string
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "reserved" {
			// Load tenant config to check token
			config, err := tenant.LoadTenantConfig(tenantID)
			if err != nil {
				continue // Skip this tenant if config can't be loaded
			}

			if config.ActivationToken == request.Token {
				foundTenantID = tenantID
				break
			}
		}
	}

	if foundTenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired activation token"})
		return
	}

	// Load and update tenant config
	config, err := tenant.LoadTenantConfig(foundTenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tenant config"})
		return
	}

	// Clear activation token and save config using existing saveConfig pattern
	config.ActivationToken = ""
	configPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", foundTenantID, "env.json")
	if err := saveConfig(config, configPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update tenant config"})
		return
	}

	// Update tenant status to "inactive"
	if err := tenant.UpdateTenantStatus(foundTenantID, "inactive", ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update tenant status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tenant activated successfully"})
}

// Helper function to generate random keys (reuse existing pattern)
func generateRandomKey(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

// saveConfig saves tenant configuration (reuse existing pattern from config.go)
func saveConfig(config *tenant.Config, configPath string) error {
	// Ensure directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create config data (exclude computed fields)
	configData := map[string]interface{}{
		"JWT_SECRET":           config.JWTSecret,
		"AES_KEY":              config.AESKey,
		"TURSO_ENABLED":        config.TursoEnabled,
		"HOME_SLUG":            config.HomeSlug,
		"TRACTSTACK_HOME_SLUG": config.TractStackHomeSlug,
	}

	// Include Turso credentials if present
	if config.TursoDatabase != "" {
		configData["TURSO_DATABASE_URL"] = config.TursoDatabase
	}
	if config.TursoToken != "" {
		configData["TURSO_AUTH_TOKEN"] = config.TursoToken
	}

	// Include admin password if present
	if config.AdminPassword != "" {
		configData["ADMIN_PASSWORD"] = config.AdminPassword
	}

	// Include activation token if present
	if config.ActivationToken != "" {
		configData["ACTIVATION_TOKEN"] = config.ActivationToken
	}

	// Write env.json
	data, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
