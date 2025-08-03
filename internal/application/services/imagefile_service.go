// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// ImageFileService orchestrates imagefile operations with cache-first repository pattern
type ImageFileService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewImageFileService creates a new imagefile service singleton
func NewImageFileService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ImageFileService {
	return &ImageFileService{
		logger:      logger,
		perfTracker: perfTracker,
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
