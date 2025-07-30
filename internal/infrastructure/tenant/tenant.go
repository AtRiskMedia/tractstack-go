// Package tenant manages tenant-specific configurations and context,
// isolating multi-tenancy logic from the rest of the application.
package tenant

import (
	"fmt"
	"sync"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/gin-gonic/gin"
)

// Manager coordinates tenant detection and context creation
type Manager struct {
	detector     *Detector
	cacheManager *manager.Manager
	contexts     map[string]*Context
	mutex        sync.RWMutex
}

// NewManager creates and initializes a new tenant manager.
func NewManager() *Manager {
	detector, err := NewDetector()
	if err != nil {
		// In startup context, we can't return error, so we'll panic
		// This should be handled by startup error handling
		panic(fmt.Sprintf("Failed to initialize tenant detector: %v", err))
	}

	cacheManager := manager.NewManager()

	return &Manager{
		detector:     detector,
		cacheManager: cacheManager,
		contexts:     make(map[string]*Context),
	}
}

// GetContext creates or retrieves a tenant context for the request
func (m *Manager) GetContext(c *gin.Context) (*Context, error) {
	// Detect tenant from request
	tenantID, err := m.detector.DetectTenant(c)
	if err != nil {
		return nil, fmt.Errorf("tenant detection failed: %w", err)
	}

	// Check if we have a cached context
	m.mutex.RLock()
	if ctx, exists := m.contexts[tenantID]; exists {
		m.mutex.RUnlock()
		return ctx, nil
	}
	m.mutex.RUnlock()

	// Create new context
	return m.createContext(tenantID)
}

// NewContextFromID creates a new tenant context from a tenant ID string.
// This is used for background tasks that are not tied to a specific HTTP request.
func (m *Manager) NewContextFromID(tenantID string) (*Context, error) {
	return m.createContext(tenantID)
}

// createContext creates a new tenant context
func (m *Manager) createContext(tenantID string) (*Context, error) {
	// Load tenant configuration using existing config loader
	config, err := LoadTenantConfig(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to load tenant config: %w", err)
	}

	// Create database connection using existing database infrastructure
	db, err := NewDatabase(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection: %w", err)
	}

	// Get tenant status from detector
	status := m.detector.GetTenantStatus(tenantID)

	// Create context
	ctx := &Context{
		TenantID:     tenantID,
		Config:       config,
		Database:     db,
		Status:       status,
		CacheManager: m.cacheManager,
	}

	// Cache the context
	m.mutex.Lock()
	m.contexts[tenantID] = ctx
	m.mutex.Unlock()

	return ctx, nil
}

// PreActivateAllTenants activates all tenants in the registry during startup
func (m *Manager) PreActivateAllTenants() error {
	// Load the tenant registry to get all known tenants
	registry, err := LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load tenant registry for pre-activation: %w", err)
	}

	if len(registry.Tenants) == 0 {
		return nil
	}

	// Track activation results
	var failedTenants []string

	// Pre-activate each tenant that isn't already active
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			continue
		}

		if err := m.preActivateSingleTenant(tenantID); err != nil {
			failedTenants = append(failedTenants, tenantID)
			continue
		}
	}

	// Update detector's registry cache after all activations
	if err := m.detector.RefreshRegistry(); err != nil {
		return fmt.Errorf("failed to refresh detector registry: %w", err)
	}

	if len(failedTenants) > 0 {
		return fmt.Errorf("pre-activation failed for tenants: %v", failedTenants)
	}

	return nil
}

// preActivateSingleTenant activates a single tenant during startup
func (m *Manager) preActivateSingleTenant(tenantID string) error {
	// Create context for activation
	ctx, err := m.createContext(tenantID)
	if err != nil {
		return fmt.Errorf("failed to create context for tenant %s: %w", tenantID, err)
	}

	// Verify database connection
	if err := ctx.Database.Conn.Ping(); err != nil {
		return fmt.Errorf("database connection test failed for tenant %s: %w", tenantID, err)
	}

	// Update tenant status to active
	dbType := "sqlite3"
	if ctx.Database.UseTurso {
		dbType = "turso"
	}
	m.detector.UpdateTenantStatus(tenantID, "active", dbType)

	return nil
}

// GetDetector returns the detector for external access (needed by startup code)
func (m *Manager) GetDetector() *Detector {
	return m.detector
}

// Close cleans up all tenant contexts
func (m *Manager) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, ctx := range m.contexts {
		if err := ctx.Close(); err != nil {
			// Log error but continue cleanup
			continue
		}
	}

	m.contexts = make(map[string]*Context)
	return nil
}
