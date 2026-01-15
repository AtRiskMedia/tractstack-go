// Package tenant manages tenant-specific configurations and context,
// isolating multi-tenancy logic from the rest of the application.
package tenant

import (
	"fmt"
	"log"
	"sync"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/database"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/fts"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
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
	ftsService     *fts.Service
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

// createContext creates a new tenant context - FAST, no migrations
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

	var domains []string
	if registry := m.detector.GetRegistry(); registry != nil {
		if info, ok := registry.Tenants[tenantID]; ok {
			domains = info.Domains
		}
	}

	ctx := &Context{
		TenantID:     tenantID,
		Domains:      domains,
		Config:       config,
		Database:     db,
		Status:       status,
		CacheManager: m.cacheManager,
		Logger:       m.logger,
		ftsService:   m.ftsService,
	}

	m.globalMutex.Lock()
	m.contexts[tenantID] = ctx
	m.globalMutex.Unlock()

	return ctx, nil
}

// RunStartupMigrations ensures all tenants have correct schema and FTS indexes during startup
func (m *Manager) RunStartupMigrations() error {
	detector := m.GetDetector()
	registry := detector.GetRegistry()

	if len(registry.Tenants) == 0 {
		return nil
	}

	var failedTenants []string

	for tenantID, tenantInfo := range registry.Tenants {
		// Skip inactive tenants - they don't have config files yet
		if tenantInfo.Status == "inactive" {
			log.Printf("[MIGRATION] Skipping inactive tenant: %s", tenantID)
			continue
		}

		log.Printf("[MIGRATION] Processing tenant: %s", tenantID)

		// Create a temporary context for migration (will be discarded)
		ctx, err := m.createContext(tenantID)
		if err != nil {
			m.logger.System().Warn("Failed to create context for migration", "error", err, "tenantId", tenantID)
			failedTenants = append(failedTenants, tenantID)
			continue
		}

		// 1. Run schema migrations to ensure all tables and indexes exist (idempotent)
		tableCreator := database.NewTableCreator()
		if err := tableCreator.CreateSchema(ctx.Database.Conn); err != nil {
			m.logger.System().Error("Schema migration failed for tenant", "error", err, "tenantId", tenantID)
			if closeErr := ctx.Close(); closeErr != nil {
				m.logger.System().Warn("Failed to close context after schema error", "error", closeErr, "tenantId", tenantID)
			}
			failedTenants = append(failedTenants, tenantID)
			continue
		}

		// 2. Check if FTS tables are populated
		var paneCount, sfCount int

		err = ctx.Database.Conn.QueryRow("SELECT count(*) FROM pane_content_fts").Scan(&paneCount)
		if err != nil {
			m.logger.System().Debug("Could not query pane_content_fts table, assuming re-index is needed", "error", err, "tenantId", tenantID)
			paneCount = 0
		}

		err = ctx.Database.Conn.QueryRow("SELECT count(*) FROM storyfragment_metadata_fts").Scan(&sfCount)
		if err != nil {
			m.logger.System().Debug("Could not query storyfragment_metadata_fts table, assuming re-index is needed", "error", err, "tenantId", tenantID)
			sfCount = 0
		}

		needsReindex := false
		if paneCount == 0 || sfCount == 0 {
			needsReindex = true
		}

		// Only check resource_body_fts if COLLECTION_ROUTES is configured
		if len(config.CollectionRoutes) > 0 {
			var resourceCount int
			err = ctx.Database.Conn.QueryRow("SELECT count(*) FROM resource_body_fts").Scan(&resourceCount)
			if err != nil {
				m.logger.System().Debug("Could not query resource_body_fts table, assuming re-index is needed", "error", err, "tenantId", tenantID)
				resourceCount = 0
			}
			if resourceCount == 0 {
				needsReindex = true
			}
		}

		// 3. If any FTS tables are empty, trigger a one-time re-index
		if needsReindex {
			log.Printf("    %s▒▓ Priming FTS Index for: %s%s", "\033[35;1m", tenantID, "\033[0m")
			if err := database.ReindexFTSTables(ctx.Database.Conn, m.ftsService, m.logger); err != nil {
				// Log as a warning but don't prevent startup for FTS failure
				m.logger.System().Warn("FTS re-indexing failed for tenant", "error", err, "tenantId", tenantID)
			}
		}

		// Close the temporary context
		if err := ctx.Close(); err != nil {
			m.logger.System().Warn("Failed to close temporary migration context", "error", err, "tenantId", tenantID)
		}
	}

	if len(failedTenants) > 0 {
		return fmt.Errorf("migration failed for tenants: %v", failedTenants)
	}

	return nil
}

// preActivateSingleTenant activates a single tenant during startup (status transition only)
func (m *Manager) preActivateSingleTenant(tenantID string) error {
	ctx, err := m.createContext(tenantID)
	if err != nil {
		return fmt.Errorf("failed to create context for tenant %s: %w", tenantID, err)
	}
	defer func() {
		if err := ctx.Close(); err != nil {
			m.logger.System().Warn("Failed to close context in preActivateSingleTenant", "error", err, "tenantId", tenantID)
		}
	}()

	// Test database connection
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

// SetService sets the FTS service for the manager to pass to tenant contexts.
func (m *Manager) SetService(ftsService *fts.Service) {
	m.ftsService = ftsService
}

// GetLogger returns the logger for middleware access
func (m *Manager) GetLogger() *logging.ChanneledLogger {
	return m.logger
}

// InvalidateTenantContext removes a tenant context from cache to force recreation
func (m *Manager) InvalidateTenantContext(tenantID string) {
	m.globalMutex.Lock()
	defer m.globalMutex.Unlock()

	if ctx, exists := m.contexts[tenantID]; exists {
		if err := ctx.Close(); err != nil {
			m.logger.Tenant().Warn("Error closing tenant context during invalidation",
				"error", err, "tenantId", tenantID)
		}
		delete(m.contexts, tenantID)
		m.logger.Tenant().Debug("Tenant context invalidated", "tenantId", tenantID)
	}
}

// GetActiveTenantCount returns the number of active tenants
func (m *Manager) GetActiveTenantCount() (int, error) {
	// Use detector's in-memory registry instead of reading filesystem
	detector := m.GetDetector()
	registry := detector.GetRegistry()

	activeCount := 0
	for _, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeCount++
		}
	}

	return activeCount, nil
}

// ValidatePreActivation verifies all tenants are active after pre-activation
func (m *Manager) ValidatePreActivation() error {
	log.Println("=== Validating pre-activation results ===")

	// Use detector's in-memory registry instead of reading filesystem
	detector := m.GetDetector()
	registry := detector.GetRegistry()

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
		log.Printf("Inactive tenants (awaiting setup): %v", inactiveTenants)
	}

	log.Printf("✓ Validation passed - %d tenants active, %d reserved", len(activeTenants), len(reservedTenants))
	return nil
}

// PreActivateAllTenants activates all tenants in the registry during startup
func (m *Manager) PreActivateAllTenants() error {
	detector := m.GetDetector()
	registry := detector.GetRegistry()

	if len(registry.Tenants) == 0 {
		return nil
	}

	var failedTenants []string

	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			continue // Skip already active tenants
		}

		if tenantInfo.Status == "inactive" {
			continue // Skip inactive tenants (no config files yet)
		}

		// Only try to pre-activate "reserved" tenants (which have config files)
		if err := m.preActivateSingleTenant(tenantID); err != nil {
			failedTenants = append(failedTenants, tenantID)
			continue
		}
	}

	if err := detector.RefreshRegistry(); err != nil {
		return fmt.Errorf("failed to refresh detector registry: %w", err)
	}

	if len(failedTenants) > 0 {
		return fmt.Errorf("pre-activation failed for tenants: %v", failedTenants)
	}

	return nil
}
