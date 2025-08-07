// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// BeliefService orchestrates belief operations with cache-first repository pattern
type BeliefService struct {
	logger            *logging.ChanneledLogger
	perfTracker       *performance.Tracker
	contentMapService *ContentMapService
}

// NewBeliefService creates a new belief service singleton
func NewBeliefService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker, contentMapService *ContentMapService) *BeliefService {
	return &BeliefService{
		logger:            logger,
		perfTracker:       perfTracker,
		contentMapService: contentMapService,
	}
}

// GetAllIDs returns all belief IDs for a tenant by leveraging the robust repository.
func (s *BeliefService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_all_belief_ids", tenantCtx.TenantID)
	defer marker.Complete()
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
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetAllBeliefIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return ids, nil
}

// GetByID returns a belief by ID (cache-first via repository)
func (s *BeliefService) GetByID(tenantCtx *tenant.Context, id string) (*content.BeliefNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_belief_by_id", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return nil, fmt.Errorf("belief ID cannot be empty")
	}

	beliefRepo := tenantCtx.BeliefRepo()
	belief, err := beliefRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get belief %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved belief by ID", "tenantId", tenantCtx.TenantID, "beliefId", id, "found", belief != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetBeliefByID", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "beliefId", id)

	return belief, nil
}

// GetByIDs returns multiple beliefs by IDs (cache-first with bulk loading via repository)
func (s *BeliefService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.BeliefNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_beliefs_by_ids", tenantCtx.TenantID)
	defer marker.Complete()
	if len(ids) == 0 {
		return []*content.BeliefNode{}, nil
	}

	beliefRepo := tenantCtx.BeliefRepo()
	beliefs, err := beliefRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get beliefs by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved beliefs by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(beliefs), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetBeliefsByIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(ids))

	return beliefs, nil
}

// GetBySlug returns a belief by slug (cache-first via repository)
func (s *BeliefService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.BeliefNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_belief_by_slug", tenantCtx.TenantID)
	defer marker.Complete()
	if slug == "" {
		return nil, fmt.Errorf("belief slug cannot be empty")
	}

	beliefRepo := tenantCtx.BeliefRepo()
	belief, err := beliefRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get belief by slug %s: %w", slug, err)
	}

	s.logger.Content().Info("Successfully retrieved belief by slug", "tenantId", tenantCtx.TenantID, "slug", slug, "found", belief != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetBeliefBySlug", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	return belief, nil
}

// Create creates a new belief
func (s *BeliefService) Create(tenantCtx *tenant.Context, belief *content.BeliefNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("create_belief", tenantCtx.TenantID)
	defer marker.Complete()
	if belief.ID == "" {
		belief.ID = security.GenerateULID()
	}
	if belief == nil {
		return fmt.Errorf("belief cannot be nil")
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

	// Surgically add the new item to the item cache and the master ID list
	tenantCtx.CacheManager.SetBelief(tenantCtx.TenantID, belief)
	tenantCtx.CacheManager.AddBeliefID(tenantCtx.TenantID, belief.ID)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after belief creation",
			"error", err, "beliefId", belief.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully created belief", "tenantId", tenantCtx.TenantID, "beliefId", belief.ID, "title", belief.Title, "slug", belief.Slug, "scale", belief.Scale, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for CreateBelief", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "beliefId", belief.ID)

	return nil
}

// Update updates an existing belief
func (s *BeliefService) Update(tenantCtx *tenant.Context, belief *content.BeliefNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("update_belief", tenantCtx.TenantID)
	defer marker.Complete()
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

	// Surgically update the item in the item cache. The ID list is not affected.
	tenantCtx.CacheManager.SetBelief(tenantCtx.TenantID, belief)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after belief update",
			"error", err, "beliefId", belief.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully updated belief", "tenantId", tenantCtx.TenantID, "beliefId", belief.ID, "title", belief.Title, "slug", belief.Slug, "scale", belief.Scale, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for UpdateBelief", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "beliefId", belief.ID)

	return nil
}

// Delete deletes a belief
func (s *BeliefService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("delete_belief", tenantCtx.TenantID)
	defer marker.Complete()
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

	// Surgically remove the single item from the item cache.
	tenantCtx.CacheManager.InvalidateBelief(tenantCtx.TenantID, id)
	// Surgically remove the ID from the master ID list.
	tenantCtx.CacheManager.RemoveBeliefID(tenantCtx.TenantID, id)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after belief deletion",
			"error", err, "beliefId", id, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully deleted belief", "tenantId", tenantCtx.TenantID, "beliefId", id, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for DeleteBelief", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "beliefId", id)

	return nil
}
