// Package content provides storyfragments
package content

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// StoryFragmentRowData represents raw database structure
type StoryFragmentRowData struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Slug             string  `json:"slug"`
	TractStackID     string  `json:"tractstack_id"`
	MenuID           *string `json:"menu_id,omitempty"`
	TailwindBgColour *string `json:"tailwind_background_colour,omitempty"`
	SocialImagePath  *string `json:"social_image_path,omitempty"`
	Created          string  `json:"created"`
	Changed          *string `json:"changed,omitempty"`
}

// StoryFragmentService handles cache-first storyfragment operations
type StoryFragmentService struct {
	ctx *tenant.Context
}

// NewStoryFragmentService creates a cache-first storyfragment service
func NewStoryFragmentService(ctx *tenant.Context, _ any) *StoryFragmentService {
	return &StoryFragmentService{ctx: ctx}
}

// GetAllIDs returns all storyfragment IDs (cache-first)
func (sfs *StoryFragmentService) GetAllIDs() ([]string, error) {
	if ids, found := cache.GetGlobalManager().GetAllStoryFragmentIDs(sfs.ctx.TenantID); found {
		return ids, nil
	}

	ids, err := sfs.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	storyFragments, err := sfs.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	for _, storyFragment := range storyFragments {
		cache.GetGlobalManager().SetStoryFragment(sfs.ctx.TenantID, storyFragment)
	}

	return ids, nil
}

// GetByID returns a storyfragment by ID (cache-first)
func (sfs *StoryFragmentService) GetByID(id string) (*models.StoryFragmentNode, error) {
	if storyFragment, found := cache.GetGlobalManager().GetStoryFragment(sfs.ctx.TenantID, id); found {
		return storyFragment, nil
	}

	storyFragment, err := sfs.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if storyFragment == nil {
		return nil, nil
	}

	cache.GetGlobalManager().SetStoryFragment(sfs.ctx.TenantID, storyFragment)
	return storyFragment, nil
}

// GetByIDs returns multiple storyfragments by IDs (cache-first with bulk loading)
func (sfs *StoryFragmentService) GetByIDs(ids []string) ([]*models.StoryFragmentNode, error) {
	var result []*models.StoryFragmentNode
	var missingIDs []string

	for _, id := range ids {
		if storyFragment, found := cache.GetGlobalManager().GetStoryFragment(sfs.ctx.TenantID, id); found {
			result = append(result, storyFragment)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingStoryFragments, err := sfs.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, storyFragment := range missingStoryFragments {
			cache.GetGlobalManager().SetStoryFragment(sfs.ctx.TenantID, storyFragment)
			result = append(result, storyFragment)
		}
	}

	return result, nil
}

// GetBySlug returns a storyfragment by slug (cache-first)
func (sfs *StoryFragmentService) GetBySlug(slug string) (*models.StoryFragmentNode, error) {
	if storyFragment, found := cache.GetGlobalManager().GetStoryFragmentBySlug(sfs.ctx.TenantID, slug); found {
		return storyFragment, nil
	}

	id, err := sfs.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}

	return sfs.GetByID(id)
}

// Private database loading methods

func (sfs *StoryFragmentService) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM storyfragments ORDER BY title`
	rows, err := sfs.ctx.Database.Conn.Query(query)
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

func (sfs *StoryFragmentService) loadFromDB(id string) (*models.StoryFragmentNode, error) {
	storyFragmentRow, err := sfs.getStoryFragmentRowData(id)
	if err != nil || storyFragmentRow == nil {
		return nil, err
	}

	paneIDs, err := sfs.getPaneIDsFromDB(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane IDs: %w", err)
	}

	return sfs.deserializeRowData(storyFragmentRow, paneIDs)
}

func (sfs *StoryFragmentService) loadMultipleFromDB(ids []string) ([]*models.StoryFragmentNode, error) {
	if len(ids) == 0 {
		return []*models.StoryFragmentNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, tractstack_id, menu_id, tailwind_background_colour, social_image_path, created, changed 
          FROM storyfragments WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := sfs.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query storyfragments: %w", err)
	}
	defer rows.Close()

	var storyFragments []*models.StoryFragmentNode
	var storyFragmentRows []*StoryFragmentRowData

	for rows.Next() {
		var storyFragmentRow StoryFragmentRowData
		var menuID, tailwindBgColour, socialImagePath, changed sql.NullString

		err := rows.Scan(&storyFragmentRow.ID, &storyFragmentRow.Title, &storyFragmentRow.Slug,
			&storyFragmentRow.TractStackID, &menuID, &tailwindBgColour, &socialImagePath,
			&storyFragmentRow.Created, &changed)
		if err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment: %w", err)
		}

		if menuID.Valid {
			storyFragmentRow.MenuID = &menuID.String
		}
		if tailwindBgColour.Valid {
			storyFragmentRow.TailwindBgColour = &tailwindBgColour.String
		}
		if socialImagePath.Valid {
			storyFragmentRow.SocialImagePath = &socialImagePath.String
		}
		if changed.Valid {
			storyFragmentRow.Changed = &changed.String
		}

		storyFragmentRows = append(storyFragmentRows, &storyFragmentRow)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	paneRelationships, err := sfs.getBulkPaneIDsFromDB(ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane relationships: %w", err)
	}

	for _, storyFragmentRow := range storyFragmentRows {
		paneIDs := paneRelationships[storyFragmentRow.ID]
		storyFragmentNode, err := sfs.deserializeRowData(storyFragmentRow, paneIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize storyfragment %s: %w", storyFragmentRow.ID, err)
		}
		storyFragments = append(storyFragments, storyFragmentNode)
	}

	return storyFragments, nil
}

func (sfs *StoryFragmentService) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM storyfragments WHERE slug = ? LIMIT 1`
	var id string
	err := sfs.ctx.Database.Conn.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (sfs *StoryFragmentService) getStoryFragmentRowData(id string) (*StoryFragmentRowData, error) {
	query := `SELECT id, title, slug, tractstack_id, menu_id, tailwind_background_colour, social_image_path, created, changed FROM storyfragments WHERE id = ?`

	row := sfs.ctx.Database.Conn.QueryRow(query, id)
	var storyFragmentRow StoryFragmentRowData
	var menuID, tailwindBgColour, socialImagePath, changed sql.NullString

	err := row.Scan(&storyFragmentRow.ID, &storyFragmentRow.Title, &storyFragmentRow.Slug,
		&storyFragmentRow.TractStackID, &menuID, &tailwindBgColour, &socialImagePath,
		&storyFragmentRow.Created, &changed)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan storyfragment: %w", err)
	}

	if menuID.Valid {
		storyFragmentRow.MenuID = &menuID.String
	}
	if tailwindBgColour.Valid {
		storyFragmentRow.TailwindBgColour = &tailwindBgColour.String
	}
	if socialImagePath.Valid {
		storyFragmentRow.SocialImagePath = &socialImagePath.String
	}
	if changed.Valid {
		storyFragmentRow.Changed = &changed.String
	}

	return &storyFragmentRow, nil
}

func (sfs *StoryFragmentService) getPaneIDsFromDB(storyFragmentID string) ([]string, error) {
	query := `SELECT pane_id FROM storyfragment_panes WHERE storyfragment_id = ? ORDER BY weight`
	rows, err := sfs.ctx.Database.Conn.Query(query, storyFragmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paneIDs []string
	for rows.Next() {
		var paneID string
		if err := rows.Scan(&paneID); err != nil {
			return nil, err
		}
		paneIDs = append(paneIDs, paneID)
	}
	return paneIDs, rows.Err()
}

func (sfs *StoryFragmentService) getBulkPaneIDsFromDB(storyFragmentIDs []string) (map[string][]string, error) {
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
          WHERE storyfragment_id IN (` + strings.Join(placeholders, ",") + `) ORDER BY storyfragment_id, weight`

	rows, err := sfs.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paneRelationships := make(map[string][]string)
	for rows.Next() {
		var storyFragmentID, paneID string
		if err := rows.Scan(&storyFragmentID, &paneID); err != nil {
			return nil, err
		}
		paneRelationships[storyFragmentID] = append(paneRelationships[storyFragmentID], paneID)
	}

	return paneRelationships, rows.Err()
}

func (sfs *StoryFragmentService) loadByTractStackFromDB(tractStackID string) ([]*models.StoryFragmentNode, error) {
	query := `SELECT id, title, slug, tractstack_id, menu_id, tailwind_background_colour, social_image_path, created, changed 
          FROM storyfragments WHERE tractstack_id = ? ORDER BY title`

	rows, err := sfs.ctx.Database.Conn.Query(query, tractStackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var storyFragments []*models.StoryFragmentNode

	for rows.Next() {
		var storyFragmentRow StoryFragmentRowData
		var menuID, tailwindBgColour, socialImagePath, changed sql.NullString

		err := rows.Scan(&storyFragmentRow.ID, &storyFragmentRow.Title, &storyFragmentRow.Slug,
			&storyFragmentRow.TractStackID, &menuID, &tailwindBgColour, &socialImagePath,
			&storyFragmentRow.Created, &changed)
		if err != nil {
			return nil, err
		}

		if menuID.Valid {
			storyFragmentRow.MenuID = &menuID.String
		}
		if tailwindBgColour.Valid {
			storyFragmentRow.TailwindBgColour = &tailwindBgColour.String
		}
		if socialImagePath.Valid {
			storyFragmentRow.SocialImagePath = &socialImagePath.String
		}
		if changed.Valid {
			storyFragmentRow.Changed = &changed.String
		}

		paneIDs, err := sfs.getPaneIDsFromDB(storyFragmentRow.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get pane IDs for storyfragment %s: %w", storyFragmentRow.ID, err)
		}

		storyFragmentNode, err := sfs.deserializeRowData(&storyFragmentRow, paneIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize storyfragment %s: %w", storyFragmentRow.ID, err)
		}

		storyFragments = append(storyFragments, storyFragmentNode)
	}

	return storyFragments, rows.Err()
}

func (sfs *StoryFragmentService) deserializeRowData(storyFragmentRow *StoryFragmentRowData, paneIDs []string) (*models.StoryFragmentNode, error) {
	created, err := time.Parse(time.RFC3339, storyFragmentRow.Created)
	if err != nil {
		created = time.Now()
	}

	var changed *time.Time
	if storyFragmentRow.Changed != nil {
		if parsedChanged, err := time.Parse(time.RFC3339, *storyFragmentRow.Changed); err == nil {
			changed = &parsedChanged
		}
	}

	storyFragmentNode := &models.StoryFragmentNode{
		ID:           storyFragmentRow.ID,
		Title:        storyFragmentRow.Title,
		Slug:         storyFragmentRow.Slug,
		TractStackID: storyFragmentRow.TractStackID,
		PaneIDs:      paneIDs,
		Created:      created,
		Changed:      changed,
	}

	if storyFragmentRow.MenuID != nil {
		storyFragmentNode.MenuID = storyFragmentRow.MenuID
	}
	if storyFragmentRow.TailwindBgColour != nil {
		storyFragmentNode.TailwindBgColour = storyFragmentRow.TailwindBgColour
	}
	if storyFragmentRow.SocialImagePath != nil {
		storyFragmentNode.SocialImagePath = storyFragmentRow.SocialImagePath
	}

	return storyFragmentNode, nil
}
