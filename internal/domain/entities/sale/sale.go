// Package sale defines paid Shopify receipt log entities.
package sale

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/booking"
)

// SaleStatus represents the persisted payment state for a Shopify order.
type SaleStatus string

const (
	SaleStatusPaid SaleStatus = "PAID"
)

// SaleProductLine captures the immutable line-item details received from Shopify.
type SaleProductLine struct {
	ResourceID    string `json:"resourceId"`
	GID           string `json:"gid"`
	VariantID     string `json:"variantId"`
	Quantity      int    `json:"quantity"`
	Title         string `json:"title"`
	Price         string `json:"price"`
	CurrencyCode  string `json:"currencyCode"`
	IsLocalPickup bool   `json:"isLocalPickup"`
}

// Sale is the receipt log row written from Shopify orders/paid webhooks.
type Sale struct {
	ID                string            `json:"id"`
	LeadID            string            `json:"leadId"`
	BookingID         *string           `json:"bookingId"`
	ShopifyOrderID    string            `json:"shopifyOrderId"`
	TotalAmount       string            `json:"totalAmount"`
	Status            SaleStatus        `json:"status"`
	Products          []SaleProductLine `json:"products"`
	AppointmentIntent bool              `json:"appointmentIntent"`
	CreatedAt         time.Time         `json:"createdAt"`
}

// SaleListItem is the administrative list projection with lead and booking data.
type SaleListItem struct {
	ID                string            `json:"id"`
	LeadID            string            `json:"leadId"`
	LeadEmail         string            `json:"leadEmail,omitempty"`
	LeadName          string            `json:"leadName,omitempty"`
	BookingID         *string           `json:"bookingId"`
	ShopifyOrderID    string            `json:"shopifyOrderId"`
	TotalAmount       string            `json:"totalAmount"`
	Status            SaleStatus        `json:"status"`
	Products          []SaleProductLine `json:"products"`
	AppointmentIntent bool              `json:"appointmentIntent"`
	Tags              []string          `json:"tags"`
	Booking           *booking.Booking  `json:"booking"`
	CreatedAt         time.Time         `json:"createdAt"`
}

// SaleMetrics contains paid Shopify receipt aggregates for the dashboard.
type SaleMetrics struct {
	PaidOrderTotalMonth       string `json:"paidOrderTotalMonth"`
	PaidOrderTotalYear        string `json:"paidOrderTotalYear"`
	PaidOrderTotalAllTime     string `json:"paidOrderTotalAllTime"`
	PaidOrdersMonth           int    `json:"paidOrdersMonth"`
	PaidOrdersYear            int    `json:"paidOrdersYear"`
	PaidOrdersAllTime         int    `json:"paidOrdersAllTime"`
	AveragePaidOrderMonth     string `json:"averagePaidOrderMonth"`
	UniquePayingCustomers     int    `json:"uniquePayingCustomers"`
	LocalPickupLineTotalMonth string `json:"localPickupLineTotalMonth"`
	LocalPickupOrdersMonth    int    `json:"localPickupOrdersMonth"`
	AppointmentOrdersMonth    int    `json:"appointmentOrdersMonth"`
	ProductOnlyOrdersMonth    int    `json:"productOnlyOrdersMonth"`
	OrphanOrdersMonth         int    `json:"orphanOrdersMonth"`
	CurrencyCode              string `json:"currencyCode"`
}
