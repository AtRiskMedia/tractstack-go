// Package analytics provides action data processing functionality.
package analytics

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// processActionData processes all action-related data in one consolidated query
func processActionData(ctx *tenant.Context, hourKeys []string, epinets []EpinetConfig,
	analysis *EpinetAnalysis, startTime, endTime time.Time, contentItems map[string]ContentItem,
) error {
	// Prepare query parameters
	var verbValues []string
	for verb := range analysis.ActionVerbs {
		verbValues = append(verbValues, verb)
	}

	var objectTypes []string
	for objType := range analysis.ActionTypes {
		objectTypes = append(objectTypes, objType)
	}

	// Build the where clause
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

	// Execute query
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

	rows, err := ctx.Database.Conn.Query(query, params...)
	if err != nil {
		return fmt.Errorf("failed to query actions: %w", err)
	}
	defer rows.Close()

	actionEvents := make([]ActionEvent, 0)
	for rows.Next() {
		var event ActionEvent
		err := rows.Scan(&event.HourKey, &event.ObjectID, &event.ObjectType, &event.FingerprintID, &event.Verb)
		if err != nil {
			log.Printf("ERROR: Failed to scan action row: %v", err)
			continue
		}

		if !containsString(hourKeys, event.HourKey) {
			continue
		}

		// log.Printf("DEBUG: Scanned action event: hourKey=%s, objectID=%s, objectType=%s, fingerprintID=%s, verb=%s",
		//	event.HourKey, event.ObjectID, event.ObjectType, event.FingerprintID, event.Verb)

		actionEvents = append(actionEvents, event)
	}

	log.Printf("DEBUG: Processed %d action events", len(actionEvents))

	// Process all rows and match against epinet steps
	for _, actionEvent := range actionEvents {
		for _, epinet := range epinets {
			for stepIndex, step := range epinet.Steps {
				if isActionStep(step) && containsString(step.Values, actionEvent.Verb) {
					if step.ObjectType != "" && step.ObjectType != actionEvent.ObjectType {
						continue
					}
					if len(step.ObjectIDs) > 0 && !containsString(step.ObjectIDs, actionEvent.ObjectID) {
						continue
					}
					err = addNodeVisitor(ctx, epinet.ID, actionEvent.HourKey, step, actionEvent.ObjectID,
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

// isActionStep checks if a step is an action type (exact V1 pattern)
func isActionStep(step EpinetStep) bool {
	return step.GateType == "commitmentAction" || step.GateType == "conversionAction"
}
