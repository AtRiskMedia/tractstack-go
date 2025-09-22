// Package content provides storyfragments repository
package content

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/fts"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

type StoryFragmentRepository struct {
	db         *sql.DB
	cache      interfaces.ContentCache
	logger     *logging.ChanneledLogger
	ftsService *fts.FTSService
}

func NewStoryFragmentRepository(db *sql.DB, cache interfaces.ContentCache, logger *logging.ChanneledLogger, ftsService *fts.FTSService) *StoryFragmentRepository {
	return &StoryFragmentRepository{
		db:         db,
		cache:      cache,
		logger:     logger,
		ftsService: ftsService,
	}
}

func (r *StoryFragmentRepository) FindByID(tenantID, id string) (*content.StoryFragmentNode, error) {
	if storyFragment, found := r.cache.GetStoryFragment(tenantID, id); found {
		return storyFragment, nil
	}

	storyFragment, err := r.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if storyFragment == nil {
		return nil, nil
	}

	r.cache.SetStoryFragment(tenantID, storyFragment)
	return storyFragment, nil
}

func (r *StoryFragmentRepository) FindBySlug(tenantID, slug string) (*content.StoryFragmentNode, error) {
	id, err := r.getIDBySlugFromDB(slug)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}

	return r.FindByID(tenantID, id)
}

func (r *StoryFragmentRepository) FindByTractStackID(tenantID, tractStackID string) ([]*content.StoryFragmentNode, error) {
	ids, err := r.getIDsByTractStackFromDB(tractStackID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.StoryFragmentNode{}, nil
	}
	return r.FindByIDs(tenantID, ids)
}

// FindAll retrieves all storyfragments for a tenant, employing a cache-first strategy.
func (r *StoryFragmentRepository) FindAll(tenantID string) ([]*content.StoryFragmentNode, error) {
	// 1. Check cache for the master list of IDs first.
	if ids, found := r.cache.GetAllStoryFragmentIDs(tenantID); found {
		return r.FindByIDs(tenantID, ids)
	}

	// --- CACHE MISS FALLBACK ---
	// 2. Load all IDs from the database.
	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.StoryFragmentNode{}, nil
	}

	// 3. Set the master ID list in the cache immediately.
	r.cache.SetAllStoryFragmentIDs(tenantID, ids)

	// 4. Use the robust FindByIDs method to load the actual objects.
	return r.FindByIDs(tenantID, ids)
}

func (r *StoryFragmentRepository) FindByIDs(tenantID string, ids []string) ([]*content.StoryFragmentNode, error) {
	var result []*content.StoryFragmentNode
	var missingIDs []string

	for _, id := range ids {
		if storyFragment, found := r.cache.GetStoryFragment(tenantID, id); found {
			result = append(result, storyFragment)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingStoryFragments, err := r.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, storyFragment := range missingStoryFragments {
			r.cache.SetStoryFragment(tenantID, storyFragment)
			result = append(result, storyFragment)
		}
	}

	return result, nil
}

func (r *StoryFragmentRepository) Store(tenantID string, storyFragment *content.StoryFragmentNode) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for storyfragment store: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("failed to rollback transaction for storyfragment store", "error", err)
		}
	}()

	query := `INSERT INTO storyfragments (id, title, slug, tractstack_id, menu_id, tailwind_background_colour, social_image_path, created, changed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.Exec(query, storyFragment.ID, storyFragment.Title, storyFragment.Slug, storyFragment.TractStackID, storyFragment.MenuID, storyFragment.TailwindBgColour, storyFragment.SocialImagePath, storyFragment.Created, storyFragment.Changed); err != nil {
		return fmt.Errorf("failed to insert storyfragment: %w", err)
	}

	// Index title (description is not available on initial store)
	if err := r.ftsService.IndexStoryFragmentMetadata(tx, storyFragment.ID, storyFragment.Title, ""); err != nil {
		r.logger.System().Warn("Failed to index new storyfragment metadata", "error", err, "storyFragmentId", storyFragment.ID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction for storyfragment store: %w", err)
	}

	r.cache.SetStoryFragment(tenantID, storyFragment)
	return nil
}

func (r *StoryFragmentRepository) Update(tenantID string, storyFragment *content.StoryFragmentNode) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for storyfragment update: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("failed to rollback transaction for storyfragment update", "error", err)
		}
	}()

	query := `UPDATE storyfragments SET title = ?, slug = ?, tractstack_id = ?, menu_id = ?, tailwind_background_colour = ?, social_image_path = ?, changed = ? WHERE id = ?`
	if _, err := tx.Exec(query, storyFragment.Title, storyFragment.Slug, storyFragment.TractStackID, storyFragment.MenuID, storyFragment.TailwindBgColour, storyFragment.SocialImagePath, storyFragment.Changed, storyFragment.ID); err != nil {
		return fmt.Errorf("failed to update storyfragment: %w", err)
	}

	// For FTS, get existing description to re-index with new title
	var existingDesc string
	descQuery := `SELECT description FROM storyfragment_details WHERE storyfragment_id = ?`
	if err := tx.QueryRow(descQuery, storyFragment.ID).Scan(&existingDesc); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to query existing description for fts update: %w", err)
	}

	if err := r.ftsService.IndexStoryFragmentMetadata(tx, storyFragment.ID, storyFragment.Title, existingDesc); err != nil {
		r.logger.System().Warn("Failed to index updated storyfragment metadata", "error", err, "storyFragmentId", storyFragment.ID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction for storyfragment update: %w", err)
	}

	r.cache.SetStoryFragment(tenantID, storyFragment)
	return nil
}

func (r *StoryFragmentRepository) Delete(tenantID, id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(); err != nil {
				r.logger.Database().Error("Failed to rollback transaction", "error", err)
			}
		}
	}()

	start := time.Now()
	r.logger.Database().Debug("Executing storyfragment delete with cleanup", "id", id)

	// Delete related data first (to handle foreign key constraints)

	// 1. Delete pane relationships
	_, err = tx.Exec("DELETE FROM storyfragment_panes WHERE storyfragment_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete pane relationships: %w", err)
	}

	// 2. Delete topic relationships
	_, err = tx.Exec("DELETE FROM storyfragment_has_topic WHERE storyfragment_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete topic relationships: %w", err)
	}

	// 3. Delete description details
	_, err = tx.Exec("DELETE FROM storyfragment_details WHERE storyfragment_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete description details: %w", err)
	}

	// 4. Finally delete the main storyfragment record
	_, err = tx.Exec("DELETE FROM storyfragments WHERE id = ?", id)
	if err != nil {
		r.logger.Database().Error("Storyfragment delete failed", "error", err.Error(), "id", id)
		return fmt.Errorf("failed to delete storyfragment: %w", err)
	}

	// 5. Delete from FTS index
	if _, err = tx.Exec("DELETE FROM storyfragment_metadata_fts WHERE storyfragment_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete storyfragment fts metadata: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete transaction: %w", err)
	}
	committed = true

	r.logger.Database().Info("Storyfragment delete completed", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery("DELETE storyfragment with cleanup", duration, tenantID)
	}
	return nil
}

func (r *StoryFragmentRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM storyfragments ORDER BY title`

	start := time.Now()
	r.logger.Database().Debug("Loading all storyfragment IDs from database")

	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Database().Error("Failed to query storyfragment IDs", "error", err.Error())
		return nil, fmt.Errorf("failed to query storyfragments: %w", err)
	}
	defer rows.Close()

	var storyFragmentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment ID: %w", err)
		}
		storyFragmentIDs = append(storyFragmentIDs, id)
	}

	r.logger.Database().Info("Loaded storyfragment IDs from database", "count", len(storyFragmentIDs), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return storyFragmentIDs, rows.Err()
}

func (r *StoryFragmentRepository) loadFromDB(id string) (*content.StoryFragmentNode, error) {
	query := `SELECT id, title, slug, tractstack_id, menu_id, tailwind_background_colour, 
              social_image_path, created, changed 
              FROM storyfragments WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading storyfragment from database", "id", id)

	row := r.db.QueryRow(query, id)

	var sf content.StoryFragmentNode
	var menuID, tailwindBg, socialImage, changed sql.NullString
	var createdStr string

	err := row.Scan(&sf.ID, &sf.Title, &sf.Slug, &sf.TractStackID, &menuID,
		&tailwindBg, &socialImage, &createdStr, &changed)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to scan storyfragment", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to scan storyfragment: %w", err)
	}

	sf.Created, err = time.Parse(time.RFC3339, createdStr)
	if err != nil {
		sf.Created, err = time.Parse("2006-01-02 15:04:05", createdStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse created timestamp: %w", err)
		}
	}
	if changed.Valid {
		if changedTime, err := time.Parse("2006-01-02 15:04:05", changed.String); err == nil {
			sf.Changed = &changedTime
		}
	}

	if menuID.Valid {
		sf.MenuID = &menuID.String
	}
	if tailwindBg.Valid {
		sf.TailwindBgColour = &tailwindBg.String
	}
	if socialImage.Valid {
		sf.SocialImagePath = &socialImage.String
	}

	paneIDs, err := r.getPaneIDsForStoryFragment(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane relationships: %w", err)
	}
	sf.PaneIDs = paneIDs

	sf.NodeType = "StoryFragment"

	r.logger.Database().Info("Storyfragment loaded from database", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return &sf, nil
}

func (r *StoryFragmentRepository) loadMultipleFromDB(ids []string) ([]*content.StoryFragmentNode, error) {
	if len(ids) == 0 {
		return []*content.StoryFragmentNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, title, slug, tractstack_id, menu_id, tailwind_background_colour, 
              social_image_path, created, changed 
              FROM storyfragments WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	start := time.Now()
	r.logger.Database().Debug("Loading multiple storyfragments from database", "count", len(ids))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query multiple storyfragments", "error", err.Error(), "count", len(ids))
		return nil, fmt.Errorf("failed to query storyfragments: %w", err)
	}
	defer rows.Close()

	var storyFragments []*content.StoryFragmentNode
	var sfIDs []string

	for rows.Next() {
		var sf content.StoryFragmentNode
		var menuID, tailwindBg, socialImage, changed sql.NullString
		var createdStr string

		err := rows.Scan(&sf.ID, &sf.Title, &sf.Slug, &sf.TractStackID, &menuID,
			&tailwindBg, &socialImage, &createdStr, &changed)
		if err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment: %w", err)
		}

		sf.Created, err = time.Parse(time.RFC3339, createdStr)
		if err != nil {
			sf.Created, err = time.Parse("2006-01-02 15:04:05", createdStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse created timestamp: %w", err)
			}
		}
		if changed.Valid {
			if changedTime, err := time.Parse("2006-01-02 15:04:05", changed.String); err == nil {
				sf.Changed = &changedTime
			}
		}

		if menuID.Valid {
			sf.MenuID = &menuID.String
		}
		if tailwindBg.Valid {
			sf.TailwindBgColour = &tailwindBg.String
		}
		if socialImage.Valid {
			sf.SocialImagePath = &socialImage.String
		}

		sf.NodeType = "StoryFragment"
		storyFragments = append(storyFragments, &sf)
		sfIDs = append(sfIDs, sf.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	allPaneRelationships, err := r.getAllPaneRelationships(sfIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane relationships: %w", err)
	}

	for _, sf := range storyFragments {
		if paneIDs, exists := allPaneRelationships[sf.ID]; exists {
			sf.PaneIDs = paneIDs
		} else {
			sf.PaneIDs = make([]string, 0)
		}
	}

	r.logger.Database().Info("Multiple storyfragments loaded from database", "requested", len(ids), "loaded", len(storyFragments), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return storyFragments, nil
}

func (r *StoryFragmentRepository) getIDBySlugFromDB(slug string) (string, error) {
	query := `SELECT id FROM storyfragments WHERE slug = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading storyfragment ID by slug from database", "slug", slug)

	var id string
	err := r.db.QueryRow(query, slug).Scan(&id)
	if err == sql.ErrNoRows {
		r.logger.Database().Debug("Storyfragment not found by slug", "slug", slug)
		return "", nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to query storyfragment by slug", "error", err.Error(), "slug", slug)
		return "", fmt.Errorf("failed to query storyfragment by slug: %w", err)
	}

	r.logger.Database().Info("Storyfragment ID loaded by slug", "slug", slug, "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return id, nil
}

func (r *StoryFragmentRepository) getIDsByTractStackFromDB(tractStackID string) ([]string, error) {
	query := `SELECT id FROM storyfragments WHERE tractstack_id = ? ORDER BY title`

	start := time.Now()
	r.logger.Database().Debug("Loading storyfragment IDs by tractstack from database", "tractStackID", tractStackID)

	rows, err := r.db.Query(query, tractStackID)
	if err != nil {
		r.logger.Database().Error("Failed to query storyfragments by tractstack", "error", err.Error(), "tractStackID", tractStackID)
		return nil, fmt.Errorf("failed to query storyfragments by tractstack: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment ID: %w", err)
		}
		ids = append(ids, id)
	}

	r.logger.Database().Info("Storyfragment IDs loaded by tractstack", "tractStackID", tractStackID, "count", len(ids), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return ids, rows.Err()
}

func (r *StoryFragmentRepository) getPaneIDsForStoryFragment(storyFragmentID string) ([]string, error) {
	query := `SELECT pane_id FROM storyfragment_panes WHERE storyfragment_id = ? ORDER BY weight`

	start := time.Now()
	r.logger.Database().Debug("Loading pane relationships for storyfragment", "storyFragmentID", storyFragmentID)

	rows, err := r.db.Query(query, storyFragmentID)
	if err != nil {
		r.logger.Database().Error("Failed to query pane relationships", "error", err.Error(), "storyFragmentID", storyFragmentID)
		return nil, fmt.Errorf("failed to query pane relationships: %w", err)
	}
	defer rows.Close()

	paneIDs := make([]string, 0)
	for rows.Next() {
		var paneID string
		if err := rows.Scan(&paneID); err != nil {
			return nil, fmt.Errorf("failed to scan pane ID: %w", err)
		}
		paneIDs = append(paneIDs, paneID)
	}

	r.logger.Database().Info("Pane relationships loaded for storyfragment", "storyFragmentID", storyFragmentID, "paneCount", len(paneIDs), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return paneIDs, rows.Err()
}

func (r *StoryFragmentRepository) getAllPaneRelationships(storyFragmentIDs []string) (map[string][]string, error) {
	if len(storyFragmentIDs) == 0 {
		return make(map[string][]string), nil
	}

	placeholders := make([]string, len(storyFragmentIDs))
	args := make([]any, len(storyFragmentIDs))
	for i, id := range storyFragmentIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT storyfragment_id, pane_id FROM storyfragment_panes 
              WHERE storyfragment_id IN (` + strings.Join(placeholders, ",") + `) ORDER BY weight`

	start := time.Now()
	r.logger.Database().Debug("Loading all pane relationships", "storyFragmentCount", len(storyFragmentIDs))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query all pane relationships", "error", err.Error(), "storyFragmentCount", len(storyFragmentIDs))
		return nil, fmt.Errorf("failed to query pane relationships: %w", err)
	}
	defer rows.Close()

	relationships := make(map[string][]string)
	for rows.Next() {
		var storyFragmentID, paneID string
		if err := rows.Scan(&storyFragmentID, &paneID); err != nil {
			return nil, fmt.Errorf("failed to scan pane relationship: %w", err)
		}
		relationships[storyFragmentID] = append(relationships[storyFragmentID], paneID)
	}

	r.logger.Database().Info("All pane relationships loaded", "storyFragmentCount", len(storyFragmentIDs), "relationshipCount", len(relationships), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return relationships, rows.Err()
}

// UpdatePaneRelationships updates the storyfragment_panes relationships
func (r *StoryFragmentRepository) UpdatePaneRelationships(tenantID, storyFragmentID string, paneIDs []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(); err != nil {
				r.logger.Database().Error("Failed to rollback transaction", "error", err)
			}
		}
	}()

	// Delete existing relationships
	_, err = tx.Exec("DELETE FROM storyfragment_panes WHERE storyfragment_id = ?", storyFragmentID)
	if err != nil {
		return fmt.Errorf("failed to delete existing pane relationships: %w", err)
	}

	// Insert new relationships with weight
	if len(paneIDs) > 0 {
		stmt, err := tx.Prepare("INSERT INTO storyfragment_panes (id, storyfragment_id, pane_id, weight) VALUES (?, ?, ?, ?)")
		if err != nil {
			return fmt.Errorf("failed to prepare insert statement for pane relationships: %w", err)
		}
		defer func() {
			if err := stmt.Close(); err != nil {
				r.logger.Database().Error("Failed to close statement for pane relationships", "error", err)
			}
		}()

		for i, paneID := range paneIDs {
			_, err = stmt.Exec(security.GenerateULID(), storyFragmentID, paneID, i)
			if err != nil {
				return fmt.Errorf("failed to insert pane relationship for pane %s: %w", paneID, err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction for pane relationships: %w", err)
	}

	committed = true
	return nil
}

// UpdateTopics updates the topics for a storyfragment
func (r *StoryFragmentRepository) UpdateTopics(tenantID, storyFragmentID string, topics []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				r.logger.Database().Error("Failed to rollback transaction", "error", rollbackErr)
			}
		}
	}()

	// Delete existing topic relationships
	_, err = tx.Exec("DELETE FROM storyfragment_has_topic WHERE storyfragment_id = ?", storyFragmentID)
	if err != nil {
		return fmt.Errorf("failed to delete existing topic relationships: %w", err)
	}

	// Process each topic
	for _, topicTitle := range topics {
		var topicID int64

		// First try to get existing topic ID
		err = tx.QueryRow("SELECT id FROM storyfragment_topics WHERE title = ?", topicTitle).Scan(&topicID)
		if err != nil {
			if err == sql.ErrNoRows {
				// Topic doesn't exist, create it with explicit numeric ID
				var maxID int64
				err = tx.QueryRow("SELECT COALESCE(MAX(id), 0) FROM storyfragment_topics").Scan(&maxID)
				if err != nil {
					return fmt.Errorf("failed to get max topic ID: %w", err)
				}
				topicID = maxID + 1

				// Insert new topic with explicit ID
				_, err = tx.Exec("INSERT INTO storyfragment_topics (id, title) VALUES (?, ?)", topicID, topicTitle)
				if err != nil {
					return fmt.Errorf("failed to insert new topic: %w", err)
				}
			} else {
				return fmt.Errorf("failed to query existing topic: %w", err)
			}
		}

		// Create relationship
		_, err = tx.Exec("INSERT INTO storyfragment_has_topic (storyfragment_id, topic_id) VALUES (?, ?)",
			storyFragmentID, topicID)
		if err != nil {
			return fmt.Errorf("failed to create topic relationship: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true
	return nil
}

// UpdateDescription updates the description for a storyfragment
func (r *StoryFragmentRepository) UpdateDescription(tenantID, storyFragmentID string, description *string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for description update: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			r.logger.Database().Error("failed to rollback transaction for description update", "error", err)
		}
	}()

	var currentTitle string
	titleQuery := `SELECT title FROM storyfragments WHERE id = ?`
	if err := tx.QueryRow(titleQuery, storyFragmentID).Scan(&currentTitle); err != nil {
		return fmt.Errorf("failed to get current title for fts update: %w", err)
	}

	descText := ""
	if description != nil {
		descText = *description
		_, err = tx.Exec(`INSERT INTO storyfragment_details (storyfragment_id, description) VALUES (?, ?) ON CONFLICT(storyfragment_id) DO UPDATE SET description = excluded.description`, storyFragmentID, descText)
		if err != nil {
			return fmt.Errorf("failed to upsert description: %w", err)
		}
	} else {
		_, err = tx.Exec("DELETE FROM storyfragment_details WHERE storyfragment_id = ?", storyFragmentID)
		if err != nil {
			return fmt.Errorf("failed to delete description: %w", err)
		}
	}

	if err := r.ftsService.IndexStoryFragmentMetadata(tx, storyFragmentID, currentTitle, descText); err != nil {
		r.logger.System().Warn("Failed to index updated storyfragment metadata via description", "error", err, "storyFragmentId", storyFragmentID)
	}

	return tx.Commit()
}

// FindIDsByPaneID retrieves all storyfragment IDs that contain a specific pane ID.
func (r *StoryFragmentRepository) FindIDsByPaneID(paneID string) ([]string, error) {
	query := `SELECT DISTINCT storyfragment_id FROM storyfragment_panes WHERE pane_id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading storyfragment IDs by pane ID from database", "paneId", paneID)

	rows, err := r.db.Query(query, paneID)
	if err != nil {
		r.logger.Database().Error("Failed to query storyfragment IDs by pane ID", "error", err.Error(), "paneId", paneID)
		return nil, fmt.Errorf("failed to query storyfragments by pane ID: %w", err)
	}
	defer rows.Close()

	var storyFragmentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment ID: %w", err)
		}
		storyFragmentIDs = append(storyFragmentIDs, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error while finding storyfragments by pane ID: %w", err)
	}

	r.logger.Database().Info("Loaded storyfragment IDs by pane ID", "paneId", paneID, "count", len(storyFragmentIDs), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return storyFragmentIDs, nil
}

// SearchMetadata performs a prefix search on the storyfragment_metadata_fts table.
func (r *StoryFragmentRepository) SearchMetadata(tenantID, term string) ([]repositories.FTSResult, error) {
	query := `SELECT storyfragment_id, rank, snippet(storyfragment_metadata_fts, 1, '>>>', '<<<', '...', 1) FROM storyfragment_metadata_fts WHERE content MATCH ? ORDER BY rank LIMIT 10`
	searchTerm := term + "*"
	rows, err := r.db.Query(query, searchTerm)
	if err != nil {
		return nil, fmt.Errorf("failed to search storyfragment metadata: %w", err)
	}
	defer rows.Close()

	var results []repositories.FTSResult
	for rows.Next() {
		var res repositories.FTSResult
		if err := rows.Scan(&res.ID, &res.Relevance, &res.Term); err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment metadata search result: %w", err)
		}
		results = append(results, res)
	}
	return results, rows.Err()
}
