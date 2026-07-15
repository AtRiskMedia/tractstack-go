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

// DetectTenant extracts the tenant ID from the request and verifies it against
// the registry. Detection is read-only: the registry is authoritative and is
// populated by the provisioning flow (initialize -> ProvisionTenant -> reserved),
// so an ID absent from the registry is reported as an error rather than being
// silently registered here.
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

	// Check if tenant exists in registry. The registry is authoritative; we do
	// not register here. A config directory without a registry row indicates a
	// desync (the tenant was not created through the provisioning flow), which
	// we surface explicitly instead of masking it.
	if _, exists := d.registry.Tenants[tenantID]; !exists {
		if d.hasConfigDirectory(tenantID) {
			return "", fmt.Errorf("tenant %s has a config directory but no registry entry (registry/config desync)", tenantID)
		}
		return "", fmt.Errorf("unknown tenant: %s", tenantID)
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
	if strings.Contains(domain, ":") {
		domain = strings.Split(domain, ":")[0]
	}
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
