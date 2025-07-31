// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// BeliefService orchestrates belief operations with cache-first repository pattern
type BeliefService struct {
	// No stored dependencies - all passed via tenant context
}

// NewBeliefService creates a new belief service singleton
func NewBeliefService() *BeliefService {
	return &BeliefService{}
}

// GetAllIDs returns all belief IDs for a tenant (cache-first)
func (s *BeliefService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	beliefRepo := tenantCtx.BeliefRepo()

	beliefs, err := beliefRepo.FindAll(tenantCtx.TenantID)
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
func (s *BeliefService) GetByID(tenantCtx *tenant.Context, id string) (*content.BeliefNode, error) {
	if id == "" {
		return nil, fmt.Errorf("belief ID cannot be empty")
	}

	beliefRepo := tenantCtx.BeliefRepo()
	belief, err := beliefRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get belief %s: %w", id, err)
	}

	return belief, nil
}

// GetByIDs returns multiple beliefs by IDs (cache-first with bulk loading)
func (s *BeliefService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.BeliefNode, error) {
	if len(ids) == 0 {
		return []*content.BeliefNode{}, nil
	}

	beliefRepo := tenantCtx.BeliefRepo()
	beliefs, err := beliefRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get beliefs by IDs: %w", err)
	}

	return beliefs, nil
}

// GetBySlug returns a belief by slug (cache-first)
func (s *BeliefService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.BeliefNode, error) {
	if slug == "" {
		return nil, fmt.Errorf("belief slug cannot be empty")
	}

	beliefRepo := tenantCtx.BeliefRepo()
	belief, err := beliefRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get belief by slug %s: %w", slug, err)
	}

	return belief, nil
}

// Create creates a new belief
func (s *BeliefService) Create(tenantCtx *tenant.Context, belief *content.BeliefNode) error {
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

	beliefRepo := tenantCtx.BeliefRepo()
	err := beliefRepo.Store(tenantCtx.TenantID, belief)
	if err != nil {
		return fmt.Errorf("failed to create belief %s: %w", belief.ID, err)
	}

	return nil
}

// Update updates an existing belief
func (s *BeliefService) Update(tenantCtx *tenant.Context, belief *content.BeliefNode) error {
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

	beliefRepo := tenantCtx.BeliefRepo()

	// Verify belief exists before updating
	existing, err := beliefRepo.FindByID(tenantCtx.TenantID, belief.ID)
	if err != nil {
		return fmt.Errorf("failed to verify belief %s exists: %w", belief.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("belief %s not found", belief.ID)
	}

	err = beliefRepo.Update(tenantCtx.TenantID, belief)
	if err != nil {
		return fmt.Errorf("failed to update belief %s: %w", belief.ID, err)
	}

	return nil
}

// Delete deletes a belief
func (s *BeliefService) Delete(tenantCtx *tenant.Context, id string) error {
	if id == "" {
		return fmt.Errorf("belief ID cannot be empty")
	}

	beliefRepo := tenantCtx.BeliefRepo()

	// Verify belief exists before deleting
	existing, err := beliefRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify belief %s exists: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("belief %s not found", id)
	}

	err = beliefRepo.Delete(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete belief %s: %w", id, err)
	}

	return nil
}
