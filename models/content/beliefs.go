// Package content provides beliefs
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

// BeliefRowData represents raw database structure
type BeliefRowData struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Slug         string  `json:"slug"`
	Scale        string  `json:"scale"`
	CustomValues *string `json:"custom_values,omitempty"`
}

// BeliefService handles cache-first belief operations
type BeliefService struct {
	ctx *tenant.Context
}

// NewBeliefService creates a cache-first belief service
func NewBeliefService(ctx *tenant.Context, _ any) *BeliefService {
	// Ignore the cache manager parameter - we use the global instance directly
	return &BeliefService{
		ctx: ctx,
	}
}

// GetAllIDs returns all belief IDs (cache-first)
func (bs *BeliefService) GetAllIDs() ([]string, error) {
	// Check cache first
	if ids, found := cache.GetGlobalManager().GetAllBeliefIDs(bs.ctx.TenantID); found {
		return ids, nil
	}

	// Cache miss - load from database
	ids, err := bs.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	// Load all beliefs to populate cache
	beliefs, err := bs.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	// Populate cache
	for _, belief := range beliefs {
		cache.GetGlobalManager().SetBelief(bs.ctx.TenantID, belief)
	}

	return ids, nil
}

// GetByID returns a belief by ID (cache-first)
func (bs *BeliefService) GetByID(id string) (*models.BeliefNode, error) {
	// Check cache first
	if belief, found := cache.GetGlobalManager().GetBelief(bs.ctx.TenantID, id); found {
		return belief, nil
	}

	// Cache miss - load from database
	belief, err := bs.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if belief == nil {
		return nil, nil // Not found
	}

	// Populate cache
	cache.GetGlobalManager().SetBelief(bs.ctx.TenantID, belief)

	return belief, nil
}

// GetByIDs returns multiple beliefs by IDs (cache-first with bulk loading)
func (bs *BeliefService) GetByIDs(ids []string) ([]*models.BeliefNode, error) {
	var result []*models.BeliefNode
	var missingIDs []string

	// Check cache for each ID
	for _, id := range ids {
		if belief, found := cache.GetGlobalManager().GetBelief(bs.ctx.TenantID, id); found {
			result = append(result, belief)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// If we have cache misses, bulk load from database
	if len(missingIDs) > 0 {
		missingBeliefs, err := bs.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		// Add to cache and result
		for _, belief := range missingBeliefs {
			cache.GetGlobalManager().SetBelief(bs.ctx.TenantID, belief)
			result = append(result, belief)
		}
	}

	return result, nil
}

// GetBySlug returns a belief by slug (cache-first)
func (bs *BeliefService) GetBySlug(slug string) (*models.BeliefNode, error) {
	// Check cache first
	if belief, found := cache.GetGlobalManager().GetBeliefBySlug(bs.ctx.TenantID, slug); found {
		return belief, nil
	}

	// Cache miss - get ID from database, then load belief
	id, err := bs.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil // Not found
	}

	// Load full belief (this will cache it)
	return bs.GetByID(id)
}

// Private database loading methods

// loadAllIDsFromDB fetches all belief IDs from database
func (bs *BeliefService) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM beliefs ORDER BY title`

	rows, err := bs.ctx.Database.Conn.Query(query)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return beliefIDs, nil
}

// loadFromDB loads a complete belief from database
func (bs *BeliefService) loadFromDB(id string) (*models.BeliefNode, error) {
	// Get belief row data
	beliefRow, err := bs.getBeliefRowData(id)
	if err != nil {
		return nil, err
	}
	if beliefRow == nil {
		return nil, nil
	}

	// Deserialize to BeliefNode
	beliefNode, err := bs.deserializeRowData(beliefRow)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize belief: %w", err)
	}

	return beliefNode, nil
}

// loadMultipleFromDB loads multiple beliefs from database using IN clause
func (bs *BeliefService) loadMultipleFromDB(ids []string) ([]*models.BeliefNode, error) {
	if len(ids) == 0 {
		return []*models.BeliefNode{}, nil
	}

	// Build IN clause with placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, scale, custom_values 
          FROM beliefs WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := bs.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query beliefs: %w", err)
	}
	defer rows.Close()

	var beliefs []*models.BeliefNode

	// Process all rows
	for rows.Next() {
		var beliefRow BeliefRowData
		var customValues sql.NullString

		err := rows.Scan(
			&beliefRow.ID,
			&beliefRow.Title,
			&beliefRow.Slug,
			&beliefRow.Scale,
			&customValues,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan belief: %w", err)
		}

		if customValues.Valid {
			beliefRow.CustomValues = &customValues.String
		}

		beliefNode, err := bs.deserializeRowData(&beliefRow)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize belief %s: %w", beliefRow.ID, err)
		}

		beliefs = append(beliefs, beliefNode)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return beliefs, nil
}

// getIDBySlugFromDB gets belief ID by slug from database
func (bs *BeliefService) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM beliefs WHERE slug = ? LIMIT 1`

	var id string
	err := bs.ctx.Database.Conn.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get belief by slug: %w", err)
	}

	return id, nil
}

// getBeliefRowData fetches raw belief data from database
func (bs *BeliefService) getBeliefRowData(id string) (*BeliefRowData, error) {
	query := `SELECT id, title, slug, scale, custom_values FROM beliefs WHERE id = ?`

	row := bs.ctx.Database.Conn.QueryRow(query, id)

	var beliefRow BeliefRowData
	var customValues sql.NullString

	err := row.Scan(
		&beliefRow.ID,
		&beliefRow.Title,
		&beliefRow.Slug,
		&beliefRow.Scale,
		&customValues,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan belief: %w", err)
	}

	if customValues.Valid {
		beliefRow.CustomValues = &customValues.String
	}

	return &beliefRow, nil
}

// deserializeRowData converts database rows to client BeliefNode
func (bs *BeliefService) deserializeRowData(beliefRow *BeliefRowData) (*models.BeliefNode, error) {
	// Build BeliefNode
	beliefNode := &models.BeliefNode{
		ID:    beliefRow.ID,
		Title: beliefRow.Title,
		Slug:  beliefRow.Slug,
		Scale: beliefRow.Scale,
	}

	// Parse custom values if present
	if beliefRow.CustomValues != nil {
		var customValues []string
		if err := json.Unmarshal([]byte(*beliefRow.CustomValues), &customValues); err != nil {
			return nil, fmt.Errorf("failed to parse custom values: %w", err)
		}
		beliefNode.CustomValues = customValues
	}

	return beliefNode, nil
}
