// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// ImageFileService orchestrates imagefile operations with cache-first repository pattern
type ImageFileService struct {
	// No stored dependencies - all passed via tenant context
}

// NewImageFileService creates a new imagefile service singleton
func NewImageFileService() *ImageFileService {
	return &ImageFileService{}
}

// GetAllIDs returns all imagefile IDs for a tenant (cache-first)
func (s *ImageFileService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	imageFileRepo := tenantCtx.ImageFileRepo()

	imageFiles, err := imageFileRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all imagefiles: %w", err)
	}

	ids := make([]string, len(imageFiles))
	for i, imageFile := range imageFiles {
		ids[i] = imageFile.ID
	}

	return ids, nil
}

// GetByID returns an imagefile by ID (cache-first)
func (s *ImageFileService) GetByID(tenantCtx *tenant.Context, id string) (*content.ImageFileNode, error) {
	if id == "" {
		return nil, fmt.Errorf("imagefile ID cannot be empty")
	}

	imageFileRepo := tenantCtx.ImageFileRepo()
	imageFile, err := imageFileRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get imagefile %s: %w", id, err)
	}

	return imageFile, nil
}

// GetByIDs returns multiple imagefiles by IDs (cache-first with bulk loading)
func (s *ImageFileService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.ImageFileNode, error) {
	if len(ids) == 0 {
		return []*content.ImageFileNode{}, nil
	}

	imageFileRepo := tenantCtx.ImageFileRepo()
	imageFiles, err := imageFileRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get imagefiles by IDs: %w", err)
	}

	return imageFiles, nil
}
