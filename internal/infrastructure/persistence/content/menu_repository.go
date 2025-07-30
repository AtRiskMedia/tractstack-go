// Package content provides menus repository
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
)

type MenuRepository struct {
	db    *sql.DB
	cache interfaces.ContentCache
}

func NewMenuRepository(db *sql.DB, cache interfaces.ContentCache) *MenuRepository {
	return &MenuRepository{
		db:    db,
		cache: cache,
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

func (r *MenuRepository) FindAll(tenantID string) ([]*content.MenuNode, error) {
	if ids, found := r.cache.GetAllMenuIDs(tenantID); found {
		var menus []*content.MenuNode
		var missingIDs []string

		for _, id := range ids {
			if menu, found := r.cache.GetMenu(tenantID, id); found {
				menus = append(menus, menu)
			} else {
				missingIDs = append(missingIDs, id)
			}
		}

		if len(missingIDs) > 0 {
			missing, err := r.loadMultipleFromDB(missingIDs)
			if err != nil {
				return nil, err
			}

			for _, menu := range missing {
				r.cache.SetMenu(tenantID, menu)
				menus = append(menus, menu)
			}
		}

		return menus, nil
	}

	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	menus, err := r.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	for _, menu := range menus {
		r.cache.SetMenu(tenantID, menu)
	}

	return menus, nil
}

func (r *MenuRepository) Store(tenantID string, menu *content.MenuNode) error {
	optionsJSON, _ := json.Marshal(menu.OptionsPayload)

	query := `INSERT INTO menus (id, title, theme, options_payload) VALUES (?, ?, ?, ?)`

	_, err := r.db.Exec(query, menu.ID, menu.Title, menu.Theme, string(optionsJSON))
	if err != nil {
		return fmt.Errorf("failed to insert menu: %w", err)
	}

	r.cache.SetMenu(tenantID, menu)
	return nil
}

func (r *MenuRepository) Update(tenantID string, menu *content.MenuNode) error {
	optionsJSON, _ := json.Marshal(menu.OptionsPayload)

	query := `UPDATE menus SET title = ?, theme = ?, options_payload = ? WHERE id = ?`

	_, err := r.db.Exec(query, menu.Title, menu.Theme, string(optionsJSON), menu.ID)
	if err != nil {
		return fmt.Errorf("failed to update menu: %w", err)
	}

	r.cache.SetMenu(tenantID, menu)
	return nil
}

func (r *MenuRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM menus WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete menu: %w", err)
	}

	r.cache.InvalidateContentCache(tenantID)
	return nil
}

func (r *MenuRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM menus ORDER BY title`

	rows, err := r.db.Query(query)
	if err != nil {
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return menuIDs, nil
}

func (r *MenuRepository) loadFromDB(id string) (*content.MenuNode, error) {
	query := `SELECT id, title, theme, options_payload FROM menus WHERE id = ?`

	row := r.db.QueryRow(query, id)

	var menu content.MenuNode
	var optionsPayloadStr string

	err := row.Scan(&menu.ID, &menu.Title, &menu.Theme, &optionsPayloadStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan menu: %w", err)
	}

	if err := json.Unmarshal([]byte(optionsPayloadStr), &menu.OptionsPayload); err != nil {
		return nil, fmt.Errorf("failed to parse options payload: %w", err)
	}

	menu.NodeType = "Menu"

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

	rows, err := r.db.Query(query, args...)
	if err != nil {
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
			return nil, fmt.Errorf("failed to parse options payload: %w", err)
		}

		menu.NodeType = "Menu"
		menus = append(menus, &menu)
	}

	return menus, rows.Err()
}
