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
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewTractStackService creates a new tractstack service singleton
func NewTractStackService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *TractStackService {
	return &TractStackService{
		logger:      logger,
		perfTracker: perfTracker,
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
