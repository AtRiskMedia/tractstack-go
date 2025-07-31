// Package content provides epinets repository
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
)

type EpinetRepository struct {
	db    *sql.DB
	cache interfaces.ContentCache
}

func NewEpinetRepository(db *sql.DB, cache interfaces.ContentCache) *EpinetRepository {
	return &EpinetRepository{
		db:    db,
		cache: cache,
	}
}

func (r *EpinetRepository) FindByID(tenantID, id string) (*content.EpinetNode, error) {
	if epinet, found := r.cache.GetEpinet(tenantID, id); found {
		return epinet, nil
	}

	epinet, err := r.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if epinet == nil {
		return nil, nil
	}

	r.cache.SetEpinet(tenantID, epinet)
	return epinet, nil
}

func (r *EpinetRepository) FindAll(tenantID string) ([]*content.EpinetNode, error) {
	if ids, found := r.cache.GetAllEpinetIDs(tenantID); found {
		var epinets []*content.EpinetNode
		var missingIDs []string

		for _, id := range ids {
			if epinet, found := r.cache.GetEpinet(tenantID, id); found {
				epinets = append(epinets, epinet)
			} else {
				missingIDs = append(missingIDs, id)
			}
		}

		if len(missingIDs) > 0 {
			missing, err := r.loadMultipleFromDB(missingIDs)
			if err != nil {
				return nil, err
			}

			for _, epinet := range missing {
				r.cache.SetEpinet(tenantID, epinet)
				epinets = append(epinets, epinet)
			}
		}

		return epinets, nil
	}

	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	epinets, err := r.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	for _, epinet := range epinets {
		r.cache.SetEpinet(tenantID, epinet)
	}

	return epinets, nil
}

func (r *EpinetRepository) Store(tenantID string, epinet *content.EpinetNode) error {
	stepsJSON, _ := json.Marshal(epinet.Steps)

	query := `INSERT INTO epinets (id, title, options_payload) VALUES (?, ?, ?)`

	_, err := r.db.Exec(query, epinet.ID, epinet.Title, string(stepsJSON))
	if err != nil {
		return fmt.Errorf("failed to insert epinet: %w", err)
	}

	r.cache.SetEpinet(tenantID, epinet)
	return nil
}

func (r *EpinetRepository) Update(tenantID string, epinet *content.EpinetNode) error {
	stepsJSON, _ := json.Marshal(epinet.Steps)

	query := `UPDATE epinets SET title = ?, options_payload = ? WHERE id = ?`

	_, err := r.db.Exec(query, epinet.Title, string(stepsJSON), epinet.ID)
	if err != nil {
		return fmt.Errorf("failed to update epinet: %w", err)
	}

	r.cache.SetEpinet(tenantID, epinet)
	return nil
}

func (r *EpinetRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM epinets WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete epinet: %w", err)
	}

	r.cache.InvalidateContentCache(tenantID)
	return nil
}

func (r *EpinetRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM epinets ORDER BY id`

	rows, err := r.db.Query(query)
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

func (r *EpinetRepository) loadFromDB(id string) (*content.EpinetNode, error) {
	query := `SELECT id, title, options_payload FROM epinets WHERE id = ?`

	row := r.db.QueryRow(query, id)

	var epinet content.EpinetNode
	var optionsPayloadStr string

	err := row.Scan(&epinet.ID, &epinet.Title, &optionsPayloadStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load epinet %s: %w", id, err)
	}

	if err := r.parseOptionsPayload(&epinet, optionsPayloadStr); err != nil {
		return nil, fmt.Errorf("failed to parse epinet options: %w", err)
	}

	epinet.NodeType = "Epinet"

	return &epinet, nil
}

func (r *EpinetRepository) loadMultipleFromDB(ids []string) ([]*content.EpinetNode, error) {
	if len(ids) == 0 {
		return []*content.EpinetNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, options_payload FROM epinets WHERE id IN (` +
		strings.Join(placeholders, ",") + `) ORDER BY id`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query epinets: %w", err)
	}
	defer rows.Close()

	var epinets []*content.EpinetNode
	for rows.Next() {
		var epinet content.EpinetNode
		var optionsPayloadStr string

		err := rows.Scan(&epinet.ID, &epinet.Title, &optionsPayloadStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan epinet row: %w", err)
		}

		if err := r.parseOptionsPayload(&epinet, optionsPayloadStr); err != nil {
			continue // Skip malformed records
		}

		epinet.NodeType = "Epinet"
		epinets = append(epinets, &epinet)
	}

	return epinets, nil
}

func (r *EpinetRepository) parseOptionsPayload(epinet *content.EpinetNode, optionsPayloadStr string) error {
	if optionsPayloadStr == "" {
		return nil
	}

	var options interface{}
	if err := json.Unmarshal([]byte(optionsPayloadStr), &options); err != nil {
		return fmt.Errorf("failed to parse options_payload: %w", err)
	}

	if optionsArray, ok := options.([]interface{}); ok {
		steps := make([]*content.EpinetStep, len(optionsArray))
		for i, stepInterface := range optionsArray {
			if stepMap, ok := stepInterface.(map[string]interface{}); ok {
				step := &content.EpinetStep{}

				if gateType, ok := stepMap["gateType"].(string); ok {
					step.GateType = gateType
				}
				if title, ok := stepMap["title"].(string); ok {
					step.Title = title
				}
				if values, ok := stepMap["values"].([]interface{}); ok {
					step.Values = make([]string, len(values))
					for j, v := range values {
						if str, ok := v.(string); ok {
							step.Values[j] = str
						}
					}
				}
				if objectType, ok := stepMap["objectType"].(string); ok {
					step.ObjectType = &objectType
				}
				if objectIds, ok := stepMap["objectIds"].([]interface{}); ok {
					step.ObjectIDs = make([]string, len(objectIds))
					for j, id := range objectIds {
						if str, ok := id.(string); ok {
							step.ObjectIDs[j] = str
						}
					}
				}

				steps[i] = step
			}
		}
		epinet.Steps = steps
	} else if optionsMap, ok := options.(map[string]interface{}); ok {
		if promoted, ok := optionsMap["promoted"].(bool); ok {
			epinet.Promoted = promoted
		}
		if stepsData, ok := optionsMap["steps"].([]interface{}); ok {
			steps := make([]*content.EpinetStep, len(stepsData))
			for i, stepInterface := range stepsData {
				if stepMap, ok := stepInterface.(map[string]interface{}); ok {
					step := &content.EpinetStep{}

					if gateType, ok := stepMap["gateType"].(string); ok {
						step.GateType = gateType
					}
					if title, ok := stepMap["title"].(string); ok {
						step.Title = title
					}
					if values, ok := stepMap["values"].([]interface{}); ok {
						step.Values = make([]string, len(values))
						for j, v := range values {
							if str, ok := v.(string); ok {
								step.Values[j] = str
							}
						}
					}
					if objectType, ok := stepMap["objectType"].(string); ok {
						step.ObjectType = &objectType
					}
					if objectIds, ok := stepMap["objectIds"].([]interface{}); ok {
						step.ObjectIDs = make([]string, len(objectIds))
						for j, id := range objectIds {
							if str, ok := id.(string); ok {
								step.ObjectIDs[j] = str
							}
						}
					}

					steps[i] = step
				}
			}
			epinet.Steps = steps
		}
	}

	return nil
}

// FindByIDs returns multiple epinets by IDs (cache-first with bulk loading)
func (r *EpinetRepository) FindByIDs(tenantID string, ids []string) ([]*content.EpinetNode, error) {
	var result []*content.EpinetNode
	var missingIDs []string

	for _, id := range ids {
		if epinet, found := r.cache.GetEpinet(tenantID, id); found {
			result = append(result, epinet)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingEpinets, err := r.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, epinet := range missingEpinets {
			r.cache.SetEpinet(tenantID, epinet)
			result = append(result, epinet)
		}
	}

	return result, nil
}
