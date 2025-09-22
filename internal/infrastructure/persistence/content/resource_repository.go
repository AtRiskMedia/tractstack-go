// Package content provides resources repository
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/fts"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

type ResourceRepository struct {
	db         *sql.DB
	cache      interfaces.ContentCache
	logger     *logging.ChanneledLogger
	ftsService *fts.FTSService
}

func NewResourceRepository(db *sql.DB, cache interfaces.ContentCache, logger *logging.ChanneledLogger, ftsService *fts.FTSService) *ResourceRepository {
	return &ResourceRepository{
		db:         db,
		cache:      cache,
		logger:     logger,
		ftsService: ftsService,
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
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for resource store: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("failed to rollback transaction for resource store", "error", err)
		}
	}()

	optionsJSON, _ := json.Marshal(resource.OptionsPayload)
	query := `INSERT INTO resources (id, title, slug, category_slug, oneliner, action_lisp, options_payload) VALUES (?, ?, ?, ?, ?, ?, ?)`

	if _, err := tx.Exec(query, resource.ID, resource.Title, resource.Slug, resource.CategorySlug, resource.OneLiner, resource.ActionLisp, string(optionsJSON)); err != nil {
		return fmt.Errorf("failed to insert resource: %w", err)
	}

	// FTS Indexing (errors are logged and ignored) - only if category is in COLLECTION_ROUTES
	shouldIndex := false
	if len(config.CollectionRoutes) == 0 {
		// If no collection routes configured, index all resources
		shouldIndex = true
	} else if resource.CategorySlug != nil {
		// Check if this resource's category is in the collection routes
		for _, route := range config.CollectionRoutes {
			if *resource.CategorySlug == route {
				shouldIndex = true
				break
			}
		}
	}

	if shouldIndex {
		// Extract body content and index it - handle both string and array formats
		var bodyText string
		if body, ok := resource.OptionsPayload["body"].(string); ok && body != "" {
			bodyText = body
		} else if bodyArray, ok := resource.OptionsPayload["body"].([]any); ok && len(bodyArray) > 0 {
			// Convert array of strings to single string
			var bodyParts []string
			for _, part := range bodyArray {
				if partStr, ok := part.(string); ok {
					bodyParts = append(bodyParts, partStr)
				}
			}
			bodyText = strings.Join(bodyParts, " ")
		}

		if bodyText != "" {
			if err := r.ftsService.IndexResourceBody(tx, resource.ID, bodyText); err != nil {
				r.logger.System().Warn("Failed to index new resource body", "error", err, "resourceId", resource.ID)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction for resource store: %w", err)
	}

	r.cache.SetResource(tenantID, resource)
	return nil
}

func (r *ResourceRepository) Update(tenantID string, resource *content.ResourceNode) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for resource update: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("failed to rollback transaction for resource update", "error", err)
		}
	}()

	optionsJSON, _ := json.Marshal(resource.OptionsPayload)
	query := `UPDATE resources SET title = ?, slug = ?, category_slug = ?, oneliner = ?, action_lisp = ?, options_payload = ? WHERE id = ?`

	if _, err := tx.Exec(query, resource.Title, resource.Slug, resource.CategorySlug, resource.OneLiner, resource.ActionLisp, string(optionsJSON), resource.ID); err != nil {
		return fmt.Errorf("failed to update resource: %w", err)
	}

	// FTS Indexing (errors are logged and ignored)
	if body, ok := resource.OptionsPayload["body"].(string); ok && body != "" {
		if err := r.ftsService.IndexResourceBody(tx, resource.ID, body); err != nil {
			r.logger.System().Warn("Failed to index updated resource body", "error", err, "resourceId", resource.ID)
		}
	} else {
		// If body is removed, delete from index
		if err := r.ftsService.IndexResourceBody(tx, resource.ID, ""); err != nil {
			r.logger.System().Warn("Failed to clear resource body index", "error", err, "resourceId", resource.ID)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction for resource update: %w", err)
	}

	r.cache.SetResource(tenantID, resource)
	return nil
}

func (r *ResourceRepository) Delete(tenantID, id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for resource delete: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("failed to rollback transaction for resource delete", "error", err)
		}
	}()

	query := `DELETE FROM resources WHERE id = ?`
	if _, err := tx.Exec(query, id); err != nil {
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction for resource delete: %w", err)
	}
	return nil
}

func (r *ResourceRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM resources ORDER BY title`

	start := time.Now()
	r.logger.Database().Debug("Loading all resource IDs from database")

	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Database().Error("Failed to query resource IDs", "error", err.Error())
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

	r.logger.Database().Info("Loaded resource IDs from database", "count", len(resourceIDs), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return resourceIDs, rows.Err()
}

func (r *ResourceRepository) loadFromDB(id string) (*content.ResourceNode, error) {
	query := `SELECT id, title, slug, category_slug, oneliner, action_lisp, options_payload 
              FROM resources WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading resource from database", "id", id)

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
		r.logger.Database().Error("Failed to scan resource", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to scan resource: %w", err)
	}

	if err := json.Unmarshal([]byte(optionsPayloadStr), &resource.OptionsPayload); err != nil {
		r.logger.Database().Error("Failed to parse resource options payload", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to parse options payload: %w", err)
	}

	if categorySlug.Valid {
		resource.CategorySlug = &categorySlug.String
	}
	if actionLisp.Valid {
		resource.ActionLisp = actionLisp.String
	}

	resource.NodeType = "Resource"

	r.logger.Database().Info("Resource loaded from database", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
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

	start := time.Now()
	r.logger.Database().Debug("Loading multiple resources from database", "count", len(ids))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query multiple resources", "error", err.Error(), "count", len(ids))
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

	r.logger.Database().Info("Multiple resources loaded from database", "requested", len(ids), "loaded", len(resources), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return resources, rows.Err()
}

func (r *ResourceRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM resources WHERE slug = ? LIMIT 1`

	start := time.Now()
	r.logger.Database().Debug("Loading resource ID by slug from database", "slug", slug)

	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		r.logger.Database().Debug("Resource not found by slug", "slug", slug)
		return "", nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to query resource by slug", "error", err.Error(), "slug", slug)
		return "", fmt.Errorf("failed to get resource by slug: %w", err)
	}

	r.logger.Database().Info("Resource ID loaded by slug", "slug", slug, "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return id, nil
}

func (r *ResourceRepository) getIDsByCategoryFromDB(category string) ([]string, error) {
	query := `SELECT id FROM resources WHERE category_slug = ? ORDER BY title`

	start := time.Now()
	r.logger.Database().Debug("Loading resource IDs by category from database", "category", category)

	rows, err := r.db.Query(query, category)
	if err != nil {
		r.logger.Database().Error("Failed to query resources by category", "error", err.Error(), "category", category)
		return nil, fmt.Errorf("failed to query resources by category: %w", err)
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

	r.logger.Database().Info("Resource IDs loaded by category", "category", category, "count", len(resourceIDs), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return resourceIDs, rows.Err()
}

func (r *ResourceRepository) FindByFilters(tenantID string, queryIDs []string, categories []string, slugs []string) ([]*content.ResourceNode, error) {
	resourceMap := make(map[string]*content.ResourceNode)

	// Check cache first for individual IDs
	for _, id := range queryIDs {
		if resource, found := r.cache.GetResource(tenantID, id); found {
			resourceMap[resource.ID] = resource
		}
	}

	// Build dynamic query for remaining filters
	var queryBuilder strings.Builder
	queryBuilder.WriteString("SELECT id, title, slug, category_slug, oneliner, action_lisp, options_payload FROM resources WHERE 1=1")

	var args []any

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
		start := time.Now()
		r.logger.Database().Debug("Executing resource filter query", "filters", map[string]any{
			"queryIDs":   queryIDs,
			"categories": categories,
			"slugs":      slugs,
		})

		rows, err := r.db.Query(queryBuilder.String(), args...)
		if err != nil {
			r.logger.Database().Error("Failed to query resources by filters", "error", err.Error())
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

		r.logger.Database().Info("Resource filter query completed", "loaded", len(resourceMap), "duration", time.Since(start))
	}

	finalResources := make([]*content.ResourceNode, 0, len(resourceMap))
	for _, resource := range resourceMap {
		finalResources = append(finalResources, resource)
	}

	return finalResources, nil
}

// SearchBodies performs a prefix search on the resource_body_fts table.
func (r *ResourceRepository) SearchBodies(tenantID, term string) ([]repositories.FTSResult, error) {
	query := `SELECT resource_id, rank, snippet(resource_body_fts, 1, '>>>', '<<<', '...', 1) FROM resource_body_fts WHERE content MATCH ? ORDER BY rank LIMIT 10`
	searchTerm := term + "*"
	rows, err := r.db.Query(query, searchTerm)
	if err != nil {
		return nil, fmt.Errorf("failed to search resource bodies: %w", err)
	}
	defer rows.Close()

	var results []repositories.FTSResult
	for rows.Next() {
		var res repositories.FTSResult
		if err := rows.Scan(&res.ID, &res.Relevance, &res.Term); err != nil {
			return nil, fmt.Errorf("failed to scan resource body search result: %w", err)
		}
		results = append(results, res)
	}
	return results, rows.Err()
}
