// Package api provides advanced configuration handlers
package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
	"github.com/gin-gonic/gin"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// AdvancedConfigStatusResponse holds the response structure for advanced config status
type AdvancedConfigStatusResponse struct {
	TursoConfigured   bool `json:"tursoConfigured"`
	TursoTokenSet     bool `json:"tursoTokenSet"`
	AdminPasswordSet  bool `json:"adminPasswordSet"`
	EditorPasswordSet bool `json:"editorPasswordSet"`
	AAIAPIKeySet      bool `json:"aaiAPIKeySet"`
}

// AdvancedConfigUpdateRequest holds the request structure for advanced config updates
type AdvancedConfigUpdateRequest struct {
	TursoDatabaseURL   string `json:"TURSO_DATABASE_URL,omitempty"`
	TursoAuthToken     string `json:"TURSO_AUTH_TOKEN,omitempty"`
	AdminPassword      string `json:"ADMIN_PASSWORD,omitempty"`
	EditorPassword     string `json:"EDITOR_PASSWORD,omitempty"`
	AAIAPIKey          string `json:"AAI_API_KEY,omitempty"`
	HomeSlug           string `json:"HOME_SLUG,omitempty"`
	TractStackHomeSlug string `json:"TRACTSTACK_HOME_SLUG,omitempty"`
}

// GetAdvancedConfigStatusHandler returns boolean status flags for advanced configuration
func GetAdvancedConfigStatusHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Validate admin authentication (following brand_handlers.go pattern)
	adminCookie, err := c.Cookie("admin_auth")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Admin authentication required"})
		return
	}

	// Validate JWT token
	claims, err := utils.ValidateJWT(adminCookie, ctx.Config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	// Extract and verify role
	role, ok := claims["role"].(string)
	if !ok || role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Check configuration status (never expose actual values)
	status := AdvancedConfigStatusResponse{
		TursoConfigured:   ctx.Config.TursoDatabase != "",
		TursoTokenSet:     ctx.Config.TursoToken != "",
		AdminPasswordSet:  ctx.Config.AdminPassword != "",
		EditorPasswordSet: ctx.Config.EditorPassword != "",
		AAIAPIKeySet:      ctx.Config.AAIAPIKey != "",
	}

	c.JSON(http.StatusOK, gin.H{"data": status})
}

// UpdateAdvancedConfigHandler handles PUT requests to update advanced configuration
func UpdateAdvancedConfigHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Validate admin authentication (following brand_handlers.go pattern)
	adminCookie, err := c.Cookie("admin_auth")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Admin authentication required"})
		return
	}

	// Validate JWT token
	claims, err := utils.ValidateJWT(adminCookie, ctx.Config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
		return
	}

	// Extract and verify role
	role, ok := claims["role"].(string)
	if !ok || role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Parse request
	var request AdvancedConfigUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Validate Turso pair requirement
	hasTursoURL := request.TursoDatabaseURL != ""
	hasTursoToken := request.TursoAuthToken != ""

	if hasTursoURL != hasTursoToken {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Both Turso Database URL and Auth Token must be provided together"})
		return
	}

	// Test Turso connection if credentials provided
	if hasTursoURL && hasTursoToken {
		if err := testTursoConnection(request.TursoDatabaseURL, request.TursoAuthToken); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Turso connection test failed: %v", err)})
			return
		}
	}

	// Update configuration
	updatedConfig, err := updateAdvancedConfig(ctx.Config, &request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update configuration: %v", err)})
		return
	}

	// Save to disk
	if err := saveAdvancedConfig(ctx.Config.TenantID, updatedConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save configuration: %v", err)})
		return
	}

	// Update in-memory config
	ctx.Config = updatedConfig

	c.JSON(http.StatusOK, gin.H{"message": "Configuration updated successfully"})
}

// testTursoConnection tests the Turso database connection
func testTursoConnection(databaseURL, authToken string) error {
	// Create connection string
	connStr := fmt.Sprintf("%s?authToken=%s", databaseURL, authToken)

	// Attempt to open connection
	db, err := sql.Open("libsql", connStr)
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	// Test with a simple query
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("connection test query failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected query result: %d", result)
	}

	return nil
}

// updateAdvancedConfig applies updates to the tenant configuration
func updateAdvancedConfig(config *tenant.Config, request *AdvancedConfigUpdateRequest) (*tenant.Config, error) {
	// Create a copy of the current config
	updatedConfig := *config

	// Apply updates only for provided fields
	if request.TursoDatabaseURL != "" {
		updatedConfig.TursoDatabase = request.TursoDatabaseURL
	}

	if request.TursoAuthToken != "" {
		updatedConfig.TursoToken = request.TursoAuthToken
	}

	if request.AdminPassword != "" {
		updatedConfig.AdminPassword = request.AdminPassword
	}

	if request.EditorPassword != "" {
		updatedConfig.EditorPassword = request.EditorPassword
	}

	if request.AAIAPIKey != "" {
		updatedConfig.AAIAPIKey = request.AAIAPIKey
	}

	if request.HomeSlug != "" {
		updatedConfig.HomeSlug = request.HomeSlug
	}

	if request.TractStackHomeSlug != "" {
		updatedConfig.TractStackHomeSlug = request.TractStackHomeSlug
	}

	return &updatedConfig, nil
}

// saveAdvancedConfig saves the advanced configuration to env.json
func saveAdvancedConfig(tenantID string, config *tenant.Config) error {
	configPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantID)

	// Ensure config directory exists
	if err := os.MkdirAll(configPath, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Prepare config data for saving (exclude computed fields)
	configData := map[string]interface{}{
		"TURSO_DATABASE_URL":   config.TursoDatabase,
		"TURSO_AUTH_TOKEN":     config.TursoToken,
		"ADMIN_PASSWORD":       config.AdminPassword,
		"EDITOR_PASSWORD":      config.EditorPassword,
		"AAI_API_KEY":          config.AAIAPIKey,
		"HOME_SLUG":            config.HomeSlug,
		"TRACTSTACK_HOME_SLUG": config.TractStackHomeSlug,
		"JWT_SECRET":           config.JWTSecret,
		"AES_KEY":              config.AESKey,
	}

	// Write env.json
	envPath := filepath.Join(configPath, "env.json")
	data, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(envPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
