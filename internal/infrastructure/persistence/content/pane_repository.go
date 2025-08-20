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
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

type PaneRepository struct {
	db     *sql.DB
	cache  interfaces.ContentCache
	logger *logging.ChanneledLogger
}

func NewPaneRepository(db *sql.DB, cache interfaces.ContentCache, logger *logging.ChanneledLogger) *PaneRepository {
	return &PaneRepository{
		db:     db,
		cache:  cache,
		logger: logger,
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

// FindAll retrieves all panes for a tenant, employing a cache-first strategy.
func (r *PaneRepository) FindAll(tenantID string) ([]*content.PaneNode, error) {
	// 1. Check cache for the master list of IDs first.
	if ids, found := r.cache.GetAllPaneIDs(tenantID); found {
		return r.FindByIDs(tenantID, ids)
	}

	// --- CACHE MISS FALLBACK ---
	// 2. Load all IDs from the database.
	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.PaneNode{}, nil
	}

	// 3. Set the master ID list in the cache immediately.
	r.cache.SetAllPaneIDs(tenantID, ids)

	// 4. Use the robust FindByIDs method to load the actual objects.
	return r.FindByIDs(tenantID, ids)
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

func (r *PaneRepository) Store(tenantID string, pane *content.PaneNode) error {
	optionsJSON, _ := json.Marshal(pane.OptionsPayload)

	query := `INSERT INTO panes (id, title, slug, pane_type, created, changed, options_payload, 
              is_context_pane, markdown_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	start := time.Now()
	r.logger.Database().Debug("Executing pane insert", "id", pane.ID)

	_, err := r.db.Exec(query, pane.ID, pane.Title, pane.Slug, "component",
		pane.Created, pane.Changed, string(optionsJSON), pane.IsContextPane, pane.MarkdownID)
	if err != nil {
		r.logger.Database().Error("Pane insert failed", "error", err.Error(), "id", pane.ID)
		return fmt.Errorf("failed to insert pane: %w", err)
	}

	r.logger.Database().Info("Pane insert completed", "id", pane.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetPane(tenantID, pane)
	return nil
}

func (r *PaneRepository) Update(tenantID string, pane *content.PaneNode) error {
	optionsJSON, _ := json.Marshal(pane.OptionsPayload)

	query := `UPDATE panes SET title = ?, slug = ?, changed = ?, options_payload = ?, 
              is_context_pane = ?, markdown_id = ? WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing pane update", "id", pane.ID)

	_, err := r.db.Exec(query, pane.Title, pane.Slug, pane.Changed, string(optionsJSON),
		pane.IsContextPane, pane.MarkdownID, pane.ID)
	if err != nil {
		r.logger.Database().Error("Pane update failed", "error", err.Error(), "id", pane.ID)
		return fmt.Errorf("failed to update pane: %w", err)
	}

	r.logger.Database().Info("Pane update completed", "id", pane.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetPane(tenantID, pane)
	return nil
}

func (r *PaneRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM panes WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing pane delete", "id", id)

	_, err := r.db.Exec(query, id)
	if err != nil {
		r.logger.Database().Error("Pane delete failed", "error", err.Error(), "id", id)
		return fmt.Errorf("failed to delete pane: %w", err)
	}

	r.logger.Database().Info("Pane delete completed", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	return nil
}

func (r *PaneRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM panes ORDER BY title`

	start := time.Now()
	r.logger.Database().Debug("Loading all pane IDs from database")

	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Database().Error("Failed to query pane IDs", "error", err.Error())
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

	r.logger.Database().Info("Loaded pane IDs from database", "count", len(paneIDs), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return paneIDs, nil
}

func (r *PaneRepository) loadFromDB(id string) (*content.PaneNode, error) {
	query := `SELECT id, title, slug, pane_type, created, changed, options_payload, 
              is_context_pane, markdown_id 
              FROM panes WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading pane from database", "id", id)

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
		r.logger.Database().Error("Failed to scan pane", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to scan pane: %w", err)
	}

	pane.Created, err = time.Parse(time.RFC3339, createdStr)
	if err != nil {
		pane.Created, err = time.Parse("2006-01-02 15:04:05", createdStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse created timestamp: %w", err)
		}
	}
	if changed.Valid {
		if changedTime, err := time.Parse("2006-01-02 15:04:05", changed.String); err == nil {
			pane.Changed = &changedTime
		}
	}

	if err := json.Unmarshal([]byte(optionsPayloadStr), &pane.OptionsPayload); err != nil {
		r.logger.Database().Error("Failed to parse pane options payload", "error", err.Error(), "id", id)
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

	r.logger.Database().Info("Pane loaded from database", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
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

	start := time.Now()
	r.logger.Database().Debug("Loading multiple panes from database", "count", len(ids))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query multiple panes", "error", err.Error(), "count", len(ids))
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

		pane.Created, err = time.Parse(time.RFC3339, createdStr)
		if err != nil {
			pane.Created, err = time.Parse("2006-01-02 15:04:05", createdStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse created timestamp: %w", err)
			}
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

	r.logger.Database().Info("Multiple panes loaded from database", "requested", len(ids), "loaded", len(panes), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return panes, nil
}

func (r *PaneRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM panes WHERE slug = ? LIMIT 1`

	start := time.Now()
	r.logger.Database().Debug("Loading pane ID by slug from database", "slug", slug)

	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		r.logger.Database().Debug("Pane not found by slug", "slug", slug)
		return "", nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to query pane by slug", "error", err.Error(), "slug", slug)
		return "", fmt.Errorf("failed to get pane by slug: %w", err)
	}

	r.logger.Database().Info("Pane ID loaded by slug", "slug", slug, "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return id, nil
}

func (r *PaneRepository) getMarkdownBody(id string) (string, error) {
	query := `SELECT body FROM markdowns WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading markdown body", "id", id)

	var body string
	err := r.db.QueryRow(query, id).Scan(&body)
	if err == sql.ErrNoRows {
		r.logger.Database().Debug("Markdown not found", "id", id)
		return "", nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to query markdown", "error", err.Error(), "id", id)
		return "", fmt.Errorf("failed to query markdown: %w", err)
	}

	r.logger.Database().Info("Markdown body loaded", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
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

	start := time.Now()
	r.logger.Database().Debug("Loading multiple markdown bodies", "count", len(ids))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query multiple markdowns", "error", err.Error(), "count", len(ids))
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

	r.logger.Database().Info("Multiple markdown bodies loaded", "requested", len(ids), "loaded", len(markdownMap), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
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
	// Get all panes first, relying on the now-robust FindAll method.
	allPanes, err := r.FindAll(tenantID)
	if err != nil {
		return nil, err
	}

	// Filter for context panes in-memory.
	var contextPanes []*content.PaneNode
	for _, pane := range allPanes {
		if pane.IsContextPane {
			contextPanes = append(contextPanes, pane)
		}
	}

	return contextPanes, nil
}
