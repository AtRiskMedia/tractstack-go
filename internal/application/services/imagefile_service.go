// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// ImageFileService orchestrates imagefile operations with cache-first repository pattern
type ImageFileService struct {
	logger            *logging.ChanneledLogger
	perfTracker       *performance.Tracker
	contentMapService *ContentMapService
}

// NewImageFileService creates a new imagefile service singleton
func NewImageFileService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker, contentMapService *ContentMapService) *ImageFileService {
	return &ImageFileService{
		logger:            logger,
		perfTracker:       perfTracker,
		contentMapService: contentMapService,
	}
}

// GetAllIDs returns all imagefile IDs for a tenant by leveraging the robust repository.
func (s *ImageFileService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_all_imagefile_ids", tenantCtx.TenantID)
	defer marker.Complete()
	imageFileRepo := tenantCtx.ImageFileRepo()

	// The repository's FindAll method is now the cache-aware entry point.
	imageFiles, err := imageFileRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all imagefiles from repository: %w", err)
	}

	// Extract IDs from the full objects.
	ids := make([]string, len(imageFiles))
	for i, imageFile := range imageFiles {
		ids[i] = imageFile.ID
	}

	s.logger.Content().Info("Successfully retrieved all imagefile IDs", "tenantId", tenantCtx.TenantID, "count", len(ids), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetAllImageFileIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return ids, nil
}

// GetByID returns an imagefile by ID (cache-first via repository)
func (s *ImageFileService) GetByID(tenantCtx *tenant.Context, id string) (*content.ImageFileNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_imagefile_by_id", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return nil, fmt.Errorf("imagefile ID cannot be empty")
	}

	imageFileRepo := tenantCtx.ImageFileRepo()
	imageFile, err := imageFileRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get imagefile %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved imagefile by ID", "tenantId", tenantCtx.TenantID, "imagefileId", id, "found", imageFile != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetImageFileByID", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "imageFileId", id)

	return imageFile, nil
}

// GetByIDs returns multiple imagefiles by IDs (cache-first with bulk loading via repository)
func (s *ImageFileService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.ImageFileNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_imagefiles_by_ids", tenantCtx.TenantID)
	defer marker.Complete()
	if len(ids) == 0 {
		return []*content.ImageFileNode{}, nil
	}

	imageFileRepo := tenantCtx.ImageFileRepo()
	imageFiles, err := imageFileRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get imagefiles by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved imagefiles by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(imageFiles), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetImageFilesByIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(ids))

	return imageFiles, nil
}

// Create creates a new imagefile
func (s *ImageFileService) Create(tenantCtx *tenant.Context, imageFile *content.ImageFileNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("create_imagefile", tenantCtx.TenantID)
	defer marker.Complete()
	if imageFile.ID == "" {
		imageFile.ID = security.GenerateULID()
	}
	if imageFile == nil {
		return fmt.Errorf("imagefile cannot be nil")
	}
	if imageFile.Filename == "" {
		return fmt.Errorf("imagefile filename cannot be empty")
	}
	if imageFile.Src == "" {
		return fmt.Errorf("imagefile Src cannot be empty")
	}

	imageFileRepo := tenantCtx.ImageFileRepo()
	err := imageFileRepo.Store(tenantCtx.TenantID, imageFile)
	if err != nil {
		return fmt.Errorf("failed to create imagefile %s: %w", imageFile.ID, err)
	}

	// Surgically add the new item to the item cache and the master ID list
	tenantCtx.CacheManager.SetFile(tenantCtx.TenantID, imageFile)
	tenantCtx.CacheManager.AddFileID(tenantCtx.TenantID, imageFile.ID)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after imagefile creation",
			"error", err, "imageFileId", imageFile.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully created imagefile", "tenantId", tenantCtx.TenantID, "imagefileId", imageFile.ID, "filename", imageFile.Filename, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for CreateImageFile", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "imageFileId", imageFile.ID)

	return nil
}

// Update updates an existing imagefile
func (s *ImageFileService) Update(tenantCtx *tenant.Context, imageFile *content.ImageFileNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("update_imagefile", tenantCtx.TenantID)
	defer marker.Complete()
	if imageFile == nil {
		return fmt.Errorf("imagefile cannot be nil")
	}
	if imageFile.ID == "" {
		return fmt.Errorf("imagefile ID cannot be empty")
	}
	if imageFile.Filename == "" {
		return fmt.Errorf("imagefile filename cannot be empty")
	}
	if imageFile.Src == "" {
		return fmt.Errorf("imagefile Src cannot be empty")
	}

	imageFileRepo := tenantCtx.ImageFileRepo()

	existing, err := imageFileRepo.FindByID(tenantCtx.TenantID, imageFile.ID)
	if err != nil {
		return fmt.Errorf("failed to verify imagefile %s exists: %w", imageFile.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("imagefile %s not found", imageFile.ID)
	}

	err = imageFileRepo.Update(tenantCtx.TenantID, imageFile)
	if err != nil {
		return fmt.Errorf("failed to update imagefile %s: %w", imageFile.ID, err)
	}

	// Surgically update the item in the item cache. The ID list is not affected.
	tenantCtx.CacheManager.SetFile(tenantCtx.TenantID, imageFile)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after imagefile update",
			"error", err, "imageFileId", imageFile.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully updated imagefile", "tenantId", tenantCtx.TenantID, "imagefileId", imageFile.ID, "filename", imageFile.Filename, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for UpdateImageFile", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "imageFileId", imageFile.ID)

	return nil
}

// Delete deletes an imagefile from the database and removes its associated physical files from disk.
func (s *ImageFileService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("delete_imagefile", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return fmt.Errorf("imagefile ID cannot be empty")
	}

	imageFileRepo := tenantCtx.ImageFileRepo()

	// Step 1: Find the file record to get its paths before deleting it.
	imageFileToDelete, err := imageFileRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify imagefile %s exists: %w", id, err)
	}
	// If the record doesn't exist, we can consider the operation successful.
	if imageFileToDelete == nil {
		s.logger.Content().Warn("Attempted to delete a non-existent imagefile record", "tenantId", tenantCtx.TenantID, "imagefileId", id)
		return nil
	}

	// Step 2: Delete the physical files from disk.
	mediaPath := filepath.Join(config.BackendPath, "config", tenantCtx.TenantID, "media")
	var pathsToDelete []string

	// Add the main src file
	if imageFileToDelete.Src != "" {
		pathsToDelete = append(pathsToDelete, imageFileToDelete.Src)
	}

	// Add all files from the srcSet
	if imageFileToDelete.SrcSet != nil && *imageFileToDelete.SrcSet != "" {
		// Example srcSet: "/path1 1920w, /path2 1080w"
		pairs := strings.Split(*imageFileToDelete.SrcSet, ",")
		for _, pair := range pairs {
			parts := strings.Fields(strings.TrimSpace(pair))
			if len(parts) > 0 {
				pathsToDelete = append(pathsToDelete, parts[0])
			}
		}
	}

	for _, relativePath := range pathsToDelete {
		// Convert relative URL path to absolute disk path
		// e.g., /media/images/resources/file.webp -> /server/config/tenant/media/images/resources/file.webp
		if !strings.HasPrefix(relativePath, "/media/") {
			s.logger.Content().Warn("Cannot delete file with invalid path prefix", "path", relativePath)
			continue
		}
		absolutePath := filepath.Join(mediaPath, strings.TrimPrefix(relativePath, "/media/"))

		err := os.Remove(absolutePath)
		// We log an error but don't fail the whole operation if a file doesn't exist.
		if err != nil && !os.IsNotExist(err) {
			s.logger.Content().Error("Failed to delete physical image file", "path", absolutePath, "error", err)
		} else if err == nil {
			s.logger.Content().Debug("Successfully deleted physical image file", "path", absolutePath)
		}
	}

	// Step 3: Delete the database record.
	err = imageFileRepo.Delete(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete imagefile record %s: %w", id, err)
	}

	// Step 4: Invalidate caches.
	tenantCtx.CacheManager.InvalidateFile(tenantCtx.TenantID, id)
	tenantCtx.CacheManager.RemoveFileID(tenantCtx.TenantID, id)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after imagefile deletion",
			"error", err, "imageFileId", id, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully deleted imagefile", "tenantId", tenantCtx.TenantID, "imagefileId", id, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for DeleteImageFile", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "imageFileId", id)

	return nil
}
