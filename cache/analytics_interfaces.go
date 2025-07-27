package cache

import (
	"database/sql"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// ReadOnlyAnalyticsCache prevents analytics service from writing to cache
type ReadOnlyAnalyticsCache interface {
	// Epinet analytics operations (read-only)
	GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*models.HourlyEpinetBin, bool)
	GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*models.HourlyEpinetBin, []string)

	// Content analytics operations (read-only)
	GetHourlyContentBin(tenantID, contentID, hourKey string) (*models.HourlyContentBin, bool)

	// Site analytics operations (read-only)
	GetHourlySiteBin(tenantID, hourKey string) (*models.HourlySiteBin, bool)

	// Computed metrics operations (read-only)
	GetLeadMetrics(tenantID string) (*models.LeadMetricsCache, bool)
	GetDashboardData(tenantID string) (*models.DashboardCache, bool)
}

// WriteOnlyAnalyticsCache prevents cache warmer from reading during computation
type WriteOnlyAnalyticsCache interface {
	// Epinet analytics operations (write-only)
	SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *models.HourlyEpinetBin)

	// Content analytics operations (write-only)
	SetHourlyContentBin(tenantID, contentID, hourKey string, bin *models.HourlyContentBin)

	// Site analytics operations (write-only)
	SetHourlySiteBin(tenantID, hourKey string, bin *models.HourlySiteBin)

	// Computed metrics operations (write-only)
	SetLeadMetrics(tenantID string, metrics *models.LeadMetricsCache)
	SetDashboardData(tenantID string, data *models.DashboardCache)

	// Batch operations (write-only)
	PurgeExpiredBins(tenantID string, olderThan string)

	// Utility operations (write-only)
	InvalidateAnalyticsCache(tenantID string)
	UpdateLastFullHour(tenantID, hourKey string)
}

// DatabaseOperations defines database access for cache warming
type DatabaseOperations interface {
	QueryWithContext(query string, args ...interface{}) (*sql.Rows, error)
	QueryRowWithContext(query string, args ...interface{}) *sql.Row
	ExecWithContext(query string, args ...interface{}) (sql.Result, error)
	GetTenantID() string
}

// DatabaseAdapter wraps tenant.Context to provide controlled database access
type DatabaseAdapter struct {
	ctx *tenant.Context
}

func NewDatabaseAdapter(ctx *tenant.Context) DatabaseOperations {
	return &DatabaseAdapter{ctx: ctx}
}

func (da *DatabaseAdapter) QueryWithContext(query string, args ...interface{}) (*sql.Rows, error) {
	return da.ctx.Database.Conn.Query(query, args...)
}

func (da *DatabaseAdapter) QueryRowWithContext(query string, args ...interface{}) *sql.Row {
	return da.ctx.Database.Conn.QueryRow(query, args...)
}

func (da *DatabaseAdapter) ExecWithContext(query string, args ...interface{}) (sql.Result, error) {
	return da.ctx.Database.Conn.Exec(query, args...)
}

func (da *DatabaseAdapter) GetTenantID() string {
	return da.ctx.TenantID
}

func (da *DatabaseAdapter) GetContext() *tenant.Context {
	return da.ctx
}
