// Package services provides business logic and orchestration for the application.
package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/booking"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// ErrBookingNotFound used for error return value
var ErrBookingNotFound = errors.New("booking not found for trace ID")

// BookingService handles business logic and orchestration for reservations.
type BookingService struct {
	logger          *logging.ChanneledLogger
	resourceService *ResourceService
	emailWorker     *EmailWorker
	locks           sync.Map // Maps tenantID string to *sync.Mutex for WAL-mode queueing
}

// NewBookingService creates a new booking service instance.
func NewBookingService(logger *logging.ChanneledLogger, resourceService *ResourceService, emailWorker *EmailWorker) *BookingService {
	return &BookingService{
		logger:          logger,
		resourceService: resourceService,
		emailWorker:     emailWorker,
	}
}

// getTenantLock retrieves or creates a mutex for the specific tenant to prevent SQLite WAL-mode concurrency crashes.
func (s *BookingService) getTenantLock(tenantID string) *sync.Mutex {
	lock, _ := s.locks.LoadOrStore(tenantID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// GetAvailability returns overlapping bookings for a given time window to support availability math.
func (s *BookingService) GetAvailability(tenantCtx *tenant.Context, start, end time.Time) ([]*booking.Booking, error) {
	repo := tenantCtx.BookingRepo()

	existingBookings, err := repo.FindOverlapping(tenantCtx.TenantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch overlapping bookings: %w", err)
	}

	return existingBookings, nil
}

// HoldSlot attempts to lock a time slot for a user, using a tenant-level mutex before writing to the DB.
// It enforces UnavailableHours checks and saves the initial hold as pending.
func (s *BookingService) HoldSlot(ctx context.Context, tenantCtx *tenant.Context, traceID string, resourceIDs []string, leadID string, start, end time.Time) error {
	mu := s.getTenantLock(tenantCtx.TenantID)
	mu.Lock()
	defer mu.Unlock()

	// Check if client disconnected/timed out while waiting for the WAL queue
	if ctx.Err() != nil {
		return fmt.Errorf("client aborted request before lock acquisition: %w", ctx.Err())
	}

	// 0. Reject if in past
	if start.Before(time.Now()) {
		return fmt.Errorf("requested start time is in the past")
	}

	// Strict Backend Duration Validation ---
	// Fetch the requested resources to calculate the true required duration
	resources, err := s.resourceService.GetByIDs(tenantCtx, resourceIDs)
	if err != nil {
		return fmt.Errorf("failed to validate resource durations: %w", err)
	}

	var rawMinutes float64 = 0
	for _, res := range resources {
		if val, ok := res.OptionsPayload["bookingLengthMinutes"]; ok {
			// JSON unmarshaler converts numbers to float64 for any/interface{} types
			if minutes, ok := val.(float64); ok {
				rawMinutes += minutes
			}
		}
	}

	// Snap to 15-minute intervals matching the frontend's scheduling grid
	interval := 15.0
	snappedMinutes := math.Ceil(rawMinutes/interval) * interval

	maxLength := float64(tenantCtx.Config.BrandConfig.Scheduling.MaxLengthMinutes)
	if snappedMinutes > maxLength {
		snappedMinutes = maxLength
	}

	// Enforce a minimum 15m duration if a booking is required but missing duration config
	if snappedMinutes == 0 && len(resources) > 0 {
		snappedMinutes = 15.0
	}

	// Override the user-provided end time with the securely calculated end time
	secureEnd := start.Add(time.Duration(snappedMinutes) * time.Minute)

	// Final sanity check against global max length limit
	if secureEnd.Sub(start).Minutes() > maxLength {
		return fmt.Errorf("requested duration exceeds maximum allowed length")
	}
	// -----------------------------------------------

	// 1. Check Unavailable Hours (Strict Backend Validation)
	for _, block := range tenantCtx.Config.BrandConfig.Scheduling.UnavailableHours {
		blockStart, err1 := time.Parse(time.RFC3339, block.Start)
		blockEnd, err2 := time.Parse(time.RFC3339, block.End)
		if err1 == nil && err2 == nil {
			if start.Before(blockEnd) && secureEnd.After(blockStart) {
				return fmt.Errorf("time slot overlaps with unavailable hours")
			}
		}
	}

	repo := tenantCtx.BookingRepo()

	// 2. Check for overlapping database bookings EXACTLY within the locked context
	overlapping, err := repo.FindOverlapping(tenantCtx.TenantID, start, secureEnd)
	if err != nil {
		return fmt.Errorf("failed to check availability: %w", err)
	}

	if len(overlapping) > 0 {
		return fmt.Errorf("time slot is no longer available")
	}

	// 3. Create the Booking (Defaulting to Pending)
	newBooking := &booking.Booking{
		ID:          traceID,
		ResourceIDs: resourceIDs,
		LeadID:      leadID,
		StartTime:   start,
		EndTime:     secureEnd,
		Status:      booking.StatusPending,
		CreatedAt:   time.Now().UTC(),
	}

	if err := repo.Store(tenantCtx.TenantID, newBooking); err != nil {
		return fmt.Errorf("failed to store booking hold: %w", err)
	}

	s.logger.System().Info("Booking slot held successfully",
		"traceId", traceID,
		"tenantId", tenantCtx.TenantID,
		"durationMinutes", snappedMinutes)

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
		return ErrBookingNotFound
	}
	// Verify free cart bypass is legitimate
	if shopifyOrderID == nil {
		resources, err := s.resourceService.GetByIDs(tenantCtx, b.ResourceIDs)
		if err != nil {
			return fmt.Errorf("failed to retrieve resources for validation: %w", err)
		}
		for _, res := range resources {
			if _, hasGID := res.OptionsPayload["gid"]; hasGID {
				return fmt.Errorf("unauthorized: cannot natively confirm paid resource %s", res.ID)
			}
		}
	}

	if err := repo.UpdateStatus(tenantCtx.TenantID, traceID, booking.StatusConfirmed, shopifyOrderID); err != nil {
		return fmt.Errorf("failed to confirm booking: %w", err)
	}

	s.logger.System().Info("Booking confirmed", "traceId", traceID, "tenantId", tenantCtx.TenantID)

	if s.emailWorker != nil && shopifyOrderID != nil {
		lead, _ := tenantCtx.LeadRepo().FindByID(b.LeadID)
		if lead != nil {
			siteURL := tenantCtx.Config.BrandConfig.SiteURL
			if siteURL == "" {
				siteURL = "https://tractstack.com"
			}
			orderURL := fmt.Sprintf("%s/account/orders/%s", siteURL, *shopifyOrderID)
			s.emailWorker.Enqueue(EmailJob{
				TenantID:     tenantCtx.TenantID,
				To:           []string{lead.Email},
				Category:     "shopify",
				TemplateName: "booking-confirmed",
				Data: map[string]any{
					"LeadName":        lead.FirstName,
					"ShopifyOrderID":  *shopifyOrderID,
					"ShopifyOrderUrl": orderURL,
				},
			})
		}
	}

	return nil
}

// ReleaseHold drops a pending booking proactively
func (s *BookingService) ReleaseHold(tenantCtx *tenant.Context, traceID string) error {
	repo := tenantCtx.BookingRepo()
	return repo.DeletePendingByTraceID(tenantCtx.TenantID, traceID)
}

// ListBookings retrieves a paginated list of bookings for the administrative dashboard.
func (s *BookingService) ListBookings(tenantCtx *tenant.Context, limit, offset int, status string) ([]*booking.Booking, int, error) {
	repo := tenantCtx.BookingRepo()

	bookings, count, err := repo.FindAllPaginated(tenantCtx.TenantID, limit, offset, status)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list bookings: %w", err)
	}

	return bookings, count, nil
}

// GetMetrics calculates aggregated booking volume and conversion statistics.
func (s *BookingService) GetMetrics(tenantCtx *tenant.Context) (*booking.BookingMetrics, error) {
	repo := tenantCtx.BookingRepo()
	now := time.Now().UTC()

	metrics, err := repo.GetMetrics(tenantCtx.TenantID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate booking metrics: %w", err)
	}

	return metrics, nil
}

// CancelBooking manually transitions a booking to the CANCELLED state.
// It acquires a tenant-level lock to prevent race conditions with incoming webhooks.
func (s *BookingService) CancelBooking(tenantCtx *tenant.Context, traceID string) error {
	mu := s.getTenantLock(tenantCtx.TenantID)
	mu.Lock()
	defer mu.Unlock()

	repo := tenantCtx.BookingRepo()

	b, err := repo.FindByID(tenantCtx.TenantID, traceID)
	if err != nil {
		return fmt.Errorf("failed to retrieve booking for cancellation: %w", err)
	}
	if b == nil {
		return fmt.Errorf("booking not found for trace ID: %s", traceID)
	}

	if err := repo.UpdateStatus(tenantCtx.TenantID, traceID, booking.StatusCancelled, nil); err != nil {
		return fmt.Errorf("failed to cancel booking: %w", err)
	}

	s.logger.System().Info("Booking manually cancelled", "traceId", traceID, "tenantId", tenantCtx.TenantID)

	if s.emailWorker != nil {
		lead, _ := tenantCtx.LeadRepo().FindByID(b.LeadID)
		if lead != nil {
			orderID := traceID
			s.emailWorker.Enqueue(EmailJob{
				TenantID:     tenantCtx.TenantID,
				To:           []string{lead.Email},
				Category:     "shopify",
				TemplateName: "booking-cancelled",
				Data: map[string]any{
					"LeadName":       lead.FirstName,
					"ShopifyOrderID": orderID,
				},
			})
		}
	}

	return nil
}
