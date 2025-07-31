// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
)

// ImageFileService orchestrates imagefile operations with cache-first repository pattern
type ImageFileService struct {
	imageFileRepo repositories.ImageFileRepository
}

// NewImageFileService creates a new imagefile application service
func NewImageFileService(imageFileRepo repositories.ImageFileRepository) *ImageFileService {
	return &ImageFileService{
		imageFileRepo: imageFileRepo,
	}
}

// GetAllIDs returns all imagefile IDs for a tenant (cache-first)
func (s *ImageFileService) GetAllIDs(tenantID string) ([]string, error) {
	imageFiles, err := s.imageFileRepo.FindAll(tenantID)
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
func (s *ImageFileService) GetByID(tenantID, id string) (*content.ImageFileNode, error) {
	if id == "" {
		return nil, fmt.Errorf("imagefile ID cannot be empty")
	}

	imageFile, err := s.imageFileRepo.FindByID(tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get imagefile %s: %w", id, err)
	}

	return imageFile, nil
}

// GetByIDs returns multiple imagefiles by IDs (cache-first with bulk loading)
func (s *ImageFileService) GetByIDs(tenantID string, ids []string) ([]*content.ImageFileNode, error) {
	if len(ids) == 0 {
		return []*content.ImageFileNode{}, nil
	}

	imageFiles, err := s.imageFileRepo.FindByIDs(tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get imagefiles by IDs: %w", err)
	}

	return imageFiles, nil
}
