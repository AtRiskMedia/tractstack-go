// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// BeliefService orchestrates belief operations with cache-first repository pattern
type BeliefService struct {
	logger *logging.ChanneledLogger
}

// NewBeliefService creates a new belief service singleton
func NewBeliefService(logger *logging.ChanneledLogger) *BeliefService {
	return &BeliefService{
		logger: logger,
	}
}

// GetAllIDs returns all belief IDs for a tenant by leveraging the robust repository.
func (s *BeliefService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	beliefRepo := tenantCtx.BeliefRepo()

	// The repository's FindAll method is now the cache-aware entry point.
	beliefs, err := beliefRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all beliefs from repository: %w", err)
	}

	// Extract IDs from the full objects.
	ids := make([]string, len(beliefs))
	for i, belief := range beliefs {
		ids[i] = belief.ID
	}

	s.logger.Content().Info("Successfully retrieved all belief IDs", "tenantId", tenantCtx.TenantID, "count", len(ids), "duration", time.Since(start))

	return ids, nil
}

// GetByID returns a belief by ID (cache-first via repository)
func (s *BeliefService) GetByID(tenantCtx *tenant.Context, id string) (*content.BeliefNode, error) {
	start := time.Now()
	if id == "" {
		return nil, fmt.Errorf("belief ID cannot be empty")
	}

	beliefRepo := tenantCtx.BeliefRepo()
	belief, err := beliefRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get belief %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved belief by ID", "tenantId", tenantCtx.TenantID, "beliefId", id, "found", belief != nil, "duration", time.Since(start))

	return belief, nil
}

// GetByIDs returns multiple beliefs by IDs (cache-first with bulk loading via repository)
func (s *BeliefService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.BeliefNode, error) {
	start := time.Now()
	if len(ids) == 0 {
		return []*content.BeliefNode{}, nil
	}

	beliefRepo := tenantCtx.BeliefRepo()
	beliefs, err := beliefRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get beliefs by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved beliefs by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(beliefs), "duration", time.Since(start))

	return beliefs, nil
}

// GetBySlug returns a belief by slug (cache-first via repository)
func (s *BeliefService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.BeliefNode, error) {
	start := time.Now()
	if slug == "" {
		return nil, fmt.Errorf("belief slug cannot be empty")
	}

	beliefRepo := tenantCtx.BeliefRepo()
	belief, err := beliefRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get belief by slug %s: %w", slug, err)
	}

	s.logger.Content().Info("Successfully retrieved belief by slug", "tenantId", tenantCtx.TenantID, "slug", slug, "found", belief != nil, "duration", time.Since(start))

	return belief, nil
}

// Create creates a new belief
func (s *BeliefService) Create(tenantCtx *tenant.Context, belief *content.BeliefNode) error {
	start := time.Now()
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

	s.logger.Content().Info("Successfully created belief", "tenantId", tenantCtx.TenantID, "beliefId", belief.ID, "title", belief.Title, "slug", belief.Slug, "scale", belief.Scale, "duration", time.Since(start))

	return nil
}

// Update updates an existing belief
func (s *BeliefService) Update(tenantCtx *tenant.Context, belief *content.BeliefNode) error {
	start := time.Now()
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

	s.logger.Content().Info("Successfully updated belief", "tenantId", tenantCtx.TenantID, "beliefId", belief.ID, "title", belief.Title, "slug", belief.Slug, "scale", belief.Scale, "duration", time.Since(start))

	return nil
}

// Delete deletes a belief
func (s *BeliefService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	if id == "" {
		return fmt.Errorf("belief ID cannot be empty")
	}

	beliefRepo := tenantCtx.BeliefRepo()

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

	s.logger.Content().Info("Successfully deleted belief", "tenantId", tenantCtx.TenantID, "beliefId", id, "duration", time.Since(start))

	return nil
}
