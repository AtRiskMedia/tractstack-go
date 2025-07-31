// Package cleanup provides background cache cleanup with TTL-based eviction
package cleanup

import (
	"context"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// Worker handles background cache cleanup operations
type Worker struct {
	cache    interfaces.Cache
	detector *tenant.Detector
	config   *Config
}

// NewWorker creates a new cleanup worker with injected configuration
func NewWorker(cache interfaces.Cache, detector *tenant.Detector, config *Config) *Worker {
	return &Worker{
		cache:    cache,
		detector: detector,
		config:   config,
	}
}

// Start begins the cleanup worker routine, using the configured interval
func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.config.CleanupInterval)
	defer ticker.Stop()

	log.Printf("Cache cleanup worker started (interval: %v, verbose: %v)",
		w.config.CleanupInterval, w.config.VerboseReporting)

	for {
		select {
		case <-ctx.Done():
			log.Println("Cache cleanup worker stopping...")
			return
		case <-ticker.C:
			w.performCleanup(ctx)
		}
	}
}

// performCleanup executes cleanup for all active tenants
func (w *Worker) performCleanup(ctx context.Context) {
	start := time.Now()
	reporter := NewReporter(w.cache) // <-- CREATE REPORTER INSTANCE

	tenants, err := w.getActiveTenants()
	if err != nil {
		reporter.LogError("Cache cleanup failed to get active tenants", err)
		return
	}

	if w.config.VerboseReporting {
		reporter.LogHeader("PERIODIC CACHE CLEANUP")
	}

	var totalCleaned int
	for _, tenantID := range tenants {
		select {
		case <-ctx.Done():
			return
		default:
			cleaned := w.cleanupTenant(tenantID)
			totalCleaned += cleaned
		}
	}

	duration := time.Since(start)
	if totalCleaned > 0 {
		// Use the reporter for consistent logging
		reporter.LogSuccess("Cache cleanup finished: %d items cleaned from %d tenants in %v",
			totalCleaned, len(tenants), duration)
	}
}

// cleanupTenant performs TTL-based cleanup for a single tenant
func (w *Worker) cleanupTenant(tenantID string) int {
	var cleaned int
	// The actual cleanup logic would go here, using w.config.ContentCacheTTL, etc.
	// For example:
	// cleaned += w.cache.PurgeExpiredContent(tenantID, w.config.ContentCacheTTL)
	return cleaned
}

// getActiveTenants loads the tenant registry and returns active tenant IDs
func (w *Worker) getActiveTenants() ([]string, error) {
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return nil, err
	}

	activeTenants := make([]string, 0)
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeTenants = append(activeTenants, tenantID)
		}
	}

	return activeTenants, nil
}
