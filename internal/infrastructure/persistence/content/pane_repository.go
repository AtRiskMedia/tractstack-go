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
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
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
	if ids, found := r.cache.GetAllPaneIDs(tenantID); found {
		return r.FindByIDs(tenantID, ids)
	}

	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.PaneNode{}, nil
	}

	r.cache.SetAllPaneIDs(tenantID, ids)

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

// Store creates a pane and its associated markdown content within a single transaction.
func (r *PaneRepository) Store(tenantID string, pane *content.PaneNode, markdownBody string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("Failed to rollback Store transaction", "error", err)
		}
	}()

	if markdownBody != "" {
		markdownID := security.GenerateULID()
		pane.MarkdownID = &markdownID
		if _, err := tx.Exec(`INSERT INTO markdowns (id, body) VALUES (?, ?)`, markdownID, markdownBody); err != nil {
			return fmt.Errorf("failed to insert markdown: %w", err)
		}
	} else {
		pane.MarkdownID = nil
	}

	optionsJSON, _ := json.Marshal(pane.OptionsPayload)
	query := `INSERT INTO panes (id, title, slug, pane_type, created, changed, options_payload, is_context_pane, markdown_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.Exec(query, pane.ID, pane.Title, pane.Slug, "component", pane.Created, pane.Changed, string(optionsJSON), pane.IsContextPane, pane.MarkdownID); err != nil {
		return fmt.Errorf("failed to insert pane: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	r.cache.SetPane(tenantID, pane)
	return nil
}

// Update modifies a pane and its associated markdown within a single transaction.
// It handles creating, updating, or deleting the markdown record as needed.
func (r *PaneRepository) Update(tenantID string, pane *content.PaneNode, markdownBody string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("Failed to rollback Update transaction", "error", err)
		}
	}()

	var existingMarkdownID sql.NullString
	if err := tx.QueryRow(`SELECT markdown_id FROM panes WHERE id = ?`, pane.ID).Scan(&existingMarkdownID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to query existing markdown_id: %w", err)
	}

	if markdownBody != "" {
		if existingMarkdownID.Valid {
			pane.MarkdownID = &existingMarkdownID.String
			if _, err := tx.Exec(`UPDATE markdowns SET body = ? WHERE id = ?`, markdownBody, *pane.MarkdownID); err != nil {
				return fmt.Errorf("failed to update markdown: %w", err)
			}
		} else {
			markdownID := security.GenerateULID()
			pane.MarkdownID = &markdownID
			if _, err := tx.Exec(`INSERT INTO markdowns (id, body) VALUES (?, ?)`, markdownID, markdownBody); err != nil {
				return fmt.Errorf("failed to insert markdown: %w", err)
			}
		}
	} else {
		if existingMarkdownID.Valid {
			if _, err := tx.Exec(`DELETE FROM markdowns WHERE id = ?`, existingMarkdownID.String); err != nil {
				return fmt.Errorf("failed to delete unused markdown: %w", err)
			}
		}
		pane.MarkdownID = nil
	}

	optionsJSON, _ := json.Marshal(pane.OptionsPayload)
	query := `UPDATE panes SET title = ?, slug = ?, changed = ?, options_payload = ?, is_context_pane = ?, markdown_id = ? WHERE id = ?`
	if _, err := tx.Exec(query, pane.Title, pane.Slug, pane.Changed, string(optionsJSON), pane.IsContextPane, pane.MarkdownID, pane.ID); err != nil {
		return fmt.Errorf("failed to update pane: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	r.cache.SetPane(tenantID, pane)
	return nil
}

// Delete removes a pane and its associated markdown record within a single transaction.
func (r *PaneRepository) Delete(tenantID, id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("Failed to rollback Delete transaction", "error", err)
		}
	}()

	var markdownID sql.NullString
	if err := tx.QueryRow(`SELECT markdown_id FROM panes WHERE id = ?`, id).Scan(&markdownID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to query markdown_id for deletion: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM panes WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete pane: %w", err)
	}

	if markdownID.Valid {
		if _, err := tx.Exec(`DELETE FROM markdowns WHERE id = ?`, markdownID.String); err != nil {
			return fmt.Errorf("failed to delete associated markdown: %w", err)
		}
	}

	return tx.Commit()
}

func (r *PaneRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM panes ORDER BY title`
	start := time.Now()
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
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return paneIDs, nil
}

func (r *PaneRepository) loadFromDB(id string) (*content.PaneNode, error) {
	query := `SELECT id, title, slug, pane_type, created, changed, options_payload, is_context_pane, markdown_id FROM panes WHERE id = ?`
	start := time.Now()
	row := r.db.QueryRow(query, id)

	var pane content.PaneNode
	var paneType, optionsPayloadStr, createdStr string
	var markdownID, changed sql.NullString

	err := row.Scan(&pane.ID, &pane.Title, &pane.Slug, &paneType, &createdStr, &changed, &optionsPayloadStr, &pane.IsContextPane, &markdownID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
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
	query := `SELECT id, title, slug, pane_type, created, changed, options_payload, is_context_pane, markdown_id FROM panes WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	start := time.Now()
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
		var paneType, optionsPayloadStr, createdStr string
		var markdownID, changed sql.NullString

		err := rows.Scan(&pane.ID, &pane.Title, &pane.Slug, &paneType, &createdStr, &changed, &optionsPayloadStr, &pane.IsContextPane, &markdownID)
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
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return panes, nil
}

func (r *PaneRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM panes WHERE slug = ? LIMIT 1`
	start := time.Now()
	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get pane by slug: %w", err)
	}
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return id, nil
}

func (r *PaneRepository) getMarkdownBody(id string) (string, error) {
	query := `SELECT body FROM markdowns WHERE id = ?`
	start := time.Now()
	var body string
	err := r.db.QueryRow(query, id).Scan(&body)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query markdown: %w", err)
	}
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
	allPanes, err := r.FindAll(tenantID)
	if err != nil {
		return nil, err
	}
	var contextPanes []*content.PaneNode
	for _, pane := range allPanes {
		if pane.IsContextPane {
			contextPanes = append(contextPanes, pane)
		}
	}
	return contextPanes, nil
}

// UpdateFilePaneRelationships updates file-pane relationships for multiple panes
func (r *PaneRepository) UpdateFilePaneRelationships(tenantID string, relationships map[string][]string) error {
	start := time.Now()
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("Failed to rollback UpdateFilePaneRelationships transaction", "error", err)
		}
	}()

	totalRelationships := 0

	for paneID, fileIDs := range relationships {
		if _, err := tx.Exec("DELETE FROM file_panes WHERE pane_id = ?", paneID); err != nil {
			return fmt.Errorf("failed to delete existing file-pane relationships for pane %s: %w", paneID, err)
		}

		for _, fileID := range fileIDs {
			relationshipID := security.GenerateULID()

			if _, err := tx.Exec("INSERT INTO file_panes (id, file_id, pane_id) VALUES (?, ?, ?)",
				relationshipID, fileID, paneID); err != nil {
				return fmt.Errorf("failed to insert file-pane relationship for pane %s, file %s: %w", paneID, fileID, err)
			}
			totalRelationships++
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery("BULK UPDATE file_panes", duration, tenantID)
	}

	return nil
}
