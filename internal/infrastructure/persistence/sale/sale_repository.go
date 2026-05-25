// Package sale provides the SQLite implementation for Shopify sales persistence.
package sale

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	bookingEntity "github.com/AtRiskMedia/tractstack-go/internal/domain/entities/booking"
	saleEntity "github.com/AtRiskMedia/tractstack-go/internal/domain/entities/sale"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
)

// SQLSaleRepository implements the sale repository interface using SQLite.
type SQLSaleRepository struct {
	db     *sql.DB
	logger *logging.ChanneledLogger
}

// NewSaleRepository creates a new SQLite-backed sale repository.
func NewSaleRepository(db *sql.DB, logger *logging.ChanneledLogger) *SQLSaleRepository {
	return &SQLSaleRepository{
		db:     db,
		logger: logger,
	}
}

// UpsertByShopifyOrderID creates or refreshes a receipt row for a Shopify order.
func (r *SQLSaleRepository) UpsertByShopifyOrderID(tenantID string, s *saleEntity.Sale) error {
	productsJSON, err := json.Marshal(s.Products)
	if err != nil {
		return fmt.Errorf("failed to marshal sale products: %w", err)
	}

	var bookingID any
	if s.BookingID != nil && *s.BookingID != "" {
		bookingID = *s.BookingID
	}

	query := `
		INSERT INTO sales (id, lead_id, booking_id, shopify_order_id, total_amount, status, products, appointment_intent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(shopify_order_id) DO UPDATE SET
			lead_id = excluded.lead_id,
			booking_id = excluded.booking_id,
			total_amount = excluded.total_amount,
			status = excluded.status,
			products = excluded.products,
			appointment_intent = excluded.appointment_intent`

	if _, err := r.db.Exec(
		query,
		s.ID,
		s.LeadID,
		bookingID,
		s.ShopifyOrderID,
		s.TotalAmount,
		string(s.Status),
		string(productsJSON),
		boolToSQLiteInt(s.AppointmentIntent),
	); err != nil {
		return fmt.Errorf("failed to upsert sale: %w", err)
	}

	return nil
}

// FindByID retrieves a sale by its trace ID.
func (r *SQLSaleRepository) FindByID(tenantID, id string) (*saleEntity.Sale, error) {
	query := `
		SELECT id, lead_id, booking_id, shopify_order_id, total_amount, status, products, appointment_intent, created_at
		FROM sales
		WHERE id = ?`

	var s saleEntity.Sale
	var leadID sql.NullString
	var bookingID sql.NullString
	var status string
	var productsJSON string
	var appointmentIntent int

	err := r.db.QueryRow(query, id).Scan(
		&s.ID,
		&leadID,
		&bookingID,
		&s.ShopifyOrderID,
		&s.TotalAmount,
		&status,
		&productsJSON,
		&appointmentIntent,
		&s.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find sale by id: %w", err)
	}

	if leadID.Valid {
		s.LeadID = leadID.String
	}
	if bookingID.Valid {
		s.BookingID = &bookingID.String
	}
	s.Status = saleEntity.SaleStatus(status)
	s.AppointmentIntent = appointmentIntent == 1
	if err := json.Unmarshal([]byte(productsJSON), &s.Products); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sale products: %w", err)
	}

	return &s, nil
}

// FindAllPaginatedWithBooking lists sales with sale-level lead and nested booking data.
func (r *SQLSaleRepository) FindAllPaginatedWithBooking(tenantID string, limit, offset int) ([]*saleEntity.SaleListItem, int, error) {
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM sales`).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("failed to count sales: %w", err)
	}

	query := `
		SELECT s.id, s.lead_id, s.booking_id, s.shopify_order_id, s.total_amount, s.status, s.products, s.appointment_intent, s.created_at,
		       sl.email AS sale_lead_email, sl.first_name AS sale_lead_first_name,
		       b.id AS b_id, b.resource_ids, b.lead_id AS b_lead_id, l.email, l.first_name,
		       b.start_time, b.end_time, b.status AS b_status, b.shopify_order_id AS b_shopify_order_id,
		       b.appointment_mode, b.google_event_id, b.google_meet_url, b.google_sync_status,
		       b.google_last_error, b.confirmation_email_sent, b.link_added_email_sent, b.created_at AS b_created_at
		FROM sales s
		LEFT JOIN leads sl ON s.lead_id = sl.id
		LEFT JOIN bookings b ON s.booking_id = b.id
		LEFT JOIN leads l ON b.lead_id = l.id
		ORDER BY s.created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query paginated sales: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.logger.System().Warn("Failed to close rows", "error", closeErr)
		}
	}()

	var sales []*saleEntity.SaleListItem
	for rows.Next() {
		item, err := scanSaleListItem(rows)
		if err != nil {
			return nil, 0, err
		}
		sales = append(sales, item)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("row iteration error in paginated sales: %w", err)
	}

	return sales, count, nil
}

// GetMetrics calculates paid receipt aggregates for the Shopify dashboard.
func (r *SQLSaleRepository) GetMetrics(tenantID string, now time.Time) (*saleEntity.SaleMetrics, error) {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	metrics := &saleEntity.SaleMetrics{
		PaidOrderTotalMonth:       "0.00",
		PaidOrderTotalYear:        "0.00",
		PaidOrderTotalAllTime:     "0.00",
		AveragePaidOrderMonth:     "0.00",
		LocalPickupLineTotalMonth: "0.00",
		CurrencyCode:              "USD",
	}

	countQuery := `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'PAID' AND created_at >= ? THEN 1 ELSE 0 END), 0) AS paid_orders_month,
			COALESCE(SUM(CASE WHEN status = 'PAID' AND created_at >= ? THEN 1 ELSE 0 END), 0) AS paid_orders_year,
			COALESCE(SUM(CASE WHEN status = 'PAID' THEN 1 ELSE 0 END), 0) AS paid_orders_all_time,
			COALESCE(COUNT(DISTINCT CASE WHEN status = 'PAID' AND lead_id IS NOT NULL AND lead_id != '' THEN lead_id END), 0) AS unique_paying_customers,
			COALESCE(SUM(CASE WHEN status = 'PAID' AND created_at >= ? AND booking_id IS NOT NULL THEN 1 ELSE 0 END), 0) AS appointment_orders_month,
			COALESCE(SUM(CASE WHEN status = 'PAID' AND created_at >= ? AND booking_id IS NULL AND appointment_intent = 0 THEN 1 ELSE 0 END), 0) AS product_only_orders_month,
			COALESCE(SUM(CASE WHEN status = 'PAID' AND created_at >= ? AND booking_id IS NULL AND appointment_intent = 1 THEN 1 ELSE 0 END), 0) AS orphan_orders_month
		FROM sales`

	if err := r.db.QueryRow(countQuery, monthStart, yearStart, monthStart, monthStart, monthStart).Scan(
		&metrics.PaidOrdersMonth,
		&metrics.PaidOrdersYear,
		&metrics.PaidOrdersAllTime,
		&metrics.UniquePayingCustomers,
		&metrics.AppointmentOrdersMonth,
		&metrics.ProductOnlyOrdersMonth,
		&metrics.OrphanOrdersMonth,
	); err != nil {
		return nil, fmt.Errorf("failed to fetch sales count metrics: %w", err)
	}

	rows, err := r.db.Query(`
		SELECT total_amount, products, created_at
		FROM sales
		WHERE status = 'PAID'`)
	if err != nil {
		return nil, fmt.Errorf("failed to query sales totals for metrics: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.logger.System().Warn("Failed to close rows", "error", closeErr)
		}
	}()

	var monthTotal float64
	var yearTotal float64
	var allTimeTotal float64
	var localPickupMonthTotal float64
	for rows.Next() {
		var totalAmount string
		var productsJSON string
		var createdAt time.Time
		if err := rows.Scan(&totalAmount, &productsJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan sale metrics row: %w", err)
		}

		total, err := parseSaleMetricAmount(totalAmount)
		if err != nil {
			return nil, fmt.Errorf("failed to parse sale total amount %q: %w", totalAmount, err)
		}
		allTimeTotal += total
		if !createdAt.Before(yearStart) {
			yearTotal += total
		}
		isCurrentMonth := !createdAt.Before(monthStart)
		if isCurrentMonth {
			monthTotal += total
		}

		var products []saleEntity.SaleProductLine
		if err := json.Unmarshal([]byte(productsJSON), &products); err != nil {
			return nil, fmt.Errorf("failed to unmarshal sale products for metrics: %w", err)
		}

		hasLocalPickup := false
		for _, product := range products {
			if metrics.CurrencyCode == "USD" && strings.TrimSpace(product.CurrencyCode) != "" {
				metrics.CurrencyCode = strings.TrimSpace(product.CurrencyCode)
			}
			if !isCurrentMonth || !product.IsLocalPickup {
				continue
			}

			price, err := parseSaleMetricAmount(product.Price)
			if err != nil {
				return nil, fmt.Errorf("failed to parse local pickup product price %q: %w", product.Price, err)
			}
			localPickupMonthTotal += price * float64(product.Quantity)
			hasLocalPickup = true
		}
		if isCurrentMonth && hasLocalPickup {
			metrics.LocalPickupOrdersMonth++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error in sale metrics: %w", err)
	}

	metrics.PaidOrderTotalMonth = formatSaleMetricAmount(monthTotal)
	metrics.PaidOrderTotalYear = formatSaleMetricAmount(yearTotal)
	metrics.PaidOrderTotalAllTime = formatSaleMetricAmount(allTimeTotal)
	metrics.LocalPickupLineTotalMonth = formatSaleMetricAmount(localPickupMonthTotal)
	if metrics.PaidOrdersMonth > 0 {
		metrics.AveragePaidOrderMonth = formatSaleMetricAmount(monthTotal / float64(metrics.PaidOrdersMonth))
	}

	return metrics, nil
}

func scanSaleListItem(rows *sql.Rows) (*saleEntity.SaleListItem, error) {
	var item saleEntity.SaleListItem
	var saleLeadID sql.NullString
	var saleBookingID sql.NullString
	var saleStatus string
	var productsJSON string
	var appointmentIntent int
	var saleLeadEmail sql.NullString
	var saleLeadName sql.NullString

	var bID sql.NullString
	var bResourceIDs sql.NullString
	var bLeadID sql.NullString
	var bLeadEmail sql.NullString
	var bLeadName sql.NullString
	var bStartTime sql.NullTime
	var bEndTime sql.NullTime
	var bStatus sql.NullString
	var bShopifyOrderID sql.NullString
	var bAppointmentMode sql.NullString
	var bGoogleEventID sql.NullString
	var bGoogleMeetURL sql.NullString
	var bGoogleSyncStatus sql.NullString
	var bGoogleLastError sql.NullString
	var bConfirmationEmailSent sql.NullInt64
	var bLinkAddedEmailSent sql.NullInt64
	var bCreatedAt sql.NullTime

	if err := rows.Scan(
		&item.ID,
		&saleLeadID,
		&saleBookingID,
		&item.ShopifyOrderID,
		&item.TotalAmount,
		&saleStatus,
		&productsJSON,
		&appointmentIntent,
		&item.CreatedAt,
		&saleLeadEmail,
		&saleLeadName,
		&bID,
		&bResourceIDs,
		&bLeadID,
		&bLeadEmail,
		&bLeadName,
		&bStartTime,
		&bEndTime,
		&bStatus,
		&bShopifyOrderID,
		&bAppointmentMode,
		&bGoogleEventID,
		&bGoogleMeetURL,
		&bGoogleSyncStatus,
		&bGoogleLastError,
		&bConfirmationEmailSent,
		&bLinkAddedEmailSent,
		&bCreatedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to scan paginated sale: %w", err)
	}

	if saleLeadID.Valid {
		item.LeadID = saleLeadID.String
	}
	if saleBookingID.Valid {
		item.BookingID = &saleBookingID.String
	}
	item.Status = saleEntity.SaleStatus(saleStatus)
	item.AppointmentIntent = appointmentIntent == 1
	if saleLeadEmail.Valid {
		item.LeadEmail = saleLeadEmail.String
	}
	if saleLeadName.Valid {
		item.LeadName = saleLeadName.String
	}
	if err := json.Unmarshal([]byte(productsJSON), &item.Products); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sale products: %w", err)
	}

	if bID.Valid {
		b := &bookingEntity.Booking{
			ID: bID.String,
		}
		if bResourceIDs.Valid {
			if err := json.Unmarshal([]byte(bResourceIDs.String), &b.ResourceIDs); err != nil {
				return nil, fmt.Errorf("failed to unmarshal booking resource ids: %w", err)
			}
		}
		if bLeadID.Valid {
			b.LeadID = bLeadID.String
		}
		if bLeadEmail.Valid {
			b.LeadEmail = bLeadEmail.String
		}
		if bLeadName.Valid {
			b.LeadName = bLeadName.String
		}
		if bStartTime.Valid {
			b.StartTime = bStartTime.Time
		}
		if bEndTime.Valid {
			b.EndTime = bEndTime.Time
		}
		if bStatus.Valid {
			b.Status = bookingEntity.BookingStatus(bStatus.String)
		}
		if bShopifyOrderID.Valid {
			b.ShopifyOrderID = &bShopifyOrderID.String
		}
		if bAppointmentMode.Valid {
			b.AppointmentMode = bookingEntity.AppointmentMode(bAppointmentMode.String)
		}
		if bGoogleEventID.Valid {
			b.GoogleEventID = &bGoogleEventID.String
		}
		if bGoogleMeetURL.Valid {
			b.GoogleMeetURL = &bGoogleMeetURL.String
		}
		if bGoogleSyncStatus.Valid {
			b.GoogleSyncStatus = bookingEntity.GoogleSyncStatus(bGoogleSyncStatus.String)
		}
		if bGoogleLastError.Valid {
			b.GoogleLastError = &bGoogleLastError.String
		}
		if bConfirmationEmailSent.Valid {
			b.ConfirmationEmailSent = bConfirmationEmailSent.Int64 == 1
		}
		if bLinkAddedEmailSent.Valid {
			b.LinkAddedEmailSent = bLinkAddedEmailSent.Int64 == 1
		}
		if bCreatedAt.Valid {
			b.CreatedAt = bCreatedAt.Time
		}
		item.Booking = b
	}

	return &item, nil
}

func boolToSQLiteInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func parseSaleMetricAmount(value string) (float64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("empty amount")
	}
	amount, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, err
	}
	return amount, nil
}

func formatSaleMetricAmount(value float64) string {
	return fmt.Sprintf("%.2f", value)
}
