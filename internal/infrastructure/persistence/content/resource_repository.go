// Package content provides resources repository
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
)

type ResourceRepository struct {
	db    *sql.DB
	cache interfaces.ContentCache
}

func NewResourceRepository(db *sql.DB, cache interfaces.ContentCache) *ResourceRepository {
	return &ResourceRepository{
		db:    db,
		cache: cache,
	}
}

func (r *ResourceRepository) FindByID(tenantID, id string) (*content.ResourceNode, error) {
	if resource, found := r.cache.GetResource(tenantID, id); found {
		return resource, nil
	}

	resource, err := r.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, nil
	}

	r.cache.SetResource(tenantID, resource)
	return resource, nil
}

func (r *ResourceRepository) FindBySlug(tenantID, slug string) (*content.ResourceNode, error) {
	id, err := r.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}

	return r.FindByID(tenantID, id)
}

func (r *ResourceRepository) FindByCategory(tenantID, category string) ([]*content.ResourceNode, error) {
	if resourceIDs, found := r.cache.GetResourcesByCategory(tenantID, category); found {
		return r.FindByIDs(tenantID, resourceIDs)
	}

	ids, err := r.getIDsByCategoryFromDB(category)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.ResourceNode{}, nil
	}

	return r.FindByIDs(tenantID, ids)
}

// FindAll retrieves all resources for a tenant, employing a cache-first strategy.
func (r *ResourceRepository) FindAll(tenantID string) ([]*content.ResourceNode, error) {
	// 1. Check cache for the master list of IDs first.
	if ids, found := r.cache.GetAllResourceIDs(tenantID); found {
		return r.FindByIDs(tenantID, ids)
	}

	// --- CACHE MISS FALLBACK ---
	// 2. Load all IDs from the database.
	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.ResourceNode{}, nil
	}

	// 3. Set the master ID list in the cache immediately.
	r.cache.SetAllResourceIDs(tenantID, ids)

	// 4. Use the robust FindByIDs method to load the actual objects.
	return r.FindByIDs(tenantID, ids)
}

func (r *ResourceRepository) FindByIDs(tenantID string, ids []string) ([]*content.ResourceNode, error) {
	var result []*content.ResourceNode
	var missingIDs []string

	for _, id := range ids {
		if resource, found := r.cache.GetResource(tenantID, id); found {
			result = append(result, resource)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingResources, err := r.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, resource := range missingResources {
			r.cache.SetResource(tenantID, resource)
			result = append(result, resource)
		}
	}

	return result, nil
}

func (r *ResourceRepository) Store(tenantID string, resource *content.ResourceNode) error {
	optionsJSON, _ := json.Marshal(resource.OptionsPayload)

	query := `INSERT INTO resources (id, title, slug, category_slug, oneliner, action_lisp, options_payload) 
              VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.Exec(query, resource.ID, resource.Title, resource.Slug,
		resource.CategorySlug, resource.OneLiner, resource.ActionLisp, string(optionsJSON))
	if err != nil {
		return fmt.Errorf("failed to insert resource: %w", err)
	}

	r.cache.SetResource(tenantID, resource)
	return nil
}

func (r *ResourceRepository) Update(tenantID string, resource *content.ResourceNode) error {
	optionsJSON, _ := json.Marshal(resource.OptionsPayload)

	query := `UPDATE resources SET title = ?, slug = ?, category_slug = ?, oneliner = ?, 
              action_lisp = ?, options_payload = ? WHERE id = ?`

	_, err := r.db.Exec(query, resource.Title, resource.Slug, resource.CategorySlug,
		resource.OneLiner, resource.ActionLisp, string(optionsJSON), resource.ID)
	if err != nil {
		return fmt.Errorf("failed to update resource: %w", err)
	}

	r.cache.SetResource(tenantID, resource)
	return nil
}

func (r *ResourceRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM resources WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	r.cache.InvalidateContentCache(tenantID)
	return nil
}

func (r *ResourceRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM resources ORDER BY title`

	rows, err := r.db.Query(query)
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

	return resourceIDs, rows.Err()
}

// ... (rest of the helper methods: loadFromDB, loadMultipleFromDB, etc. remain the same)
func (r *ResourceRepository) loadFromDB(id string) (*content.ResourceNode, error) {
	query := `SELECT id, title, slug, category_slug, oneliner, action_lisp, options_payload 
              FROM resources WHERE id = ?`

	row := r.db.QueryRow(query, id)

	var resource content.ResourceNode
	var categorySlug, actionLisp sql.NullString
	var optionsPayloadStr string

	err := row.Scan(&resource.ID, &resource.Title, &resource.Slug, &categorySlug,
		&resource.OneLiner, &actionLisp, &optionsPayloadStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan resource: %w", err)
	}

	if err := json.Unmarshal([]byte(optionsPayloadStr), &resource.OptionsPayload); err != nil {
		return nil, fmt.Errorf("failed to parse options payload: %w", err)
	}

	if categorySlug.Valid {
		resource.CategorySlug = &categorySlug.String
	}
	if actionLisp.Valid {
		resource.ActionLisp = actionLisp.String
	}

	resource.NodeType = "Resource"

	return &resource, nil
}

func (r *ResourceRepository) loadMultipleFromDB(ids []string) ([]*content.ResourceNode, error) {
	if len(ids) == 0 {
		return []*content.ResourceNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, category_slug, oneliner, action_lisp, options_payload 
              FROM resources WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query resources: %w", err)
	}
	defer rows.Close()

	var resources []*content.ResourceNode

	for rows.Next() {
		var resource content.ResourceNode
		var categorySlug, actionLisp sql.NullString
		var optionsPayloadStr string

		err := rows.Scan(&resource.ID, &resource.Title, &resource.Slug, &categorySlug,
			&resource.OneLiner, &actionLisp, &optionsPayloadStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan resource: %w", err)
		}

		if err := json.Unmarshal([]byte(optionsPayloadStr), &resource.OptionsPayload); err != nil {
			continue // Skip malformed records
		}

		if categorySlug.Valid {
			resource.CategorySlug = &categorySlug.String
		}
		if actionLisp.Valid {
			resource.ActionLisp = actionLisp.String
		}

		resource.NodeType = "Resource"
		resources = append(resources, &resource)
	}

	return resources, rows.Err()
}

func (r *ResourceRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM resources WHERE slug = ? LIMIT 1`

	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get resource by slug: %w", err)
	}

	return id, nil
}

func (r *ResourceRepository) getIDsByCategoryFromDB(category string) ([]string, error) {
	query := `SELECT id FROM resources WHERE category_slug = ? ORDER BY title`

	rows, err := r.db.Query(query, category)
	if err != nil {
		return nil, fmt.Errorf("failed to query resources by category: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan resource ID: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

func (r *ResourceRepository) FindByFilters(tenantID string, ids []string, categories []string, slugs []string) ([]*content.ResourceNode, error) {
	resourceMap := make(map[string]*content.ResourceNode)
	var queryIDs []string

	for _, id := range ids {
		if resource, found := r.cache.GetResource(tenantID, id); found {
			resourceMap[id] = resource
		} else {
			queryIDs = append(queryIDs, id)
		}
	}

	var queryBuilder strings.Builder
	queryBuilder.WriteString("SELECT id, title, slug, category_slug, oneliner, action_lisp, options_payload FROM resources WHERE 1=1")
	args := make([]interface{}, 0)

	if len(queryIDs) > 0 {
		queryBuilder.WriteString(" AND id IN (?" + strings.Repeat(",?", len(queryIDs)-1) + ")")
		for _, id := range queryIDs {
			args = append(args, id)
		}
	}

	if len(categories) > 0 {
		queryBuilder.WriteString(" AND category_slug IN (?" + strings.Repeat(",?", len(categories)-1) + ")")
		for _, category := range categories {
			args = append(args, category)
		}
	}

	if len(slugs) > 0 {
		queryBuilder.WriteString(" AND slug IN (?" + strings.Repeat(",?", len(slugs)-1) + ")")
		for _, slug := range slugs {
			args = append(args, slug)
		}
	}

	if len(args) > 0 {
		rows, err := r.db.Query(queryBuilder.String(), args...)
		if err != nil {
			return nil, fmt.Errorf("failed to query resources by filters: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var resource content.ResourceNode
			var categorySlug, actionLisp sql.NullString
			var optionsPayloadStr string

			if err := rows.Scan(&resource.ID, &resource.Title, &resource.Slug, &categorySlug, &resource.OneLiner, &actionLisp, &optionsPayloadStr); err != nil {
				return nil, fmt.Errorf("failed to scan filtered resource: %w", err)
			}

			if err := json.Unmarshal([]byte(optionsPayloadStr), &resource.OptionsPayload); err != nil {
				continue
			}

			if categorySlug.Valid {
				resource.CategorySlug = &categorySlug.String
			}
			if actionLisp.Valid {
				resource.ActionLisp = actionLisp.String
			}

			resource.NodeType = "Resource"
			if _, exists := resourceMap[resource.ID]; !exists {
				resourceMap[resource.ID] = &resource
				r.cache.SetResource(tenantID, &resource)
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	finalResources := make([]*content.ResourceNode, 0, len(resourceMap))
	for _, resource := range resourceMap {
		finalResources = append(finalResources, resource)
	}

	return finalResources, nil
}
