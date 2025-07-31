// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// PaneService orchestrates pane operations with cache-first repository pattern
type PaneService struct {
	// No stored dependencies - all passed via tenant context
}

// NewPaneService creates a new pane service singleton
func NewPaneService() *PaneService {
	return &PaneService{}
}

// GetAllIDs returns all pane IDs for a tenant by leveraging the robust repository.
func (s *PaneService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
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

	return ids, nil
}

// GetByID returns a pane by ID (cache-first via repository)
func (s *PaneService) GetByID(tenantCtx *tenant.Context, id string) (*content.PaneNode, error) {
	if id == "" {
		return nil, fmt.Errorf("pane ID cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane %s: %w", id, err)
	}

	return pane, nil
}

// GetByIDs returns multiple panes by IDs (cache-first with bulk loading via repository)
func (s *PaneService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.PaneNode, error) {
	if len(ids) == 0 {
		return []*content.PaneNode{}, nil
	}

	paneRepo := tenantCtx.PaneRepo()
	panes, err := paneRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get panes by IDs from repository: %w", err)
	}

	return panes, nil
}

// GetBySlug returns a pane by slug (cache-first via repository)
func (s *PaneService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.PaneNode, error) {
	if slug == "" {
		return nil, fmt.Errorf("pane slug cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane by slug %s: %w", slug, err)
	}

	return pane, nil
}

// GetContextPanes returns all context panes (cache-first with filtering via repository)
func (s *PaneService) GetContextPanes(tenantCtx *tenant.Context) ([]*content.PaneNode, error) {
	paneRepo := tenantCtx.PaneRepo()
	contextPanes, err := paneRepo.FindContext(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get context panes: %w", err)
	}

	return contextPanes, nil
}
