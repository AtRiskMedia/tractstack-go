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
	"crypto/rand"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/analytics"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// Global counter for ensuring unique IDs under high concurrency
var idCounter uint64

// SQLEventRepository handles real-time event persistence to database.
type SQLEventRepository struct {
	db     *database.DB
	logger *logging.ChanneledLogger
}

// NewSQLEventRepository creates a new instance of the repository.
func NewSQLEventRepository(db *database.DB, logger *logging.ChanneledLogger) *SQLEventRepository {
	return &SQLEventRepository{
		db:     db,
		logger: logger,
	}
}

// generateUniqueActionID creates a truly unique action ID safe for concurrent use
func (r *SQLEventRepository) generateUniqueActionID() string {
	// Use atomic counter + nanosecond timestamp + random component for uniqueness
	counter := atomic.AddUint64(&idCounter, 1)
	timestamp := time.Now().UnixNano()

	// Add 4 random bytes for extra uniqueness under extreme concurrency
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := fmt.Sprintf("%x", randomBytes)

	return fmt.Sprintf("action_%d_%d_%s", timestamp, counter, randomHex)
}

// generateUniqueBeliefID creates a truly unique belief ID safe for concurrent use
func (r *SQLEventRepository) generateUniqueBeliefID() string {
	// Same strategy for belief events
	counter := atomic.AddUint64(&idCounter, 1)
	timestamp := time.Now().UnixNano()

	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := fmt.Sprintf("%x", randomBytes)

	return fmt.Sprintf("belief_%d_%d_%s", timestamp, counter, randomHex)
}

// StoreActionEvent saves a user action event to the database with duplicate protection.
func (r *SQLEventRepository) StoreActionEvent(event *analytics.ActionEvent) error {
	// Generate truly unique ID
	actionID := r.generateUniqueActionID()

	const query = `
		INSERT INTO actions (id, object_id, object_type, duration, visit_id, fingerprint_id, verb, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	start := time.Now()
	r.logger.Database().Debug("Executing action event insert",
		"actionId", actionID,
		"objectId", event.ObjectID,
		"objectType", event.ObjectType,
		"verb", event.Verb,
		"fingerprintId", event.FingerprintID)

	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
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

		if err == nil {
			// Success!
			r.logger.Database().Info("Action event insert completed",
				"actionId", actionID,
				"objectId", event.ObjectID,
				"objectType", event.ObjectType,
				"verb", event.Verb,
				"fingerprintId", event.FingerprintID,
				"attempts", attempt,
				"duration", time.Since(start))

			duration := time.Since(start)
			if duration > config.SlowQueryThreshold {
				r.logger.LogSlowQuery(query, duration, "system")
			}
			return nil
		}

		// Check if it's a UNIQUE constraint error
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			if attempt < maxRetries {
				// Generate new ID and retry
				oldActionID := actionID
				actionID = r.generateUniqueActionID()
				r.logger.Database().Warn("Action ID collision detected, retrying with new ID",
					"oldActionId", oldActionID,
					"newActionId", actionID,
					"attempt", attempt,
					"objectId", event.ObjectID,
					"verb", event.Verb)
				continue
			} else {
				// Max retries exceeded - this should be extremely rare
				r.logger.Database().Error("Action event insert failed after max retries",
					"error", err.Error(),
					"actionId", actionID,
					"objectId", event.ObjectID,
					"verb", event.Verb,
					"fingerprintId", event.FingerprintID,
					"attempts", attempt)
				return fmt.Errorf("failed to store action event after %d attempts: %w", maxRetries, err)
			}
		} else {
			// Non-unique constraint error - fail immediately
			r.logger.Database().Error("Action event insert failed",
				"error", err.Error(),
				"actionId", actionID,
				"objectId", event.ObjectID,
				"verb", event.Verb,
				"fingerprintId", event.FingerprintID)
			return fmt.Errorf("failed to store action event: %w", err)
		}
	}

	// Should never reach here
	return fmt.Errorf("unexpected error in action event storage")
}

// StoreBeliefEvent saves a user belief event to the database with duplicate protection.
func (r *SQLEventRepository) StoreBeliefEvent(event *analytics.BeliefEvent) error {
	// Generate truly unique ID
	beliefID := r.generateUniqueBeliefID()

	const query = `
		INSERT INTO heldbeliefs (id, belief_id, fingerprint_id, verb, object, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`

	start := time.Now()
	r.logger.Database().Debug("Executing belief event insert",
		"beliefEventId", beliefID,
		"beliefId", event.BeliefID,
		"verb", event.Verb,
		"fingerprintId", event.FingerprintID)

	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, err := r.db.Exec(
			query,
			beliefID,
			event.BeliefID,      // Required per schema
			event.FingerprintID, // Required per schema
			event.Verb,          // Required per schema
			event.Object,        // Can be NULL per schema (for identifyAs events)
			event.UpdatedAt.Format("2006-01-02 15:04:05"), // SQLite format
		)

		if err == nil {
			// Success!
			r.logger.Database().Info("Belief event insert completed",
				"beliefEventId", beliefID,
				"beliefId", event.BeliefID,
				"verb", event.Verb,
				"fingerprintId", event.FingerprintID,
				"attempts", attempt,
				"duration", time.Since(start))

			duration := time.Since(start)
			if duration > config.SlowQueryThreshold {
				r.logger.LogSlowQuery(query, duration, "system")
			}
			return nil
		}

		// Check if it's a UNIQUE constraint error
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			if attempt < maxRetries {
				// Generate new ID and retry
				oldBeliefID := beliefID
				beliefID = r.generateUniqueBeliefID()
				r.logger.Database().Warn("Belief ID collision detected, retrying with new ID",
					"oldBeliefId", oldBeliefID,
					"newBeliefId", beliefID,
					"attempt", attempt,
					"beliefId", event.BeliefID,
					"verb", event.Verb)
				continue
			} else {
				// Max retries exceeded - this should be extremely rare
				r.logger.Database().Error("Belief event insert failed after max retries",
					"error", err.Error(),
					"beliefEventId", beliefID,
					"beliefId", event.BeliefID,
					"verb", event.Verb,
					"fingerprintId", event.FingerprintID,
					"attempts", attempt)
				return fmt.Errorf("failed to store belief event after %d attempts: %w", maxRetries, err)
			}
		} else {
			// Non-unique constraint error - fail immediately
			r.logger.Database().Error("Belief event insert failed",
				"error", err.Error(),
				"beliefEventId", beliefID,
				"beliefId", event.BeliefID,
				"verb", event.Verb,
				"fingerprintId", event.FingerprintID)
			return fmt.Errorf("failed to store belief event: %w", err)
		}
	}

	// Should never reach here
	return fmt.Errorf("unexpected error in belief event storage")
}

// FindActionEventsInRange retrieves action events for cache warming.
func (r *SQLEventRepository) FindActionEventsInRange(startTime, endTime time.Time, verbFilter []string) ([]*analytics.ActionEvent, error) {
	if len(verbFilter) == 0 {
		return []*analytics.ActionEvent{}, nil
	}

	// Build query with verb filtering
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
	args := make([]any, 0, 2+len(verbFilter))
	args = append(args, startTime.Format("2006-01-02 15:04:05"))
	args = append(args, endTime.Format("2006-01-02 15:04:05"))
	for _, verb := range verbFilter {
		args = append(args, verb)
	}

	start := time.Now()
	r.logger.Database().Debug("Loading action events in range",
		"startTime", startTime,
		"endTime", endTime,
		"verbFilter", verbFilter)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query action events in range",
			"error", err.Error(),
			"startTime", startTime,
			"endTime", endTime)
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
			r.logger.Database().Error("Failed to scan action event row", "error", err.Error())
			continue
		}

		// Parse timestamp
		event.CreatedAt, err = r.parseTimestamp(createdAtStr)
		if err != nil {
			// Log warning but continue
			r.logger.Database().Error("Failed to parse action event timestamp", "error", err.Error(), "timestamp", createdAtStr)
			continue
		}

		event.VisitID = visitID
		if duration != nil {
			event.Duration = *duration
		}

		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		r.logger.Database().Error("Row iteration error for action events", "error", err.Error())
		return nil, err
	}

	r.logger.Database().Info("Action events loaded in range",
		"startTime", startTime,
		"endTime", endTime,
		"count", len(events),
		"duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return events, nil
}

// FindBeliefEventsInRange retrieves belief events for cache warming.
func (r *SQLEventRepository) FindBeliefEventsInRange(startTime, endTime time.Time, valueFilter []string) ([]*analytics.BeliefEvent, error) {
	if len(valueFilter) == 0 {
		return []*analytics.BeliefEvent{}, nil
	}

	// Build query with value filtering
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
	args := make([]any, 0, 2+len(valueFilter))
	args = append(args, startTime.Format("2006-01-02 15:04:05"))
	args = append(args, endTime.Format("2006-01-02 15:04:05"))
	for _, value := range valueFilter {
		args = append(args, value)
	}

	start := time.Now()
	r.logger.Database().Debug("Loading belief events in range",
		"startTime", startTime,
		"endTime", endTime,
		"valueFilter", valueFilter)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query belief events in range",
			"error", err.Error(),
			"startTime", startTime,
			"endTime", endTime)
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
			r.logger.Database().Error("Failed to scan belief event row", "error", err.Error())
			continue
		}

		// Parse timestamp
		event.UpdatedAt, err = r.parseTimestamp(updatedAtStr)
		if err != nil {
			// Log warning but continue
			r.logger.Database().Error("Failed to parse belief event timestamp", "error", err.Error(), "timestamp", updatedAtStr)
			continue
		}

		if object != nil {
			event.Object = object
		}

		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		r.logger.Database().Error("Row iteration error for belief events", "error", err.Error())
		return nil, err
	}

	r.logger.Database().Info("Belief events loaded in range",
		"startTime", startTime,
		"endTime", endTime,
		"count", len(events),
		"duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return events, nil
}

// CountEventsInRange returns total event count for batching decisions.
func (r *SQLEventRepository) CountEventsInRange(startTime, endTime time.Time) (int, error) {
	start := time.Now()
	r.logger.Database().Debug("Counting events in range", "startTime", startTime, "endTime", endTime)

	var actionCount, beliefCount int

	// Count actions
	actionQuery := `SELECT COUNT(*) FROM actions WHERE created_at >= ? AND created_at < ?`
	err := r.db.QueryRow(actionQuery,
		startTime.Format("2006-01-02 15:04:05"),
		endTime.Format("2006-01-02 15:04:05")).Scan(&actionCount)
	if err != nil {
		r.logger.Database().Error("Failed to count action events", "error", err.Error(), "startTime", startTime, "endTime", endTime)
		return 0, fmt.Errorf("failed to count action events: %w", err)
	}

	// Count beliefs
	beliefQuery := `SELECT COUNT(*) FROM heldbeliefs WHERE updated_at >= ? AND updated_at < ?`
	err = r.db.QueryRow(beliefQuery,
		startTime.Format("2006-01-02 15:04:05"),
		endTime.Format("2006-01-02 15:04:05")).Scan(&beliefCount)
	if err != nil {
		r.logger.Database().Error("Failed to count belief events", "error", err.Error(), "startTime", startTime, "endTime", endTime)
		return 0, fmt.Errorf("failed to count belief events: %w", err)
	}

	totalCount := actionCount + beliefCount
	r.logger.Database().Info("Event count completed",
		"startTime", startTime,
		"endTime", endTime,
		"actionCount", actionCount,
		"beliefCount", beliefCount,
		"totalCount", totalCount,
		"duration", time.Since(start))

	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		// Log both action and belief queries as slow if the total operation is slow
		r.logger.LogSlowQuery("SELECT COUNT(*) FROM actions WHERE created_at >= ? AND created_at < ?", duration, "system")
		r.logger.LogSlowQuery("SELECT COUNT(*) FROM heldbeliefs WHERE updated_at >= ? AND updated_at < ?", duration, "system")
	}

	return totalCount, nil
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

// LoadFingerprintBeliefs reconstructs the belief state for a fingerprint from the heldbeliefs table
func (r *SQLEventRepository) LoadFingerprintBeliefs(fingerprintID string) (map[string][]string, error) {
	const query = `
		SELECT b.slug, hb.verb, hb.object
		FROM heldbeliefs hb
		JOIN beliefs b ON hb.belief_id = b.id
		WHERE hb.fingerprint_id = ?
		ORDER BY hb.updated_at ASC`

	start := time.Now()
	r.logger.Database().Debug("Loading fingerprint beliefs", "fingerprintId", fingerprintID)

	rows, err := r.db.Query(query, fingerprintID)
	if err != nil {
		r.logger.Database().Error("Failed to query fingerprint beliefs", "error", err.Error(), "fingerprintId", fingerprintID)
		return nil, fmt.Errorf("failed to query fingerprint beliefs: %w", err)
	}
	defer rows.Close()

	beliefs := make(map[string][]string)
	for rows.Next() {
		var slug, verb string
		var object *string

		err := rows.Scan(&slug, &verb, &object)
		if err != nil {
			r.logger.Database().Error("Failed to scan belief row", "error", err.Error())
			continue
		}

		// Handle different belief types
		switch verb {
		case "UNSET":
			delete(beliefs, slug)
		case "IDENTIFY_AS":
			if object != nil {
				beliefs[slug] = []string{*object}
			}
		default:
			// Regular belief values (BELIEVES_YES, BELIEVES_NO, etc.)
			currentValues := beliefs[slug]
			if !slices.Contains(currentValues, verb) {
				beliefs[slug] = append(currentValues, verb)
			}
		}
	}

	if err := rows.Err(); err != nil {
		r.logger.Database().Error("Row iteration error for fingerprint beliefs", "error", err.Error())
		return nil, err
	}

	r.logger.Database().Info("Fingerprint beliefs loaded",
		"fingerprintId", fingerprintID,
		"beliefCount", len(beliefs),
		"duration", time.Since(start))

	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}

	return beliefs, nil
}
