// Package repositories defines the repository interfaces for domain entities.
package repositories

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/booking"
)

// BookingRepository defines the persistence interface for Booking entities.
type BookingRepository interface {
	FindByID(tenantID, id string) (*booking.Booking, error)
	FindOverlapping(tenantID string, resourceIDs []string, start, end time.Time) ([]*booking.Booking, error)
	Store(tenantID string, b *booking.Booking) error
	UpdateStatus(tenantID, id string, status booking.BookingStatus, shopifyOrderID *string) error
	DeleteExpiredPending(tenantID string, expirationTime time.Time) (int, error)
	DeletePendingByTraceID(tenantID string, traceID string) error
}
