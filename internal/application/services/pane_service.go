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

// PaneService orchestrates pane operations with cache-first repository pattern
type PaneService struct {
	logger            *logging.ChanneledLogger
	perfTracker       *performance.Tracker
	contentMapService *ContentMapService
}

// NewPaneService creates a new pane service singleton
func NewPaneService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker, contentMapService *ContentMapService) *PaneService {
	return &PaneService{
		logger:            logger,
		perfTracker:       perfTracker,
		contentMapService: contentMapService,
	}
}

// GetAllIDs returns all pane IDs for a tenant by leveraging the robust repository.
func (s *PaneService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_all_pane_ids", tenantCtx.TenantID)
	defer marker.Complete()
	paneRepo := tenantCtx.PaneRepo()

	// The repository's FindAll method is now the cache-aware entry point.
	panes, err := paneRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all panes from repository: %w", err)
	}

	// We just need to extract the IDs from the full objects.
	ids := make([]string, len(panes))
	for i, pane := range panes {
		ids[i] = pane.ID
	}

	s.logger.Content().Info("Successfully retrieved all pane IDs", "tenantId", tenantCtx.TenantID, "count", len(ids), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetAllPaneIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return ids, nil
}

// GetByID returns a pane by ID (cache-first via repository)
func (s *PaneService) GetByID(tenantCtx *tenant.Context, id string) (*content.PaneNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_pane_by_id", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return nil, fmt.Errorf("pane ID cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved pane by ID", "tenantId", tenantCtx.TenantID, "paneId", id, "found", pane != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetPaneByID", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", id)

	return pane, nil
}

// GetByIDs returns multiple panes by IDs (cache-first with bulk loading via repository)
func (s *PaneService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.PaneNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_panes_by_ids", tenantCtx.TenantID)
	defer marker.Complete()
	if len(ids) == 0 {
		return []*content.PaneNode{}, nil
	}

	paneRepo := tenantCtx.PaneRepo()
	panes, err := paneRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get panes by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved panes by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(panes), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetPanesByIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(ids))

	return panes, nil
}

// GetBySlug returns a pane by slug (cache-first via repository)
func (s *PaneService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.PaneNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_pane_by_slug", tenantCtx.TenantID)
	defer marker.Complete()
	if slug == "" {
		return nil, fmt.Errorf("pane slug cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane by slug %s: %w", slug, err)
	}

	s.logger.Content().Info("Successfully retrieved pane by slug", "tenantId", tenantCtx.TenantID, "slug", slug, "found", pane != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetPaneBySlug", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	return pane, nil
}

// GetContextPanes returns all context panes (cache-first with filtering via repository)
func (s *PaneService) GetContextPanes(tenantCtx *tenant.Context) ([]*content.PaneNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_context_panes", tenantCtx.TenantID)
	defer marker.Complete()
	paneRepo := tenantCtx.PaneRepo()
	contextPanes, err := paneRepo.FindContext(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get context panes: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved context panes", "tenantId", tenantCtx.TenantID, "count", len(contextPanes), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetContextPanes", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return contextPanes, nil
}

// Create creates a new pane
func (s *PaneService) Create(tenantCtx *tenant.Context, pane *content.PaneNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("create_pane", tenantCtx.TenantID)
	defer marker.Complete()
	if pane == nil {
		return fmt.Errorf("pane cannot be nil")
	}
	if pane.ID == "" {
		return fmt.Errorf("pane ID cannot be empty")
	}
	if pane.Title == "" {
		return fmt.Errorf("pane title cannot be empty")
	}
	if pane.Slug == "" {
		return fmt.Errorf("pane slug cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()
	err := paneRepo.Store(tenantCtx.TenantID, pane)
	if err != nil {
		return fmt.Errorf("failed to create pane %s: %w", pane.ID, err)
	}

	// Surgically add the new item to the item cache and the master ID list
	tenantCtx.CacheManager.SetPane(tenantCtx.TenantID, pane)
	tenantCtx.CacheManager.AddPaneID(tenantCtx.TenantID, pane.ID)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after pane creation",
			"error", err, "paneId", pane.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully created pane", "tenantId", tenantCtx.TenantID, "paneId", pane.ID, "title", pane.Title, "slug", pane.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for CreatePane", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", pane.ID)

	return nil
}

// Update updates an existing pane
func (s *PaneService) Update(tenantCtx *tenant.Context, pane *content.PaneNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("update_pane", tenantCtx.TenantID)
	defer marker.Complete()
	if pane == nil {
		return fmt.Errorf("pane cannot be nil")
	}
	if pane.ID == "" {
		return fmt.Errorf("pane ID cannot be empty")
	}
	if pane.Title == "" {
		return fmt.Errorf("pane title cannot be empty")
	}
	if pane.Slug == "" {
		return fmt.Errorf("pane slug cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()

	existing, err := paneRepo.FindByID(tenantCtx.TenantID, pane.ID)
	if err != nil {
		return fmt.Errorf("failed to verify pane %s exists: %w", pane.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("pane %s not found", pane.ID)
	}

	err = paneRepo.Update(tenantCtx.TenantID, pane)
	if err != nil {
		return fmt.Errorf("failed to update pane %s: %w", pane.ID, err)
	}

	// Surgically update the item in the item cache. The ID list is not affected.
	tenantCtx.CacheManager.SetPane(tenantCtx.TenantID, pane)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after pane update",
			"error", err, "paneId", pane.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully updated pane", "tenantId", tenantCtx.TenantID, "paneId", pane.ID, "title", pane.Title, "slug", pane.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for UpdatePane", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", pane.ID)

	return nil
}

// Delete deletes a pane
func (s *PaneService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("delete_pane", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return fmt.Errorf("pane ID cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()

	existing, err := paneRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify pane %s exists: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("pane %s not found", id)
	}

	err = paneRepo.Delete(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete pane %s: %w", id, err)
	}

	// Surgically remove the single item from the item cache.
	tenantCtx.CacheManager.InvalidatePane(tenantCtx.TenantID, id)
	// Surgically remove the ID from the master ID list.
	tenantCtx.CacheManager.RemovePaneID(tenantCtx.TenantID, id)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after pane deletion",
			"error", err, "paneId", id, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully deleted pane", "tenantId", tenantCtx.TenantID, "paneId", id, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for DeletePane", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", id)

	return nil
}
