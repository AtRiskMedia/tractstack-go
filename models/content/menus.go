// Package content provides menus
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

// MenuRowData represents raw database structure
type MenuRowData struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Theme          string `json:"theme"`
	OptionsPayload string `json:"options_payload"`
}

// MenuService handles cache-first menu operations
type MenuService struct {
	ctx *tenant.Context
}

// NewMenuService creates a cache-first menu service
func NewMenuService(ctx *tenant.Context, _ any) *MenuService {
	// Ignore the cache manager parameter - we use the global instance directly
	return &MenuService{
		ctx: ctx,
	}
}

// GetAllIDs returns all menu IDs (cache-first)
func (ms *MenuService) GetAllIDs() ([]string, error) {
	// Check cache first
	if ids, found := cache.GetGlobalManager().GetAllMenuIDs(ms.ctx.TenantID); found {
		return ids, nil
	}

	// Cache miss - load from database
	ids, err := ms.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	// Load all menus to populate cache
	menus, err := ms.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	// Populate cache
	for _, menu := range menus {
		cache.GetGlobalManager().SetMenu(ms.ctx.TenantID, menu)
	}

	return ids, nil
}

// GetByID returns a menu by ID (cache-first)
func (ms *MenuService) GetByID(id string) (*models.MenuNode, error) {
	// Check cache first
	if menu, found := cache.GetGlobalManager().GetMenu(ms.ctx.TenantID, id); found {
		return menu, nil
	}

	// Cache miss - load from database
	menu, err := ms.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if menu == nil {
		return nil, nil // Not found
	}

	// Populate cache
	cache.GetGlobalManager().SetMenu(ms.ctx.TenantID, menu)

	return menu, nil
}

// GetByIDs returns multiple menus by IDs (cache-first with bulk loading)
func (ms *MenuService) GetByIDs(ids []string) ([]*models.MenuNode, error) {
	var result []*models.MenuNode
	var missingIDs []string

	// Check cache for each ID
	for _, id := range ids {
		if menu, found := cache.GetGlobalManager().GetMenu(ms.ctx.TenantID, id); found {
			result = append(result, menu)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// If we have cache misses, bulk load from database
	if len(missingIDs) > 0 {
		missingMenus, err := ms.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		// Add to cache and result
		for _, menu := range missingMenus {
			cache.GetGlobalManager().SetMenu(ms.ctx.TenantID, menu)
			result = append(result, menu)
		}
	}

	return result, nil
}

// Private database loading methods

// loadAllIDsFromDB fetches all menu IDs from database
func (ms *MenuService) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM menus ORDER BY title`

	rows, err := ms.ctx.Database.Conn.Query(query)
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

// loadFromDB loads a complete menu from database
func (ms *MenuService) loadFromDB(id string) (*models.MenuNode, error) {
	// Get menu row data
	menuRow, err := ms.getMenuRowData(id)
	if err != nil {
		return nil, err
	}
	if menuRow == nil {
		return nil, nil
	}

	// Deserialize to MenuNode
	menuNode, err := ms.deserializeRowData(menuRow)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize menu: %w", err)
	}

	return menuNode, nil
}

// loadMultipleFromDB loads multiple menus from database using IN clause
func (ms *MenuService) loadMultipleFromDB(ids []string) ([]*models.MenuNode, error) {
	if len(ids) == 0 {
		return []*models.MenuNode{}, nil
	}

	// Build IN clause with placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, theme, options_payload 
          FROM menus WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := ms.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query menus: %w", err)
	}
	defer rows.Close()

	var menus []*models.MenuNode

	// Process all rows
	for rows.Next() {
		var menuRow MenuRowData

		err := rows.Scan(
			&menuRow.ID,
			&menuRow.Title,
			&menuRow.Theme,
			&menuRow.OptionsPayload,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan menu: %w", err)
		}

		menuNode, err := ms.deserializeRowData(&menuRow)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize menu %s: %w", menuRow.ID, err)
		}

		menus = append(menus, menuNode)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return menus, nil
}

// getMenuRowData fetches raw menu data from database
func (ms *MenuService) getMenuRowData(id string) (*MenuRowData, error) {
	query := `SELECT id, title, theme, options_payload FROM menus WHERE id = ?`

	row := ms.ctx.Database.Conn.QueryRow(query, id)

	var menuRow MenuRowData

	err := row.Scan(
		&menuRow.ID,
		&menuRow.Title,
		&menuRow.Theme,
		&menuRow.OptionsPayload,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan menu: %w", err)
	}

	return &menuRow, nil
}

// deserializeRowData converts database rows to client MenuNode
func (ms *MenuService) deserializeRowData(menuRow *MenuRowData) (*models.MenuNode, error) {
	// Parse options payload to extract menu links
	var optionsPayload []models.MenuLink
	if err := json.Unmarshal([]byte(menuRow.OptionsPayload), &optionsPayload); err != nil {
		return nil, fmt.Errorf("failed to parse options payload: %w", err)
	}

	// Build MenuNode
	menuNode := &models.MenuNode{
		ID:             menuRow.ID,
		Title:          menuRow.Title,
		NodeType:       "Menu",
		Theme:          menuRow.Theme,
		OptionsPayload: optionsPayload,
	}

	return menuNode, nil
}
