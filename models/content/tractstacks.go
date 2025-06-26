// Package content provides tractstacks
package content

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// TractStackRowData represents raw database structure
type TractStackRowData struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Slug            string  `json:"slug"`
	SocialImagePath *string `json:"social_image_path,omitempty"`
}

// TractStackService handles cache-first tractstack operations
type TractStackService struct {
	ctx *tenant.Context
}

// NewTractStackService creates a cache-first tractstack service
func NewTractStackService(ctx *tenant.Context, _ any) *TractStackService {
	// Ignore the cache manager parameter - we use the global instance directly
	return &TractStackService{
		ctx: ctx,
	}
}

// GetAllIDs returns all tractstack IDs (cache-first)
func (tss *TractStackService) GetAllIDs() ([]string, error) {
	// Check cache first
	if ids, found := cache.GetGlobalManager().GetAllTractStackIDs(tss.ctx.TenantID); found {
		return ids, nil
	}

	// Cache miss - load from database
	ids, err := tss.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	// Load all tractstacks to populate cache
	tractstacks, err := tss.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	// Populate cache
	for _, tractstack := range tractstacks {
		cache.GetGlobalManager().SetTractStack(tss.ctx.TenantID, tractstack)
	}

	return ids, nil
}

// GetByID returns a tractstack by ID (cache-first)
func (tss *TractStackService) GetByID(id string) (*models.TractStackNode, error) {
	// Check cache first
	if tractstack, found := cache.GetGlobalManager().GetTractStack(tss.ctx.TenantID, id); found {
		return tractstack, nil
	}

	// Cache miss - load from database
	tractstack, err := tss.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if tractstack == nil {
		return nil, nil // Not found
	}

	// Populate cache
	cache.GetGlobalManager().SetTractStack(tss.ctx.TenantID, tractstack)

	return tractstack, nil
}

// GetByIDs returns multiple tractstacks by IDs (cache-first with bulk loading)
func (tss *TractStackService) GetByIDs(ids []string) ([]*models.TractStackNode, error) {
	var result []*models.TractStackNode
	var missingIDs []string

	// Check cache for each ID
	for _, id := range ids {
		if tractstack, found := cache.GetGlobalManager().GetTractStack(tss.ctx.TenantID, id); found {
			result = append(result, tractstack)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// If we have cache misses, bulk load from database
	if len(missingIDs) > 0 {
		missingTractStacks, err := tss.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		// Add to cache and result
		for _, tractstack := range missingTractStacks {
			cache.GetGlobalManager().SetTractStack(tss.ctx.TenantID, tractstack)
			result = append(result, tractstack)
		}
	}

	return result, nil
}

// GetBySlug returns a tractstack by slug (cache-first)
func (tss *TractStackService) GetBySlug(slug string) (*models.TractStackNode, error) {
	// Check cache first
	if tractstack, found := cache.GetGlobalManager().GetTractStackBySlug(tss.ctx.TenantID, slug); found {
		return tractstack, nil
	}

	// Cache miss - get ID from database, then load tractstack
	id, err := tss.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil // Not found
	}

	// Load full tractstack (this will cache it)
	return tss.GetByID(id)
}

// Private database loading methods

// loadAllIDsFromDB fetches all tractstack IDs from database
func (tss *TractStackService) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM tractstacks ORDER BY title`

	rows, err := tss.ctx.Database.Conn.Query(query)
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

// loadFromDB loads a complete tractstack from database
func (tss *TractStackService) loadFromDB(id string) (*models.TractStackNode, error) {
	// Get tractstack row data
	tractStackRow, err := tss.getTractStackRowData(id)
	if err != nil {
		return nil, err
	}
	if tractStackRow == nil {
		return nil, nil
	}

	// Deserialize to TractStackNode
	tractStackNode := tss.deserializeRowData(tractStackRow)

	return tractStackNode, nil
}

// loadMultipleFromDB loads multiple tractstacks from database using IN clause
func (tss *TractStackService) loadMultipleFromDB(ids []string) ([]*models.TractStackNode, error) {
	if len(ids) == 0 {
		return []*models.TractStackNode{}, nil
	}

	// Build IN clause with placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, social_image_path 
          FROM tractstacks WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := tss.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tractstacks: %w", err)
	}
	defer rows.Close()

	var tractstacks []*models.TractStackNode

	// Process all rows
	for rows.Next() {
		var tractStackRow TractStackRowData
		var socialImagePath sql.NullString

		err := rows.Scan(
			&tractStackRow.ID,
			&tractStackRow.Title,
			&tractStackRow.Slug,
			&socialImagePath,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tractstack: %w", err)
		}

		if socialImagePath.Valid {
			tractStackRow.SocialImagePath = &socialImagePath.String
		}

		tractStackNode := tss.deserializeRowData(&tractStackRow)
		tractstacks = append(tractstacks, tractStackNode)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return tractstacks, nil
}

// getIDBySlugFromDB gets tractstack ID by slug from database
func (tss *TractStackService) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM tractstacks WHERE slug = ? LIMIT 1`

	var id string
	err := tss.ctx.Database.Conn.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get tractstack by slug: %w", err)
	}

	return id, nil
}

// getTractStackRowData fetches raw tractstack data from database
func (tss *TractStackService) getTractStackRowData(id string) (*TractStackRowData, error) {
	query := `SELECT id, title, slug, social_image_path FROM tractstacks WHERE id = ?`

	row := tss.ctx.Database.Conn.QueryRow(query, id)

	var tractStackRow TractStackRowData
	var socialImagePath sql.NullString

	err := row.Scan(
		&tractStackRow.ID,
		&tractStackRow.Title,
		&tractStackRow.Slug,
		&socialImagePath,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan tractstack: %w", err)
	}

	if socialImagePath.Valid {
		tractStackRow.SocialImagePath = &socialImagePath.String
	}

	return &tractStackRow, nil
}

// deserializeRowData converts database rows to client TractStackNode
func (tss *TractStackService) deserializeRowData(tractStackRow *TractStackRowData) *models.TractStackNode {
	// Build TractStackNode
	tractStackNode := &models.TractStackNode{
		ID:    tractStackRow.ID,
		Title: tractStackRow.Title,
		Slug:  tractStackRow.Slug,
	}

	// Optional fields
	if tractStackRow.SocialImagePath != nil {
		tractStackNode.SocialImagePath = tractStackRow.SocialImagePath
	}

	return tractStackNode
}
