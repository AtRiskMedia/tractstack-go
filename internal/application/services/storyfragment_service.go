// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// StoryFragmentFullPayload represents the full editorial payload for a storyfragment
type StoryFragmentFullPayload struct {
	StoryFragment *content.StoryFragmentNode `json:"storyFragment"`
	TractStack    *content.TractStackNode    `json:"tractStack,omitempty"`
	Menu          *content.MenuNode          `json:"menu,omitempty"`
	Panes         []*content.PaneNode        `json:"panes,omitempty"`
}

// StoryFragmentService orchestrates storyfragment operations with cache-first repository pattern
type StoryFragmentService struct {
	storyFragmentRepo repositories.StoryFragmentRepository
	tractStackRepo    repositories.TractStackRepository
	menuRepo          repositories.MenuRepository
	paneRepo          repositories.PaneRepository
}

// NewStoryFragmentService creates a new storyfragment application service
func NewStoryFragmentService(
	storyFragmentRepo repositories.StoryFragmentRepository,
	tractStackRepo repositories.TractStackRepository,
	menuRepo repositories.MenuRepository,
	paneRepo repositories.PaneRepository,
) *StoryFragmentService {
	return &StoryFragmentService{
		storyFragmentRepo: storyFragmentRepo,
		tractStackRepo:    tractStackRepo,
		menuRepo:          menuRepo,
		paneRepo:          paneRepo,
	}
}

// GetAllIDs returns all storyfragment IDs for a tenant (cache-first)
func (s *StoryFragmentService) GetAllIDs(tenantID string) ([]string, error) {
	storyFragments, err := s.storyFragmentRepo.FindAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all storyfragments: %w", err)
	}

	ids := make([]string, len(storyFragments))
	for i, storyFragment := range storyFragments {
		ids[i] = storyFragment.ID
	}

	return ids, nil
}

// GetByID returns a storyfragment by ID (cache-first)
func (s *StoryFragmentService) GetByID(tenantID, id string) (*content.StoryFragmentNode, error) {
	if id == "" {
		return nil, fmt.Errorf("storyfragment ID cannot be empty")
	}

	storyFragment, err := s.storyFragmentRepo.FindByID(tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragment %s: %w", id, err)
	}

	return storyFragment, nil
}

// GetByIDs returns multiple storyfragments by IDs (cache-first with bulk loading)
func (s *StoryFragmentService) GetByIDs(tenantID string, ids []string) ([]*content.StoryFragmentNode, error) {
	if len(ids) == 0 {
		return []*content.StoryFragmentNode{}, nil
	}

	storyFragments, err := s.storyFragmentRepo.FindByIDs(tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragments by IDs: %w", err)
	}

	return storyFragments, nil
}

// GetBySlug returns a storyfragment by slug (cache-first)
func (s *StoryFragmentService) GetBySlug(tenantID, slug string) (*content.StoryFragmentNode, error) {
	if slug == "" {
		return nil, fmt.Errorf("storyfragment slug cannot be empty")
	}

	storyFragment, err := s.storyFragmentRepo.FindBySlug(tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragment by slug %s: %w", slug, err)
	}

	return storyFragment, nil
}

// GetFullPayloadBySlug returns a storyfragment with full editorial payload (cache-first)
func (s *StoryFragmentService) GetFullPayloadBySlug(tenantID, slug string) (*StoryFragmentFullPayload, error) {
	if slug == "" {
		return nil, fmt.Errorf("storyfragment slug cannot be empty")
	}

	// Get the storyfragment
	storyFragment, err := s.storyFragmentRepo.FindBySlug(tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragment by slug %s: %w", slug, err)
	}
	if storyFragment == nil {
		return nil, fmt.Errorf("storyfragment not found with slug %s", slug)
	}

	payload := &StoryFragmentFullPayload{
		StoryFragment: storyFragment,
	}

	// Get related tractstack
	if storyFragment.TractStackID != "" {
		tractStack, err := s.tractStackRepo.FindByID(tenantID, storyFragment.TractStackID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tractstack %s: %w", storyFragment.TractStackID, err)
		}
		payload.TractStack = tractStack
	}

	// Get related menu
	if storyFragment.MenuID != nil && *storyFragment.MenuID != "" {
		menu, err := s.menuRepo.FindByID(tenantID, *storyFragment.MenuID)
		if err != nil {
			return nil, fmt.Errorf("failed to get menu %s: %w", *storyFragment.MenuID, err)
		}
		payload.Menu = menu
	}

	// Get related panes
	if len(storyFragment.PaneIDs) > 0 {
		panes, err := s.paneRepo.FindByIDs(tenantID, storyFragment.PaneIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get panes for storyfragment %s: %w", storyFragment.ID, err)
		}
		payload.Panes = panes
	}

	return payload, nil
}

// GetHome returns the home storyfragment by reading the home slug from the tenant's configuration.
func (s *StoryFragmentService) GetHome(ctx *tenant.Context) (*content.StoryFragmentNode, error) {
	if ctx == nil || ctx.Config == nil || ctx.Config.BrandConfig == nil {
		return nil, fmt.Errorf("tenant context or configuration is not available")
	}
	// Get the configured home slug, with a safe fallback
	homeSlug := ctx.Config.BrandConfig.HomeSlug
	if homeSlug == "" {
		homeSlug = "hello" // Fallback to the default
	}
	storyFragment, err := s.storyFragmentRepo.FindBySlug(ctx.TenantID, homeSlug)
	if err != nil {
		return nil, fmt.Errorf("failed to get home storyfragment by slug '%s': %w", homeSlug, err)
	}
	return storyFragment, nil
}
