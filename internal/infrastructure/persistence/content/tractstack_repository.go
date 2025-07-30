// Package content provides tractstacks repository
package content

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
)

type TractStackRepository struct {
	db    *sql.DB
	cache interfaces.ContentCache
}

func NewTractStackRepository(db *sql.DB, cache interfaces.ContentCache) *TractStackRepository {
	return &TractStackRepository{
		db:    db,
		cache: cache,
	}
}

func (r *TractStackRepository) FindByID(tenantID, id string) (*content.TractStackNode, error) {
	if tractStack, found := r.cache.GetTractStack(tenantID, id); found {
		return tractStack, nil
	}

	tractStack, err := r.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if tractStack == nil {
		return nil, nil
	}

	r.cache.SetTractStack(tenantID, tractStack)
	return tractStack, nil
}

func (r *TractStackRepository) FindBySlug(tenantID, slug string) (*content.TractStackNode, error) {
	id, err := r.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}

	return r.FindByID(tenantID, id)
}

func (r *TractStackRepository) FindAll(tenantID string) ([]*content.TractStackNode, error) {
	if ids, found := r.cache.GetAllTractStackIDs(tenantID); found {
		var tractStacks []*content.TractStackNode
		var missingIDs []string

		for _, id := range ids {
			if tractStack, found := r.cache.GetTractStack(tenantID, id); found {
				tractStacks = append(tractStacks, tractStack)
			} else {
				missingIDs = append(missingIDs, id)
			}
		}

		if len(missingIDs) > 0 {
			missing, err := r.loadMultipleFromDB(missingIDs)
			if err != nil {
				return nil, err
			}

			for _, ts := range missing {
				r.cache.SetTractStack(tenantID, ts)
				tractStacks = append(tractStacks, ts)
			}
		}

		return tractStacks, nil
	}

	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	tractStacks, err := r.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	for _, ts := range tractStacks {
		r.cache.SetTractStack(tenantID, ts)
	}

	return tractStacks, nil
}

func (r *TractStackRepository) FindByIDs(tenantID string, ids []string) ([]*content.TractStackNode, error) {
	var result []*content.TractStackNode
	var missingIDs []string

	for _, id := range ids {
		if tractStack, found := r.cache.GetTractStack(tenantID, id); found {
			result = append(result, tractStack)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingTractStacks, err := r.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, tractStack := range missingTractStacks {
			r.cache.SetTractStack(tenantID, tractStack)
			result = append(result, tractStack)
		}
	}

	return result, nil
}

func (r *TractStackRepository) Store(tenantID string, tractStack *content.TractStackNode) error {
	query := `INSERT INTO tractstacks (id, title, slug, social_image_path) VALUES (?, ?, ?, ?)`

	_, err := r.db.Exec(query, tractStack.ID, tractStack.Title, tractStack.Slug, tractStack.SocialImagePath)
	if err != nil {
		return fmt.Errorf("failed to insert tractstack: %w", err)
	}

	r.cache.SetTractStack(tenantID, tractStack)
	return nil
}

func (r *TractStackRepository) Update(tenantID string, tractStack *content.TractStackNode) error {
	query := `UPDATE tractstacks SET title = ?, slug = ?, social_image_path = ? WHERE id = ?`

	_, err := r.db.Exec(query, tractStack.Title, tractStack.Slug, tractStack.SocialImagePath, tractStack.ID)
	if err != nil {
		return fmt.Errorf("failed to update tractstack: %w", err)
	}

	r.cache.SetTractStack(tenantID, tractStack)
	return nil
}

func (r *TractStackRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM tractstacks WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete tractstack: %w", err)
	}

	r.cache.InvalidateContentCache(tenantID)
	return nil
}

func (r *TractStackRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM tractstacks ORDER BY title`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tractstacks: %w", err)
	}
	defer rows.Close()

	var tractStackIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan tractstack ID: %w", err)
		}
		tractStackIDs = append(tractStackIDs, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return tractStackIDs, nil
}

func (r *TractStackRepository) loadFromDB(id string) (*content.TractStackNode, error) {
	query := `SELECT id, title, slug, social_image_path FROM tractstacks WHERE id = ?`

	row := r.db.QueryRow(query, id)

	var ts content.TractStackNode
	var socialImagePath sql.NullString

	err := row.Scan(&ts.ID, &ts.Title, &ts.Slug, &socialImagePath)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan tractstack: %w", err)
	}

	if socialImagePath.Valid {
		ts.SocialImagePath = &socialImagePath.String
	}

	ts.NodeType = "TractStack"

	return &ts, nil
}

func (r *TractStackRepository) loadMultipleFromDB(ids []string) ([]*content.TractStackNode, error) {
	if len(ids) == 0 {
		return []*content.TractStackNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, social_image_path 
              FROM tractstacks WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tractstacks: %w", err)
	}
	defer rows.Close()

	var tractStacks []*content.TractStackNode
	for rows.Next() {
		var ts content.TractStackNode
		var socialImagePath sql.NullString

		err := rows.Scan(&ts.ID, &ts.Title, &ts.Slug, &socialImagePath)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tractstack: %w", err)
		}

		if socialImagePath.Valid {
			ts.SocialImagePath = &socialImagePath.String
		}

		ts.NodeType = "TractStack"
		tractStacks = append(tractStacks, &ts)
	}

	return tractStacks, rows.Err()
}

func (r *TractStackRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM tractstacks WHERE slug = ? LIMIT 1`

	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get tractstack by slug: %w", err)
	}

	return id, nil
}
