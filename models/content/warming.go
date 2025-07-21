// Package content provides storyfragments
package content

import (
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// WarmAllTenants warms critical content for all active tenants after pre-activation
func WarmAllTenants(tenantManager *tenant.Manager) error {
	log.Println("=== Starting critical content warming ===")
	start := time.Now().UTC()

	// Get list of active tenants
	activeTenants, err := getActiveTenantList()
	if err != nil {
		return fmt.Errorf("failed to get active tenant list: %w", err)
	}

	if len(activeTenants) == 0 {
		log.Println("No active tenants found - skipping content warming")
		return nil
	}

	log.Printf("Warming critical content for %d active tenants", len(activeTenants))

	// Track warming results
	warmedCount := 0
	failedTenants := make([]string, 0)

	// Warm critical content for each active tenant
	for _, tenantID := range activeTenants {
		log.Printf("Warming content for tenant: '%s'", tenantID)

		if err := warmTenantContent(tenantID); err != nil {
			log.Printf("ERROR: Failed to warm content for tenant '%s': %v", tenantID, err)
			failedTenants = append(failedTenants, tenantID)
			continue
		}

		warmedCount++
		log.Printf("✓ Successfully warmed content for tenant: '%s'", tenantID)
	}

	// Report results
	elapsed := time.Since(start)
	log.Printf("=== Content warming complete in %v ===", elapsed)
	log.Printf("  - Warmed: %d tenants", warmedCount)
	log.Printf("  - Failed: %d tenants", len(failedTenants))

	if len(failedTenants) > 0 {
		log.Printf("Failed tenants: %v", failedTenants)
		// Non-blocking: return error but don't fail startup
		return fmt.Errorf("content warming failed for %d tenants: %v", len(failedTenants), failedTenants)
	}

	log.Println("✓ All tenant content successfully warmed!")
	return nil
}

// warmTenantContent warms critical content for a single tenant
func warmTenantContent(tenantID string) error {
	tenantStart := time.Now().UTC()

	// Load tenant configuration to get HOME_SLUG
	config, err := tenant.LoadTenantConfig(tenantID)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create database connection
	database, err := tenant.NewDatabase(config)
	if err != nil {
		return fmt.Errorf("failed to create database connection: %w", err)
	}
	defer database.Close()

	// Create tenant context for storyfragment service
	ctx := &tenant.Context{
		TenantID: tenantID,
		Config:   config,
		Database: database,
		Status:   "active",
	}

	// Create storyfragment service with cache
	storyFragmentService := NewStoryFragmentService(ctx, cache.GetGlobalManager())

	// Determine which HOME storyfragment to warm
	homeSlug := config.HomeSlug
	if homeSlug == "" {
		homeSlug = "hello" // Default fallback
	}

	log.Printf("  - Warming HOME storyfragment with slug: '%s'", homeSlug)

	// Warm the HOME storyfragment (this populates the cache)
	storyFragment, err := storyFragmentService.GetBySlug(homeSlug)
	if err != nil {
		return fmt.Errorf("failed to warm HOME storyfragment '%s': %w", homeSlug, err)
	}

	if storyFragment == nil {
		log.Printf("  - Warning: HOME storyfragment '%s' not found for tenant '%s'", homeSlug, tenantID)
	} else {
		log.Printf("  - HOME storyfragment '%s' warmed (ID: %s, %d panes)",
			homeSlug, storyFragment.ID, len(storyFragment.PaneIDs))
	}

	// Optional: Also warm TRACTSTACK_HOME_SLUG if different
	if config.TractStackHomeSlug != "" && config.TractStackHomeSlug != homeSlug {
		log.Printf("  - Warming TRACTSTACK HOME storyfragment with slug: '%s'", config.TractStackHomeSlug)

		tractStackStoryFragment, err := storyFragmentService.GetBySlug(config.TractStackHomeSlug)
		if err != nil {
			log.Printf("  - Warning: Failed to warm TRACTSTACK HOME storyfragment '%s': %v", config.TractStackHomeSlug, err)
		} else if tractStackStoryFragment != nil {
			log.Printf("  - TRACTSTACK HOME storyfragment '%s' warmed (ID: %s, %d panes)",
				config.TractStackHomeSlug, tractStackStoryFragment.ID, len(tractStackStoryFragment.PaneIDs))
		}
	}

	elapsed := time.Since(tenantStart)
	log.Printf("  - Tenant '%s' content warmed in %v", tenantID, elapsed)
	return nil
}

// getActiveTenantList returns a list of all active tenant IDs from the registry
func getActiveTenantList() ([]string, error) {
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load tenant registry: %w", err)
	}

	activeTenants := make([]string, 0)
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeTenants = append(activeTenants, tenantID)
		}
	}

	return activeTenants, nil
}
