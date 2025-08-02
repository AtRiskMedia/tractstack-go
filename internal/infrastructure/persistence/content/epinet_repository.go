// Package content provides epinets repository
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

type EpinetRepository struct {
	db     *sql.DB
	cache  interfaces.ContentCache
	logger *logging.ChanneledLogger
}

func NewEpinetRepository(db *sql.DB, cache interfaces.ContentCache, logger *logging.ChanneledLogger) *EpinetRepository {
	return &EpinetRepository{
		db:     db,
		cache:  cache,
		logger: logger,
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

// FindAll retrieves all epinets for a tenant, employing a cache-first strategy.
func (r *EpinetRepository) FindAll(tenantID string) ([]*content.EpinetNode, error) {
	// 1. Check cache for the master list of IDs first.
	if ids, found := r.cache.GetAllEpinetIDs(tenantID); found {
		return r.FindByIDs(tenantID, ids)
	}

	// --- CACHE MISS FALLBACK ---
	// 2. Load all IDs from the database.
	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.EpinetNode{}, nil
	}

	// 3. Set the master ID list in the cache immediately.
	r.cache.SetAllEpinetIDs(tenantID, ids)

	// 4. Use the robust FindByIDs method to load the actual objects.
	return r.FindByIDs(tenantID, ids)
}

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

func (r *EpinetRepository) Store(tenantID string, epinet *content.EpinetNode) error {
	stepsJSON, _ := json.Marshal(epinet.Steps)

	query := `INSERT INTO epinets (id, title, options_payload) VALUES (?, ?, ?)`

	start := time.Now()
	r.logger.Database().Debug("Executing epinet insert", "id", epinet.ID)

	_, err := r.db.Exec(query, epinet.ID, epinet.Title, string(stepsJSON))
	if err != nil {
		r.logger.Database().Error("Epinet insert failed", "error", err.Error(), "id", epinet.ID)
		return fmt.Errorf("failed to insert epinet: %w", err)
	}

	r.logger.Database().Info("Epinet insert completed", "id", epinet.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetEpinet(tenantID, epinet)
	return nil
}

func (r *EpinetRepository) Update(tenantID string, epinet *content.EpinetNode) error {
	stepsJSON, _ := json.Marshal(epinet.Steps)

	query := `UPDATE epinets SET title = ?, options_payload = ? WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing epinet update", "id", epinet.ID)

	_, err := r.db.Exec(query, epinet.Title, string(stepsJSON), epinet.ID)
	if err != nil {
		r.logger.Database().Error("Epinet update failed", "error", err.Error(), "id", epinet.ID)
		return fmt.Errorf("failed to update epinet: %w", err)
	}

	r.logger.Database().Info("Epinet update completed", "id", epinet.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetEpinet(tenantID, epinet)
	return nil
}

func (r *EpinetRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM epinets WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing epinet delete", "id", id)

	_, err := r.db.Exec(query, id)
	if err != nil {
		r.logger.Database().Error("Epinet delete failed", "error", err.Error(), "id", id)
		return fmt.Errorf("failed to delete epinet: %w", err)
	}

	r.logger.Database().Info("Epinet delete completed", "id", id, "duration", time.Since(start))
	r.cache.InvalidateContentCache(tenantID)
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	return nil
}

func (r *EpinetRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM epinets ORDER BY id`

	start := time.Now()
	r.logger.Database().Debug("Loading all epinet IDs from database")

	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Database().Error("Failed to query epinet IDs", "error", err.Error())
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

	r.logger.Database().Info("Loaded epinet IDs from database", "count", len(ids), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return ids, rows.Err()
}

func (r *EpinetRepository) loadFromDB(id string) (*content.EpinetNode, error) {
	query := `SELECT id, title, options_payload FROM epinets WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading epinet from database", "id", id)

	row := r.db.QueryRow(query, id)

	var epinet content.EpinetNode
	var optionsPayloadStr string

	err := row.Scan(&epinet.ID, &epinet.Title, &optionsPayloadStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to scan epinet", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to load epinet %s: %w", id, err)
	}

	if err := r.parseOptionsPayload(&epinet, optionsPayloadStr); err != nil {
		r.logger.Database().Error("Failed to parse epinet options payload", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to parse epinet options: %w", err)
	}

	epinet.NodeType = "Epinet"

	r.logger.Database().Info("Epinet loaded from database", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
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

	start := time.Now()
	r.logger.Database().Debug("Loading multiple epinets from database", "count", len(ids))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query multiple epinets", "error", err.Error(), "count", len(ids))
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

	r.logger.Database().Info("Multiple epinets loaded from database", "requested", len(ids), "loaded", len(epinets), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return epinets, rows.Err()
}

func (r *EpinetRepository) parseOptionsPayload(epinet *content.EpinetNode, optionsPayloadStr string) error {
	if optionsPayloadStr == "" {
		return nil
	}

	var options any
	if err := json.Unmarshal([]byte(optionsPayloadStr), &options); err != nil {
		return fmt.Errorf("failed to parse options_payload: %w", err)
	}

	if optionsArray, ok := options.([]any); ok {
		steps := make([]*content.EpinetStep, len(optionsArray))
		for i, stepInterface := range optionsArray {
			if stepMap, ok := stepInterface.(map[string]any); ok {
				step := &content.EpinetStep{}

				if gateType, ok := stepMap["gateType"].(string); ok {
					step.GateType = gateType
				}
				if title, ok := stepMap["title"].(string); ok {
					step.Title = title
				}
				if values, ok := stepMap["values"].([]any); ok {
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
				if objectIds, ok := stepMap["objectIds"].([]any); ok {
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
	} else if optionsMap, ok := options.(map[string]any); ok {
		if promoted, ok := optionsMap["promoted"].(bool); ok {
			epinet.Promoted = promoted
		}
		if stepsData, ok := optionsMap["steps"].([]any); ok {
			steps := make([]*content.EpinetStep, len(stepsData))
			for i, stepInterface := range stepsData {
				if stepMap, ok := stepInterface.(map[string]any); ok {
					step := &content.EpinetStep{}

					if gateType, ok := stepMap["gateType"].(string); ok {
						step.GateType = gateType
					}
					if title, ok := stepMap["title"].(string); ok {
						step.Title = title
					}
					if values, ok := stepMap["values"].([]any); ok {
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
					if objectIds, ok := stepMap["objectIds"].([]any); ok {
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
