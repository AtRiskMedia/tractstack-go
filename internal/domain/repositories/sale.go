// Package repositories defines repository interfaces for domain entities.
package repositories

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/sale"
)

// SaleRepository defines persistence operations for Shopify receipt logs.
type SaleRepository interface {
	UpsertByShopifyOrderID(tenantID string, s *sale.Sale) error
	FindByID(tenantID, id string) (*sale.Sale, error)
	FindAllPaginatedWithBooking(tenantID string, limit, offset int) ([]*sale.SaleListItem, int, error)
	GetMetrics(tenantID string, now time.Time) (*sale.SaleMetrics, error)
}
