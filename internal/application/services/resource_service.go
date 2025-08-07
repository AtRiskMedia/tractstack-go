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

// ResourceService orchestrates resource operations with cache-first repository pattern
type ResourceService struct {
	logger            *logging.ChanneledLogger
	perfTracker       *performance.Tracker
	contentMapService *ContentMapService
}

// NewResourceService creates a new resource service singleton
func NewResourceService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker, contentMapService *ContentMapService) *ResourceService {
	return &ResourceService{
		logger:            logger,
		perfTracker:       perfTracker,
		contentMapService: contentMapService,
	}
}

// GetAllIDs returns all resource IDs for a tenant by leveraging the robust repository.
func (s *ResourceService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_all_resource_ids", tenantCtx.TenantID)
	defer marker.Complete()
	resourceRepo := tenantCtx.ResourceRepo()

	// The repository's FindAll method is now the cache-aware entry point.
	resources, err := resourceRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all resources from repository: %w", err)
	}

	// Extract IDs from the full objects.
	ids := make([]string, len(resources))
	for i, resource := range resources {
		ids[i] = resource.ID
	}

	s.logger.Content().Info("Successfully retrieved all resource IDs", "tenantId", tenantCtx.TenantID, "count", len(ids), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetAllResourceIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return ids, nil
}

// GetByID returns a resource by ID (cache-first via repository)
func (s *ResourceService) GetByID(tenantCtx *tenant.Context, id string) (*content.ResourceNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_resource_by_id", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return nil, fmt.Errorf("resource ID cannot be empty")
	}

	resourceRepo := tenantCtx.ResourceRepo()
	resource, err := resourceRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved resource by ID", "tenantId", tenantCtx.TenantID, "resourceId", id, "found", resource != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetResourceByID", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", id)

	return resource, nil
}

// GetByIDs returns multiple resources by IDs (cache-first with bulk loading via repository)
func (s *ResourceService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.ResourceNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_resources_by_ids", tenantCtx.TenantID)
	defer marker.Complete()
	if len(ids) == 0 {
		return []*content.ResourceNode{}, nil
	}

	resourceRepo := tenantCtx.ResourceRepo()
	resources, err := resourceRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved resources by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(resources), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetResourcesByIDs", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(ids))

	return resources, nil
}

// GetBySlug returns a resource by slug (cache-first via repository)
func (s *ResourceService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.ResourceNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_resource_by_slug", tenantCtx.TenantID)
	defer marker.Complete()
	if slug == "" {
		return nil, fmt.Errorf("resource slug cannot be empty")
	}

	resourceRepo := tenantCtx.ResourceRepo()
	resource, err := resourceRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource by slug %s: %w", slug, err)
	}

	s.logger.Content().Info("Successfully retrieved resource by slug", "tenantId", tenantCtx.TenantID, "slug", slug, "found", resource != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetResourceBySlug", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	return resource, nil
}

// GetByFilters returns resources by multiple filter criteria (cache-first via repository)
func (s *ResourceService) GetByFilters(tenantCtx *tenant.Context, ids []string, categories []string, slugs []string) ([]*content.ResourceNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_resources_by_filters", tenantCtx.TenantID)
	defer marker.Complete()
	if len(ids) == 0 && len(categories) == 0 && len(slugs) == 0 {
		return []*content.ResourceNode{}, nil
	}

	resourceRepo := tenantCtx.ResourceRepo()
	resources, err := resourceRepo.FindByFilters(tenantCtx.TenantID, ids, categories, slugs)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources by filters from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved resources by filters", "tenantId", tenantCtx.TenantID, "idCount", len(ids), "categoryCount", len(categories), "slugCount", len(slugs), "foundCount", len(resources), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetByFilters", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return resources, nil
}

// Create creates a new resource
func (s *ResourceService) Create(tenantCtx *tenant.Context, resource *content.ResourceNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("create_resource", tenantCtx.TenantID)
	defer marker.Complete()
	if resource.ID == "" {
		resource.ID = security.GenerateULID()
	}
	if resource == nil {
		return fmt.Errorf("resource cannot be nil")
	}
	if resource.Title == "" {
		return fmt.Errorf("resource title cannot be empty")
	}
	if resource.Slug == "" {
		return fmt.Errorf("resource slug cannot be empty")
	}

	resourceRepo := tenantCtx.ResourceRepo()
	err := resourceRepo.Store(tenantCtx.TenantID, resource)
	if err != nil {
		return fmt.Errorf("failed to create resource %s: %w", resource.ID, err)
	}

	// 1. Surgically add the new item to the item cache.
	tenantCtx.CacheManager.SetResource(tenantCtx.TenantID, resource)
	// 2. Surgically add the new ID to the master ID list.
	tenantCtx.CacheManager.AddResourceID(tenantCtx.TenantID, resource.ID)
	// 3. Refresh content map after successful creation
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after resource creation",
			"error", err, "resourceId", resource.ID, "tenantId", tenantCtx.TenantID)
		// Do not fail the operation; the content map will be refreshed on the next cache miss.
	}

	s.logger.Content().Info("Successfully created resource", "tenantId", tenantCtx.TenantID, "resourceId", resource.ID, "title", resource.Title, "slug", resource.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for CreateResource", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", resource.ID)

	return nil
}

// Update updates an existing resource
func (s *ResourceService) Update(tenantCtx *tenant.Context, resource *content.ResourceNode) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("update_resource", tenantCtx.TenantID)
	defer marker.Complete()
	if resource == nil {
		return fmt.Errorf("resource cannot be nil")
	}
	if resource.ID == "" {
		return fmt.Errorf("resource ID cannot be empty")
	}
	if resource.Title == "" {
		return fmt.Errorf("resource title cannot be empty")
	}
	if resource.Slug == "" {
		return fmt.Errorf("resource slug cannot be empty")
	}

	resourceRepo := tenantCtx.ResourceRepo()

	existing, err := resourceRepo.FindByID(tenantCtx.TenantID, resource.ID)
	if err != nil {
		return fmt.Errorf("failed to verify resource %s exists: %w", resource.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("resource %s not found", resource.ID)
	}

	err = resourceRepo.Update(tenantCtx.TenantID, resource)
	if err != nil {
		return fmt.Errorf("failed to update resource %s: %w", resource.ID, err)
	}

	// 1. Surgically update the item in the item cache. The ID list is not affected.
	tenantCtx.CacheManager.SetResource(tenantCtx.TenantID, resource)
	// 2. Refresh content map after successful creation
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after resource update",
			"error", err, "resourceId", resource.ID, "tenantId", tenantCtx.TenantID)
		// Do not fail the operation; the content map will be refreshed on the next cache miss.
	}

	s.logger.Content().Info("Successfully updated resource", "tenantId", tenantCtx.TenantID, "resourceId", resource.ID, "title", resource.Title, "slug", resource.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for UpdateResource", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", resource.ID)

	return nil
}

// Delete deletes a resource
func (s *ResourceService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("delete_resource", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return fmt.Errorf("resource ID cannot be empty")
	}

	resourceRepo := tenantCtx.ResourceRepo()

	existing, err := resourceRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify resource %s exists: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("resource %s not found", id)
	}

	err = resourceRepo.Delete(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete resource %s: %w", id, err)
	}

	// 1. Surgically remove the single item from the item cache.
	tenantCtx.CacheManager.InvalidateResource(tenantCtx.TenantID, id)
	// 2. Surgically remove the ID from the master ID list.
	tenantCtx.CacheManager.RemoveResourceID(tenantCtx.TenantID, id)
	// 3. Refresh content map after successful creation
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after resource deletion",
			"error", err, "resourceId", id, "tenantId", tenantCtx.TenantID)
		// Do not fail the operation; the content map will be refreshed on the next cache miss.
	}

	s.logger.Content().Info("Successfully deleted resource", "tenantId", tenantCtx.TenantID, "resourceId", id, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for DeleteResource", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", id)

	return nil
}
