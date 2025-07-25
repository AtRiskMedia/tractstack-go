// Package content provides resources
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

// ResourceRowData represents raw database structure
type ResourceRowData struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Slug           string  `json:"slug"`
	CategorySlug   *string `json:"category_slug,omitempty"`
	Oneliner       string  `json:"oneliner"`
	ActionLisp     *string `json:"action_lisp,omitempty"`
	OptionsPayload string  `json:"options_payload"`
}

// ResourceService handles cache-first resource operations
type ResourceService struct {
	ctx *tenant.Context
}

// NewResourceService creates a cache-first resource service
func NewResourceService(ctx *tenant.Context, _ any) *ResourceService {
	// Ignore the cache manager parameter - we use the global instance directly
	return &ResourceService{
		ctx: ctx,
	}
}

// GetAllIDs returns all resource IDs (cache-first)
func (rs *ResourceService) GetAllIDs() ([]string, error) {
	// Check cache first
	if ids, found := cache.GetGlobalManager().GetAllResourceIDs(rs.ctx.TenantID); found {
		return ids, nil
	}

	// Cache miss - load from database
	ids, err := rs.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	// Load all resources to populate cache
	resources, err := rs.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	// Populate cache
	for _, resource := range resources {
		cache.GetGlobalManager().SetResource(rs.ctx.TenantID, resource)
	}

	return ids, nil
}

// GetByID returns a resource by ID (cache-first)
func (rs *ResourceService) GetByID(id string) (*models.ResourceNode, error) {
	// Check cache first
	if resource, found := cache.GetGlobalManager().GetResource(rs.ctx.TenantID, id); found {
		return resource, nil
	}

	// Cache miss - load from database
	resource, err := rs.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, nil // Not found
	}

	// Populate cache
	cache.GetGlobalManager().SetResource(rs.ctx.TenantID, resource)

	return resource, nil
}

// GetByIDs returns multiple resources by IDs (cache-first with bulk loading)
func (rs *ResourceService) GetByIDs(ids []string) ([]*models.ResourceNode, error) {
	var result []*models.ResourceNode
	var missingIDs []string

	// Check cache for each ID
	for _, id := range ids {
		if resource, found := cache.GetGlobalManager().GetResource(rs.ctx.TenantID, id); found {
			result = append(result, resource)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// If we have cache misses, bulk load from database
	if len(missingIDs) > 0 {
		missingResources, err := rs.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		// Add to cache and result
		for _, resource := range missingResources {
			cache.GetGlobalManager().SetResource(rs.ctx.TenantID, resource)
			result = append(result, resource)
		}
	}

	return result, nil
}

// GetBySlug returns a resource by slug (cache-first)
func (rs *ResourceService) GetBySlug(slug string) (*models.ResourceNode, error) {
	// Check cache first
	if resource, found := cache.GetGlobalManager().GetResourceBySlug(rs.ctx.TenantID, slug); found {
		return resource, nil
	}

	// Cache miss - get ID from database, then load resource
	id, err := rs.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil // Not found
	}

	// Load full resource (this will cache it)
	return rs.GetByID(id)
}

// GetByCategory returns resources by category (cache-first)
func (rs *ResourceService) GetByCategory(category string) ([]*models.ResourceNode, error) {
	// Check cache first
	if resources, found := cache.GetGlobalManager().GetResourcesByCategory(rs.ctx.TenantID, category); found {
		return resources, nil
	}

	// Cache miss - load from database
	resources, err := rs.loadByCategoryFromDB(category)
	if err != nil {
		return nil, err
	}

	// Populate cache for individual resources (category index will be built automatically)
	for _, resource := range resources {
		cache.GetGlobalManager().SetResource(rs.ctx.TenantID, resource)
	}

	return resources, nil
}

// Private database loading methods

// loadAllIDsFromDB fetches all resource IDs from database
func (rs *ResourceService) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM resources ORDER BY title`

	rows, err := rs.ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query resources: %w", err)
	}
	defer rows.Close()

	var resourceIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan resource ID: %w", err)
		}
		resourceIDs = append(resourceIDs, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return resourceIDs, nil
}

// loadFromDB loads a complete resource from database
func (rs *ResourceService) loadFromDB(id string) (*models.ResourceNode, error) {
	// Get resource row data
	resourceRow, err := rs.getResourceRowData(id)
	if err != nil {
		return nil, err
	}
	if resourceRow == nil {
		return nil, nil
	}

	// Deserialize to ResourceNode
	resourceNode, err := rs.deserializeRowData(resourceRow)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize resource: %w", err)
	}

	return resourceNode, nil
}

// loadMultipleFromDB loads multiple resources from database using IN clause
func (rs *ResourceService) loadMultipleFromDB(ids []string) ([]*models.ResourceNode, error) {
	if len(ids) == 0 {
		return []*models.ResourceNode{}, nil
	}

	// Build IN clause with placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, category_slug, oneliner, action_lisp, options_payload 
          FROM resources WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := rs.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query resources: %w", err)
	}
	defer rows.Close()

	var resources []*models.ResourceNode

	// Process all rows
	for rows.Next() {
		var resourceRow ResourceRowData
		var categorySlug, actionLisp sql.NullString

		err := rows.Scan(
			&resourceRow.ID,
			&resourceRow.Title,
			&resourceRow.Slug,
			&categorySlug,
			&resourceRow.Oneliner,
			&actionLisp,
			&resourceRow.OptionsPayload,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan resource: %w", err)
		}

		if categorySlug.Valid {
			resourceRow.CategorySlug = &categorySlug.String
		}
		if actionLisp.Valid {
			resourceRow.ActionLisp = &actionLisp.String
		}

		resourceNode, err := rs.deserializeRowData(&resourceRow)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize resource %s: %w", resourceRow.ID, err)
		}

		resources = append(resources, resourceNode)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return resources, nil
}

// loadByCategoryFromDB loads resources by category from database
func (rs *ResourceService) loadByCategoryFromDB(category string) ([]*models.ResourceNode, error) {
	query := `SELECT id, title, slug, category_slug, oneliner, action_lisp, options_payload 
          FROM resources WHERE category_slug = ? ORDER BY title`

	rows, err := rs.ctx.Database.Conn.Query(query, category)
	if err != nil {
		return nil, fmt.Errorf("failed to query resources by category: %w", err)
	}
	defer rows.Close()

	var resources []*models.ResourceNode

	for rows.Next() {
		var resourceRow ResourceRowData
		var categorySlug, actionLisp sql.NullString

		err := rows.Scan(
			&resourceRow.ID,
			&resourceRow.Title,
			&resourceRow.Slug,
			&categorySlug,
			&resourceRow.Oneliner,
			&actionLisp,
			&resourceRow.OptionsPayload,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan resource: %w", err)
		}

		if categorySlug.Valid {
			resourceRow.CategorySlug = &categorySlug.String
		}
		if actionLisp.Valid {
			resourceRow.ActionLisp = &actionLisp.String
		}

		resourceNode, err := rs.deserializeRowData(&resourceRow)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize resource %s: %w", resourceRow.ID, err)
		}

		resources = append(resources, resourceNode)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return resources, nil
}

// getIDBySlugFromDB gets resource ID by slug from database
func (rs *ResourceService) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM resources WHERE slug = ? LIMIT 1`

	var id string
	err := rs.ctx.Database.Conn.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get resource by slug: %w", err)
	}

	return id, nil
}

// getResourceRowData fetches raw resource data from database
func (rs *ResourceService) getResourceRowData(id string) (*ResourceRowData, error) {
	query := `SELECT id, title, slug, category_slug, oneliner, action_lisp, options_payload FROM resources WHERE id = ?`

	row := rs.ctx.Database.Conn.QueryRow(query, id)

	var resourceRow ResourceRowData
	var categorySlug, actionLisp sql.NullString

	err := row.Scan(
		&resourceRow.ID,
		&resourceRow.Title,
		&resourceRow.Slug,
		&categorySlug,
		&resourceRow.Oneliner,
		&actionLisp,
		&resourceRow.OptionsPayload,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan resource: %w", err)
	}

	if categorySlug.Valid {
		resourceRow.CategorySlug = &categorySlug.String
	}
	if actionLisp.Valid {
		resourceRow.ActionLisp = &actionLisp.String
	}

	return &resourceRow, nil
}

// deserializeRowData converts database rows to client ResourceNode
func (rs *ResourceService) deserializeRowData(resourceRow *ResourceRowData) (*models.ResourceNode, error) {
	// Parse options payload
	var optionsPayload map[string]any
	if err := json.Unmarshal([]byte(resourceRow.OptionsPayload), &optionsPayload); err != nil {
		return nil, fmt.Errorf("failed to parse options payload: %w", err)
	}

	// Build ResourceNode
	resourceNode := &models.ResourceNode{
		ID:             resourceRow.ID,
		Title:          resourceRow.Title,
		NodeType:       "Resource",
		Slug:           resourceRow.Slug,
		Oneliner:       resourceRow.Oneliner,
		OptionsPayload: optionsPayload,
	}

	// Optional fields
	if resourceRow.CategorySlug != nil {
		resourceNode.CategorySlug = resourceRow.CategorySlug
	}
	if resourceRow.ActionLisp != nil {
		resourceNode.ActionLisp = resourceRow.ActionLisp
	}

	return resourceNode, nil
}
