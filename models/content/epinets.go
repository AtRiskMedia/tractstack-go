// Package content provides epinets
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// EpinetRowData represents raw database structure
type EpinetRowData struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	OptionsPayload string `json:"options_payload"`
}

// EpinetService handles cache-first epinet operations
type EpinetService struct {
	ctx *tenant.Context
}

// NewEpinetService creates a cache-first epinet service
func NewEpinetService(ctx *tenant.Context, _ any) *EpinetService {
	// Ignore the cache manager parameter - we use the global instance directly
	return &EpinetService{
		ctx: ctx,
	}
}

// GetAllIDs returns all epinet IDs (cache-first)
func (es *EpinetService) GetAllIDs() ([]string, error) {
	// Check cache first
	if ids, found := cache.GetGlobalManager().GetAllEpinetIDs(es.ctx.TenantID); found {
		return ids, nil
	}

	// Cache miss - load from database
	ids, err := es.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	// Load all epinets to populate cache
	epinets, err := es.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	// Populate cache
	for _, epinet := range epinets {
		cache.GetGlobalManager().SetEpinet(es.ctx.TenantID, epinet)
	}

	return ids, nil
}

// GetByID returns an epinet by ID (cache-first)
func (es *EpinetService) GetByID(id string) (*models.EpinetNode, error) {
	// Check cache first
	if epinet, found := cache.GetGlobalManager().GetEpinet(es.ctx.TenantID, id); found {
		return epinet, nil
	}

	// Cache miss - load from database
	epinet, err := es.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if epinet == nil {
		return nil, nil // Not found
	}

	// Populate cache
	cache.GetGlobalManager().SetEpinet(es.ctx.TenantID, epinet)

	return epinet, nil
}

// GetByIDs returns multiple epinets by IDs (cache-first with bulk loading)
func (es *EpinetService) GetByIDs(ids []string) ([]*models.EpinetNode, error) {
	var result []*models.EpinetNode
	var missingIDs []string

	// Check cache for each ID
	for _, id := range ids {
		if epinet, found := cache.GetGlobalManager().GetEpinet(es.ctx.TenantID, id); found {
			result = append(result, epinet)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// If we have cache misses, bulk load from database
	if len(missingIDs) > 0 {
		missingEpinets, err := es.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		// Add to cache and result
		for _, epinet := range missingEpinets {
			cache.GetGlobalManager().SetEpinet(es.ctx.TenantID, epinet)
			result = append(result, epinet)
		}
	}

	return result, nil
}

// Private database loading methods

// loadAllIDsFromDB fetches all epinet IDs from database
func (es *EpinetService) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM epinets ORDER BY id`

	rows, err := es.ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query epinet IDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan epinet ID: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// loadFromDB loads a single epinet from database
func (es *EpinetService) loadFromDB(id string) (*models.EpinetNode, error) {
	query := `SELECT id, title, options_payload FROM epinets WHERE id = ?`

	var rowData EpinetRowData
	err := es.ctx.Database.Conn.QueryRow(query, id).Scan(
		&rowData.ID,
		&rowData.Title,
		&rowData.OptionsPayload,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to load epinet %s: %w", id, err)
	}

	return es.deserializeRowData(rowData)
}

// loadMultipleFromDB loads multiple epinets from database
func (es *EpinetService) loadMultipleFromDB(ids []string) ([]*models.EpinetNode, error) {
	if len(ids) == 0 {
		return []*models.EpinetNode{}, nil
	}

	// Build query with placeholders
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT id, title, options_payload FROM epinets WHERE id IN (%s) ORDER BY id`,
		strings.Join(placeholders, ","),
	)

	rows, err := es.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query epinets: %w", err)
	}
	defer rows.Close()

	var epinets []*models.EpinetNode
	for rows.Next() {
		var rowData EpinetRowData
		err := rows.Scan(
			&rowData.ID,
			&rowData.Title,
			&rowData.OptionsPayload,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan epinet row: %w", err)
		}

		epinet, err := es.deserializeRowData(rowData)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize epinet %s: %w", rowData.ID, err)
		}

		epinets = append(epinets, epinet)
	}

	return epinets, nil
}

// deserializeRowData converts database row to EpinetNode
func (es *EpinetService) deserializeRowData(rowData EpinetRowData) (*models.EpinetNode, error) {
	epinet := &models.EpinetNode{
		ID:       rowData.ID,
		Title:    rowData.Title,
		NodeType: "Epinet",
	}

	// Parse options_payload to extract promoted flag and steps
	if rowData.OptionsPayload != "" {
		var options interface{}
		err := json.Unmarshal([]byte(rowData.OptionsPayload), &options)
		if err != nil {
			return nil, fmt.Errorf("failed to parse options_payload: %w", err)
		}

		// Handle both array format and object format
		if optionsArray, ok := options.([]interface{}); ok {
			// Array format - steps are the array
			steps, err := es.parseStepsArray(optionsArray)
			if err != nil {
				return nil, fmt.Errorf("failed to parse steps array: %w", err)
			}
			epinet.Steps = steps
		} else if optionsObject, ok := options.(map[string]interface{}); ok {
			// Object format - extract promoted and steps
			if promoted, exists := optionsObject["promoted"]; exists {
				if promotedBool, ok := promoted.(bool); ok {
					epinet.Promoted = promotedBool
				}
			}

			if stepsArray, exists := optionsObject["steps"]; exists {
				if stepsSlice, ok := stepsArray.([]interface{}); ok {
					steps, err := es.parseStepsArray(stepsSlice)
					if err != nil {
						return nil, fmt.Errorf("failed to parse steps array: %w", err)
					}
					epinet.Steps = steps
				}
			}
		}
	}

	return epinet, nil
}

// parseStepsArray parses steps from JSON array
func (es *EpinetService) parseStepsArray(stepsArray []interface{}) ([]models.EpinetNodeStep, error) {
	var steps []models.EpinetNodeStep

	for _, stepInterface := range stepsArray {
		stepMap, ok := stepInterface.(map[string]interface{})
		if !ok {
			continue
		}

		step := models.EpinetNodeStep{}

		// Parse gateType
		if gateType, exists := stepMap["gateType"]; exists {
			if gateTypeStr, ok := gateType.(string); ok {
				step.GateType = gateTypeStr
			}
		}

		// Parse title
		if title, exists := stepMap["title"]; exists {
			if titleStr, ok := title.(string); ok {
				step.Title = titleStr
			}
		}

		// Parse values
		if values, exists := stepMap["values"]; exists {
			if valuesArray, ok := values.([]interface{}); ok {
				for _, val := range valuesArray {
					if valStr, ok := val.(string); ok {
						step.Values = append(step.Values, valStr)
					}
				}
			}
		}

		// Parse objectType
		if objectType, exists := stepMap["objectType"]; exists {
			if objectTypeStr, ok := objectType.(string); ok {
				step.ObjectType = &objectTypeStr
			}
		}

		// Parse objectIds
		if objectIDs, exists := stepMap["objectIds"]; exists {
			if objectIDsArray, ok := objectIDs.([]interface{}); ok {
				for _, id := range objectIDsArray {
					if idStr, ok := id.(string); ok {
						step.ObjectIDs = append(step.ObjectIDs, idStr)
					}
				}
			}
		}

		steps = append(steps, step)
	}

	return steps, nil
}
