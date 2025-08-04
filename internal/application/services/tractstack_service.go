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

// TractStackService orchestrates tractstack operations with cache-first repository pattern
type TractStackService struct {
	logger            *logging.ChanneledLogger
	perfTracker       *performance.Tracker
	contentMapService *ContentMapService
}

// NewTractStackService creates a new tractstack service singleton
func NewTractStackService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker, contentMapService *ContentMapService) *TractStackService {
	return &TractStackService{
		logger:            logger,
		perfTracker:       perfTracker,
		contentMapService: contentMapService,
	}
}

// GetAllIDs returns all tractstack IDs for a tenant by leveraging the robust repository.
func (s *TractStackService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_all_tractstack_ids", tenantCtx.TenantID)
	defer marker.Complete()
	tractStackRepo := tenantCtx.TractStackRepo()

	// The repository's FindAll method is now the cache-aware entry point.
	tractStacks, err := tractStackRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all tractstacks from repository: %w", err)
	}

	// Extract IDs from the full objects.
	ids := make([]string, len(tractStacks))
	for i, tractStack := range tractStacks {
		ids[i] = tractStack.ID
	}

	s.logger.Content().Info("Successfully retrieved all tractstack IDs", "tenantId", tenantCtx.TenantID, "count", len(ids), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetAllTractStackIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return ids, nil
}

// GetByID returns a tractstack by ID (cache-first via repository)
func (s *TractStackService) GetByID(tenantCtx *tenant.Context, id string) (*content.TractStackNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_tractstack_by_id", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return nil, fmt.Errorf("tractstack ID cannot be empty")
	}

	tractStackRepo := tenantCtx.TractStackRepo()
	tractStack, err := tractStackRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get tractstack %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved tractstack by ID", "tenantId", tenantCtx.TenantID, "tractstackId", id, "found", tractStack != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetTractStackByID", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "tractStackId", id)

	return tractStack, nil
}

// GetByIDs returns multiple tractstacks by IDs (cache-first with bulk loading via repository)
func (s *TractStackService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.TractStackNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_tractstacks_by_ids", tenantCtx.TenantID)
	defer marker.Complete()
	if len(ids) == 0 {
		return []*content.TractStackNode{}, nil
	}

	tractStackRepo := tenantCtx.TractStackRepo()
	tractStacks, err := tractStackRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get tractstacks by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved tractstacks by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(tractStacks), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetTractStacksByIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(ids))

	return tractStacks, nil
}

// GetBySlug returns a tractstack by slug (cache-first via repository)
func (s *TractStackService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.TractStackNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_tractstack_by_slug", tenantCtx.TenantID)
	defer marker.Complete()
	if slug == "" {
		return nil, fmt.Errorf("tractstack slug cannot be empty")
	}

	tractStackRepo := tenantCtx.TractStackRepo()
	tractStack, err := tractStackRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get tractstack by slug %s: %w", slug, err)
	}

	s.logger.Content().Info("Successfully retrieved tractstack by slug", "tenantId", tenantCtx.TenantID, "slug", slug, "found", tractStack != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetTractStackBySlug", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	return tractStack, nil
}

// Create creates a new tractstack
func (s *TractStackService) Create(tenantCtx *tenant.Context, ts *content.TractStackNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("create_tractstack", tenantCtx.TenantID)
	defer marker.Complete()
	if ts == nil {
		return fmt.Errorf("tractstack cannot be nil")
	}
	if ts.ID == "" {
		return fmt.Errorf("tractstack ID cannot be empty")
	}
	if ts.Title == "" {
		return fmt.Errorf("tractstack title cannot be empty")
	}
	if ts.Slug == "" {
		return fmt.Errorf("tractstack slug cannot be empty")
	}

	tractStackRepo := tenantCtx.TractStackRepo()
	err := tractStackRepo.Store(tenantCtx.TenantID, ts)
	if err != nil {
		return fmt.Errorf("failed to create tractstack %s: %w", ts.ID, err)
	}

	// Surgically add the new item to the item cache and the master ID list
	tenantCtx.CacheManager.SetTractStack(tenantCtx.TenantID, ts)
	tenantCtx.CacheManager.AddTractStackID(tenantCtx.TenantID, ts.ID)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after tractstack creation",
			"error", err, "tractStackId", ts.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully created tractstack", "tenantId", tenantCtx.TenantID, "tractstackId", ts.ID, "title", ts.Title, "slug", ts.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for CreateTractStack", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "tractStackId", ts.ID)

	return nil
}

// Update updates an existing tractstack
func (s *TractStackService) Update(tenantCtx *tenant.Context, ts *content.TractStackNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("update_tractstack", tenantCtx.TenantID)
	defer marker.Complete()
	if ts == nil {
		return fmt.Errorf("tractstack cannot be nil")
	}
	if ts.ID == "" {
		return fmt.Errorf("tractstack ID cannot be empty")
	}
	if ts.Title == "" {
		return fmt.Errorf("tractstack title cannot be empty")
	}
	if ts.Slug == "" {
		return fmt.Errorf("tractstack slug cannot be empty")
	}

	tractStackRepo := tenantCtx.TractStackRepo()

	existing, err := tractStackRepo.FindByID(tenantCtx.TenantID, ts.ID)
	if err != nil {
		return fmt.Errorf("failed to verify tractstack %s exists: %w", ts.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("tractstack %s not found", ts.ID)
	}

	err = tractStackRepo.Update(tenantCtx.TenantID, ts)
	if err != nil {
		return fmt.Errorf("failed to update tractstack %s: %w", ts.ID, err)
	}

	// Surgically update the item in the item cache. The ID list is not affected.
	tenantCtx.CacheManager.SetTractStack(tenantCtx.TenantID, ts)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after tractstack update",
			"error", err, "tractStackId", ts.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully updated tractstack", "tenantId", tenantCtx.TenantID, "tractstackId", ts.ID, "title", ts.Title, "slug", ts.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for UpdateTractStack", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "tractStackId", ts.ID)

	return nil
}

// Delete deletes a tractstack
func (s *TractStackService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("delete_tractstack", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return fmt.Errorf("tractstack ID cannot be empty")
	}

	tractStackRepo := tenantCtx.TractStackRepo()

	existing, err := tractStackRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify tractstack %s exists: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("tractstack %s not found", id)
	}

	err = tractStackRepo.Delete(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete tractstack %s: %w", id, err)
	}

	// Surgically remove the single item from the item cache.
	tenantCtx.CacheManager.InvalidateTractStack(tenantCtx.TenantID, id)
	// Surgically remove the ID from the master ID list.
	tenantCtx.CacheManager.RemoveTractStackID(tenantCtx.TenantID, id)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after tractstack deletion",
			"error", err, "tractStackId", id, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully deleted tractstack", "tenantId", tenantCtx.TenantID, "tractstackId", id, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for DeleteTractStack", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "tractStackId", id)

	return nil
}
