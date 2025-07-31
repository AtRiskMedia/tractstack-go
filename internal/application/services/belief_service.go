// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
)

// BeliefService orchestrates belief operations with cache-first repository pattern
type BeliefService struct {
	beliefRepo repositories.BeliefRepository
}

// NewBeliefService creates a new belief application service
func NewBeliefService(beliefRepo repositories.BeliefRepository) *BeliefService {
	return &BeliefService{
		beliefRepo: beliefRepo,
	}
}

// GetAllIDs returns all belief IDs for a tenant (cache-first)
func (s *BeliefService) GetAllIDs(tenantID string) ([]string, error) {
	beliefs, err := s.beliefRepo.FindAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all beliefs: %w", err)
	}

	ids := make([]string, len(beliefs))
	for i, belief := range beliefs {
		ids[i] = belief.ID
	}

	return ids, nil
}

// GetByID returns a belief by ID (cache-first)
func (s *BeliefService) GetByID(tenantID, id string) (*content.BeliefNode, error) {
	if id == "" {
		return nil, fmt.Errorf("belief ID cannot be empty")
	}

	belief, err := s.beliefRepo.FindByID(tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get belief %s: %w", id, err)
	}

	return belief, nil
}

// GetByIDs returns multiple beliefs by IDs (cache-first with bulk loading)
func (s *BeliefService) GetByIDs(tenantID string, ids []string) ([]*content.BeliefNode, error) {
	if len(ids) == 0 {
		return []*content.BeliefNode{}, nil
	}

	beliefs, err := s.beliefRepo.FindByIDs(tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get beliefs by IDs: %w", err)
	}

	return beliefs, nil
}

// GetBySlug returns a belief by slug (cache-first)
func (s *BeliefService) GetBySlug(tenantID, slug string) (*content.BeliefNode, error) {
	if slug == "" {
		return nil, fmt.Errorf("belief slug cannot be empty")
	}

	belief, err := s.beliefRepo.FindBySlug(tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get belief by slug %s: %w", slug, err)
	}

	return belief, nil
}

// Create creates a new belief
func (s *BeliefService) Create(tenantID string, belief *content.BeliefNode) error {
	if belief == nil {
		return fmt.Errorf("belief cannot be nil")
	}
	if belief.ID == "" {
		return fmt.Errorf("belief ID cannot be empty")
	}
	if belief.Title == "" {
		return fmt.Errorf("belief title cannot be empty")
	}
	if belief.Slug == "" {
		return fmt.Errorf("belief slug cannot be empty")
	}
	if belief.Scale == "" {
		return fmt.Errorf("belief scale cannot be empty")
	}

	err := s.beliefRepo.Store(tenantID, belief)
	if err != nil {
		return fmt.Errorf("failed to create belief %s: %w", belief.ID, err)
	}

	return nil
}

// Update updates an existing belief
func (s *BeliefService) Update(tenantID string, belief *content.BeliefNode) error {
	if belief == nil {
		return fmt.Errorf("belief cannot be nil")
	}
	if belief.ID == "" {
		return fmt.Errorf("belief ID cannot be empty")
	}
	if belief.Title == "" {
		return fmt.Errorf("belief title cannot be empty")
	}
	if belief.Slug == "" {
		return fmt.Errorf("belief slug cannot be empty")
	}
	if belief.Scale == "" {
		return fmt.Errorf("belief scale cannot be empty")
	}

	// Verify belief exists before updating
	existing, err := s.beliefRepo.FindByID(tenantID, belief.ID)
	if err != nil {
		return fmt.Errorf("failed to verify belief %s exists: %w", belief.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("belief %s not found", belief.ID)
	}

	err = s.beliefRepo.Update(tenantID, belief)
	if err != nil {
		return fmt.Errorf("failed to update belief %s: %w", belief.ID, err)
	}

	return nil
}

// Delete deletes a belief
func (s *BeliefService) Delete(tenantID, id string) error {
	if id == "" {
		return fmt.Errorf("belief ID cannot be empty")
	}

	// Verify belief exists before deleting
	existing, err := s.beliefRepo.FindByID(tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify belief %s exists: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("belief %s not found", id)
	}

	err = s.beliefRepo.Delete(tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete belief %s: %w", id, err)
	}

	return nil
}
