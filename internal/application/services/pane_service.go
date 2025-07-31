// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
)

// PaneService orchestrates pane operations with cache-first repository pattern
type PaneService struct {
	paneRepo repositories.PaneRepository
}

// NewPaneService creates a new pane application service
func NewPaneService(paneRepo repositories.PaneRepository) *PaneService {
	return &PaneService{
		paneRepo: paneRepo,
	}
}

// GetAllIDs returns all pane IDs for a tenant (cache-first)
func (s *PaneService) GetAllIDs(tenantID string) ([]string, error) {
	panes, err := s.paneRepo.FindAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all panes: %w", err)
	}

	ids := make([]string, len(panes))
	for i, pane := range panes {
		ids[i] = pane.ID
	}

	return ids, nil
}

// GetByID returns a pane by ID (cache-first)
func (s *PaneService) GetByID(tenantID, id string) (*content.PaneNode, error) {
	if id == "" {
		return nil, fmt.Errorf("pane ID cannot be empty")
	}

	pane, err := s.paneRepo.FindByID(tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane %s: %w", id, err)
	}

	return pane, nil
}

// GetByIDs returns multiple panes by IDs (cache-first with bulk loading)
func (s *PaneService) GetByIDs(tenantID string, ids []string) ([]*content.PaneNode, error) {
	if len(ids) == 0 {
		return []*content.PaneNode{}, nil
	}

	panes, err := s.paneRepo.FindByIDs(tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get panes by IDs: %w", err)
	}

	return panes, nil
}

// GetBySlug returns a pane by slug (cache-first)
func (s *PaneService) GetBySlug(tenantID, slug string) (*content.PaneNode, error) {
	if slug == "" {
		return nil, fmt.Errorf("pane slug cannot be empty")
	}

	pane, err := s.paneRepo.FindBySlug(tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane by slug %s: %w", slug, err)
	}

	return pane, nil
}

// GetContextPanes returns all context panes (cache-first with filtering)
func (s *PaneService) GetContextPanes(tenantID string) ([]*content.PaneNode, error) {
	contextPanes, err := s.paneRepo.FindContext(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get context panes: %w", err)
	}

	return contextPanes, nil
}
