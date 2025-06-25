// Package tenant provides request context management for multi-tenant support.
package tenant

import (
	"log"

	"github.com/gin-gonic/gin"
)

// Context holds tenant-specific request context
type Context struct {
	TenantID string
	Config   *Config
	Database *Database
	Status   string
}

// Manager coordinates tenant detection and context creation
type Manager struct {
	detector *Detector
}

// NewManager creates a new tenant manager
func NewManager() (*Manager, error) {
	detector, err := NewDetector()
	if err != nil {
		return nil, err
	}

	return &Manager{
		detector: detector,
	}, nil
}

// GetContext creates a tenant context for the request
func (m *Manager) GetContext(c *gin.Context) (*Context, error) {
	// Detect tenant from request
	tenantID, err := m.detector.DetectTenant(c)
	if err != nil {
		return nil, err
	}

	// Load tenant configuration
	config, err := LoadTenantConfig(tenantID)
	if err != nil {
		return nil, err
	}

	// Get tenant status
	status := m.detector.GetTenantStatus(tenantID)

	// Create database connection
	database, err := NewDatabase(config)
	if err != nil {
		return nil, err
	}

	log.Printf("Tenant context created: %s (%s) - %s",
		tenantID, database.GetConnectionInfo(), status)

	return &Context{
		TenantID: tenantID,
		Config:   config,
		Database: database,
		Status:   status,
	}, nil
}

// GetDetector returns the detector for cache updates
func (m *Manager) GetDetector() *Detector {
	return m.detector
}

// Close cleans up the tenant context
func (ctx *Context) Close() {
	if ctx.Database != nil {
		ctx.Database.Close()
	}
}
