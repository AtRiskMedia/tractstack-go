// Package analytics provides action data processing functionality.
package analytics

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// processActionData processes all action-related data in one consolidated query (exact V1 pattern)
func processActionData(ctx *tenant.Context, hourKeys []string, epinets []EpinetConfig,
	analysis *EpinetAnalysis, startTime, endTime time.Time, contentItems map[string]ContentItem,
) error {
	// Prepare query parameters (exact V1 pattern)
	var verbValues []string
	for verb := range analysis.ActionVerbs {
		verbValues = append(verbValues, verb)
	}

	var objectTypes []string
	for objType := range analysis.ActionTypes {
		objectTypes = append(objectTypes, objType)
	}

	// Build the where clause for the query (exact V1 pattern)
	whereClause := "created_at >= ? AND created_at < ?"
	var params []interface{}
	params = append(params, startTime, endTime)

	if len(verbValues) > 0 {
		placeholders := make([]string, len(verbValues))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		whereClause += fmt.Sprintf(" AND verb IN (%s)", strings.Join(placeholders, ","))
		for _, verb := range verbValues {
			params = append(params, verb)
		}
	}

	if len(objectTypes) > 0 {
		placeholders := make([]string, len(objectTypes))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		whereClause += fmt.Sprintf(" AND object_type IN (%s)", strings.Join(placeholders, ","))
		for _, objType := range objectTypes {
			params = append(params, objType)
		}
	}

	// Execute a single efficient query for all actions (exact V1 pattern)
	query := fmt.Sprintf(`
		SELECT 
			strftime('%%Y-%%m-%%d-%%H', created_at, 'utc') as hour_key,
			object_id,
			object_type,
			fingerprint_id,
			verb
		FROM actions
		WHERE %s
	`, whereClause)

	log.Printf("DEBUG: Executing action query with %d parameters", len(params))

	rows, err := ctx.Database.Conn.Query(query, params...)
	if err != nil {
		return fmt.Errorf("failed to query actions: %w", err)
	}
	defer rows.Close()

	actionEvents := make([]ActionEvent, 0)
	for rows.Next() {
		var hourKey, objectID, objectType, fingerprintID, verb string

		err := rows.Scan(&hourKey, &objectID, &objectType, &fingerprintID, &verb)
		if err != nil {
			return fmt.Errorf("failed to scan action row: %w", err)
		}

		// Only process hours we're interested in
		if !containsString(hourKeys, hourKey) {
			continue
		}

		actionEvents = append(actionEvents, ActionEvent{
			ObjectID:      objectID,
			ObjectType:    objectType,
			Verb:          verb,
			FingerprintID: fingerprintID,
		})
	}

	log.Printf("DEBUG: Processed %d action events", len(actionEvents))

	// Process all rows and match against epinet steps (exact V1 pattern)
	for _, actionEvent := range actionEvents {
		hourKey := formatHourKey(time.Now()) // This should be derived from the event timestamp

		// Match this action data against all epinets and their steps
		for _, epinet := range epinets {
			for stepIndex, step := range epinet.Steps {
				if isActionStep(step) && containsString(step.Values, actionEvent.Verb) {
					// Check object type constraint
					if step.ObjectType != "" && step.ObjectType != actionEvent.ObjectType {
						continue
					}

					// Check object ID constraint if specified
					if len(step.ObjectIDs) > 0 && !containsString(step.ObjectIDs, actionEvent.ObjectID) {
						continue
					}

					err = addNodeVisitor(ctx, epinet.ID, hourKey, step, actionEvent.ObjectID,
						actionEvent.FingerprintID, stepIndex, contentItems, actionEvent.Verb)
					if err != nil {
						log.Printf("WARNING: Failed to add node visitor: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// getActionsForTimeRange gets action events for a time range (exact V1 pattern)
func getActionsForTimeRange(ctx *tenant.Context, start, end time.Time) ([]ActionEvent, error) {
	query := `
		SELECT object_id, object_type, verb, fingerprint_id, created_at
		FROM actions
		WHERE created_at >= ? AND created_at < ?
		ORDER BY created_at ASC
	`

	rows, err := ctx.Database.Conn.Query(query, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query actions: %w", err)
	}
	defer rows.Close()

	var events []ActionEvent
	for rows.Next() {
		var event ActionEvent
		var createdAtStr string

		err := rows.Scan(&event.ObjectID, &event.ObjectType, &event.Verb, &event.FingerprintID, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan action row: %w", err)
		}

		// Parse the timestamp string
		event.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			// Try RFC3339 format as fallback
			event.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse created_at timestamp: %w", err)
			}
		}

		events = append(events, event)
	}

	return events, nil
}

// isActionStep checks if a step is an action type (exact V1 pattern)
func isActionStep(step EpinetStep) bool {
	return step.GateType == "commitmentAction" || step.GateType == "conversionAction"
}
