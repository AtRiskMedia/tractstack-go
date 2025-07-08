// Package tenant provides startup pre-activation functionality.
package tenant

import (
	"fmt"
	"log"
	"time"
)

// PreActivateAllTenants activates all tenants in the registry during startup
func PreActivateAllTenants(manager *Manager) error {
	log.Println("=== Starting tenant pre-activation ===")
	start := time.Now().UTC()

	// Load the tenant registry to get all known tenants
	registry, err := LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load tenant registry for pre-activation: %w", err)
	}

	if len(registry.Tenants) == 0 {
		log.Println("No tenants found in registry - skipping pre-activation")
		return nil
	}

	log.Printf("Found %d tenants in registry", len(registry.Tenants))

	// Track activation results
	activatedCount := 0
	skippedCount := 0
	failedTenants := make([]string, 0)

	// Pre-activate each tenant that isn't already active
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			log.Printf("Tenant '%s' already active - skipping", tenantID)
			skippedCount++
			continue
		}

		log.Printf("Pre-activating tenant: '%s' (current status: %s)", tenantID, tenantInfo.Status)

		if err := preActivateSingleTenant(tenantID); err != nil {
			log.Printf("ERROR: Failed to pre-activate tenant '%s': %v", tenantID, err)
			failedTenants = append(failedTenants, tenantID)
			continue
		}

		activatedCount++
		log.Printf("✓ Successfully pre-activated tenant: '%s'", tenantID)
	}

	// Update manager's detector cache after all activations
	if activatedCount > 0 {
		log.Printf("Refreshing tenant manager detector cache...")
		updatedRegistry, err := LoadTenantRegistry()
		if err != nil {
			log.Printf("Warning: Failed to reload registry for cache update: %v", err)
		} else {
			manager.detector.registry = updatedRegistry
		}
	}

	// Report results
	elapsed := time.Since(start)
	log.Printf("=== Pre-activation complete in %v ===", elapsed)
	log.Printf("  - Activated: %d tenants", activatedCount)
	log.Printf("  - Skipped (already active): %d tenants", skippedCount)
	log.Printf("  - Failed: %d tenants", len(failedTenants))

	if len(failedTenants) > 0 {
		log.Printf("Failed tenants: %v", failedTenants)
		return fmt.Errorf("pre-activation failed for %d tenants: %v", len(failedTenants), failedTenants)
	}

	log.Println("✓ All tenants successfully pre-activated!")
	return nil
}

// preActivateSingleTenant activates a single tenant during startup
func preActivateSingleTenant(tenantID string) error {
	tenantStart := time.Now().UTC()

	// Load tenant configuration
	config, err := LoadTenantConfig(tenantID)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create database connection
	database, err := NewDatabase(config)
	if err != nil {
		return fmt.Errorf("failed to create database connection: %w", err)
	}
	defer database.Close()

	// Verify database connection
	if err := database.Conn.Ping(); err != nil {
		return fmt.Errorf("database connection test failed: %w", err)
	}

	// Log connection type
	connType := "SQLite"
	if database.UseTurso {
		connType = "Turso"
	}
	log.Printf("  - Database connection (%s): %s", connType, database.GetConnectionInfo())

	// Create startup context (no HTTP request context needed)
	ctx := &Context{
		TenantID: tenantID,
		Config:   config,
		Database: database,
		Status:   "inactive", // Force activation attempt
	}

	// Run activation process
	if err := ActivateTenant(ctx); err != nil {
		return fmt.Errorf("activation failed: %w", err)
	}

	elapsed := time.Since(tenantStart)
	log.Printf("  - Tenant '%s' activated in %v", tenantID, elapsed)
	return nil
}

// ValidatePreActivation verifies all tenants are active after pre-activation
func ValidatePreActivation() error {
	log.Println("=== Validating pre-activation results ===")

	registry, err := LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry for validation: %w", err)
	}

	if len(registry.Tenants) == 0 {
		log.Println("No tenants to validate")
		return nil
	}

	inactiveTenants := make([]string, 0)
	activeTenants := make([]string, 0)

	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status != "active" {
			inactiveTenants = append(inactiveTenants, tenantID)
		} else {
			activeTenants = append(activeTenants, tenantID)
		}
	}

	log.Printf("Active tenants: %v", activeTenants)

	if len(inactiveTenants) > 0 {
		log.Printf("Inactive tenants: %v", inactiveTenants)
		return fmt.Errorf("validation failed - %d tenants still inactive: %v",
			len(inactiveTenants), inactiveTenants)
	}

	log.Printf("✓ Validation passed - all %d tenants are active", len(registry.Tenants))
	return nil
}

// GetTenantList returns a list of all tenant IDs from the registry
func GetTenantList() ([]string, error) {
	registry, err := LoadTenantRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load tenant registry: %w", err)
	}

	tenantIDs := make([]string, 0, len(registry.Tenants))
	for tenantID := range registry.Tenants {
		tenantIDs = append(tenantIDs, tenantID)
	}

	return tenantIDs, nil
}

// GetActiveTenantCount returns the number of active tenants
func GetActiveTenantCount() (int, error) {
	registry, err := LoadTenantRegistry()
	if err != nil {
		return 0, fmt.Errorf("failed to load tenant registry: %w", err)
	}

	activeCount := 0
	for _, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeCount++
		}
	}

	return activeCount, nil
}
