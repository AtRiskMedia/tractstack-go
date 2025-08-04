// Package content provides images repository
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

type ImageFileRepository struct {
	db     *sql.DB
	cache  interfaces.ContentCache
	logger *logging.ChanneledLogger
}

func NewImageFileRepository(db *sql.DB, cache interfaces.ContentCache, logger *logging.ChanneledLogger) *ImageFileRepository {
	return &ImageFileRepository{
		db:     db,
		cache:  cache,
		logger: logger,
	}
}

func (r *ImageFileRepository) FindByID(tenantID, id string) (*content.ImageFileNode, error) {
	if imageFile, found := r.cache.GetFile(tenantID, id); found {
		return imageFile, nil
	}

	imageFile, err := r.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if imageFile == nil {
		return nil, nil
	}

	r.cache.SetFile(tenantID, imageFile)
	return imageFile, nil
}

// FindAll retrieves all imagefiles for a tenant, employing a cache-first strategy.
func (r *ImageFileRepository) FindAll(tenantID string) ([]*content.ImageFileNode, error) {
	// 1. Check cache for the master list of IDs first.
	if ids, found := r.cache.GetAllFileIDs(tenantID); found {
		return r.FindByIDs(tenantID, ids)
	}

	// --- CACHE MISS FALLBACK ---
	// 2. Load all IDs from the database.
	ids, err := r.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*content.ImageFileNode{}, nil
	}

	// 3. Set the master ID list in the cache immediately.
	r.cache.SetAllFileIDs(tenantID, ids)

	// 4. Use the robust FindByIDs method to load the actual objects.
	return r.FindByIDs(tenantID, ids)
}

func (r *ImageFileRepository) FindByIDs(tenantID string, ids []string) ([]*content.ImageFileNode, error) {
	var result []*content.ImageFileNode
	var missingIDs []string

	for _, id := range ids {
		if imageFile, found := r.cache.GetFile(tenantID, id); found {
			result = append(result, imageFile)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		missingFiles, err := r.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		for _, imageFile := range missingFiles {
			r.cache.SetFile(tenantID, imageFile)
			result = append(result, imageFile)
		}
	}

	return result, nil
}

func (r *ImageFileRepository) Store(tenantID string, imageFile *content.ImageFileNode) error {
	query := `INSERT INTO files (id, filename, alt_description, url, src_set) VALUES (?, ?, ?, ?, ?)`

	start := time.Now()
	r.logger.Database().Debug("Executing file insert", "id", imageFile.ID)

	_, err := r.db.Exec(query, imageFile.ID, imageFile.Filename,
		imageFile.AltDescription, imageFile.URL, imageFile.SrcSet)
	if err != nil {
		r.logger.Database().Error("File insert failed", "error", err.Error(), "id", imageFile.ID)
		return fmt.Errorf("failed to insert file: %w", err)
	}

	r.logger.Database().Info("File insert completed", "id", imageFile.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetFile(tenantID, imageFile)
	return nil
}

func (r *ImageFileRepository) Update(tenantID string, imageFile *content.ImageFileNode) error {
	query := `UPDATE files SET filename = ?, alt_description = ?, url = ?, src_set = ? WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing file update", "id", imageFile.ID)

	_, err := r.db.Exec(query, imageFile.Filename, imageFile.AltDescription,
		imageFile.URL, imageFile.SrcSet, imageFile.ID)
	if err != nil {
		r.logger.Database().Error("File update failed", "error", err.Error(), "id", imageFile.ID)
		return fmt.Errorf("failed to update file: %w", err)
	}

	r.logger.Database().Info("File update completed", "id", imageFile.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	r.cache.SetFile(tenantID, imageFile)
	return nil
}

func (r *ImageFileRepository) Delete(tenantID, id string) error {
	query := `DELETE FROM files WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing file delete", "id", id)

	_, err := r.db.Exec(query, id)
	if err != nil {
		r.logger.Database().Error("File delete failed", "error", err.Error(), "id", id)
		return fmt.Errorf("failed to delete file: %w", err)
	}

	r.logger.Database().Info("File delete completed", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, tenantID)
	}
	return nil
}

func (r *ImageFileRepository) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM files ORDER BY filename`

	start := time.Now()
	r.logger.Database().Debug("Loading all file IDs from database")

	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Database().Error("Failed to query file IDs", "error", err.Error())
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var fileIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan file ID: %w", err)
		}
		fileIDs = append(fileIDs, id)
	}

	r.logger.Database().Info("Loaded file IDs from database", "count", len(fileIDs), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return fileIDs, rows.Err()
}

func (r *ImageFileRepository) loadFromDB(id string) (*content.ImageFileNode, error) {
	query := `SELECT id, filename, alt_description, url, src_set FROM files WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading file from database", "id", id)

	row := r.db.QueryRow(query, id)

	var imageFile content.ImageFileNode
	var srcSet sql.NullString

	err := row.Scan(&imageFile.ID, &imageFile.Filename, &imageFile.AltDescription,
		&imageFile.URL, &srcSet)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		r.logger.Database().Error("Failed to scan file", "error", err.Error(), "id", id)
		return nil, fmt.Errorf("failed to scan file: %w", err)
	}

	if srcSet.Valid {
		imageFile.SrcSet = &srcSet.String
	}

	imageFile.NodeType = "File"

	r.logger.Database().Info("File loaded from database", "id", id, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return &imageFile, nil
}

func (r *ImageFileRepository) loadMultipleFromDB(ids []string) ([]*content.ImageFileNode, error) {
	if len(ids) == 0 {
		return []*content.ImageFileNode{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, filename, alt_description, url, src_set 
              FROM files WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	start := time.Now()
	r.logger.Database().Debug("Loading multiple files from database", "count", len(ids))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.Database().Error("Failed to query multiple files", "error", err.Error(), "count", len(ids))
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var imageFiles []*content.ImageFileNode
	for rows.Next() {
		var imageFile content.ImageFileNode
		var srcSet sql.NullString

		err := rows.Scan(&imageFile.ID, &imageFile.Filename, &imageFile.AltDescription,
			&imageFile.URL, &srcSet)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}

		if srcSet.Valid {
			imageFile.SrcSet = &srcSet.String
		}

		imageFile.NodeType = "File"
		imageFiles = append(imageFiles, &imageFile)
	}

	r.logger.Database().Info("Multiple files loaded from database", "requested", len(ids), "loaded", len(imageFiles), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return imageFiles, rows.Err()
}
