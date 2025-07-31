// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// TractStackService orchestrates tractstack operations with cache-first repository pattern
type TractStackService struct {
	// No stored dependencies - all passed via tenant context
}

// NewTractStackService creates a new tractstack service singleton
func NewTractStackService() *TractStackService {
	return &TractStackService{}
}

// GetAllIDs returns all tractstack IDs for a tenant (cache-first)
func (s *TractStackService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	tractStackRepo := tenantCtx.TractStackRepo()

	tractStacks, err := tractStackRepo.FindAll(tenantCtx.TenantID)
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
func (s *TractStackService) GetByID(tenantCtx *tenant.Context, id string) (*content.TractStackNode, error) {
	if id == "" {
		return nil, fmt.Errorf("tractstack ID cannot be empty")
	}

	tractStackRepo := tenantCtx.TractStackRepo()
	tractStack, err := tractStackRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get tractstack %s: %w", id, err)
	}

	return tractStack, nil
}

// GetByIDs returns multiple tractstacks by IDs (cache-first with bulk loading)
func (s *TractStackService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.TractStackNode, error) {
	if len(ids) == 0 {
		return []*content.TractStackNode{}, nil
	}

	tractStackRepo := tenantCtx.TractStackRepo()
	tractStacks, err := tractStackRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get tractstacks by IDs: %w", err)
	}

	return tractStacks, nil
}

// GetBySlug returns a tractstack by slug (cache-first)
func (s *TractStackService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.TractStackNode, error) {
	if slug == "" {
		return nil, fmt.Errorf("tractstack slug cannot be empty")
	}

	tractStackRepo := tenantCtx.TractStackRepo()
	tractStack, err := tractStackRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get tractstack by slug %s: %w", slug, err)
	}

	return tractStack, nil
}
