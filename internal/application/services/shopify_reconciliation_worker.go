package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// ShopifyReconciliationWorker handles periodic background synchronization
// of products for all active tenants.
type ShopifyReconciliationWorker struct {
	shopifyService *ShopifyService
	tenantManager  *tenant.Manager
	logger         *logging.ChanneledLogger

	// In-memory state tracking (resets on server restart)
	lastSync  map[string]time.Time
	syncMutex sync.RWMutex
}

// NewShopifyReconciliationWorker creates a new background worker instance.
func NewShopifyReconciliationWorker(
	shopifyService *ShopifyService,
	tenantManager *tenant.Manager,
	logger *logging.ChanneledLogger,
) *ShopifyReconciliationWorker {
	return &ShopifyReconciliationWorker{
		shopifyService: shopifyService,
		tenantManager:  tenantManager,
		logger:         logger,
		lastSync:       make(map[string]time.Time),
	}
}

// Start begins the background loop with a staggered startup delay.
func (w *ShopifyReconciliationWorker) Start(ctx context.Context) {
	// 1. Initial Staggered Delay
	startupDelay := config.ShopifyReconcileStartupDelay
	w.logger.System().Info("Shopify reconciliation worker scheduled", "delayMinutes", startupDelay.Minutes())

	select {
	case <-ctx.Done():
		return
	case <-time.After(startupDelay):
		// Proceed to first pass
	}

	// 2. Periodic Ticker
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	w.logger.System().Info("Shopify reconciliation worker active", "intervalHours", config.ShopifyReconcileInterval.Hours())

	// Run immediate first pass after the startup delay
	w.runPass(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.System().Info("Shopify reconciliation worker stopping...")
			return
		case <-ticker.C:
			w.runPass(ctx)
		}
	}
}

// runPass iterates through all active tenants and reconciles those that are due.
func (w *ShopifyReconciliationWorker) runPass(ctx context.Context) {
	activeTenants, err := w.getActiveTenantIDs()
	if err != nil {
		w.logger.System().Error("Reconciliation worker failed to get active tenants", "error", err)
		return
	}

	for _, tenantID := range activeTenants {
		select {
		case <-ctx.Done():
			return
		default:
			if w.isDue(tenantID) {
				w.performReconciliation(tenantID)
			}
		}
	}
}

// isDue checks if the configured interval has elapsed for a specific tenant.
func (w *ShopifyReconciliationWorker) isDue(tenantID string) bool {
	w.syncMutex.RLock()
	defer w.syncMutex.RUnlock()

	last, exists := w.lastSync[tenantID]
	if !exists {
		return true // Never synced this session
	}

	return time.Since(last) >= config.ShopifyReconcileInterval
}

// performReconciliation executes the actual sync via ShopifyService and logs results.
func (w *ShopifyReconciliationWorker) performReconciliation(tenantID string) {
	start := time.Now()

	fmt.Printf("▶ Starting Shopify product reconciliation for tenant: %s\n", tenantID)
	w.logger.System().Info("Starting background Shopify reconciliation", "tenantId", tenantID)

	tenantCtx, err := w.tenantManager.NewContextFromID(tenantID)
	if err != nil {
		fmt.Printf("✗ Failed to get tenant context for %s: %v\n", tenantID, err)
		w.logger.System().Error("Failed to get tenant context for reconciliation", "tenantId", tenantID, "error", err)
		return
	}

	// 1. Execute the reconciliation pass
	// Capturing the new deletedCount alongside processed and reconciled counts
	processed, reconciled, deleted, err := w.shopifyService.ReconcileAll(tenantCtx)

	w.syncMutex.Lock()
	w.lastSync[tenantID] = time.Now()
	w.syncMutex.Unlock()

	if err != nil {
		fmt.Printf("✗ Shopify reconciliation failed for %s: %v\n", tenantID, err)
		w.logger.System().Error("Shopify reconciliation failed", "tenantId", tenantID, "error", err)
		return
	}

	duration := time.Since(start)

	// 2. Log the summary including the count of pruned (deleted) orphaned resources
	fmt.Printf("✓ Shopify reconciliation complete for %s: %d updated/created, %d pruned, %d total processed (%v)\n",
		tenantID, reconciled, deleted, processed, duration.Round(time.Second))

	w.logger.System().Info("Shopify reconciliation successful",
		"tenantId", tenantID,
		"totalProcessed", processed,
		"reconciledCount", reconciled,
		"deletedCount", deleted,
		"duration", duration)
}

func (w *ShopifyReconciliationWorker) getActiveTenantIDs() ([]string, error) {
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return nil, err
	}

	active := make([]string, 0)
	for id, info := range registry.Tenants {
		if info.Status == "active" {
			active = append(active, id)
		}
	}
	return active, nil
}
