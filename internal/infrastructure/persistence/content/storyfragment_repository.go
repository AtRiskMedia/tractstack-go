// Package content provides storyfragments repository
package content

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
)

type StoryFragmentRepository struct {
	db    *sql.DB
	cache interfaces.ContentCache
}

func NewStoryFragmentRepository(db *sql.DB, cache interfaces.ContentCache) *StoryFragmentRepository {
	return &StoryFragmentRepository{
		db:    db,
		cache: cache,
	}
}

func (r *StoryFragmentRepository) FindByID(tenantID, id string) (*content.StoryFragmentNode, error) {
	if storyFragment, found := r.cache.GetStoryFragment(tenantID, id); found {
		return storyFragment, nil
	}

	storyFragment, err := r.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if storyFragment == nil {
		return nil, nil
	}

	r.cache.SetStoryFragment(tenantID, storyFragment)
	return storyFragment, nil
}

func (r *StoryFragmentRepository) FindBySlug(tenantID, slug string) (*content.StoryFragmentNode, error) {
	id, err := r.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}

	return r.FindByID(tenantID, id)
}

func (r *StoryFragmentRepository) FindByTractStackID(tenantID, tractStackID string) ([]*content.StoryFragmentNode, error) {
	ids, err := r.getIDsByTractStackFromDB(tractStackID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.StoryFragmentNode{}, nil
	}
	return r.FindByIDs(tenantID, ids)
}

// FindAll retrieves all storyfragments for a tenant, employing a cache-first strategy.
func (r *StoryFragmentRepository) FindAll(tenantID string) ([]*content.StoryFragmentNode, error) {
	// 1. Check cache for the master list of IDs first.
	if ids, found := r.cache.GetAllStoryFragmentIDs(tenantID); found {
		return r.FindByIDs(tenantID, ids)
	}

	// --- CACHE MISS FALLBACK ---
	// 2. Load all IDs from the database.
	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.StoryFragmentNode{}, nil
	}

	// 3. Set the master ID list in the cache immediately.
	r.cache.SetAllStoryFragmentIDs(tenantID, ids)

	// 4. Use the robust FindByIDs method to load the actual objects.
	return r.FindByIDs(tenantID, ids)
}

func (r *StoryFragmentRepository) FindByIDs(tenantID string, ids []string) ([]*content.StoryFragmentNode, error) {
	var result []*content.StoryFragmentNode
	var missingIDs []string

	for _, id := range ids {
		if storyFragment, found := r.cache.GetStoryFragment(tenantID, id); found {
			result = append(result, storyFragment)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingStoryFragments, err := r.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, storyFragment := range missingStoryFragments {
			r.cache.SetStoryFragment(tenantID, storyFragment)
			result = append(result, storyFragment)
		}
	}

	return result, nil
}

func (r *StoryFragmentRepository) Store(tenantID string, storyFragment *content.StoryFragmentNode) error {
	query := `INSERT INTO storyfragments (id, title, slug, tractstack_id, menu_id, tailwind_background_colour, social_image_path, created, changed) 
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.Exec(query, storyFragment.ID, storyFragment.Title, storyFragment.Slug,
		storyFragment.TractStackID, storyFragment.MenuID, storyFragment.TailwindBgColour,
		storyFragment.SocialImagePath, storyFragment.Created, storyFragment.Changed)
	if err != nil {
		return fmt.Errorf("failed to insert storyfragment: %w", err)
	}

	r.cache.SetStoryFragment(tenantID, storyFragment)
	return nil
}

func (r *StoryFragmentRepository) Update(tenantID string, storyFragment *content.StoryFragmentNode) error {
	query := `UPDATE storyfragments SET title = ?, slug = ?, tractstack_id = ?, menu_id = ?, 
              tailwind_background_colour = ?, social_image_path = ?, changed = ? WHERE id = ?`

	_, err := r.db.Exec(query, storyFragment.Title, storyFragment.Slug, storyFragment.TractStackID,
		storyFragment.MenuID, storyFragment.TailwindBgColour, storyFragment.SocialImagePath,
		storyFragment.Changed, storyFragment.ID)
	if err != nil {
		return fmt.Errorf("failed to update storyfragment: %w", err)
	}

	r.cache.SetStoryFragment(tenantID, storyFragment)
	return nil
}

func (r *StoryFragmentRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM storyfragments WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete storyfragment: %w", err)
	}

	r.cache.InvalidateContentCache(tenantID)
	return nil
}

func (r *StoryFragmentRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM storyfragments ORDER BY title`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query storyfragments: %w", err)
	}
	defer rows.Close()

	var storyFragmentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment ID: %w", err)
		}
		storyFragmentIDs = append(storyFragmentIDs, id)
	}

	return storyFragmentIDs, rows.Err()
}

// ... (rest of the helper methods: loadFromDB, loadMultipleFromDB, etc. remain the same)
func (r *StoryFragmentRepository) loadFromDB(id string) (*content.StoryFragmentNode, error) {
	query := `SELECT id, title, slug, tractstack_id, menu_id, tailwind_background_colour, 
              social_image_path, created, changed 
              FROM storyfragments WHERE id = ?`

	row := r.db.QueryRow(query, id)

	var sf content.StoryFragmentNode
	var menuID, tailwindBg, socialImage, changed sql.NullString
	var createdStr string

	err := row.Scan(&sf.ID, &sf.Title, &sf.Slug, &sf.TractStackID, &menuID,
		&tailwindBg, &socialImage, &createdStr, &changed)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan storyfragment: %w", err)
	}

	if created, err := time.Parse("2006-01-02 15:04:05", createdStr); err == nil {
		sf.Created = created
	}
	if changed.Valid {
		if changedTime, err := time.Parse("2006-01-02 15:04:05", changed.String); err == nil {
			sf.Changed = &changedTime
		}
	}

	if menuID.Valid {
		sf.MenuID = &menuID.String
	}
	if tailwindBg.Valid {
		sf.TailwindBgColour = &tailwindBg.String
	}
	if socialImage.Valid {
		sf.SocialImagePath = &socialImage.String
	}

	paneIDs, err := r.getPaneIDsForStoryFragment(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane relationships: %w", err)
	}
	sf.PaneIDs = paneIDs

	sf.NodeType = "StoryFragment"

	return &sf, nil
}

func (r *StoryFragmentRepository) loadMultipleFromDB(ids []string) ([]*content.StoryFragmentNode, error) {
	if len(ids) == 0 {
		return []*content.StoryFragmentNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, tractstack_id, menu_id, tailwind_background_colour, 
              social_image_path, created, changed 
              FROM storyfragments WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query storyfragments: %w", err)
	}
	defer rows.Close()

	var storyFragments []*content.StoryFragmentNode
	var sfIDs []string

	for rows.Next() {
		var sf content.StoryFragmentNode
		var menuID, tailwindBg, socialImage, changed sql.NullString
		var createdStr string

		err := rows.Scan(&sf.ID, &sf.Title, &sf.Slug, &sf.TractStackID, &menuID,
			&tailwindBg, &socialImage, &createdStr, &changed)
		if err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment: %w", err)
		}

		if created, err := time.Parse("2006-01-02 15:04:05", createdStr); err == nil {
			sf.Created = created
		}
		if changed.Valid {
			if changedTime, err := time.Parse("2006-01-02 15:04:05", changed.String); err == nil {
				sf.Changed = &changedTime
			}
		}

		if menuID.Valid {
			sf.MenuID = &menuID.String
		}
		if tailwindBg.Valid {
			sf.TailwindBgColour = &tailwindBg.String
		}
		if socialImage.Valid {
			sf.SocialImagePath = &socialImage.String
		}

		sf.NodeType = "StoryFragment"
		storyFragments = append(storyFragments, &sf)
		sfIDs = append(sfIDs, sf.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	allPaneRelationships, err := r.getAllPaneRelationships(sfIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane relationships: %w", err)
	}

	for _, sf := range storyFragments {
		sf.PaneIDs = allPaneRelationships[sf.ID]
	}

	return storyFragments, nil
}

func (r *StoryFragmentRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM storyfragments WHERE slug = ?`

	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query storyfragment by slug: %w", err)
	}

	return id, nil
}

func (r *StoryFragmentRepository) getIDsByTractStackFromDB(tractStackID string) ([]string, error) {
	query := `SELECT id FROM storyfragments WHERE tractstack_id = ? ORDER BY title`

	rows, err := r.db.Query(query, tractStackID)
	if err != nil {
		return nil, fmt.Errorf("failed to query storyfragments by tractstack: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment ID: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

func (r *StoryFragmentRepository) getPaneIDsForStoryFragment(storyFragmentID string) ([]string, error) {
	query := `SELECT pane_id FROM storyfragment_panes WHERE storyfragment_id = ? ORDER BY weight`

	rows, err := r.db.Query(query, storyFragmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query pane relationships: %w", err)
	}
	defer rows.Close()

	var paneIDs []string
	for rows.Next() {
		var paneID string
		if err := rows.Scan(&paneID); err != nil {
			return nil, fmt.Errorf("failed to scan pane ID: %w", err)
		}
		paneIDs = append(paneIDs, paneID)
	}

	return paneIDs, rows.Err()
}

func (r *StoryFragmentRepository) getAllPaneRelationships(storyFragmentIDs []string) (map[string][]string, error) {
	if len(storyFragmentIDs) == 0 {
		return make(map[string][]string), nil
	}

	placeholders := make([]string, len(storyFragmentIDs))
	args := make([]any, len(storyFragmentIDs))
	for i, id := range storyFragmentIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT storyfragment_id, pane_id FROM storyfragment_panes 
              WHERE storyfragment_id IN (` + strings.Join(placeholders, ",") + `) ORDER BY weight`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query pane relationships: %w", err)
	}
	defer rows.Close()

	relationships := make(map[string][]string)
	for rows.Next() {
		var storyFragmentID, paneID string
		if err := rows.Scan(&storyFragmentID, &paneID); err != nil {
			return nil, fmt.Errorf("failed to scan pane relationship: %w", err)
		}
		relationships[storyFragmentID] = append(relationships[storyFragmentID], paneID)
	}

	return relationships, rows.Err()
}
