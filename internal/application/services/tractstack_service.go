// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
)

// TractStackService orchestrates tractstack operations with cache-first repository pattern
type TractStackService struct {
	tractStackRepo repositories.TractStackRepository
}

// NewTractStackService creates a new tractstack application service
func NewTractStackService(tractStackRepo repositories.TractStackRepository) *TractStackService {
	return &TractStackService{
		tractStackRepo: tractStackRepo,
	}
}

// GetAllIDs returns all tractstack IDs for a tenant (cache-first)
func (s *TractStackService) GetAllIDs(tenantID string) ([]string, error) {
	tractStacks, err := s.tractStackRepo.FindAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all tractstacks: %w", err)
	}

	ids := make([]string, len(tractStacks))
	for i, tractStack := range tractStacks {
		ids[i] = tractStack.ID
	}

	return ids, nil
}

// GetByID returns a tractstack by ID (cache-first)
func (s *TractStackService) GetByID(tenantID, id string) (*content.TractStackNode, error) {
	if id == "" {
		return nil, fmt.Errorf("tractstack ID cannot be empty")
	}

	tractStack, err := s.tractStackRepo.FindByID(tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get tractstack %s: %w", id, err)
	}

	return tractStack, nil
}

// GetByIDs returns multiple tractstacks by IDs (cache-first with bulk loading)
func (s *TractStackService) GetByIDs(tenantID string, ids []string) ([]*content.TractStackNode, error) {
	if len(ids) == 0 {
		return []*content.TractStackNode{}, nil
	}

	tractStacks, err := s.tractStackRepo.FindByIDs(tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get tractstacks by IDs: %w", err)
	}

	return tractStacks, nil
}

// GetBySlug returns a tractstack by slug (cache-first)
func (s *TractStackService) GetBySlug(tenantID, slug string) (*content.TractStackNode, error) {
	if slug == "" {
		return nil, fmt.Errorf("tractstack slug cannot be empty")
	}

	tractStack, err := s.tractStackRepo.FindBySlug(tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get tractstack by slug %s: %w", slug, err)
	}

	return tractStack, nil
}
