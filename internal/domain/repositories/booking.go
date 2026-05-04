// Package repositories defines the repository interfaces for domain entities.
package repositories

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/booking"
)

// BookingRepository defines the persistence interface for Booking entities.
type BookingRepository interface {
	FindByID(tenantID, id string) (*booking.Booking, error)
	FindOverlapping(tenantID string, start, end time.Time) ([]*booking.Booking, error)
	FindAllPaginated(tenantID string, limit, offset int, status string) ([]*booking.Booking, int, error)
	GetMetrics(tenantID string, now time.Time) (*booking.BookingMetrics, error)
	Store(tenantID string, b *booking.Booking) error
	UpdateStatus(tenantID, id string, status booking.BookingStatus, shopifyOrderID *string) error
	UpdateGoogleSyncPending(tenantID, id string) error
	UpdateGoogleSyncSuccess(tenantID, id string, googleEventID, meetURL *string) error
	UpdateGoogleDeletePending(tenantID, id string) error
	UpdateGoogleDeleteSuccess(tenantID, id string) error
	UpdateGoogleSyncFailure(tenantID, id string, syncStatus booking.GoogleSyncStatus, errorSummary string) error
	MarkConfirmationEmailSent(tenantID, id string) error
	MarkLinkAddedEmailSent(tenantID, id string) error
	DeleteExpiredPending(tenantID string, expirationTime time.Time) (int, error)
	DeletePendingByTraceID(tenantID string, traceID string) error
}
