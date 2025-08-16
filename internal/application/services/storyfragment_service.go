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

// StoryFragmentFullPayload represents the full editorial payload for a storyfragment
type StoryFragmentFullPayload struct {
	StoryFragment *content.StoryFragmentNode `json:"storyFragment"`
	TractStack    *content.TractStackNode    `json:"tractStack,omitempty"`
	Menu          *content.MenuNode          `json:"menu,omitempty"`
	Panes         []*content.PaneNode        `json:"panes,omitempty"`
}

// StoryFragmentService orchestrates storyfragment operations with cache-first repository pattern
type StoryFragmentService struct {
	logger               *logging.ChanneledLogger
	perfTracker          *performance.Tracker
	contentMapService    *ContentMapService
	sessionBeliefService *SessionBeliefService
}

// NewStoryFragmentService creates a new storyfragment service singleton
func NewStoryFragmentService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker, contentMapService *ContentMapService, sessionBeliefService *SessionBeliefService) *StoryFragmentService {
	return &StoryFragmentService{
		logger:               logger,
		perfTracker:          perfTracker,
		contentMapService:    contentMapService,
		sessionBeliefService: sessionBeliefService,
	}
}

// GetAllIDs returns all storyfragment IDs for a tenant by leveraging the robust repository.
func (s *StoryFragmentService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_all_storyfragment_ids", tenantCtx.TenantID)
	defer marker.Complete()
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
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetAllStoryFragmentIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return ids, nil
}

// GetByID returns a storyfragment by ID (cache-first via repository)
func (s *StoryFragmentService) GetByID(tenantCtx *tenant.Context, id string) (*content.StoryFragmentNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_storyfragment_by_id", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return nil, fmt.Errorf("storyfragment ID cannot be empty")
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragment, err := storyFragmentRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragment %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved storyfragment by ID", "tenantId", tenantCtx.TenantID, "storyfragmentId", id, "found", storyFragment != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetStoryFragmentByID", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "storyFragmentId", id)

	return storyFragment, nil
}

// GetByIDs returns multiple storyfragments by IDs (cache-first with bulk loading via repository)
func (s *StoryFragmentService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.StoryFragmentNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_storyfragments_by_ids", tenantCtx.TenantID)
	defer marker.Complete()
	if len(ids) == 0 {
		return []*content.StoryFragmentNode{}, nil
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragments, err := storyFragmentRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragments by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved storyfragments by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(storyFragments), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetStoryFragmentsByIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(ids))

	return storyFragments, nil
}

// GetBySlug returns a storyfragment by slug (cache-first via repository)
func (s *StoryFragmentService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.StoryFragmentNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_storyfragment_by_slug", tenantCtx.TenantID)
	defer marker.Complete()
	if slug == "" {
		return nil, fmt.Errorf("storyfragment slug cannot be empty")
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragment, err := storyFragmentRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get storyfragment by slug %s: %w", slug, err)
	}

	s.logger.Content().Info("Successfully retrieved storyfragment by slug", "tenantId", tenantCtx.TenantID, "slug", slug, "found", storyFragment != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetStoryFragmentBySlug", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	return storyFragment, nil
}

// GetFullPayloadBySlug returns a storyfragment with full editorial payload (cache-first)
func (s *StoryFragmentService) GetFullPayloadBySlug(tenantCtx *tenant.Context, slug string) (*StoryFragmentFullPayload, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_storyfragment_full_payload", tenantCtx.TenantID)
	defer marker.Complete()
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
		return nil, nil
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
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetFullPayloadBySlug", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	return payload, nil
}

// GetHome returns the home storyfragment by reading the home slug from the tenant's configuration.
func (s *StoryFragmentService) GetHome(tenantCtx *tenant.Context, sessionID string) (*content.StoryFragmentNode, error) {
	if tenantCtx == nil || tenantCtx.Config == nil || tenantCtx.Config.BrandConfig == nil {
		return nil, fmt.Errorf("tenant context or configuration is not available")
	}
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_home_storyfragment", tenantCtx.TenantID)
	defer marker.Complete()

	homeSlug := tenantCtx.Config.BrandConfig.HomeSlug
	if homeSlug == "" {
		homeSlug = "hello" // Fallback to the default
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragment, err := storyFragmentRepo.FindBySlug(tenantCtx.TenantID, homeSlug)
	if err != nil {
		return nil, fmt.Errorf("failed to get home storyfragment by slug '%s': %w", homeSlug, err)
	}

	// Enrich with metadata (menu, isHome flag, etc.)
	err = s.EnrichWithMetadata(tenantCtx, storyFragment, sessionID)
	if err != nil {
		s.logger.Content().Debug("Failed to enrich home storyfragment metadata", "error", err)
	}

	s.logger.Content().Info("Successfully retrieved home storyfragment", "tenantId", tenantCtx.TenantID, "homeSlug", homeSlug, "found", storyFragment != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetHomeStoryFragment", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return storyFragment, nil
}

// Create creates a new storyfragment
func (s *StoryFragmentService) Create(tenantCtx *tenant.Context, sf *content.StoryFragmentNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("create_storyfragment", tenantCtx.TenantID)
	defer marker.Complete()
	if sf.ID == "" {
		sf.ID = security.GenerateULID()
	}
	if sf == nil {
		return fmt.Errorf("storyfragment cannot be nil")
	}
	if sf.Title == "" {
		return fmt.Errorf("storyfragment title cannot be empty")
	}
	if sf.Slug == "" {
		return fmt.Errorf("storyfragment slug cannot be empty")
	}
	if sf.TractStackID == "" {
		return fmt.Errorf("tractstack ID cannot be empty")
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	err := storyFragmentRepo.Store(tenantCtx.TenantID, sf)
	if err != nil {
		return fmt.Errorf("failed to create storyfragment %s: %w", sf.ID, err)
	}

	// Surgically add the new item to the item cache and the master ID list
	tenantCtx.CacheManager.SetStoryFragment(tenantCtx.TenantID, sf)
	tenantCtx.CacheManager.AddStoryFragmentID(tenantCtx.TenantID, sf.ID)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after storyfragment creation",
			"error", err, "storyFragmentId", sf.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully created storyfragment", "tenantId", tenantCtx.TenantID, "storyfragmentId", sf.ID, "title", sf.Title, "slug", sf.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for CreateStoryFragment", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "storyFragmentId", sf.ID)

	return nil
}

// Update updates an existing storyfragment
func (s *StoryFragmentService) Update(tenantCtx *tenant.Context, sf *content.StoryFragmentNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("update_storyfragment", tenantCtx.TenantID)
	defer marker.Complete()
	if sf == nil {
		return fmt.Errorf("storyfragment cannot be nil")
	}
	if sf.ID == "" {
		return fmt.Errorf("storyfragment ID cannot be empty")
	}
	if sf.Title == "" {
		return fmt.Errorf("storyfragment title cannot be empty")
	}
	if sf.Slug == "" {
		return fmt.Errorf("storyfragment slug cannot be empty")
	}
	if sf.TractStackID == "" {
		return fmt.Errorf("tractstack ID cannot be empty")
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()

	existing, err := storyFragmentRepo.FindByID(tenantCtx.TenantID, sf.ID)
	if err != nil {
		return fmt.Errorf("failed to verify storyfragment %s exists: %w", sf.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("storyfragment %s not found", sf.ID)
	}

	err = storyFragmentRepo.Update(tenantCtx.TenantID, sf)
	if err != nil {
		return fmt.Errorf("failed to update storyfragment %s: %w", sf.ID, err)
	}

	// Surgically update the item in the item cache. The ID list is not affected.
	tenantCtx.CacheManager.SetStoryFragment(tenantCtx.TenantID, sf)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after storyfragment update",
			"error", err, "storyFragmentId", sf.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully updated storyfragment", "tenantId", tenantCtx.TenantID, "storyfragmentId", sf.ID, "title", sf.Title, "slug", sf.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for UpdateStoryFragment", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "storyFragmentId", sf.ID)

	return nil
}

// Delete deletes a storyfragment
func (s *StoryFragmentService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("delete_storyfragment", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return fmt.Errorf("storyfragment ID cannot be empty")
	}

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()

	existing, err := storyFragmentRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify storyfragment %s exists: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("storyfragment %s not found", id)
	}

	err = storyFragmentRepo.Delete(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete storyfragment %s: %w", id, err)
	}

	// Surgically remove the single item from the item cache.
	tenantCtx.CacheManager.InvalidateStoryFragment(tenantCtx.TenantID, id)
	// Surgically remove the ID from the master ID list.
	tenantCtx.CacheManager.RemoveStoryFragmentID(tenantCtx.TenantID, id)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after storyfragment deletion",
			"error", err, "storyFragmentId", id, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully deleted storyfragment", "tenantId", tenantCtx.TenantID, "storyfragmentId", id, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for DeleteStoryFragment", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "storyFragmentId", id)

	return nil
}

func (s *StoryFragmentService) EnrichWithMetadata(tenantCtx *tenant.Context, storyFragment *content.StoryFragmentNode, sessionID string) error {
	// 1. Set IsHome flag
	homeSlug := ""
	if tenantCtx.Config != nil && tenantCtx.Config.BrandConfig != nil {
		homeSlug = tenantCtx.Config.BrandConfig.HomeSlug
	}
	if homeSlug == "" {
		homeSlug = "hello"
	}
	storyFragment.IsHome = (storyFragment.Slug == homeSlug)

	// 2. Load and attach Menu
	if storyFragment.MenuID != nil && storyFragment.Menu == nil {
		menuRepo := tenantCtx.MenuRepo()
		menu, err := menuRepo.FindByID(tenantCtx.TenantID, *storyFragment.MenuID)
		if err != nil {
			s.logger.Content().Debug("Failed to load menu for storyfragment", "menuId", *storyFragment.MenuID, "error", err)
		} else {
			storyFragment.Menu = menu
		}
	}

	// 3. Extract and attach CodeHookTargets
	if storyFragment.CodeHookTargets == nil && len(storyFragment.PaneIDs) > 0 {
		paneRepo := tenantCtx.PaneRepo()
		panes, err := paneRepo.FindByIDs(tenantCtx.TenantID, storyFragment.PaneIDs)
		if err != nil {
			s.logger.Content().Debug("Failed to load panes for codeHook extraction", "error", err)
		} else {
			codeHookTargets := make(map[string]string)
			for _, pane := range panes {
				if pane != nil && pane.CodeHookTarget != nil && *pane.CodeHookTarget != "" {
					codeHookTargets[pane.ID] = *pane.CodeHookTarget
				}
			}
			storyFragment.CodeHookTargets = codeHookTargets
		}
	}

	// 4. Session belief context warming
	if sessionID != "" {
		_, err := s.sessionBeliefService.CreateSessionBeliefContext(tenantCtx, sessionID, storyFragment.ID)
		if err != nil {
			s.logger.Content().Debug("Failed to create session belief context", "error", err)
		}
	}

	return nil
}

func (s *StoryFragmentService) GetImpressionsByPaneIDs(tenantCtx *tenant.Context, paneIDs []string) ([]map[string]any, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_impressions_by_pane_ids", tenantCtx.TenantID)
	defer marker.Complete()

	s.logger.Content().Debug("=== IMPRESSIONS DEBUG START ===", "paneIDsCount", len(paneIDs), "paneIDs", paneIDs)

	if len(paneIDs) == 0 {
		s.logger.Content().Debug("No pane IDs provided for impressions")
		return []map[string]any{}, nil
	}

	// Get the pane repository from tenant context
	paneRepo := tenantCtx.PaneRepo()

	// Use the existing bulk pane retrieval method
	panes, err := paneRepo.FindByIDs(tenantCtx.TenantID, paneIDs)
	if err != nil {
		s.logger.Content().Debug("Failed to get panes for impressions", "error", err.Error())
		return nil, fmt.Errorf("failed to get panes for impressions: %w", err)
	}

	s.logger.Content().Debug("Retrieved panes for impressions", "panesCount", len(panes))

	var impressions []map[string]any

	// Extract impressions from each pane's OptionsPayload.nodes
	for i, pane := range panes {
		s.logger.Content().Debug("Processing pane", "index", i, "paneID", pane.ID, "hasOptionsPayload", pane.OptionsPayload != nil)

		if pane == nil || pane.OptionsPayload == nil {
			s.logger.Content().Debug("Skipping pane - nil or no options payload", "paneID", pane.ID)
			continue
		}

		// Check if pane has nodes in its OptionsPayload
		if nodes, exists := pane.OptionsPayload["nodes"]; exists {
			s.logger.Content().Debug("Found nodes in pane options payload", "paneID", pane.ID, "nodesType", fmt.Sprintf("%T", nodes))

			if nodesArray, ok := nodes.([]any); ok {
				s.logger.Content().Debug("Nodes is array", "paneID", pane.ID, "nodeCount", len(nodesArray))

				// Iterate through nodes looking for impressions
				for j, nodeInterface := range nodesArray {
					if nodeMap, ok := nodeInterface.(map[string]any); ok {
						nodeType, hasNodeType := nodeMap["nodeType"]
						tagName, hasTagName := nodeMap["tagName"]

						s.logger.Content().Debug("Processing node",
							"paneID", pane.ID,
							"nodeIndex", j,
							"hasNodeType", hasNodeType,
							"nodeType", nodeType,
							"hasTagName", hasTagName,
							"tagName", tagName)

						// Check if this node is an impression
						if nodeType, exists := nodeMap["nodeType"]; exists {
							if nodeTypeStr, ok := nodeType.(string); ok && nodeTypeStr == "Impression" {
								if tagName, exists := nodeMap["tagName"]; exists {
									if tagNameStr, ok := tagName.(string); ok && tagNameStr == "impression" {

										s.logger.Content().Debug("FOUND IMPRESSION NODE!",
											"paneID", pane.ID,
											"nodeID", nodeMap["id"],
											"title", nodeMap["title"],
											"body", nodeMap["body"],
											"buttonText", nodeMap["buttonText"],
											"actionsLisp", nodeMap["actionsLisp"])

										// Create impression object with all required fields
										impression := map[string]any{
											"id":          nodeMap["id"],
											"nodeType":    "Impression",
											"tagName":     "impression",
											"parentId":    pane.ID,
											"title":       nodeMap["title"],
											"body":        nodeMap["body"],
											"buttonText":  nodeMap["buttonText"],
											"actionsLisp": nodeMap["actionsLisp"],
										}

										// Only add if required fields are present
										if impression["id"] != "" && impression["title"] != "" &&
											impression["body"] != "" && impression["buttonText"] != "" &&
											impression["actionsLisp"] != "" {
											s.logger.Content().Debug("Adding impression to results", "impressionID", impression["id"])
											impressions = append(impressions, impression)
										} else {
											s.logger.Content().Debug("Skipping incomplete impression",
												"impressionID", impression["id"],
												"hasTitle", impression["title"] != "",
												"hasBody", impression["body"] != "",
												"hasButtonText", impression["buttonText"] != "",
												"hasActionsLisp", impression["actionsLisp"] != "")
										}
									} else {
										s.logger.Content().Debug("Node has Impression type but wrong tagName", "paneID", pane.ID, "tagName", tagName)
									}
								} else {
									s.logger.Content().Debug("Node has Impression type but no tagName", "paneID", pane.ID)
								}
							} else {
								s.logger.Content().Debug("Node type not Impression", "paneID", pane.ID, "nodeType", nodeType)
							}
						} else {
							s.logger.Content().Debug("Node has no nodeType field", "paneID", pane.ID)
						}
					} else {
						s.logger.Content().Debug("Node is not a map", "paneID", pane.ID, "nodeIndex", j, "nodeType", fmt.Sprintf("%T", nodeInterface))
					}
				}
			} else {
				s.logger.Content().Debug("Nodes is not an array", "paneID", pane.ID, "nodesType", fmt.Sprintf("%T", nodes))
			}
		} else {
			s.logger.Content().Debug("No nodes field in pane options payload", "paneID", pane.ID, "optionsKeys", func() []string {
				keys := make([]string, 0, len(pane.OptionsPayload))
				for k := range pane.OptionsPayload {
					keys = append(keys, k)
				}
				return keys
			}())
		}
	}

	s.logger.Content().Debug("=== IMPRESSIONS DEBUG END ===",
		"finalImpressionCount", len(impressions),
		"duration", time.Since(start))

	marker.SetSuccess(true)
	return impressions, nil
}
