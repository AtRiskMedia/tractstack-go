// Package analytics provides belief data processing functionality.
package analytics

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// processBeliefData processes all belief-related data in one consolidated query (exact V1 pattern)
func processBeliefData(ctx *tenant.Context, hourKeys []string, epinets []EpinetConfig,
	analysis *EpinetAnalysis, startTime, endTime time.Time, contentItems map[string]ContentItem,
) error {
	// Prepare query parameters (exact V1 pattern)
	var verbValues []string
	for verb := range analysis.BeliefValues {
		verbValues = append(verbValues, verb)
	}

	var objectValues []string
	for obj := range analysis.IdentifyAsValues {
		objectValues = append(objectValues, obj)
	}

	// Build the where clause for the query (exact V1 pattern)
	var whereConditions []string
	var params []interface{}
	params = append(params, startTime, endTime)

	if len(verbValues) > 0 {
		placeholders := make([]string, len(verbValues))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		whereConditions = append(whereConditions, fmt.Sprintf("verb IN (%s)", strings.Join(placeholders, ",")))
		for _, verb := range verbValues {
			params = append(params, verb)
		}
	}

	if len(objectValues) > 0 {
		placeholders := make([]string, len(objectValues))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		whereConditions = append(whereConditions, fmt.Sprintf("object IN (%s)", strings.Join(placeholders, ",")))
		for _, obj := range objectValues {
			params = append(params, obj)
		}
	}

	if len(whereConditions) == 0 {
		return nil
	}

	// Execute a single efficient query for all beliefs (exact V1 pattern)
	query := fmt.Sprintf(`
		SELECT 
			strftime('%%Y-%%m-%%d-%%H', updated_at, 'utc') as hour_key,
			belief_id,
			fingerprint_id,
			verb,
			object
		FROM heldbeliefs
		JOIN beliefs ON heldbeliefs.belief_id = beliefs.id
		WHERE 
			updated_at >= ? AND updated_at < ?
			AND (%s)
	`, strings.Join(whereConditions, " OR "))

	log.Printf("DEBUG: Executing belief query with %d parameters", len(params))

	rows, err := ctx.Database.Conn.Query(query, params...)
	if err != nil {
		return fmt.Errorf("failed to query beliefs: %w", err)
	}
	defer rows.Close()

	beliefEvents := make([]BeliefEvent, 0)
	for rows.Next() {
		var hourKey, beliefID, fingerprintID, verb string
		var object *string

		err := rows.Scan(&hourKey, &beliefID, &fingerprintID, &verb, &object)
		if err != nil {
			return fmt.Errorf("failed to scan belief row: %w", err)
		}

		// Only process hours we're interested in
		if !containsString(hourKeys, hourKey) {
			continue
		}

		beliefEvents = append(beliefEvents, BeliefEvent{
			BeliefID:      beliefID,
			FingerprintID: fingerprintID,
			Verb:          verb,
			Object:        object,
		})
	}

	log.Printf("DEBUG: Processed %d belief events", len(beliefEvents))

	// Process all rows and match against epinet steps (exact V1 pattern)
	for _, beliefEvent := range beliefEvents {
		hourKey := formatHourKey(time.Now()) // This should be derived from the event timestamp

		// Match this belief data against all epinets and their steps
		for _, epinet := range epinets {
			for stepIndex, step := range epinet.Steps {
				matched := false
				var matchedVerb string

				if step.GateType == "belief" && containsString(step.Values, beliefEvent.Verb) {
					matched = true
					matchedVerb = beliefEvent.Verb
				} else if step.GateType == "identifyAs" && beliefEvent.Object != nil && containsString(step.Values, *beliefEvent.Object) {
					matched = true
					matchedVerb = beliefEvent.Verb
				}

				if matched {
					err = addNodeVisitor(ctx, epinet.ID, hourKey, step, beliefEvent.BeliefID,
						beliefEvent.FingerprintID, stepIndex, contentItems, matchedVerb)
					if err != nil {
						log.Printf("WARNING: Failed to add node visitor: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// getBeliefsForTimeRange gets belief events for a time range (exact V1 pattern)
func getBeliefsForTimeRange(ctx *tenant.Context, start, end time.Time) ([]BeliefEvent, error) {
	query := `
		SELECT hb.belief_id, hb.fingerprint_id, hb.verb, hb.object, hb.updated_at
		FROM heldbeliefs hb
		JOIN beliefs b ON hb.belief_id = b.id
		WHERE hb.updated_at >= ? AND hb.updated_at < ?
		ORDER BY hb.updated_at ASC
	`

	rows, err := ctx.Database.Conn.Query(query, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query beliefs: %w", err)
	}
	defer rows.Close()

	var events []BeliefEvent
	for rows.Next() {
		var event BeliefEvent
		var updatedAtStr string

		err := rows.Scan(&event.BeliefID, &event.FingerprintID, &event.Verb, &event.Object, &updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan belief row: %w", err)
		}

		// Parse the timestamp string
		event.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAtStr)
		if err != nil {
			// Try RFC3339 format as fallback
			event.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse updated_at timestamp: %w", err)
			}
		}

		events = append(events, event)
	}

	return events, nil
}

// containsString checks if a slice contains a string
func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
