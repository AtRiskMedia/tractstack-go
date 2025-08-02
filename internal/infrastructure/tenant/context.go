// Package tenant provides tenant context management for multi-tenant support.
package tenant

import (
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
	domainUser "github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/bulk"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
	persistenceUser "github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/user"
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

// =============================================================================
// Repository Factory Methods
// =============================================================================

// BeliefRepo returns a belief repository instance
func (ctx *Context) BeliefRepo() repositories.BeliefRepository {
	return content.NewBeliefRepository(ctx.Database.Conn, ctx.CacheManager)
}

// MenuRepo returns a menu repository instance
func (ctx *Context) MenuRepo() repositories.MenuRepository {
	return content.NewMenuRepository(ctx.Database.Conn, ctx.CacheManager)
}

// PaneRepo returns a pane repository instance
func (ctx *Context) PaneRepo() repositories.PaneRepository {
	return content.NewPaneRepository(ctx.Database.Conn, ctx.CacheManager)
}

// ResourceRepo returns a resource repository instance
func (ctx *Context) ResourceRepo() repositories.ResourceRepository {
	return content.NewResourceRepository(ctx.Database.Conn, ctx.CacheManager)
}

// StoryFragmentRepo returns a storyfragment repository instance
func (ctx *Context) StoryFragmentRepo() repositories.StoryFragmentRepository {
	return content.NewStoryFragmentRepository(ctx.Database.Conn, ctx.CacheManager)
}

// TractStackRepo returns a tractstack repository instance
func (ctx *Context) TractStackRepo() repositories.TractStackRepository {
	return content.NewTractStackRepository(ctx.Database.Conn, ctx.CacheManager)
}

// EpinetRepo returns an epinet repository instance
func (ctx *Context) EpinetRepo() repositories.EpinetRepository {
	return content.NewEpinetRepository(ctx.Database.Conn, ctx.CacheManager)
}

// ImageFileRepo returns an imagefile repository instance
func (ctx *Context) ImageFileRepo() repositories.ImageFileRepository {
	return content.NewImageFileRepository(ctx.Database.Conn, ctx.CacheManager)
}

// BulkRepo returns a bulk repository instance for complex operations
func (ctx *Context) BulkRepo() bulk.BulkQueryRepository {
	db := &database.DB{DB: ctx.Database.Conn}
	return bulk.NewRepository(db)
}

// LeadRepo returns a lead repository instance.
// It returns the interface type from the domain layer.
func (ctx *Context) LeadRepo() domainUser.LeadRepository {
	db := &database.DB{DB: ctx.Database.Conn}
	return persistenceUser.NewSQLLeadRepository(db)
}

// FingerprintRepo returns a fingerprint repository instance.
// It returns the interface type from the domain layer.
func (ctx *Context) FingerprintRepo() domainUser.FingerprintRepository {
	db := &database.DB{DB: ctx.Database.Conn}
	return persistenceUser.NewSQLFingerprintRepository(db)
}

// VisitRepo returns a visit repository instance.
// It returns the interface type from the domain layer.
func (ctx *Context) VisitRepo() domainUser.VisitRepository {
	db := &database.DB{DB: ctx.Database.Conn}
	return persistenceUser.NewSQLVisitRepository(db)
}
