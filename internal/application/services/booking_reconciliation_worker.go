// Package services provides business logic and orchestration for the application.
package services

import (
	"context"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// BookingReconciliationWorker handles periodic background cleanup of expired
// pending bookings for all active tenants.
type BookingReconciliationWorker struct {
	tenantManager *tenant.Manager
	logger        *logging.ChanneledLogger
}

// NewBookingReconciliationWorker creates a new background worker instance.
func NewBookingReconciliationWorker(
	tenantManager *tenant.Manager,
	logger *logging.ChanneledLogger,
) *BookingReconciliationWorker {
	return &BookingReconciliationWorker{
		tenantManager: tenantManager,
		logger:        logger,
	}
}

// Start begins the background loop for cleaning up expired bookings.
func (w *BookingReconciliationWorker) Start(ctx context.Context) {
	// 1. Initial Startup Delay (Short delay to allow container and database initialization)
	startupDelay := 30 * time.Second
	w.logger.System().Info("Booking reconciliation worker scheduled", "delaySeconds", startupDelay.Seconds())

	select {
	case <-ctx.Done():
		return
	case <-time.After(startupDelay):
		// Proceed
	}

	// 2. Periodic Ticker (Runs every 5 minutes as specified in the backend roadmap)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	w.logger.System().Info("Booking reconciliation worker active", "intervalMinutes", 5)

	// Run immediate first pass
	w.runPass(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.System().Info("Booking reconciliation worker stopping...")
			return
		case <-ticker.C:
			w.runPass(ctx)
		}
	}
}

// runPass iterates through all active tenants and performs cleanup.
func (w *BookingReconciliationWorker) runPass(ctx context.Context) {
	activeTenants, err := w.getActiveTenantIDs()
	if err != nil {
		w.logger.System().Error("Booking reconciliation worker failed to get active tenants", "error", err)
		return
	}

	for _, tenantID := range activeTenants {
		select {
		case <-ctx.Done():
			return
		default:
			w.performCleanup(tenantID)
		}
	}
}

// performCleanup executes the actual deletion of expired pending bookings.
func (w *BookingReconciliationWorker) performCleanup(tenantID string) {
	tenantCtx, err := w.tenantManager.NewContextFromID(tenantID)
	if err != nil {
		w.logger.System().Error("Failed to get tenant context for booking cleanup", "tenantId", tenantID, "error", err)
		return
	}

	// Calculate the expiration boundary based on the configured system-wide timeout
	expirationTime := time.Now().UTC().Add(-config.BookingHoldTimeout)
	repo := tenantCtx.BookingRepo()

	deleted, err := repo.DeleteExpiredPending(tenantID, expirationTime)
	if err != nil {
		w.logger.System().Error("Failed to delete expired pending bookings", "tenantId", tenantID, "error", err)
		return
	}

	if deleted > 0 {
		w.logger.System().Info("Cleaned up expired pending bookings",
			"tenantId", tenantID,
			"deletedCount", deleted,
			"expirationBoundary", expirationTime)
	}
}

// getActiveTenantIDs retrieves the list of tenants with 'active' status.
func (w *BookingReconciliationWorker) getActiveTenantIDs() ([]string, error) {
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
