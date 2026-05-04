// Package booking provides the SQLite implementation for booking persistence.
package booking

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/booking"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
)

// SQLBookingRepository implements the booking repository interface using SQLite.
type SQLBookingRepository struct {
	db     *sql.DB
	logger *logging.ChanneledLogger
}

// NewBookingRepository creates a new SQLite-backed booking repository.
func NewBookingRepository(db *sql.DB, logger *logging.ChanneledLogger) *SQLBookingRepository {
	return &SQLBookingRepository{
		db:     db,
		logger: logger,
	}
}

// FindByID retrieves a booking by its exact trace ID.
func (r *SQLBookingRepository) FindByID(tenantID, id string) (*booking.Booking, error) {
	query := `
		SELECT id, resource_ids, lead_id, start_time, end_time, status, shopify_order_id, appointment_mode, google_event_id, google_meet_url, google_sync_status, google_last_error, confirmation_email_sent, link_added_email_sent, created_at
		FROM bookings
		WHERE id = ?`

	var b booking.Booking
	var statusStr string
	var shopifyOrderID sql.NullString
	var appointmentMode string
	var googleEventID sql.NullString
	var googleMeetURL sql.NullString
	var googleSyncStatus string
	var googleLastError sql.NullString
	var confirmationEmailSent int
	var linkAddedEmailSent int
	var resourceIDsJSON string

	err := r.db.QueryRow(query, id).Scan(
		&b.ID,
		&resourceIDsJSON,
		&b.LeadID,
		&b.StartTime,
		&b.EndTime,
		&statusStr,
		&shopifyOrderID,
		&appointmentMode,
		&googleEventID,
		&googleMeetURL,
		&googleSyncStatus,
		&googleLastError,
		&confirmationEmailSent,
		&linkAddedEmailSent,
		&b.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find booking by id: %w", err)
	}

	if err := json.Unmarshal([]byte(resourceIDsJSON), &b.ResourceIDs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource ids: %w", err)
	}

	b.Status = booking.BookingStatus(statusStr)
	if shopifyOrderID.Valid {
		b.ShopifyOrderID = &shopifyOrderID.String
	}
	b.AppointmentMode = booking.AppointmentMode(appointmentMode)
	if googleEventID.Valid {
		b.GoogleEventID = &googleEventID.String
	}
	if googleMeetURL.Valid {
		b.GoogleMeetURL = &googleMeetURL.String
	}
	b.GoogleSyncStatus = booking.GoogleSyncStatus(googleSyncStatus)
	if googleLastError.Valid {
		b.GoogleLastError = &googleLastError.String
	}
	b.ConfirmationEmailSent = confirmationEmailSent == 1
	b.LinkAddedEmailSent = linkAddedEmailSent == 1

	return &b, nil
}

// FindOverlapping retrieves any non-cancelled bookings that overlap with the provided time window globally.
func (r *SQLBookingRepository) FindOverlapping(tenantID string, start, end time.Time) ([]*booking.Booking, error) {
	args := []any{string(booking.StatusCancelled), end, start}

	query := `
		SELECT b.id, b.resource_ids, b.lead_id, b.start_time, b.end_time, b.status, b.shopify_order_id, b.appointment_mode, b.google_event_id, b.google_meet_url, b.google_sync_status, b.google_last_error, b.confirmation_email_sent, b.link_added_email_sent, b.created_at
		FROM bookings b
		WHERE b.status != ?
		AND b.start_time < ?
		AND b.end_time > ?`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query overlapping bookings: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.logger.System().Warn("Failed to close rows", "error", closeErr)
		}
	}()

	var bookings []*booking.Booking
	for rows.Next() {
		var b booking.Booking
		var statusStr string
		var shopifyOrderID sql.NullString
		var appointmentMode string
		var googleEventID sql.NullString
		var googleMeetURL sql.NullString
		var googleSyncStatus string
		var googleLastError sql.NullString
		var confirmationEmailSent int
		var linkAddedEmailSent int
		var resourceIDsJSON string

		err := rows.Scan(
			&b.ID,
			&resourceIDsJSON,
			&b.LeadID,
			&b.StartTime,
			&b.EndTime,
			&statusStr,
			&shopifyOrderID,
			&appointmentMode,
			&googleEventID,
			&googleMeetURL,
			&googleSyncStatus,
			&googleLastError,
			&confirmationEmailSent,
			&linkAddedEmailSent,
			&b.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan overlapping booking: %w", err)
		}

		if err := json.Unmarshal([]byte(resourceIDsJSON), &b.ResourceIDs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal overlapping resource ids: %w", err)
		}

		b.Status = booking.BookingStatus(statusStr)
		if shopifyOrderID.Valid {
			b.ShopifyOrderID = &shopifyOrderID.String
		}
		b.AppointmentMode = booking.AppointmentMode(appointmentMode)
		if googleEventID.Valid {
			b.GoogleEventID = &googleEventID.String
		}
		if googleMeetURL.Valid {
			b.GoogleMeetURL = &googleMeetURL.String
		}
		b.GoogleSyncStatus = booking.GoogleSyncStatus(googleSyncStatus)
		if googleLastError.Valid {
			b.GoogleLastError = &googleLastError.String
		}
		b.ConfirmationEmailSent = confirmationEmailSent == 1
		b.LinkAddedEmailSent = linkAddedEmailSent == 1

		bookings = append(bookings, &b)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error in overlapping bookings: %w", err)
	}

	return bookings, nil
}

// Store securely persists a new booking, wrapping the write in a transaction to respect WAL concurrency constraints.
func (r *SQLBookingRepository) Store(tenantID string, b *booking.Booking) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for store: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr != sql.ErrTxDone {
			r.logger.System().Warn("Failed to rollback transaction", "error", rollbackErr)
		}
	}()

	query := `
		INSERT INTO bookings (id, resource_ids, lead_id, start_time, end_time, status, shopify_order_id, appointment_mode, google_event_id, google_meet_url, google_sync_status, google_last_error, confirmation_email_sent, link_added_email_sent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var shopifyOrderID sql.NullString
	if b.ShopifyOrderID != nil {
		shopifyOrderID = sql.NullString{String: *b.ShopifyOrderID, Valid: true}
	}

	resourceIDsBytes, err := json.Marshal(b.ResourceIDs)
	if err != nil {
		return fmt.Errorf("failed to serialize resource ids: %w", err)
	}

	_, err = tx.Exec(query,
		b.ID,
		string(resourceIDsBytes),
		b.LeadID,
		b.StartTime,
		b.EndTime,
		string(b.Status),
		shopifyOrderID,
		string(b.AppointmentMode),
		b.GoogleEventID,
		b.GoogleMeetURL,
		string(booking.GoogleSyncNotSynced),
		b.GoogleLastError,
		boolToSQLiteInt(b.ConfirmationEmailSent),
		boolToSQLiteInt(b.LinkAddedEmailSent),
		b.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert booking: %w", err)
	}

	return tx.Commit()
}

// UpdateStatus transitions a booking's state (e.g., from PENDING to CONFIRMED) and optionally saves the Shopify Order ID.
func (r *SQLBookingRepository) UpdateStatus(tenantID, id string, status booking.BookingStatus, shopifyOrderID *string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for update: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr != sql.ErrTxDone {
			r.logger.System().Warn("Failed to rollback transaction", "error", rollbackErr)
		}
	}()

	query := `
		UPDATE bookings
		SET status = ?, shopify_order_id = COALESCE(?, shopify_order_id)
		WHERE id = ?`

	var shopifyOrderIDNull sql.NullString
	if shopifyOrderID != nil {
		shopifyOrderIDNull = sql.NullString{String: *shopifyOrderID, Valid: true}
	}

	_, err = tx.Exec(query, string(status), shopifyOrderIDNull, id)
	if err != nil {
		return fmt.Errorf("failed to update booking status: %w", err)
	}

	return tx.Commit()
}

// UpdateGoogleSyncPending marks a booking as pending google sync.
func (r *SQLBookingRepository) UpdateGoogleSyncPending(tenantID, id string) error {
	_, err := r.db.Exec(
		`UPDATE bookings SET google_sync_status = ?, google_last_error = NULL WHERE id = ? AND status != ?`,
		string(booking.GoogleSyncPending), id, string(booking.StatusCancelled),
	)
	if err != nil {
		return fmt.Errorf("failed to update google sync pending: %w", err)
	}
	return nil
}

// UpdateGoogleSyncSuccess stores successful google event linkage.
func (r *SQLBookingRepository) UpdateGoogleSyncSuccess(tenantID, id string, googleEventID, meetURL *string) error {
	_, err := r.db.Exec(
		`UPDATE bookings SET google_event_id = COALESCE(?, google_event_id), google_meet_url = COALESCE(?, google_meet_url), google_sync_status = ?, google_last_error = NULL WHERE id = ? AND status != ?`,
		googleEventID,
		meetURL,
		string(booking.GoogleSyncSynced),
		id,
		string(booking.StatusCancelled),
	)
	if err != nil {
		return fmt.Errorf("failed to update google sync success: %w", err)
	}
	return nil
}

// UpdateGoogleDeletePending marks cancellation-side delete as pending.
func (r *SQLBookingRepository) UpdateGoogleDeletePending(tenantID, id string) error {
	_, err := r.db.Exec(
		`UPDATE bookings SET google_sync_status = ? WHERE id = ?`,
		string(booking.GoogleSyncDeletePend), id,
	)
	if err != nil {
		return fmt.Errorf("failed to update google delete pending: %w", err)
	}
	return nil
}

// UpdateGoogleDeleteSuccess marks cancellation delete synced and clears linkage.
func (r *SQLBookingRepository) UpdateGoogleDeleteSuccess(tenantID, id string) error {
	_, err := r.db.Exec(
		`UPDATE bookings SET google_sync_status = ?, google_last_error = NULL, google_event_id = NULL, google_meet_url = NULL WHERE id = ?`,
		string(booking.GoogleSyncDeleteSynced), id,
	)
	if err != nil {
		return fmt.Errorf("failed to update google delete success: %w", err)
	}
	return nil
}

// UpdateGoogleSyncFailure stores sync/delete failures.
func (r *SQLBookingRepository) UpdateGoogleSyncFailure(tenantID, id string, syncStatus booking.GoogleSyncStatus, errorSummary string) error {
	_, err := r.db.Exec(
		`UPDATE bookings SET google_sync_status = ?, google_last_error = ? WHERE id = ?`,
		string(syncStatus), errorSummary, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update google sync failure: %w", err)
	}
	return nil
}

// MarkConfirmationEmailSent sets initial confirmation email sent flag.
func (r *SQLBookingRepository) MarkConfirmationEmailSent(tenantID, id string) error {
	_, err := r.db.Exec(`UPDATE bookings SET confirmation_email_sent = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to mark confirmation email sent: %w", err)
	}
	return nil
}

// MarkLinkAddedEmailSent sets follow-up link-added email sent flag.
func (r *SQLBookingRepository) MarkLinkAddedEmailSent(tenantID, id string) error {
	_, err := r.db.Exec(`UPDATE bookings SET link_added_email_sent = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to mark link-added email sent: %w", err)
	}
	return nil
}

// DeleteExpiredPending frees up abandoned slots by deleting PENDING rows older than the provided expiration boundary.
func (r *SQLBookingRepository) DeleteExpiredPending(tenantID string, expirationTime time.Time) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction for deletion: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr != sql.ErrTxDone {
			r.logger.System().Warn("Failed to rollback transaction", "error", rollbackErr)
		}
	}()

	query := `
		DELETE FROM bookings
		WHERE status = ? AND created_at <= ?`

	res, err := tx.Exec(query, string(booking.StatusPending), expirationTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired pending bookings: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit expired pending deletion: %w", err)
	}

	return int(rowsAffected), nil
}

// DeletePendingByTraceID proactively removes a hold if it is still pending
func (r *SQLBookingRepository) DeletePendingByTraceID(tenantID string, traceID string) error {
	const query = `
		DELETE FROM bookings
		WHERE id = ? AND status = 'PENDING'
	`
	r.logger.Database().Debug("Executing proactive hold release", "traceId", traceID)

	res, err := r.db.Exec(query, traceID)
	if err != nil {
		r.logger.Database().Error("Failed to release hold", "error", err, "traceId", traceID)
		return err
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		r.logger.Database().Debug("No pending hold found to release", "traceId", traceID)
	} else {
		r.logger.Database().Info("Proactive hold release successful", "traceId", traceID)
	}

	return nil
}

// FindAllPaginated retrieves a paginated list of bookings, optionally filtered by status.
func (r *SQLBookingRepository) FindAllPaginated(tenantID string, limit, offset int, status string) ([]*booking.Booking, int, error) {
	var count int
	countQuery := `SELECT COUNT(*) FROM bookings`
	var countArgs []any

	if status != "ALL" && status != "" {
		countQuery += ` WHERE status = ?`
		countArgs = append(countArgs, status)
	}

	if err := r.db.QueryRow(countQuery, countArgs...).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("failed to count bookings: %w", err)
	}

	query := `
		SELECT b.id, b.resource_ids, b.lead_id, l.email, l.first_name, b.start_time, b.end_time, b.status, b.shopify_order_id, b.appointment_mode, b.google_event_id, b.google_meet_url, b.google_sync_status, b.google_last_error, b.confirmation_email_sent, b.link_added_email_sent, b.created_at
		FROM bookings b
		LEFT JOIN leads l ON b.lead_id = l.id`
	var args []any

	if status != "ALL" && status != "" {
		query += ` WHERE b.status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY b.start_time DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query paginated bookings: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.logger.System().Warn("Failed to close rows", "error", closeErr)
		}
	}()

	var bookings []*booking.Booking
	for rows.Next() {
		var b booking.Booking
		var statusStr string
		var shopifyOrderID sql.NullString
		var appointmentMode string
		var googleEventID sql.NullString
		var googleMeetURL sql.NullString
		var googleSyncStatus string
		var googleLastError sql.NullString
		var confirmationEmailSent int
		var linkAddedEmailSent int
		var leadEmail sql.NullString
		var leadName sql.NullString
		var resourceIDsJSON string

		err := rows.Scan(
			&b.ID,
			&resourceIDsJSON,
			&b.LeadID,
			&leadEmail,
			&leadName,
			&b.StartTime,
			&b.EndTime,
			&statusStr,
			&shopifyOrderID,
			&appointmentMode,
			&googleEventID,
			&googleMeetURL,
			&googleSyncStatus,
			&googleLastError,
			&confirmationEmailSent,
			&linkAddedEmailSent,
			&b.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan paginated booking: %w", err)
		}

		if err := json.Unmarshal([]byte(resourceIDsJSON), &b.ResourceIDs); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal resource ids: %w", err)
		}

		b.Status = booking.BookingStatus(statusStr)
		if shopifyOrderID.Valid {
			b.ShopifyOrderID = &shopifyOrderID.String
		}
		b.AppointmentMode = booking.AppointmentMode(appointmentMode)
		if googleEventID.Valid {
			b.GoogleEventID = &googleEventID.String
		}
		if googleMeetURL.Valid {
			b.GoogleMeetURL = &googleMeetURL.String
		}
		b.GoogleSyncStatus = booking.GoogleSyncStatus(googleSyncStatus)
		if googleLastError.Valid {
			b.GoogleLastError = &googleLastError.String
		}
		b.ConfirmationEmailSent = confirmationEmailSent == 1
		b.LinkAddedEmailSent = linkAddedEmailSent == 1
		if leadEmail.Valid {
			b.LeadEmail = leadEmail.String
		}
		if leadName.Valid {
			b.LeadName = leadName.String
		}

		bookings = append(bookings, &b)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("row iteration error in paginated bookings: %w", err)
	}

	return bookings, count, nil
}

func boolToSQLiteInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// GetMetrics executes a conditional aggregation to calculate booking volume and conversion metrics.
func (r *SQLBookingRepository) GetMetrics(tenantID string, now time.Time) (*booking.BookingMetrics, error) {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	dayStart := now.Add(-24 * time.Hour)

	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN status = 'CONFIRMED' AND start_time >= ? THEN 1 ELSE 0 END), 0) as monthly_confirmed,
			COALESCE(SUM(CASE WHEN status = 'CONFIRMED' AND start_time >= ? THEN 1 ELSE 0 END), 0) as annual_confirmed,
			COALESCE(SUM(CASE WHEN status = 'CONFIRMED' AND start_time >= ? THEN 1 ELSE 0 END), 0) as weekly_confirmed,
			COALESCE(COUNT(DISTINCT CASE WHEN status = 'CONFIRMED' THEN lead_id END), 0) as lead_conversion_anchor,
			COALESCE(SUM(CASE WHEN status = 'PENDING' AND created_at >= ? THEN 1 ELSE 0 END), 0) as pending_24h,
			COALESCE(SUM(CASE WHEN status = 'CONFIRMED' AND created_at >= ? THEN 1 ELSE 0 END), 0) as confirmed_24h
		FROM bookings`

	var m booking.BookingMetrics
	err := r.db.QueryRow(query, monthStart, yearStart, weekStart, dayStart, dayStart).Scan(
		&m.TotalMonthlyConfirmed,
		&m.TotalAnnualConfirmed,
		&m.TotalWeeklyConfirmed,
		&m.LeadConversionAnchor,
		&m.PendingLast24h,
		&m.ConfirmedLast24h,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch booking metrics: %w", err)
	}

	return &m, nil
}
