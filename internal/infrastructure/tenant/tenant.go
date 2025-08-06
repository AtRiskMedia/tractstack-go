// Package tenant manages tenant-specific configurations and context,
// isolating multi-tenancy logic from the rest of the application.
package tenant

import (
	"fmt"
	"log"
	"sync"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/gin-gonic/gin"
)

// Manager coordinates tenant detection and context creation
type Manager struct {
	detector       *Detector
	cacheManager   *manager.Manager
	contexts       map[string]*Context
	contextMutexes sync.Map // Per-tenant mutexes for fine-grained locking
	globalMutex    sync.RWMutex
	logger         *logging.ChanneledLogger
}

// NewManager creates and initializes a new tenant manager.
func NewManager(logger *logging.ChanneledLogger) *Manager {
	detector, err := NewDetector(logger)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize tenant detector: %v", err))
	}

	cacheManager := manager.NewManager(logger)

	return &Manager{
		detector:     detector,
		cacheManager: cacheManager,
		contexts:     make(map[string]*Context),
		logger:       logger,
	}
}

// GetContext creates or retrieves a tenant context for the request
func (m *Manager) GetContext(c *gin.Context) (*Context, error) {
	tenantID, err := m.detector.DetectTenant(c)
	if err != nil {
		return nil, fmt.Errorf("tenant detection failed: %w", err)
	}

	m.globalMutex.RLock()
	if ctx, exists := m.contexts[tenantID]; exists {
		m.globalMutex.RUnlock()
		if ctx.Database != nil && ctx.Database.Conn != nil {
			return ctx, nil
		}
	} else {
		m.globalMutex.RUnlock()
	}

	tenantMutexInterface, _ := m.contextMutexes.LoadOrStore(tenantID, &sync.Mutex{})
	tenantMutex := tenantMutexInterface.(*sync.Mutex)

	tenantMutex.Lock()
	defer tenantMutex.Unlock()

	m.globalMutex.RLock()
	if ctx, exists := m.contexts[tenantID]; exists {
		m.globalMutex.RUnlock()
		if ctx.Database != nil && ctx.Database.Conn != nil {
			return ctx, nil
		}
	} else {
		m.globalMutex.RUnlock()
	}

	return m.createContext(tenantID)
}

// NewContextFromID creates a new tenant context from a tenant ID string.
func (m *Manager) NewContextFromID(tenantID string) (*Context, error) {
	return m.createContext(tenantID)
}

// createContext creates a new tenant context
func (m *Manager) createContext(tenantID string) (*Context, error) {
	config, err := LoadTenantConfig(tenantID, m.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to load tenant config: %w", err)
	}

	db, err := NewDatabase(config, m.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection: %w", err)
	}

	status := m.detector.GetTenantStatus(tenantID)

	ctx := &Context{
		TenantID:     tenantID,
		Config:       config,
		Database:     db,
		Status:       status,
		CacheManager: m.cacheManager,
		Logger:       m.logger,
	}

	m.globalMutex.Lock()
	m.contexts[tenantID] = ctx
	m.globalMutex.Unlock()

	return ctx, nil
}

// PreActivateAllTenants activates all tenants in the registry during startup
func (m *Manager) PreActivateAllTenants() error {
	registry, err := LoadTenantRegistry()
	if err != nil {
		return fmt.Errorf("failed to load tenant registry for pre-activation: %w", err)
	}

	if len(registry.Tenants) == 0 {
		return nil
	}

	var failedTenants []string

	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			continue
		}

		if err := m.preActivateSingleTenant(tenantID); err != nil {
			failedTenants = append(failedTenants, tenantID)
			continue
		}
	}

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
	ctx, err := m.createContext(tenantID)
	if err != nil {
		return fmt.Errorf("failed to create context for tenant %s: %w", tenantID, err)
	}

	if err := ctx.Database.Conn.Ping(); err != nil {
		return fmt.Errorf("database connection test failed for tenant %s: %w", tenantID, err)
	}

	dbType := "sqlite3"
	if ctx.Database.UseTurso {
		dbType = "turso"
	}
	m.detector.UpdateTenantStatus(tenantID, "active", dbType)

	return nil
}

// ValidatePreActivation verifies all tenants are active after pre-activation
func (m *Manager) ValidatePreActivation() error {
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
	reservedTenants := make([]string, 0)

	for tenantID, tenantInfo := range registry.Tenants {
		switch tenantInfo.Status {
		case "active":
			activeTenants = append(activeTenants, tenantID)
		case "reserved":
			reservedTenants = append(reservedTenants, tenantID)
		default:
			inactiveTenants = append(inactiveTenants, tenantID)
		}
	}

	log.Printf("Active tenants: %v", activeTenants)
	if len(reservedTenants) > 0 {
		log.Printf("Reserved tenants (awaiting activation): %v", reservedTenants)
	}

	if len(inactiveTenants) > 0 {
		log.Printf("Inactive tenants: %v", inactiveTenants)
		return fmt.Errorf("validation failed - %d tenants still inactive: %v",
			len(inactiveTenants), inactiveTenants)
	}

	log.Printf("✓ Validation passed - %d tenants active, %d reserved", len(activeTenants), len(reservedTenants))
	return nil
}

// GetActiveTenantCount returns the number of active tenants
func (m *Manager) GetActiveTenantCount() (int, error) {
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

// GetCacheManager returns the cache manager for external access
func (m *Manager) GetCacheManager() *manager.Manager {
	return m.cacheManager
}

// GetDetector returns the detector for external access (needed by startup code)
func (m *Manager) GetDetector() *Detector {
	return m.detector
}

// Close cleans up all tenant contexts
func (m *Manager) Close() error {
	m.globalMutex.Lock()
	defer m.globalMutex.Unlock()

	for _, ctx := range m.contexts {
		if err := ctx.Close(); err != nil {
			continue
		}
	}

	m.contexts = make(map[string]*Context)
	return nil
}

// SetLogger sets the logger for the tenant manager after container initialization
func (m *Manager) SetLogger(logger *logging.ChanneledLogger) {
	m.logger = logger

	if m.detector != nil && logger != nil {
		m.detector.logger = logger
	}
}

// GetLogger returns the logger for middleware access
func (m *Manager) GetLogger() *logging.ChanneledLogger {
	return m.logger
}
