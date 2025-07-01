// Package content provides panes
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// PaneRowData represents raw database structure
type PaneRowData struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Slug           string  `json:"slug"`
	PaneType       string  `json:"pane_type"`
	Created        string  `json:"created"`
	Changed        string  `json:"changed"`
	OptionsPayload string  `json:"options_payload"`
	IsContextPane  int     `json:"is_context_pane"`
	MarkdownID     *string `json:"markdown_id,omitempty"`
}

// MarkdownRowData represents markdown content
type MarkdownRowData struct {
	ID           string `json:"id"`
	MarkdownBody string `json:"markdown_body"`
}

// PaneService handles cache-first pane operations
type PaneService struct {
	ctx *tenant.Context
}

// NewPaneService creates a cache-first pane service
func NewPaneService(ctx *tenant.Context, _ any) *PaneService {
	// Ignore the cache manager parameter - we use the global instance directly
	return &PaneService{
		ctx: ctx,
	}
}

// GetAllIDs returns all pane IDs (cache-first)
func (ps *PaneService) GetAllIDs() ([]string, error) {
	// Check cache first
	if ids, found := cache.GetGlobalManager().GetAllPaneIDs(ps.ctx.TenantID); found {
		return ids, nil
	}

	// Cache miss - load from database
	ids, err := ps.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	// Populate cache
	cache.GetGlobalManager().SetAllPaneIDs(ps.ctx.TenantID, ids)

	return ids, nil
}

// GetByID returns a pane by ID (cache-first)
func (ps *PaneService) GetByID(id string) (*models.PaneNode, error) {
	// Check cache first
	if pane, found := cache.GetGlobalManager().GetPane(ps.ctx.TenantID, id); found {
		return pane, nil
	}

	// Cache miss - load from database
	pane, err := ps.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if pane == nil {
		return nil, nil // Not found
	}

	// Populate cache
	cache.GetGlobalManager().SetPane(ps.ctx.TenantID, pane)

	return pane, nil
}

// GetByIDs returns multiple panes by IDs (cache-first with bulk loading)
func (ps *PaneService) GetByIDs(ids []string) ([]*models.PaneNode, error) {
	var result []*models.PaneNode
	var missingIDs []string

	// Check cache for each ID
	for _, id := range ids {
		if pane, found := cache.GetGlobalManager().GetPane(ps.ctx.TenantID, id); found {
			result = append(result, pane)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// If we have cache misses, bulk load from database
	if len(missingIDs) > 0 {
		missingPanes, err := ps.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		// Add to cache and result
		for _, pane := range missingPanes {
			cache.GetGlobalManager().SetPane(ps.ctx.TenantID, pane)
			result = append(result, pane)
		}
	}

	return result, nil
}

// GetBySlug returns a pane by slug (cache-first)
func (ps *PaneService) GetBySlug(slug string) (*models.PaneNode, error) {
	// Check cache first
	if pane, found := cache.GetGlobalManager().GetPaneBySlug(ps.ctx.TenantID, slug); found {
		return pane, nil
	}

	// Cache miss - get ID from database, then load pane
	id, err := ps.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil // Not found
	}

	// Load full pane (this will cache it)
	return ps.GetByID(id)
}

// GetContextPanes returns all context panes
func (ps *PaneService) GetContextPanes() ([]*models.PaneNode, error) {
	// Get all IDs first (cache-first)
	allIDs, err := ps.GetAllIDs()
	if err != nil {
		return nil, err
	}

	var contextPanes []*models.PaneNode
	for _, id := range allIDs {
		pane, err := ps.GetByID(id)
		if err != nil {
			return nil, fmt.Errorf("failed to get pane %s: %w", id, err)
		}
		if pane != nil && pane.IsContextPane {
			contextPanes = append(contextPanes, pane)
		}
	}

	return contextPanes, nil
}

// Private database loading methods

// loadAllIDsFromDB fetches all pane IDs from database
func (ps *PaneService) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM panes ORDER BY title`

	rows, err := ps.ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query panes: %w", err)
	}
	defer rows.Close()

	var paneIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan pane ID: %w", err)
		}
		paneIDs = append(paneIDs, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return paneIDs, nil
}

// loadFromDB loads a complete pane from database
func (ps *PaneService) loadFromDB(id string) (*models.PaneNode, error) {
	// Get pane row data
	paneRow, err := ps.getPaneRowData(id)
	if err != nil {
		return nil, err
	}
	if paneRow == nil {
		return nil, nil
	}

	// Get markdown if pane has one
	var markdownRow *MarkdownRowData
	if paneRow.MarkdownID != nil {
		markdownRow, err = ps.getMarkdownRowData(*paneRow.MarkdownID)
		if err != nil {
			return nil, fmt.Errorf("failed to get markdown: %w", err)
		}
	}

	// Deserialize to PaneNode
	paneNode, err := ps.deserializeRowData(paneRow, markdownRow)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize pane: %w", err)
	}

	return paneNode, nil
}

// loadMultipleFromDB loads multiple panes from database using IN clause
func (ps *PaneService) loadMultipleFromDB(ids []string) ([]*models.PaneNode, error) {
	if len(ids) == 0 {
		return []*models.PaneNode{}, nil
	}

	// Build IN clause with placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, pane_type, created, changed, options_payload, is_context_pane, markdown_id 
          FROM panes WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := ps.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query panes: %w", err)
	}
	defer rows.Close()

	var panes []*models.PaneNode
	var markdownIDs []string
	markdownMap := make(map[string]*MarkdownRowData)

	// First pass: collect pane data and markdown IDs
	var paneRows []*PaneRowData
	for rows.Next() {
		var paneRow PaneRowData
		var markdownID sql.NullString

		err := rows.Scan(
			&paneRow.ID,
			&paneRow.Title,
			&paneRow.Slug,
			&paneRow.PaneType,
			&paneRow.Created,
			&paneRow.Changed,
			&paneRow.OptionsPayload,
			&paneRow.IsContextPane,
			&markdownID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pane: %w", err)
		}

		if markdownID.Valid {
			paneRow.MarkdownID = &markdownID.String
			markdownIDs = append(markdownIDs, markdownID.String)
		}

		paneRows = append(paneRows, &paneRow)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	// Second pass: bulk load markdown data if needed
	if len(markdownIDs) > 0 {
		markdownMap, err = ps.loadMultipleMarkdownFromDB(markdownIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to load markdown data: %w", err)
		}
	}

	// Third pass: deserialize all panes
	for _, paneRow := range paneRows {
		var markdownRow *MarkdownRowData
		if paneRow.MarkdownID != nil {
			markdownRow = markdownMap[*paneRow.MarkdownID]
		}

		paneNode, err := ps.deserializeRowData(paneRow, markdownRow)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize pane %s: %w", paneRow.ID, err)
		}

		panes = append(panes, paneNode)
	}

	return panes, nil
}

// loadMultipleMarkdownFromDB loads multiple markdown records
func (ps *PaneService) loadMultipleMarkdownFromDB(ids []string) (map[string]*MarkdownRowData, error) {
	if len(ids) == 0 {
		return make(map[string]*MarkdownRowData), nil
	}

	// Build IN clause with placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT id, body FROM markdowns WHERE id IN (%s)`,
		strings.Join(placeholders, ","))

	rows, err := ps.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query markdowns: %w", err)
	}
	defer rows.Close()

	markdownMap := make(map[string]*MarkdownRowData)
	for rows.Next() {
		var markdownRow MarkdownRowData
		err := rows.Scan(&markdownRow.ID, &markdownRow.MarkdownBody)
		if err != nil {
			return nil, fmt.Errorf("failed to scan markdown: %w", err)
		}
		markdownMap[markdownRow.ID] = &markdownRow
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("markdown row iteration error: %w", err)
	}

	return markdownMap, nil
}

// getIDBySlugFromDB gets pane ID by slug from database
func (ps *PaneService) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM panes WHERE slug = ? LIMIT 1`

	var id string
	err := ps.ctx.Database.Conn.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get pane by slug: %w", err)
	}

	return id, nil
}

// getPaneRowData fetches raw pane data from database
func (ps *PaneService) getPaneRowData(id string) (*PaneRowData, error) {
	query := `SELECT id, title, slug, pane_type, created, changed, options_payload, is_context_pane, markdown_id 
			  FROM panes WHERE id = ?`

	row := ps.ctx.Database.Conn.QueryRow(query, id)

	var paneRow PaneRowData
	var markdownID sql.NullString

	err := row.Scan(
		&paneRow.ID,
		&paneRow.Title,
		&paneRow.Slug,
		&paneRow.PaneType,
		&paneRow.Created,
		&paneRow.Changed,
		&paneRow.OptionsPayload,
		&paneRow.IsContextPane,
		&markdownID,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan pane: %w", err)
	}

	if markdownID.Valid {
		paneRow.MarkdownID = &markdownID.String
	}

	return &paneRow, nil
}

// getMarkdownRowData fetches markdown data
func (ps *PaneService) getMarkdownRowData(id string) (*MarkdownRowData, error) {
	query := `SELECT id, body FROM markdowns WHERE id = ?`

	row := ps.ctx.Database.Conn.QueryRow(query, id)

	var markdownRow MarkdownRowData
	err := row.Scan(&markdownRow.ID, &markdownRow.MarkdownBody)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan markdown: %w", err)
	}

	return &markdownRow, nil
}

// deserializeRowData converts database rows to client PaneNode
func (ps *PaneService) deserializeRowData(paneRow *PaneRowData, markdownRow *MarkdownRowData) (*models.PaneNode, error) {
	// Parse options payload
	var optionsPayload map[string]any
	if err := json.Unmarshal([]byte(paneRow.OptionsPayload), &optionsPayload); err != nil {
		return nil, fmt.Errorf("failed to parse options payload: %w", err)
	}

	// Parse timestamps
	created, err := time.Parse(time.RFC3339, paneRow.Created)
	if err != nil {
		created = time.Now() // Fallback
	}

	changed, err := time.Parse(time.RFC3339, paneRow.Changed)
	if err != nil {
		changed = created
	}

	// Extract structured data from options payload
	heldBeliefs := ps.extractBeliefsAsArrays(optionsPayload, "heldBeliefs")
	withheldBeliefs := ps.extractBeliefsAsArrays(optionsPayload, "withheldBeliefs")

	// Extract other fields
	var bgColour *string
	if bg, ok := optionsPayload["bgColour"].(string); ok {
		bgColour = &bg
	}

	var codeHookTarget *string
	if target, ok := optionsPayload["codeHookTarget"].(string); ok {
		codeHookTarget = &target
	}

	var codeHookPayload map[string]string
	if payload, ok := optionsPayload["codeHookPayload"].(map[string]any); ok {
		codeHookPayload = make(map[string]string)
		for k, v := range payload {
			if str, ok := v.(string); ok {
				codeHookPayload[k] = str
			}
		}
	}

	isDecorative := false
	if decorative, ok := optionsPayload["isDecorative"].(bool); ok {
		isDecorative = decorative
	}

	// Build PaneNode
	paneNode := &models.PaneNode{
		ID:             paneRow.ID,
		Title:          paneRow.Title,
		Slug:           paneRow.Slug,
		IsContextPane:  paneRow.IsContextPane == 1,
		IsDecorative:   isDecorative,
		Created:        created,
		Changed:        &changed,
		OptionsPayload: optionsPayload,
	}

	// Optional fields
	if bgColour != nil {
		paneNode.BgColour = bgColour
	}
	if codeHookTarget != nil {
		paneNode.CodeHookTarget = codeHookTarget
	}
	if len(codeHookPayload) > 0 {
		paneNode.CodeHookPayload = codeHookPayload
	}
	if len(heldBeliefs) > 0 {
		paneNode.HeldBeliefs = heldBeliefs
	}
	if len(withheldBeliefs) > 0 {
		paneNode.WithheldBeliefs = withheldBeliefs
	}

	return paneNode, nil
}

// extractBeliefs helper to parse belief structures from options payload
func (ps *PaneService) extractBeliefsAsArrays(optionsPayload map[string]any, key string) map[string][]string {
	beliefs := make(map[string][]string)

	if beliefsData, ok := optionsPayload[key]; ok {
		if beliefsMap, ok := beliefsData.(map[string]any); ok {
			for beliefKey, value := range beliefsMap {
				if valueArray, ok := value.([]any); ok {
					// Convert []any to []string
					stringArray := make([]string, len(valueArray))
					for i, v := range valueArray {
						if str, ok := v.(string); ok {
							stringArray[i] = str
						}
					}
					beliefs[beliefKey] = stringArray
				}
			}
		}
	}

	return beliefs
}
