// Package content provides imagefiles
package content

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// ImageFileRowData represents raw database structure
type ImageFileRowData struct {
	ID             string  `json:"id"`
	Filename       string  `json:"filename"`
	AltDescription string  `json:"alt_description"`
	URL            string  `json:"url"`
	SrcSet         *string `json:"src_set,omitempty"`
}

// ImageFileService handles cache-first imagefile operations
type ImageFileService struct {
	ctx *tenant.Context
}

// NewImageFileService creates a cache-first imagefile service
func NewImageFileService(ctx *tenant.Context, _ any) *ImageFileService {
	// Ignore the cache manager parameter - we use the global instance directly
	return &ImageFileService{
		ctx: ctx,
	}
}

// GetAllIDs returns all imagefile IDs (cache-first)
func (ifs *ImageFileService) GetAllIDs() ([]string, error) {
	// Check cache first
	if ids, found := cache.GetGlobalManager().GetAllFileIDs(ifs.ctx.TenantID); found {
		return ids, nil
	}

	// Cache miss - load from database
	ids, err := ifs.loadAllIDsFromDB()
	if err != nil {
		return nil, err
	}

	// Load all imagefiles to populate cache
	imagefiles, err := ifs.loadMultipleFromDB(ids)
	if err != nil {
		return nil, err
	}

	// Populate cache
	for _, imagefile := range imagefiles {
		cache.GetGlobalManager().SetFile(ifs.ctx.TenantID, imagefile)
	}

	return ids, nil
}

// GetByID returns an imagefile by ID (cache-first)
func (ifs *ImageFileService) GetByID(id string) (*models.ImageFileNode, error) {
	// Check cache first
	if imagefile, found := cache.GetGlobalManager().GetFile(ifs.ctx.TenantID, id); found {
		return imagefile, nil
	}

	// Cache miss - load from database
	imagefile, err := ifs.loadFromDB(id)
	if err != nil {
		return nil, err
	}
	if imagefile == nil {
		return nil, nil // Not found
	}

	// Populate cache
	cache.GetGlobalManager().SetFile(ifs.ctx.TenantID, imagefile)

	return imagefile, nil
}

// GetByIDs returns multiple imagefiles by IDs (cache-first with bulk loading)
func (ifs *ImageFileService) GetByIDs(ids []string) ([]*models.ImageFileNode, error) {
	var result []*models.ImageFileNode
	var missingIDs []string

	// Check cache for each ID
	for _, id := range ids {
		if imagefile, found := cache.GetGlobalManager().GetFile(ifs.ctx.TenantID, id); found {
			result = append(result, imagefile)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// If we have cache misses, bulk load from database
	if len(missingIDs) > 0 {
		missingImageFiles, err := ifs.loadMultipleFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		// Add to cache and result
		for _, imagefile := range missingImageFiles {
			cache.GetGlobalManager().SetFile(ifs.ctx.TenantID, imagefile)
			result = append(result, imagefile)
		}
	}

	return result, nil
}

// Private database loading methods

// loadAllIDsFromDB fetches all imagefile IDs from database
func (ifs *ImageFileService) loadAllIDsFromDB() ([]string, error) {
	query := `SELECT id FROM files ORDER BY filename`

	rows, err := ifs.ctx.Database.Conn.Query(query)
	if err != nil {
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return fileIDs, nil
}

// loadFromDB loads a complete imagefile from database
func (ifs *ImageFileService) loadFromDB(id string) (*models.ImageFileNode, error) {
	// Get imagefile row data
	imagefileRow, err := ifs.getImageFileRowData(id)
	if err != nil {
		return nil, err
	}
	if imagefileRow == nil {
		return nil, nil
	}

	// Deserialize to ImageFileNode
	imagefileNode := ifs.deserializeRowData(imagefileRow)

	return imagefileNode, nil
}

// loadMultipleFromDB loads multiple imagefiles from database using IN clause
func (ifs *ImageFileService) loadMultipleFromDB(ids []string) ([]*models.ImageFileNode, error) {
	if len(ids) == 0 {
		return []*models.ImageFileNode{}, nil
	}

	// Build IN clause with placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, filename, alt_description, url, src_set 
          FROM files WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := ifs.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var imagefiles []*models.ImageFileNode

	// Process all rows
	for rows.Next() {
		var imagefileRow ImageFileRowData
		var srcSet sql.NullString

		err := rows.Scan(
			&imagefileRow.ID,
			&imagefileRow.Filename,
			&imagefileRow.AltDescription,
			&imagefileRow.URL,
			&srcSet,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}

		if srcSet.Valid {
			imagefileRow.SrcSet = &srcSet.String
		}

		imagefileNode := ifs.deserializeRowData(&imagefileRow)
		imagefiles = append(imagefiles, imagefileNode)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return imagefiles, nil
}

// getImageFileRowData fetches raw imagefile data from database
func (ifs *ImageFileService) getImageFileRowData(id string) (*ImageFileRowData, error) {
	query := `SELECT id, filename, alt_description, url, src_set FROM files WHERE id = ?`

	row := ifs.ctx.Database.Conn.QueryRow(query, id)

	var imagefileRow ImageFileRowData
	var srcSet sql.NullString

	err := row.Scan(
		&imagefileRow.ID,
		&imagefileRow.Filename,
		&imagefileRow.AltDescription,
		&imagefileRow.URL,
		&srcSet,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan file: %w", err)
	}

	if srcSet.Valid {
		imagefileRow.SrcSet = &srcSet.String
	}

	return &imagefileRow, nil
}

// deserializeRowData converts database rows to client ImageFileNode
func (ifs *ImageFileService) deserializeRowData(imagefileRow *ImageFileRowData) *models.ImageFileNode {
	// Build ImageFileNode
	imagefileNode := &models.ImageFileNode{
		ID:             imagefileRow.ID,
		Filename:       imagefileRow.Filename,
		AltDescription: imagefileRow.AltDescription,
		URL:            imagefileRow.URL,
	}

	// Optional fields
	if imagefileRow.SrcSet != nil {
		imagefileNode.SrcSet = imagefileRow.SrcSet
	}

	return imagefileNode
}
