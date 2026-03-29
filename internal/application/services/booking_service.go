// Package services provides business logic and orchestration for the application.
package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/booking"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// BookingService handles business logic and orchestration for reservations.
type BookingService struct {
	logger *logging.ChanneledLogger
	locks  sync.Map // Maps tenantID string to *sync.Mutex for WAL-mode queueing
}

// NewBookingService creates a new booking service instance.
func NewBookingService(logger *logging.ChanneledLogger) *BookingService {
	return &BookingService{
		logger: logger,
	}
}

// getTenantLock retrieves or creates a mutex for the specific tenant to prevent SQLite WAL-mode concurrency crashes.
func (s *BookingService) getTenantLock(tenantID string) *sync.Mutex {
	lock, _ := s.locks.LoadOrStore(tenantID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// GetAvailability returns overlapping bookings for a given time window to support availability math.
func (s *BookingService) GetAvailability(tenantCtx *tenant.Context, resourceIDs []string, start, end time.Time) ([]*booking.Booking, error) {
	repo := tenantCtx.BookingRepo()

	existingBookings, err := repo.FindOverlapping(tenantCtx.TenantID, resourceIDs, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch overlapping bookings: %w", err)
	}

	return existingBookings, nil
}

// HoldSlot attempts to lock a time slot for a user, using a tenant-level mutex before writing to the DB.
func (s *BookingService) HoldSlot(tenantCtx *tenant.Context, traceID string, resourceIDs []string, leadID string, start, end time.Time) error {
	mu := s.getTenantLock(tenantCtx.TenantID)
	mu.Lock()
	defer mu.Unlock()

	repo := tenantCtx.BookingRepo()

	// 1. Check for overlapping bookings EXACTLY within the locked context
	overlapping, err := repo.FindOverlapping(tenantCtx.TenantID, resourceIDs, start, end)
	if err != nil {
		return fmt.Errorf("failed to check availability: %w", err)
	}

	if len(overlapping) > 0 {
		return fmt.Errorf("time slot is no longer available")
	}

	// 2. Create the PENDING booking
	newBooking := &booking.Booking{
		ID:          traceID,
		ResourceIDs: resourceIDs,
		LeadID:      leadID,
		StartTime:   start,
		EndTime:     end,
		Status:      booking.StatusPending,
		CreatedAt:   time.Now().UTC(),
	}

	if err := repo.Store(tenantCtx.TenantID, newBooking); err != nil {
		return fmt.Errorf("failed to store booking hold: %w", err)
	}

	s.logger.System().Info("Booking slot held successfully", "traceId", traceID, "tenantId", tenantCtx.TenantID)
	return nil
}

// ConfirmBooking finalizes a pending booking, transitioning it to CONFIRMED.
func (s *BookingService) ConfirmBooking(tenantCtx *tenant.Context, traceID string, shopifyOrderID *string) error {
	mu := s.getTenantLock(tenantCtx.TenantID)
	mu.Lock()
	defer mu.Unlock()

	repo := tenantCtx.BookingRepo()

	b, err := repo.FindByID(tenantCtx.TenantID, traceID)
	if err != nil {
		return fmt.Errorf("failed to retrieve booking for confirmation: %w", err)
	}
	if b == nil {
		return fmt.Errorf("booking not found for trace ID: %s", traceID)
	}

	if err := repo.UpdateStatus(tenantCtx.TenantID, traceID, booking.StatusConfirmed, shopifyOrderID); err != nil {
		return fmt.Errorf("failed to confirm booking: %w", err)
	}

	s.logger.System().Info("Booking confirmed", "traceId", traceID, "tenantId", tenantCtx.TenantID)
	return nil
}

// ReleaseHold drops a pending booking proactively
func (s *BookingService) ReleaseHold(tenantCtx *tenant.Context, traceID string) error {
	repo := tenantCtx.BookingRepo()
	return repo.DeletePendingByTraceID(tenantCtx.TenantID, traceID)
}
