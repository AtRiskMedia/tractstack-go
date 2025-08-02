// Package content provides menus repository
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

type MenuRepository struct {
	db     *sql.DB
	cache  interfaces.ContentCache
	logger *logging.ChanneledLogger
}

func NewMenuRepository(db *sql.DB, cache interfaces.ContentCache, logger *logging.ChanneledLogger) *MenuRepository {
	return &MenuRepository{
		db:     db,
		cache:  cache,
		logger: logger,
	}
}

func (r *MenuRepository) FindByID(tenantID, id string) (*content.MenuNode, error) {
	if menu, found := r.cache.GetMenu(tenantID, id); found {
		return menu, nil
	}

	menu, err := r.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if menu == nil {
		return nil, nil
	}

	r.cache.SetMenu(tenantID, menu)
	return menu, nil
}

// FindAll retrieves all menus for a tenant, employing a cache-first strategy.
func (r *MenuRepository) FindAll(tenantID string) ([]*content.MenuNode, error) {
	// 1. Check cache for the master list of IDs first.
	if ids, found := r.cache.GetAllMenuIDs(tenantID); found {
		// If the list exists, bulk-load the menus themselves (this is also cache-first).
		return r.FindByIDs(tenantID, ids)
	}

	// --- CACHE MISS FALLBACK ---
	// 2. The master ID list is not in the cache. Load all IDs from the database.
	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.MenuNode{}, nil
	}

	// 3. Set the master ID list in the cache immediately.
	r.cache.SetAllMenuIDs(tenantID, ids)

	// 4. Use the robust FindByIDs method to load the actual objects.
	return r.FindByIDs(tenantID, ids)
}

func (r *MenuRepository) FindByIDs(tenantID string, ids []string) ([]*content.MenuNode, error) {
	var result []*content.MenuNode
	var missingIDs []string

	for _, id := range ids {
		if menu, found := r.cache.GetMenu(tenantID, id); found {
			result = append(result, menu)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingMenus, err := r.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, menu := range missingMenus {
			r.cache.SetMenu(tenantID, menu)
			result = append(result, menu)
		}
	}

	return result, nil
}

func (r *MenuRepository) Store(tenantID string, menu *content.MenuNode) error {
	optionsJSON, _ := json.Marshal(menu.OptionsPayload)

	query := `INSERT INTO menus (id, title, theme, options_payload) VALUES (?, ?, ?, ?)`

	start := time.Now()
	r.logger.Database().Debug("Executing menu insert", "id", menu.ID)

	_, err := r.db.Exec(query, menu.ID, menu.Title, menu.Theme, string(optionsJSON))
	if err != nil {
		r.logger.Database().Error("Menu insert failed", "error", err.Error(), "id", menu.ID)
		return fmt.Errorf("failed to insert menu: %w", err)
	}

	r.logger.Database().Info("Menu insert completed", "id", menu.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetMenu(tenantID, menu)
	return nil
}

func (r *MenuRepository) Update(tenantID string, menu *content.MenuNode) error {
	optionsJSON, _ := json.Marshal(menu.OptionsPayload)

	query := `UPDATE menus SET title = ?, theme = ?, options_payload = ? WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing menu update", "id", menu.ID)

	_, err := r.db.Exec(query, menu.Title, menu.Theme, string(optionsJSON), menu.ID)
	if err != nil {
		r.logger.Database().Error("Menu update failed", "error", err.Error(), "id", menu.ID)
		return fmt.Errorf("failed to update menu: %w", err)
	}

	r.logger.Database().Info("Menu update completed", "id", menu.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetMenu(tenantID, menu)
	return nil
}

func (r *MenuRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM menus WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing menu delete", "id", id)

	_, err := r.db.Exec(query, id)
	if err != nil {
		r.logger.Database().Error("Menu delete failed", "error", err.Error(), "id", id)
		return fmt.Errorf("failed to delete menu: %w", err)
	}

	r.logger.Database().Info("Menu delete completed", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.InvalidateContentCache(tenantID)
	return nil
}

func (r *MenuRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM menus ORDER BY title`

	start := time.Now()
	r.logger.Database().Debug("Loading all menu IDs from database")

	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Database().Error("Failed to query menu IDs", "error", err.Error())
		return nil, fmt.Errorf("failed to query menus: %w", err)
	}
	defer rows.Close()

	var menuIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan menu ID: %w", err)
		}
		menuIDs = append(menuIDs, id)
	}

	r.logger.Database().Info("Loaded menu IDs from database", "count", len(menuIDs), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return menuIDs, rows.Err()
}

func (r *MenuRepository) loadFromDB(id string) (*content.MenuNode, error) {
	query := `SELECT id, title, theme, options_payload FROM menus WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading menu from database", "id", id)

	row := r.db.QueryRow(query, id)

	var menu content.MenuNode
	var optionsPayloadStr string

	err := row.Scan(&menu.ID, &menu.Title, &menu.Theme, &optionsPayloadStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to scan menu", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to scan menu: %w", err)
	}

	if err := json.Unmarshal([]byte(optionsPayloadStr), &menu.OptionsPayload); err != nil {
		r.logger.Database().Error("Failed to parse menu options payload", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to parse options payload: %w", err)
	}

	menu.NodeType = "Menu"

	r.logger.Database().Info("Menu loaded from database", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return &menu, nil
}

func (r *MenuRepository) loadMultipleFromDB(ids []string) ([]*content.MenuNode, error) {
	if len(ids) == 0 {
		return []*content.MenuNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, theme, options_payload 
              FROM menus WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	start := time.Now()
	r.logger.Database().Debug("Loading multiple menus from database", "count", len(ids))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query multiple menus", "error", err.Error(), "count", len(ids))
		return nil, fmt.Errorf("failed to query menus: %w", err)
	}
	defer rows.Close()

	var menus []*content.MenuNode

	for rows.Next() {
		var menu content.MenuNode
		var optionsPayloadStr string

		err := rows.Scan(&menu.ID, &menu.Title, &menu.Theme, &optionsPayloadStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan menu: %w", err)
		}

		if err := json.Unmarshal([]byte(optionsPayloadStr), &menu.OptionsPayload); err != nil {
			// Skip malformed records but continue processing others
			continue
		}

		menu.NodeType = "Menu"
		menus = append(menus, &menu)
	}

	r.logger.Database().Info("Multiple menus loaded from database", "requested", len(ids), "loaded", len(menus), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return menus, rows.Err()
}
