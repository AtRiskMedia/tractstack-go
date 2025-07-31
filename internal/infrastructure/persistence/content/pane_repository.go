// Package content provides panes repository
package content

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
)

type PaneRepository struct {
	db    *sql.DB
	cache interfaces.ContentCache
}

func NewPaneRepository(db *sql.DB, cache interfaces.ContentCache) *PaneRepository {
	return &PaneRepository{
		db:    db,
		cache: cache,
	}
}

func (r *PaneRepository) FindByID(tenantID, id string) (*content.PaneNode, error) {
	if pane, found := r.cache.GetPane(tenantID, id); found {
		return pane, nil
	}

	pane, err := r.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if pane == nil {
		return nil, nil
	}

	r.cache.SetPane(tenantID, pane)
	return pane, nil
}

func (r *PaneRepository) FindBySlug(tenantID, slug string) (*content.PaneNode, error) {
	id, err := r.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}

	return r.FindByID(tenantID, id)
}

func (r *PaneRepository) FindByIDs(tenantID string, ids []string) ([]*content.PaneNode, error) {
	var result []*content.PaneNode
	var missingIDs []string

	for _, id := range ids {
		if pane, found := r.cache.GetPane(tenantID, id); found {
			result = append(result, pane)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingPanes, err := r.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, pane := range missingPanes {
			r.cache.SetPane(tenantID, pane)
			result = append(result, pane)
		}
	}

	return result, nil
}

func (r *PaneRepository) FindAll(tenantID string) ([]*content.PaneNode, error) {
	if ids, found := r.cache.GetAllPaneIDs(tenantID); found {
		var panes []*content.PaneNode
		var missingIDs []string

		for _, id := range ids {
			if pane, found := r.cache.GetPane(tenantID, id); found {
				panes = append(panes, pane)
			} else {
				missingIDs = append(missingIDs, id)
			}
		}

		if len(missingIDs) > 0 {
			missing, err := r.loadMultipleFromDB(missingIDs)
			if err != nil {
				return nil, err
			}

			for _, pane := range missing {
				r.cache.SetPane(tenantID, pane)
				panes = append(panes, pane)
			}
		}

		return panes, nil
	}

	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	panes, err := r.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	for _, pane := range panes {
		r.cache.SetPane(tenantID, pane)
	}

	return panes, nil
}

func (r *PaneRepository) Store(tenantID string, pane *content.PaneNode) error {
	optionsJSON, _ := json.Marshal(pane.OptionsPayload)

	query := `INSERT INTO panes (id, title, slug, pane_type, created, changed, options_payload, 
              is_context_pane, markdown_id) 
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.Exec(query, pane.ID, pane.Title, pane.Slug, "component",
		pane.Created, pane.Changed, string(optionsJSON), pane.IsContextPane, pane.MarkdownID)
	if err != nil {
		return fmt.Errorf("failed to insert pane: %w", err)
	}

	r.cache.SetPane(tenantID, pane)
	return nil
}

func (r *PaneRepository) Update(tenantID string, pane *content.PaneNode) error {
	optionsJSON, _ := json.Marshal(pane.OptionsPayload)

	query := `UPDATE panes SET title = ?, slug = ?, changed = ?, options_payload = ?, 
              is_context_pane = ?, markdown_id = ? WHERE id = ?`

	_, err := r.db.Exec(query, pane.Title, pane.Slug, pane.Changed, string(optionsJSON),
		pane.IsContextPane, pane.MarkdownID, pane.ID)
	if err != nil {
		return fmt.Errorf("failed to update pane: %w", err)
	}

	r.cache.SetPane(tenantID, pane)
	return nil
}

func (r *PaneRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM panes WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete pane: %w", err)
	}

	r.cache.InvalidateContentCache(tenantID)
	return nil
}

func (r *PaneRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM panes ORDER BY title`

	rows, err := r.db.Query(query)
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

func (r *PaneRepository) loadFromDB(id string) (*content.PaneNode, error) {
	query := `SELECT id, title, slug, pane_type, created, changed, options_payload, 
              is_context_pane, markdown_id 
              FROM panes WHERE id = ?`

	row := r.db.QueryRow(query, id)

	var pane content.PaneNode
	var paneType string
	var optionsPayloadStr string
	var markdownID sql.NullString
	var changed sql.NullString
	var createdStr string

	err := row.Scan(&pane.ID, &pane.Title, &pane.Slug, &paneType,
		&createdStr, &changed, &optionsPayloadStr, &pane.IsContextPane, &markdownID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan pane: %w", err)
	}

	if created, err := time.Parse("2006-01-02 15:04:05", createdStr); err == nil {
		pane.Created = created
	}
	if changed.Valid {
		if changedTime, err := time.Parse("2006-01-02 15:04:05", changed.String); err == nil {
			pane.Changed = &changedTime
		}
	}

	if err := json.Unmarshal([]byte(optionsPayloadStr), &pane.OptionsPayload); err != nil {
		return nil, fmt.Errorf("failed to parse options payload: %w", err)
	}

	if markdownID.Valid {
		pane.MarkdownID = &markdownID.String

		markdownBody, err := r.getMarkdownBody(markdownID.String)
		if err != nil {
			return nil, fmt.Errorf("failed to get markdown body: %w", err)
		}
		if markdownBody != "" {
			pane.MarkdownBody = &markdownBody
		}
	}

	r.extractPaneDataFromOptions(&pane)

	pane.NodeType = "Pane"

	return &pane, nil
}

func (r *PaneRepository) loadMultipleFromDB(ids []string) ([]*content.PaneNode, error) {
	if len(ids) == 0 {
		return []*content.PaneNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, pane_type, created, changed, options_payload, 
              is_context_pane, markdown_id 
              FROM panes WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query panes: %w", err)
	}
	defer rows.Close()

	var panes []*content.PaneNode
	var markdownIDs []string
	paneMarkdownMap := make(map[string]string)

	for rows.Next() {
		var pane content.PaneNode
		var paneType string
		var optionsPayloadStr string
		var markdownID sql.NullString
		var changed sql.NullString
		var createdStr string

		err := rows.Scan(&pane.ID, &pane.Title, &pane.Slug, &paneType,
			&createdStr, &changed, &optionsPayloadStr, &pane.IsContextPane, &markdownID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pane: %w", err)
		}

		if created, err := time.Parse("2006-01-02 15:04:05", createdStr); err == nil {
			pane.Created = created
		}
		if changed.Valid {
			if changedTime, err := time.Parse("2006-01-02 15:04:05", changed.String); err == nil {
				pane.Changed = &changedTime
			}
		}

		if err := json.Unmarshal([]byte(optionsPayloadStr), &pane.OptionsPayload); err != nil {
			return nil, fmt.Errorf("failed to parse options payload: %w", err)
		}

		if markdownID.Valid {
			pane.MarkdownID = &markdownID.String
			markdownIDs = append(markdownIDs, markdownID.String)
			paneMarkdownMap[pane.ID] = markdownID.String
		}

		r.extractPaneDataFromOptions(&pane)

		pane.NodeType = "Pane"
		panes = append(panes, &pane)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	if len(markdownIDs) > 0 {
		markdownMap, err := r.loadMultipleMarkdownFromDB(markdownIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to load markdown data: %w", err)
		}

		for _, pane := range panes {
			if markdownIDForPane, exists := paneMarkdownMap[pane.ID]; exists {
				if body, exists := markdownMap[markdownIDForPane]; exists {
					pane.MarkdownBody = &body
				}
			}
		}
	}

	return panes, nil
}

func (r *PaneRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM panes WHERE slug = ? LIMIT 1`

	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get pane by slug: %w", err)
	}

	return id, nil
}

func (r *PaneRepository) getMarkdownBody(id string) (string, error) {
	query := `SELECT body FROM markdowns WHERE id = ?`

	var body string
	err := r.db.QueryRow(query, id).Scan(&body)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query markdown: %w", err)
	}

	return body, nil
}

func (r *PaneRepository) loadMultipleMarkdownFromDB(ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return make(map[string]string), nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, body FROM markdowns WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query markdowns: %w", err)
	}
	defer rows.Close()

	markdownMap := make(map[string]string)
	for rows.Next() {
		var id, body string
		if err := rows.Scan(&id, &body); err != nil {
			return nil, fmt.Errorf("failed to scan markdown: %w", err)
		}
		markdownMap[id] = body
	}

	return markdownMap, rows.Err()
}

func (r *PaneRepository) extractPaneDataFromOptions(pane *content.PaneNode) {
	if pane.OptionsPayload == nil {
		return
	}

	if bg, ok := pane.OptionsPayload["bgColour"].(string); ok {
		pane.BgColour = &bg
	}

	if target, ok := pane.OptionsPayload["codeHookTarget"].(string); ok {
		pane.CodeHookTarget = &target
	}

	if payload, ok := pane.OptionsPayload["codeHookPayload"].(map[string]any); ok {
		pane.CodeHookPayload = make(map[string]string)
		for k, v := range payload {
			if str, ok := v.(string); ok {
				pane.CodeHookPayload[k] = str
			}
		}
	}

	if decorative, ok := pane.OptionsPayload["isDecorative"].(bool); ok {
		pane.IsDecorative = decorative
	}

	if held, ok := pane.OptionsPayload["heldBeliefs"].(map[string]any); ok {
		pane.HeldBeliefs = make(map[string][]string)
		for k, v := range held {
			if arr, ok := v.([]any); ok {
				var strs []string
				for _, item := range arr {
					if str, ok := item.(string); ok {
						strs = append(strs, str)
					}
				}
				pane.HeldBeliefs[k] = strs
			}
		}
	}

	if withheld, ok := pane.OptionsPayload["withheldBeliefs"].(map[string]any); ok {
		pane.WithheldBeliefs = make(map[string][]string)
		for k, v := range withheld {
			if arr, ok := v.([]any); ok {
				var strs []string
				for _, item := range arr {
					if str, ok := item.(string); ok {
						strs = append(strs, str)
					}
				}
				pane.WithheldBeliefs[k] = strs
			}
		}
	}
}

func (r *PaneRepository) FindContext(tenantID string) ([]*content.PaneNode, error) {
	// Get all panes first
	allPanes, err := r.FindAll(tenantID)
	if err != nil {
		return nil, err
	}

	// Filter for context panes
	var contextPanes []*content.PaneNode
	for _, pane := range allPanes {
		if pane.IsContextPane {
			contextPanes = append(contextPanes, pane)
		}
	}

	return contextPanes, nil
}
