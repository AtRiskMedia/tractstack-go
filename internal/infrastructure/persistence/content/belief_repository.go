// Package content provides beliefs repository
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
)

type BeliefRepository struct {
	db    *sql.DB
	cache interfaces.ContentCache
}

func NewBeliefRepository(db *sql.DB, cache interfaces.ContentCache) *BeliefRepository {
	return &BeliefRepository{
		db:    db,
		cache: cache,
	}
}

func (r *BeliefRepository) FindByID(tenantID, id string) (*content.BeliefNode, error) {
	if belief, found := r.cache.GetBelief(tenantID, id); found {
		return belief, nil
	}

	belief, err := r.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if belief == nil {
		return nil, nil
	}

	r.cache.SetBelief(tenantID, belief)
	return belief, nil
}

func (r *BeliefRepository) FindBySlug(tenantID, slug string) (*content.BeliefNode, error) {
	id, err := r.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}

	return r.FindByID(tenantID, id)
}

// FindAll retrieves all beliefs for a tenant, employing a cache-first strategy.
func (r *BeliefRepository) FindAll(tenantID string) ([]*content.BeliefNode, error) {
	// 1. Check cache for the master list of IDs first.
	if ids, found := r.cache.GetAllBeliefIDs(tenantID); found {
		return r.FindByIDs(tenantID, ids)
	}

	// --- CACHE MISS FALLBACK ---
	// 2. Load all IDs from the database.
	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.BeliefNode{}, nil
	}

	// 3. Set the master ID list in the cache immediately.
	r.cache.SetAllBeliefIDs(tenantID, ids)

	// 4. Use the robust FindByIDs method to load the actual objects.
	return r.FindByIDs(tenantID, ids)
}

func (r *BeliefRepository) FindByIDs(tenantID string, ids []string) ([]*content.BeliefNode, error) {
	var result []*content.BeliefNode
	var missingIDs []string

	for _, id := range ids {
		if belief, found := r.cache.GetBelief(tenantID, id); found {
			result = append(result, belief)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingBeliefs, err := r.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, belief := range missingBeliefs {
			r.cache.SetBelief(tenantID, belief)
			result = append(result, belief)
		}
	}

	return result, nil
}

func (r *BeliefRepository) FindIDBySlug(tenantID, slug string) (string, error) {
	if id, found := r.cache.GetContentBySlug(tenantID, "belief:"+slug); found {
		return id, nil
	}
	id, err := r.getIDBySlugFromDB(slug)
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", nil
	}
	// Trigger a full load to populate the main object cache for future GetByID calls.
	_, _ = r.FindByID(tenantID, id)
	return id, nil
}

func (r *BeliefRepository) Store(tenantID string, belief *content.BeliefNode) error {
	customValuesJSON, _ := json.Marshal(belief.CustomValues)

	query := `INSERT INTO beliefs (id, title, slug, scale, custom_values) VALUES (?, ?, ?, ?, ?)`

	_, err := r.db.Exec(query, belief.ID, belief.Title, belief.Slug, belief.Scale, string(customValuesJSON))
	if err != nil {
		return fmt.Errorf("failed to insert belief: %w", err)
	}

	r.cache.SetBelief(tenantID, belief)
	return nil
}

func (r *BeliefRepository) Update(tenantID string, belief *content.BeliefNode) error {
	customValuesJSON, _ := json.Marshal(belief.CustomValues)

	query := `UPDATE beliefs SET title = ?, slug = ?, scale = ?, custom_values = ? WHERE id = ?`

	_, err := r.db.Exec(query, belief.Title, belief.Slug, belief.Scale, string(customValuesJSON), belief.ID)
	if err != nil {
		return fmt.Errorf("failed to update belief: %w", err)
	}

	r.cache.SetBelief(tenantID, belief)
	return nil
}

func (r *BeliefRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM beliefs WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete belief: %w", err)
	}

	r.cache.InvalidateContentCache(tenantID)
	return nil
}

func (r *BeliefRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM beliefs ORDER BY title`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query beliefs: %w", err)
	}
	defer rows.Close()

	var beliefIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan belief ID: %w", err)
		}
		beliefIDs = append(beliefIDs, id)
	}

	return beliefIDs, rows.Err()
}

// ... (rest of the helper methods: loadFromDB, loadMultipleFromDB, etc. remain the same)
func (r *BeliefRepository) loadFromDB(id string) (*content.BeliefNode, error) {
	query := `SELECT id, title, slug, scale, custom_values FROM beliefs WHERE id = ?`

	row := r.db.QueryRow(query, id)

	var belief content.BeliefNode
	var customValuesStr sql.NullString

	err := row.Scan(&belief.ID, &belief.Title, &belief.Slug, &belief.Scale, &customValuesStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan belief: %w", err)
	}

	if customValuesStr.Valid && customValuesStr.String != "" {
		if err := json.Unmarshal([]byte(customValuesStr.String), &belief.CustomValues); err != nil {
			// Try to parse as a simple comma-separated string as a fallback
			belief.CustomValues = strings.Split(customValuesStr.String, ",")
		}
	}

	belief.NodeType = "Belief"

	return &belief, nil
}

func (r *BeliefRepository) loadMultipleFromDB(ids []string) ([]*content.BeliefNode, error) {
	if len(ids) == 0 {
		return []*content.BeliefNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, scale, custom_values 
              FROM beliefs WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query beliefs: %w", err)
	}
	defer rows.Close()

	var beliefs []*content.BeliefNode
	for rows.Next() {
		var belief content.BeliefNode
		var customValuesStr sql.NullString

		err := rows.Scan(&belief.ID, &belief.Title, &belief.Slug, &belief.Scale, &customValuesStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan belief: %w", err)
		}

		if customValuesStr.Valid && customValuesStr.String != "" {
			if err := json.Unmarshal([]byte(customValuesStr.String), &belief.CustomValues); err != nil {
				belief.CustomValues = strings.Split(customValuesStr.String, ",")
			}
		}

		belief.NodeType = "Belief"
		beliefs = append(beliefs, &belief)
	}

	return beliefs, rows.Err()
}

func (r *BeliefRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM beliefs WHERE slug = ? LIMIT 1`

	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query belief by slug: %w", err)
	}

	return id, nil
}
