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

// StoryFragmentFullPayload represents the full editorial payload for a storyfragment
type StoryFragmentFullPayload struct {
	StoryFragment *content.StoryFragmentNode `json:"storyFragment"`
	TractStack    *content.TractStackNode    `json:"tractStack,omitempty"`
	Menu          *content.MenuNode          `json:"menu,omitempty"`
	Panes         []*content.PaneNode        `json:"panes,omitempty"`
}

// StoryFragmentService orchestrates storyfragment operations with cache-first repository pattern
type StoryFragmentService struct {
	logger *logging.ChanneledLogger
}

// NewStoryFragmentService creates a new storyfragment service singleton
func NewStoryFragmentService(logger *logging.ChanneledLogger) *StoryFragmentService {
	return &StoryFragmentService{
		logger: logger,
	}
}

// GetAllIDs returns all storyfragment IDs for a tenant by leveraging the robust repository.
func (s *StoryFragmentService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	storyFragmentRepo := tenantCtx.StoryFragmentRepo()

	// The repository's FindAll method is now the cache-aware entry point.
	storyFragments, err := storyFragmentRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all storyfragments from repository: %w", err)
	}

	// Extract IDs from the full objects.
	ids := make([]string, len(storyFragments))
	for i, storyFragment := range storyFragments {
		ids[i] = storyFragment.ID
	}

	s.logger.Content().Info("Successfully retrieved all storyfragment IDs", "tenantId", tenantCtx.TenantID, "count", len(ids), "duration", time.Since(start))

	return ids, nil
}

// GetByID returns a storyfragment by ID (cache-first via repository)
func (s *StoryFragmentService) GetByID(tenantCtx *tenant.Context, id string) (*content.StoryFragmentNode, error) {
	start := time.Now()
	if id == "" {
		return nil, fmt.Errorf("storyfragment ID cannot be empty")
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragment, err := storyFragmentRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragment %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved storyfragment by ID", "tenantId", tenantCtx.TenantID, "storyfragmentId", id, "found", storyFragment != nil, "duration", time.Since(start))

	return storyFragment, nil
}

// GetByIDs returns multiple storyfragments by IDs (cache-first with bulk loading via repository)
func (s *StoryFragmentService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.StoryFragmentNode, error) {
	start := time.Now()
	if len(ids) == 0 {
		return []*content.StoryFragmentNode{}, nil
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragments, err := storyFragmentRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragments by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved storyfragments by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(storyFragments), "duration", time.Since(start))

	return storyFragments, nil
}

// GetBySlug returns a storyfragment by slug (cache-first via repository)
func (s *StoryFragmentService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.StoryFragmentNode, error) {
	start := time.Now()
	if slug == "" {
		return nil, fmt.Errorf("storyfragment slug cannot be empty")
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragment, err := storyFragmentRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragment by slug %s: %w", slug, err)
	}

	s.logger.Content().Info("Successfully retrieved storyfragment by slug", "tenantId", tenantCtx.TenantID, "slug", slug, "found", storyFragment != nil, "duration", time.Since(start))

	return storyFragment, nil
}

// GetFullPayloadBySlug returns a storyfragment with full editorial payload (cache-first)
func (s *StoryFragmentService) GetFullPayloadBySlug(tenantCtx *tenant.Context, slug string) (*StoryFragmentFullPayload, error) {
	start := time.Now()
	if slug == "" {
		return nil, fmt.Errorf("storyfragment slug cannot be empty")
	}

	// Use factory pattern to get repositories from tenant context
	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	tractStackRepo := tenantCtx.TractStackRepo()
	menuRepo := tenantCtx.MenuRepo()
	paneRepo := tenantCtx.PaneRepo()

	// Get the storyfragment
	storyFragment, err := storyFragmentRepo.FindBySlug(tenantCtx.TenantID, slug)
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
		tractStack, err := tractStackRepo.FindByID(tenantCtx.TenantID, storyFragment.TractStackID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tractstack %s: %w", storyFragment.TractStackID, err)
		}
		payload.TractStack = tractStack
	}

	// Get related menu
	if storyFragment.MenuID != nil && *storyFragment.MenuID != "" {
		menu, err := menuRepo.FindByID(tenantCtx.TenantID, *storyFragment.MenuID)
		if err != nil {
			return nil, fmt.Errorf("failed to get menu %s: %w", *storyFragment.MenuID, err)
		}
		payload.Menu = menu
	}

	// Get related panes
	if len(storyFragment.PaneIDs) > 0 {
		panes, err := paneRepo.FindByIDs(tenantCtx.TenantID, storyFragment.PaneIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get panes for storyfragment %s: %w", storyFragment.ID, err)
		}
		payload.Panes = panes
	}

	s.logger.Content().Info("Successfully retrieved storyfragment full payload by slug", "tenantId", tenantCtx.TenantID, "slug", slug, "hasMenu", payload.Menu != nil, "hasTractStack", payload.TractStack != nil, "paneCount", len(payload.Panes), "duration", time.Since(start))

	return payload, nil
}

// GetHome returns the home storyfragment by reading the home slug from the tenant's configuration.
func (s *StoryFragmentService) GetHome(tenantCtx *tenant.Context) (*content.StoryFragmentNode, error) {
	start := time.Now()
	if tenantCtx == nil || tenantCtx.Config == nil || tenantCtx.Config.BrandConfig == nil {
		return nil, fmt.Errorf("tenant context or configuration is not available")
	}

	homeSlug := tenantCtx.Config.BrandConfig.HomeSlug
	if homeSlug == "" {
		homeSlug = "hello" // Fallback to the default
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragment, err := storyFragmentRepo.FindBySlug(tenantCtx.TenantID, homeSlug)
	if err != nil {
		return nil, fmt.Errorf("failed to get home storyfragment by slug '%s': %w", homeSlug, err)
	}

	s.logger.Content().Info("Successfully retrieved home storyfragment", "tenantId", tenantCtx.TenantID, "homeSlug", homeSlug, "found", storyFragment != nil, "duration", time.Since(start))

	return storyFragment, nil
}
