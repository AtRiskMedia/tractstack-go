// Package content provides tractstacks repository
package content

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

type TractStackRepository struct {
	db     *sql.DB
	cache  interfaces.ContentCache
	logger *logging.ChanneledLogger
}

func NewTractStackRepository(db *sql.DB, cache interfaces.ContentCache, logger *logging.ChanneledLogger) *TractStackRepository {
	return &TractStackRepository{
		db:     db,
		cache:  cache,
		logger: logger,
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

// FindAll retrieves all tractstacks for a tenant, employing a cache-first strategy.
func (r *TractStackRepository) FindAll(tenantID string) ([]*content.TractStackNode, error) {
	// 1. Check cache for the master list of IDs first.
	if ids, found := r.cache.GetAllTractStackIDs(tenantID); found {
		return r.FindByIDs(tenantID, ids)
	}

	// --- CACHE MISS FALLBACK ---
	// 2. Load all IDs from the database.
	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.TractStackNode{}, nil
	}

	// 3. Set the master ID list in the cache immediately.
	r.cache.SetAllTractStackIDs(tenantID, ids)

	// 4. Use the robust FindByIDs method to load the actual objects.
	return r.FindByIDs(tenantID, ids)
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

	start := time.Now()
	r.logger.Database().Debug("Executing tractstack insert", "id", tractStack.ID)

	_, err := r.db.Exec(query, tractStack.ID, tractStack.Title, tractStack.Slug, tractStack.SocialImagePath)
	if err != nil {
		r.logger.Database().Error("Tractstack insert failed", "error", err.Error(), "id", tractStack.ID)
		return fmt.Errorf("failed to insert tractstack: %w", err)
	}

	r.logger.Database().Info("Tractstack insert completed", "id", tractStack.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetTractStack(tenantID, tractStack)
	return nil
}

func (r *TractStackRepository) Update(tenantID string, tractStack *content.TractStackNode) error {
	query := `UPDATE tractstacks SET title = ?, slug = ?, social_image_path = ? WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing tractstack update", "id", tractStack.ID)

	_, err := r.db.Exec(query, tractStack.Title, tractStack.Slug, tractStack.SocialImagePath, tractStack.ID)
	if err != nil {
		r.logger.Database().Error("Tractstack update failed", "error", err.Error(), "id", tractStack.ID)
		return fmt.Errorf("failed to update tractstack: %w", err)
	}

	r.logger.Database().Info("Tractstack update completed", "id", tractStack.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetTractStack(tenantID, tractStack)
	return nil
}

func (r *TractStackRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM tractstacks WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing tractstack delete", "id", id)

	_, err := r.db.Exec(query, id)
	if err != nil {
		r.logger.Database().Error("Tractstack delete failed", "error", err.Error(), "id", id)
		return fmt.Errorf("failed to delete tractstack: %w", err)
	}

	r.logger.Database().Info("Tractstack delete completed", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.InvalidateContentCache(tenantID)
	return nil
}

func (r *TractStackRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM tractstacks ORDER BY title`

	start := time.Now()
	r.logger.Database().Debug("Loading all tractstack IDs from database")

	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Database().Error("Failed to query tractstack IDs", "error", err.Error())
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

	r.logger.Database().Info("Loaded tractstack IDs from database", "count", len(tractStackIDs), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return tractStackIDs, rows.Err()
}

func (r *TractStackRepository) loadFromDB(id string) (*content.TractStackNode, error) {
	query := `SELECT id, title, slug, social_image_path FROM tractstacks WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading tractstack from database", "id", id)

	row := r.db.QueryRow(query, id)

	var ts content.TractStackNode
	var socialImagePath sql.NullString

	err := row.Scan(&ts.ID, &ts.Title, &ts.Slug, &socialImagePath)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to scan tractstack", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to scan tractstack: %w", err)
	}

	if socialImagePath.Valid {
		ts.SocialImagePath = &socialImagePath.String
	}

	ts.NodeType = "TractStack"

	r.logger.Database().Info("Tractstack loaded from database", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
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

	start := time.Now()
	r.logger.Database().Debug("Loading multiple tractstacks from database", "count", len(ids))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query multiple tractstacks", "error", err.Error(), "count", len(ids))
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

	r.logger.Database().Info("Multiple tractstacks loaded from database", "requested", len(ids), "loaded", len(tractStacks), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return tractStacks, rows.Err()
}

func (r *TractStackRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM tractstacks WHERE slug = ? LIMIT 1`

	start := time.Now()
	r.logger.Database().Debug("Loading tractstack ID by slug from database", "slug", slug)

	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		r.logger.Database().Debug("Tractstack not found by slug", "slug", slug)
		return "", nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to query tractstack by slug", "error", err.Error(), "slug", slug)
		return "", fmt.Errorf("failed to get tractstack by slug: %w", err)
	}

	r.logger.Database().Info("Tractstack ID loaded by slug", "slug", slug, "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return id, nil
}
