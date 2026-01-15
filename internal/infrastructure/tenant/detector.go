// Package tenant provides tenant detection and validation.
package tenant

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"github.com/gin-gonic/gin"
)

// Detector handles tenant detection from HTTP requests
type Detector struct {
	registry    *Registry
	multiTenant bool
	logger      *logging.ChanneledLogger
}

// NewDetector creates a new tenant detector
func NewDetector(logger *logging.ChanneledLogger) (*Detector, error) {
	registry, err := LoadTenantRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load tenant registry: %w", err)
	}

	multiTenant := false
	if val := os.Getenv("ENABLE_MULTI_TENANT"); val != "" {
		multiTenant, _ = strconv.ParseBool(val)
	}

	return &Detector{
		registry:    registry,
		multiTenant: multiTenant,
		logger:      logger,
	}, nil
}

// DetectTenant extracts tenant ID from request and auto-registers if needed
func (d *Detector) DetectTenant(c *gin.Context) (string, error) {
	var tenantID string

	if d.multiTenant {
		// Get tenant ID from header first (set by Astro middleware)
		tenantID = c.GetHeader("X-Tenant-ID")
		// FALLBACK: Check query parameter for SSE connections
		// EventSource API cannot set custom headers, so we allow tenantId as query param
		if tenantID == "" {
			tenantID = c.Query("tenantId")
		}

		if tenantID == "" {
			return "", fmt.Errorf("missing tenant ID header in multi-tenant mode")
		}
	} else {
		// Single tenant mode - always use "default"
		tenantID = "default"
	}

	// Check if tenant exists in registry
	if _, exists := d.registry.Tenants[tenantID]; !exists {
		// Auto-register tenant if it has a config directory or if it's default
		if tenantID == "default" || d.hasConfigDirectory(tenantID) {
			if err := d.registerTenant(tenantID); err != nil {
				return "", fmt.Errorf("failed to auto-register tenant %s: %w", tenantID, err)
			}
			// Reload registry after registration
			if err := d.RefreshRegistry(); err != nil {
				return "", fmt.Errorf("failed to reload registry after auto-registration: %w", err)
			}
		} else {
			return "", fmt.Errorf("unknown tenant: %s", tenantID)
		}
	}

	return tenantID, nil
}

// hasConfigDirectory checks if a tenant has a config directory
func (d *Detector) hasConfigDirectory(tenantID string) bool {
	configDir := filepath.Join(config.BackendPath, "config", tenantID)
	if _, err := os.Stat(configDir); err == nil {
		return true
	}
	return false
}

// registerTenant registers a new tenant (simplified version for auto-registration)
func (d *Detector) registerTenant(tenantID string) error {
	// This is a simplified version - in full implementation would call
	// the actual tenant registration logic from baseline tenant/registration.go
	// For now, we'll assume the tenant directory exists and just add to registry

	// Create basic tenant info
	tenantInfo := Info{
		TenantID:     tenantID,
		Domains:      []string{"*"}, // Default to allow all domains
		Status:       "inactive",
		DatabaseType: "",
	}

	// Add to in-memory registry
	d.registry.Tenants[tenantID] = tenantInfo

	// In full implementation, would also save registry to disk
	// For now, we'll rely on the existing registry management

	return nil
}

// ValidateDomain checks if the request domain is allowed for the tenant
func (d *Detector) ValidateDomain(tenantID, domain string) bool {
	tenantInfo, exists := d.registry.Tenants[tenantID]
	if !exists {
		return false
	}

	// Check if any domain is allowed
	for _, allowedDomain := range tenantInfo.Domains {
		if allowedDomain == "*" {
			return true
		}
		if strings.EqualFold(allowedDomain, domain) {
			return true
		}
	}

	return false
}

// GetTenantStatus returns the current status of a tenant
func (d *Detector) GetTenantStatus(tenantID string) string {
	if tenantInfo, exists := d.registry.Tenants[tenantID]; exists {
		return tenantInfo.Status
	}
	return "unknown"
}

// UpdateTenantStatus updates the cached registry status
func (d *Detector) UpdateTenantStatus(tenantID, status, dbType string) {
	if tenantInfo, exists := d.registry.Tenants[tenantID]; exists {
		tenantInfo.Status = status
		if dbType != "" {
			tenantInfo.DatabaseType = dbType
		}
		d.registry.Tenants[tenantID] = tenantInfo
	}
}

// RefreshRegistry reloads the tenant registry from disk
func (d *Detector) RefreshRegistry() error {
	registry, err := LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to refresh tenant registry: %w", err)
	}
	d.registry = registry
	return nil
}

// GetRegistry returns the current registry (for external access)
func (d *Detector) GetRegistry() *Registry {
	return d.registry
}

// ResolveTenantByDomain finds a tenant ID that claims the given domain
func (d *Detector) ResolveTenantByDomain(domain string) (string, error) {
	// Normalize domain for consistent comparison
	targetDomain := strings.ToLower(domain)

	for tenantID, info := range d.registry.Tenants {
		// 1. Check if this tenant explicitly claims the domain
		claimsDomain := false
		for _, allowedDomain := range info.Domains {
			if strings.EqualFold(allowedDomain, targetDomain) {
				claimsDomain = true
				break
			}
		}

		if !claimsDomain {
			continue
		}

		// 2. Specificity Rule (Suffix Exclusion)
		// If a tenant owns "sandbox.freewebpress.com" AND "freewebpress.com",
		// and the request is for "freewebpress.com", we assume the latter is
		// just for infrastructure/CORS. We detect this by checking if the
		// requested domain is a suffix (preceded by a dot) of any other
		// domain owned by this tenant.
		isInfrastructureParent := false
		suffixCheck := "." + targetDomain

		for _, otherDomain := range info.Domains {
			other := strings.ToLower(otherDomain)
			if strings.HasSuffix(other, suffixCheck) {
				isInfrastructureParent = true
				break
			}
		}

		if isInfrastructureParent {
			d.logger.System().Debug("Skipping infrastructure domain match",
				"tenantId", tenantID,
				"domain", domain)
			continue
		}

		return tenantID, nil
	}
	return "", fmt.Errorf("domain not found: %s", domain)
}
