// Package tenant provides tenant context management for multi-tenant support.
package tenant

import (
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
)

// Context holds tenant-specific request context
type Context struct {
	TenantID     string
	Config       *Config
	Database     *Database
	Status       string
	CacheManager *manager.Manager
}

// Close cleans up the tenant context
func (ctx *Context) Close() error {
	if ctx.Database != nil {
		return ctx.Database.Close()
	}
	return nil
}

// GetTenantID returns the tenant ID for this context
func (ctx *Context) GetTenantID() string {
	return ctx.TenantID
}

// GetConfig returns the tenant configuration
func (ctx *Context) GetConfig() *Config {
	return ctx.Config
}

// GetDatabase returns the tenant database connection
func (ctx *Context) GetDatabase() *Database {
	return ctx.Database
}

// GetStatus returns the tenant status
func (ctx *Context) GetStatus() string {
	return ctx.Status
}

// GetCacheManager returns the cache manager
func (ctx *Context) GetCacheManager() *manager.Manager {
	return ctx.CacheManager
}

// IsActive returns true if the tenant is active
func (ctx *Context) IsActive() bool {
	return ctx.Status == "active"
}

// IsReserved returns true if the tenant is reserved (awaiting activation)
func (ctx *Context) IsReserved() bool {
	return ctx.Status == "reserved"
}

// GetDatabaseInfo returns database connection information for logging
func (ctx *Context) GetDatabaseInfo() string {
	if ctx.Database != nil {
		return ctx.Database.GetConnectionInfo()
	}
	return "no database connection"
}
