// Package analytics provides the concrete SQL-based implementations
// for analytics event persistence.
//
// PURPOSE: Store real-time user events to database as they happen
// - Action events → actions table
// - Belief events → heldbeliefs table
//
// This is SEPARATE from analytics computation which uses cached hourly bins.
package analytics

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/analytics"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

// SQLEventRepository handles real-time event persistence to database.
type SQLEventRepository struct {
	db *database.DB
}

// NewSQLEventRepository creates a new instance of the repository.
func NewSQLEventRepository(db *database.DB) *SQLEventRepository {
	return &SQLEventRepository{db: db}
}

// StoreActionEvent saves a user action event to the database.
func (r *SQLEventRepository) StoreActionEvent(event *analytics.ActionEvent) error {
	// Generate unique ID - using timestamp-based approach for now
	actionID := fmt.Sprintf("action_%d_%d", time.Now().UnixNano(), time.Now().Unix())

	const query = `
		INSERT INTO actions (id, object_id, object_type, duration, visit_id, fingerprint_id, verb, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.Exec(
		query,
		actionID,
		event.ObjectID,
		event.ObjectType,
		event.Duration,      // Can be NULL per schema
		event.VisitID,       // Required per schema
		event.FingerprintID, // Required per schema
		event.Verb,          // Required per schema
		event.CreatedAt.Format("2006-01-02 15:04:05"), // SQLite format
	)
	if err != nil {
		return fmt.Errorf("failed to store action event: %w", err)
	}

	return nil
}

// StoreBeliefEvent saves a user belief event to the database.
func (r *SQLEventRepository) StoreBeliefEvent(event *analytics.BeliefEvent) error {
	// Generate unique ID - using timestamp-based approach for now
	beliefID := fmt.Sprintf("belief_%d_%d", time.Now().UnixNano(), time.Now().Unix())

	const query = `
		INSERT INTO heldbeliefs (id, belief_id, fingerprint_id, verb, object, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := r.db.Exec(
		query,
		beliefID,
		event.BeliefID,      // Required per schema
		event.FingerprintID, // Required per schema
		event.Verb,          // Required per schema
		event.Object,        // Can be NULL per schema (for identifyAs events)
		event.UpdatedAt.Format("2006-01-02 15:04:05"), // SQLite format
	)
	if err != nil {
		return fmt.Errorf("failed to store belief event: %w", err)
	}

	return nil
}

// GetActionEventsInRange retrieves action events for cache warming.
// This mirrors the cache_warmer.go getActionEventsForRange function.
func (r *SQLEventRepository) GetActionEventsInRange(startTime, endTime time.Time, verbFilter []string) ([]*analytics.ActionEvent, error) {
	if len(verbFilter) == 0 {
		return []*analytics.ActionEvent{}, nil
	}

	// Build query with verb filtering (exactly like cache_warmer.go)
	verbPlaceholders := ""
	for i := range verbFilter {
		if i > 0 {
			verbPlaceholders += ","
		}
		verbPlaceholders += "?"
	}

	query := fmt.Sprintf(`
		SELECT object_id, object_type, verb, fingerprint_id, visit_id, duration, created_at
		FROM actions
		WHERE created_at >= ? AND created_at < ? AND verb IN (%s)
		ORDER BY created_at`, verbPlaceholders)

	// Prepare arguments
	args := make([]interface{}, 0, 2+len(verbFilter))
	args = append(args, startTime.Format("2006-01-02 15:04:05"))
	args = append(args, endTime.Format("2006-01-02 15:04:05"))
	for _, verb := range verbFilter {
		args = append(args, verb)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query action events: %w", err)
	}
	defer rows.Close()

	var events []*analytics.ActionEvent
	for rows.Next() {
		var event analytics.ActionEvent
		var createdAtStr string
		var duration *int
		var visitID string

		err := rows.Scan(
			&event.ObjectID,
			&event.ObjectType,
			&event.Verb,
			&event.FingerprintID,
			&visitID,
			&duration,
			&createdAtStr,
		)
		if err != nil {
			// Log warning but continue
			continue
		}

		// Parse timestamp
		event.CreatedAt, err = r.parseTimestamp(createdAtStr)
		if err != nil {
			// Log warning but continue
			continue
		}

		event.VisitID = visitID
		if duration != nil {
			event.Duration = *duration
		}

		events = append(events, &event)
	}

	return events, rows.Err()
}

// GetBeliefEventsInRange retrieves belief events for cache warming.
// This mirrors the cache_warmer.go getBeliefEventsForRange function.
func (r *SQLEventRepository) GetBeliefEventsInRange(startTime, endTime time.Time, valueFilter []string) ([]*analytics.BeliefEvent, error) {
	if len(valueFilter) == 0 {
		return []*analytics.BeliefEvent{}, nil
	}

	// Build query with value filtering (exactly like cache_warmer.go)
	valuePlaceholders := ""
	for i := range valueFilter {
		if i > 0 {
			valuePlaceholders += ","
		}
		valuePlaceholders += "?"
	}

	query := fmt.Sprintf(`
		SELECT belief_id, fingerprint_id, verb, object, updated_at
		FROM heldbeliefs
		WHERE updated_at >= ? AND updated_at < ? AND verb IN (%s)
		ORDER BY updated_at`, valuePlaceholders)

	// Prepare arguments
	args := make([]interface{}, 0, 2+len(valueFilter))
	args = append(args, startTime.Format("2006-01-02 15:04:05"))
	args = append(args, endTime.Format("2006-01-02 15:04:05"))
	for _, value := range valueFilter {
		args = append(args, value)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query belief events: %w", err)
	}
	defer rows.Close()

	var events []*analytics.BeliefEvent
	for rows.Next() {
		var event analytics.BeliefEvent
		var updatedAtStr string
		var object *string

		err := rows.Scan(
			&event.BeliefID,
			&event.FingerprintID,
			&event.Verb,
			&object,
			&updatedAtStr,
		)
		if err != nil {
			// Log warning but continue
			continue
		}

		// Parse timestamp
		event.UpdatedAt, err = r.parseTimestamp(updatedAtStr)
		if err != nil {
			// Log warning but continue
			continue
		}

		if object != nil {
			event.Object = object
		}

		events = append(events, &event)
	}

	return events, rows.Err()
}

// CountEventsInRange returns total event count for batching decisions.
// This mirrors the cache_warmer.go countEventsInRange function.
func (r *SQLEventRepository) CountEventsInRange(startTime, endTime time.Time) (int, error) {
	var actionCount, beliefCount int

	// Count actions
	actionQuery := `SELECT COUNT(*) FROM actions WHERE created_at >= ? AND created_at < ?`
	err := r.db.QueryRow(actionQuery,
		startTime.Format("2006-01-02 15:04:05"),
		endTime.Format("2006-01-02 15:04:05")).Scan(&actionCount)
	if err != nil {
		return 0, fmt.Errorf("failed to count action events: %w", err)
	}

	// Count beliefs
	beliefQuery := `SELECT COUNT(*) FROM heldbeliefs WHERE updated_at >= ? AND updated_at < ?`
	err = r.db.QueryRow(beliefQuery,
		startTime.Format("2006-01-02 15:04:05"),
		endTime.Format("2006-01-02 15:04:05")).Scan(&beliefCount)
	if err != nil {
		return 0, fmt.Errorf("failed to count belief events: %w", err)
	}

	return actionCount + beliefCount, nil
}

// parseTimestamp handles multiple timestamp formats
func (r *SQLEventRepository) parseTimestamp(timestampStr string) (time.Time, error) {
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, timestampStr); err == nil {
		return t, nil
	}

	// Try SQLite format
	if t, err := time.Parse("2006-01-02 15:04:05", timestampStr); err == nil {
		return t, nil
	}

	// Try ISO format with milliseconds
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", timestampStr); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp format: %s", timestampStr)
}
