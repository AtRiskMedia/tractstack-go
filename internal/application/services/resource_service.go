// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
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
	imageFileService  *ImageFileService
}

// NewResourceService creates a new resource service singleton
func NewResourceService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker, contentMapService *ContentMapService, imageFileService *ImageFileService) *ResourceService {
	return &ResourceService{
		logger:            logger,
		perfTracker:       perfTracker,
		contentMapService: contentMapService,
		imageFileService:  imageFileService,
	}
}

// GetAll returns all resources for a tenant, leveraging the cache-first repository.
func (s *ResourceService) GetAll(tenantCtx *tenant.Context) ([]*content.ResourceNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_all_resources", tenantCtx.TenantID)
	defer marker.Complete()
	resourceRepo := tenantCtx.ResourceRepo()

	resources, err := resourceRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all resources from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved all resources", "tenantId", tenantCtx.TenantID, "count", len(resources), "duration", time.Since(start))
	marker.SetSuccess(true)
	return resources, nil
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

// Create creates a new resource and links any associated files
func (s *ResourceService) Create(tenantCtx *tenant.Context, resource *content.ResourceNode, fileIDs []string) error {
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
	err := resourceRepo.Store(tenantCtx.TenantID, resource, fileIDs)
	if err != nil {
		return fmt.Errorf("failed to create resource %s: %w", resource.ID, err)
	}

	tenantCtx.CacheManager.SetResource(tenantCtx.TenantID, resource)
	tenantCtx.CacheManager.AddResourceID(tenantCtx.TenantID, resource.ID)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after resource creation",
			"error", err, "resourceId", resource.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully created resource", "tenantId", tenantCtx.TenantID, "resourceId", resource.ID, "title", resource.Title, "slug", resource.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for CreateResource", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", resource.ID)

	return nil
}

// Update updates an existing resource and links any associated files
func (s *ResourceService) Update(tenantCtx *tenant.Context, resource *content.ResourceNode, fileIDs []string) error {
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

	err = resourceRepo.Update(tenantCtx.TenantID, resource, fileIDs)
	if err != nil {
		return fmt.Errorf("failed to update resource %s: %w", resource.ID, err)
	}

	tenantCtx.CacheManager.SetResource(tenantCtx.TenantID, resource)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after resource update",
			"error", err, "resourceId", resource.ID, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully updated resource", "tenantId", tenantCtx.TenantID, "resourceId", resource.ID, "title", resource.Title, "slug", resource.Slug, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for UpdateResource", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", resource.ID)

	return nil
}

// Delete deletes a resource and orchestrates the deletion of its associated image files.
func (s *ResourceService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("delete_resource", tenantCtx.TenantID)
	defer marker.Complete()
	if id == "" {
		return fmt.Errorf("resource ID cannot be empty")
	}

	resourceRepo := tenantCtx.ResourceRepo()

	// Step 1: Find all file IDs associated with this resource before deleting it.
	fileIDs, err := resourceRepo.FindFileIDsByResourceID(tenantCtx.TenantID, id)
	if err != nil {
		// Log the error but proceed. It's better to have an orphaned file than a resource that can't be deleted.
		s.logger.Content().Error("Failed to find associated file IDs for resource deletion", "error", err, "resourceId", id)
	}

	// Step 2: Delete the resource itself. The repository's Delete method handles the resource and junction table relationships.
	err = resourceRepo.Delete(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete resource %s: %w", id, err)
	}

	// Step 3: Now that the resource is gone, clean up the associated image files.
	if len(fileIDs) > 0 {
		s.logger.Content().Info("Deleting associated image files for resource", "resourceId", id, "fileCount", len(fileIDs))
		for _, fileID := range fileIDs {
			if err := s.imageFileService.Delete(tenantCtx, fileID); err != nil {
				// Log errors for individual file deletions but don't stop the overall process.
				s.logger.Content().Error("Failed to delete associated image file", "error", err, "fileId", fileID, "resourceId", id)
			}
		}
	}

	// Step 4: Invalidate caches.
	tenantCtx.CacheManager.InvalidateResource(tenantCtx.TenantID, id)
	tenantCtx.CacheManager.RemoveResourceID(tenantCtx.TenantID, id)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after resource deletion",
			"error", err, "resourceId", id, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully deleted resource", "tenantId", tenantCtx.TenantID, "resourceId", id, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for DeleteResource", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", id)

	return nil
}

// SearchBodies calls the repository to perform a prefix search on resource bodies.
func (s *ResourceService) SearchBodies(tenantCtx *tenant.Context, term string) ([]repositories.FTSResult, error) {
	repo := tenantCtx.ResourceRepo()
	return repo.SearchBodies(tenantCtx.TenantID, term)
}

// BatchSyncShopify synchronizes external Shopify products with internal resources.
// It detects changes based on the 'gid' in OptionsPayload and performs a batch upsert.
func (s *ResourceService) BatchSyncShopify(tenantCtx *tenant.Context, incomingResources []*content.ResourceNode) (int, int, int, int, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("batch_sync_shopify", tenantCtx.TenantID)
	defer marker.Complete()

	resourceRepo := tenantCtx.ResourceRepo()

	existingProducts, err := resourceRepo.FindByCategory(tenantCtx.TenantID, "product")
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to fetch existing products: %w", err)
	}
	existingServices, err := resourceRepo.FindByCategory(tenantCtx.TenantID, "service")
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to fetch existing services: %w", err)
	}

	totalExisting := len(existingProducts) + len(existingServices)
	totalIncoming := len(incomingResources)

	existingMap := make(map[string]*content.ResourceNode)
	for _, r := range existingProducts {
		if gid, ok := r.OptionsPayload["gid"].(string); ok {
			existingMap[gid] = r
		}
	}
	for _, r := range existingServices {
		if gid, ok := r.OptionsPayload["gid"].(string); ok {
			existingMap[gid] = r
		}
	}

	var creates []*content.ResourceNode
	var updates []*content.ResourceNode

	for _, incoming := range incomingResources {
		gid, ok := incoming.OptionsPayload["gid"].(string)
		if !ok || gid == "" {
			continue
		}

		if existing, found := existingMap[gid]; found {
			incomingData, _ := incoming.OptionsPayload["shopifyData"].(string)
			existingData, _ := existing.OptionsPayload["shopifyData"].(string)

			hasChanged := false

			switch {
			case incoming.Title != existing.Title:
				hasChanged = true
			case incoming.Slug != existing.Slug:
				hasChanged = true
			case incoming.OneLiner != existing.OneLiner:
				hasChanged = true
			case incomingData != existingData:
				hasChanged = true
			}

			if hasChanged {
				incoming.ID = existing.ID
				updates = append(updates, incoming)
			}
		} else {
			if incoming.ID == "" {
				incoming.ID = security.GenerateULID()
			}
			creates = append(creates, incoming)
		}
	}

	if len(creates) > 0 || len(updates) > 0 {
		if err := resourceRepo.BatchUpsert(tenantCtx.TenantID, creates, updates); err != nil {
			return 0, 0, 0, 0, fmt.Errorf("batch upsert failed: %w", err)
		}

		if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
			s.logger.Content().Error("Failed to refresh content map after shopify sync", "error", err)
		}
	}

	// Logging WARN to ensure visibility as per instructions
	s.logger.Content().Warn("Shopify batch sync completed",
		"tenantId", tenantCtx.TenantID,
		"created", len(creates),
		"updated", len(updates),
		"totalIncoming", totalIncoming,
		"totalExisting", totalExisting,
		"duration", time.Since(start))

	marker.SetSuccess(true)
	return len(creates), len(updates), totalIncoming, totalExisting, nil
}
