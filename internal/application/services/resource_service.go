// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/media"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
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

// GetByCategory returns all resources associated with a specific category slug.
// This is required to support in-memory lookups for integrations.
func (s *ResourceService) GetByCategory(tenantCtx *tenant.Context, category string) ([]*content.ResourceNode, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_resources_by_category", tenantCtx.TenantID)
	defer marker.Complete()

	resourceRepo := tenantCtx.ResourceRepo()
	resources, err := resourceRepo.FindByCategory(tenantCtx.TenantID, category)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources by category %s: %w", category, err)
	}

	s.logger.Content().Debug("Retrieved resources by category",
		"tenantId", tenantCtx.TenantID,
		"category", category,
		"count", len(resources),
		"duration", time.Since(start))

	marker.SetSuccess(true)
	return resources, nil
}

// UpsertShopifyResource handles single-item synchronization from a Shopify webhook or reconciliation scan.
// It supports multi-variant image synchronization, performs source URL change detection,
// and ensures all variant images are correctly linked in the database.
// Returns the operation performed ("created", "updated", or "none") and any error.
func (s *ResourceService) UpsertShopifyResource(tenantCtx *tenant.Context, resource *content.ResourceNode) (string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("upsert_shopify_resource", tenantCtx.TenantID)
	defer marker.Complete()

	// 1. Ensure cache is populated for lookup (in-memory scan)
	products, err := s.GetByCategory(tenantCtx, "product")
	if err != nil {
		return "", fmt.Errorf("failed to pre-load products for upsert lookup: %w", err)
	}
	services, err := s.GetByCategory(tenantCtx, "service")
	if err != nil {
		return "", fmt.Errorf("failed to pre-load services for upsert lookup: %w", err)
	}

	// 2. Identify the Target GID from the incoming payload
	targetGID, ok := resource.OptionsPayload["gid"].(string)
	if !ok || targetGID == "" {
		return "", fmt.Errorf("incoming shopify resource missing required 'gid' in optionsPayload")
	}

	// 3. Scan for existing resource
	var existing *content.ResourceNode
	for _, p := range products {
		if gid, ok := p.OptionsPayload["gid"].(string); ok && gid == targetGID {
			existing = p
			break
		}
	}
	if existing == nil {
		for _, svc := range services {
			if gid, ok := svc.OptionsPayload["gid"].(string); ok && gid == targetGID {
				existing = svc
				break
			}
		}
	}

	// --- Multi-Image Sync Logic ---

	// allActiveFileIDs tracks every file that should be linked to this resource in the DB
	var allActiveFileIDs []string
	// filesToDelete tracks orphaned File IDs replaced during change detection
	var filesToDelete []string

	// Helper for image processing
	mediaPath := filepath.Join(config.BackendPath, "config", tenantCtx.TenantID, "media")
	processor := media.NewImageProcessor(mediaPath)

	// A. Process Legacy/Main Image (Preserve existing logic for 'image' field)
	activeMainFileID := ""
	if existing != nil {
		if val, ok := existing.OptionsPayload["image"].(string); ok {
			activeMainFileID = val
		}
	}

	incomingMainURL, _ := resource.OptionsPayload["shopifyImageSourceUrl"].(string)
	storedMainURL := ""
	if existing != nil {
		storedMainURL, _ = existing.OptionsPayload["shopifyImageSourceUrl"].(string)
	}

	if incomingMainURL != "" && (incomingMainURL != storedMainURL || activeMainFileID == "") {
		newFileID := security.GenerateULID()
		src, srcSet, err := processor.ProcessURL(incomingMainURL, newFileID)
		if err == nil {
			imageFile := &content.ImageFileNode{
				ID:             newFileID,
				NodeType:       "File",
				Filename:       filepath.Base(src),
				AltDescription: resource.Title,
				Src:            src,
				SrcSet:         srcSet,
			}
			if err := s.imageFileService.Create(tenantCtx, imageFile); err == nil {
				if activeMainFileID != "" {
					filesToDelete = append(filesToDelete, activeMainFileID)
				}
				activeMainFileID = newFileID
			}
		}
	}
	if activeMainFileID != "" {
		resource.OptionsPayload["image"] = activeMainFileID
		allActiveFileIDs = append(allActiveFileIDs, activeMainFileID)
	}

	// B. Process Variant Image Map (VariantGID -> {fileId, sourceUrl})
	type variantEntry struct {
		FileID    string `json:"fileId,omitempty"`
		SourceURL string `json:"sourceUrl"`
	}

	incomingVariantMap := make(map[string]variantEntry)
	if raw, ok := resource.OptionsPayload["shopifyImage"].(string); ok && raw != "" {
		_ = json.Unmarshal([]byte(raw), &incomingVariantMap)
	}

	storedVariantMap := make(map[string]variantEntry)
	if existing != nil {
		if raw, ok := existing.OptionsPayload["shopifyImage"].(string); ok && raw != "" {
			_ = json.Unmarshal([]byte(raw), &storedVariantMap)
		}
	}

	finalVariantMap := make(map[string]variantEntry)
	variantChangeDetected := false

	for vGID, incoming := range incomingVariantMap {
		stored := storedVariantMap[vGID]
		useFileID := stored.FileID

		// Change detection: Check if URL changed or if we are missing the local file ID
		if incoming.SourceURL != "" && (incoming.SourceURL != stored.SourceURL || useFileID == "") {
			newFileID := security.GenerateULID()
			src, srcSet, err := processor.ProcessURL(incoming.SourceURL, newFileID)
			if err == nil {
				imageFile := &content.ImageFileNode{
					ID:             newFileID,
					NodeType:       "File",
					Filename:       filepath.Base(src),
					AltDescription: fmt.Sprintf("%s - variant", resource.Title),
					Src:            src,
					SrcSet:         srcSet,
				}
				if err := s.imageFileService.Create(tenantCtx, imageFile); err == nil {
					if stored.FileID != "" {
						filesToDelete = append(filesToDelete, stored.FileID)
					}
					useFileID = newFileID
					variantChangeDetected = true
				}
			}
		}

		if useFileID != "" {
			finalVariantMap[vGID] = variantEntry{
				FileID:    useFileID,
				SourceURL: incoming.SourceURL,
			}
			allActiveFileIDs = append(allActiveFileIDs, useFileID)
		}
	}

	// Serialize the updated variant map back to the resource
	if mapData, err := json.Marshal(finalVariantMap); err == nil {
		resource.OptionsPayload["shopifyImage"] = string(mapData)
	}

	// 4. Persistence Path
	resourceRepo := tenantCtx.ResourceRepo()
	operation := "none"

	if existing != nil {
		// UPDATE Path
		incomingData, _ := resource.OptionsPayload["shopifyData"].(string)
		existingData, _ := existing.OptionsPayload["shopifyData"].(string)

		hasChanged := variantChangeDetected ||
			resource.Title != existing.Title ||
			resource.OneLiner != existing.OneLiner ||
			incomingData != existingData ||
			resource.OptionsPayload["image"] != existing.OptionsPayload["image"]

		if hasChanged {
			resource.ID = existing.ID
			if resource.CategorySlug == nil {
				resource.CategorySlug = existing.CategorySlug
			}
			if err := resourceRepo.Update(tenantCtx.TenantID, resource, allActiveFileIDs); err != nil {
				return "", fmt.Errorf("failed to update shopify resource %s: %w", resource.ID, err)
			}
			operation = "updated"
		}
	} else {
		// CREATE Path
		if resource.ID == "" {
			resource.ID = security.GenerateULID()
		}
		if resource.CategorySlug == nil {
			defaultCat := "product"
			resource.CategorySlug = &defaultCat
		}
		if err := resourceRepo.Store(tenantCtx.TenantID, resource, allActiveFileIDs); err != nil {
			return "", fmt.Errorf("failed to create shopify resource: %w", err)
		}
		operation = "created"
	}

	// 5. Cleanup Replaced Files
	if operation != "none" {
		for _, fileID := range filesToDelete {
			if err := s.imageFileService.Delete(tenantCtx, fileID); err != nil {
				s.logger.Content().Warn("Failed to cleanup replaced shopify image", "error", err, "fileId", fileID)
			}
		}
		// Update cache and refresh content map
		tenantCtx.CacheManager.SetResource(tenantCtx.TenantID, resource)
		if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
			s.logger.Content().Error("Failed to refresh content map after shopify upsert", "error", err)
		}
	}

	s.logger.Content().Info("Shopify resource upsert completed",
		"tenantId", tenantCtx.TenantID,
		"gid", targetGID,
		"operation", operation,
		"duration", time.Since(start))

	marker.SetSuccess(true)
	return operation, nil
}

// SyncShopifyDeletion handles the removal of a resource triggered by a Shopify deletion event.
// It is idempotent: if the resource is not found, it returns "none" and no error.
func (s *ResourceService) SyncShopifyDeletion(tenantCtx *tenant.Context, gid string) (string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("sync_shopify_deletion", tenantCtx.TenantID)
	defer marker.Complete()

	if gid == "" {
		return "", fmt.Errorf("shopify GID cannot be empty for deletion")
	}

	// 1. Ensure cache is populated for lookup (in-memory scan)
	// We check both categories where Shopify resources might live.
	products, err := s.GetByCategory(tenantCtx, "product")
	if err != nil {
		return "", fmt.Errorf("failed to load products for deletion lookup: %w", err)
	}
	services, err := s.GetByCategory(tenantCtx, "service")
	if err != nil {
		return "", fmt.Errorf("failed to load services for deletion lookup: %w", err)
	}

	// 2. Scan for existing resource by GID
	var targetID string
	for _, p := range products {
		if val, ok := p.OptionsPayload["gid"].(string); ok && val == gid {
			targetID = p.ID
			break
		}
	}
	if targetID == "" {
		for _, svc := range services {
			if val, ok := svc.OptionsPayload["gid"].(string); ok && val == gid {
				targetID = svc.ID
				break
			}
		}
	}

	// 3. Idempotent check: if not found, we are done.
	if targetID == "" {
		s.logger.Content().Debug("Shopify resource not found for deletion; skipping",
			"tenantId", tenantCtx.TenantID,
			"gid", gid)
		marker.SetSuccess(true)
		return "none", nil
	}

	// 4. Perform the actual deletion using the existing orchestration method.
	// This handles DB removal, junction tables, FTS, and cache invalidation.
	if err := s.Delete(tenantCtx, targetID); err != nil {
		return "", fmt.Errorf("failed to delete Shopify-linked resource %s: %w", targetID, err)
	}

	s.logger.Content().Info("Shopify resource deleted successfully",
		"tenantId", tenantCtx.TenantID,
		"gid", gid,
		"resourceId", targetID,
		"duration", time.Since(start))

	marker.SetSuccess(true)
	return "deleted", nil
}
