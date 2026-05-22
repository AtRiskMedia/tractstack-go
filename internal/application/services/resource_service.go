// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
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

var legacyShopifyServiceFields = []string{
	"gid",
	"shopifyData",
	"shopifyImage",
	"shopifyImageSourceUrl",
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

func clearShopifyLinkage(options map[string]any) map[string]any {
	if options == nil {
		return map[string]any{}
	}
	clean := make(map[string]any, len(options))
	for k, v := range options {
		clean[k] = v
	}
	for _, key := range legacyShopifyServiceFields {
		delete(clean, key)
	}
	return clean
}

func getResourceCategory(resource *content.ResourceNode) string {
	if resource == nil || resource.CategorySlug == nil {
		return ""
	}
	return *resource.CategorySlug
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
	if id == "" {
		return fmt.Errorf("resource ID cannot be empty")
	}

	resource, err := s.GetByID(tenantCtx, id)
	if err != nil {
		return fmt.Errorf("failed to load resource before delete: %w", err)
	}
	if resource == nil {
		return fmt.Errorf("resource %s not found", id)
	}

	category := getResourceCategory(resource)
	gid, _ := resource.OptionsPayload["gid"].(string)

	if category == "product" && gid != "" {
		services, err := s.GetByCategory(tenantCtx, "service")
		if err != nil {
			return fmt.Errorf("failed to load services for product delete guard: %w", err)
		}
		for _, svc := range services {
			if linkedGID, _ := svc.OptionsPayload["gid"].(string); linkedGID == gid {
				return fmt.Errorf("cannot delete product linked to services; unlink services first")
			}
		}
	}

	if category == "service" && gid != "" {
		services, err := s.GetByCategory(tenantCtx, "service")
		if err != nil {
			return fmt.Errorf("failed to load services for delete workflow: %w", err)
		}
		var linkedServiceCount int
		for _, svc := range services {
			if linkedGID, _ := svc.OptionsPayload["gid"].(string); linkedGID == gid {
				linkedServiceCount++
			}
		}

		if err := s.deleteResourceByID(tenantCtx, id); err != nil {
			return err
		}

		if linkedServiceCount == 1 {
			products, err := s.GetByCategory(tenantCtx, "product")
			if err != nil {
				return fmt.Errorf("failed to load products for cascading delete: %w", err)
			}
			for _, product := range products {
				if productGID, _ := product.OptionsPayload["gid"].(string); productGID == gid {
					if err := s.deleteResourceByID(tenantCtx, product.ID); err != nil {
						return fmt.Errorf("failed to delete canonical product %s after service delete: %w", product.ID, err)
					}
				}
			}
		}
		return nil
	}

	return s.deleteResourceByID(tenantCtx, id)
}

func (s *ResourceService) deleteResourceByID(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	marker := s.perfTracker.StartOperation("delete_resource", tenantCtx.TenantID)
	defer marker.Complete()

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

// ExistsByShopifyGID checks if a Shopify resource is tracked locally without hydrating the full node.
func (s *ResourceService) ExistsByShopifyGID(tenantCtx *tenant.Context, gid string) (bool, error) {
	marker := s.perfTracker.StartOperation("exists_shopify_gid", tenantCtx.TenantID)
	defer marker.Complete()

	resourceRepo := tenantCtx.ResourceRepo()
	exists, err := resourceRepo.ExistsByShopifyGID(tenantCtx.TenantID, gid)
	if err != nil {
		return false, fmt.Errorf("repository check for shopify gid failed: %w", err)
	}

	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for ExistsByShopifyGID", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "exists", exists)
	return exists, nil
}

// UpsertShopifyResource handles single-item synchronization from a Shopify webhook or reconciliation scan.
// It merges fresh Shopify data with existing local metadata to preserve tracking keys like 'group' or 'serviceBound'.
func (s *ResourceService) UpsertShopifyResource(tenantCtx *tenant.Context, resource *content.ResourceNode) (string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("upsert_shopify_resource", tenantCtx.TenantID)
	defer marker.Complete()

	// 1. Ensure cache is populated for lookup (in-memory scan)
	products, err := s.GetByCategory(tenantCtx, "product")
	if err != nil {
		return "", fmt.Errorf("failed to pre-load products for upsert lookup: %w", err)
	}

	// 2. Identify the Target GID from the incoming payload
	targetGID, ok := resource.OptionsPayload["gid"].(string)
	if !ok || targetGID == "" {
		return "", fmt.Errorf("incoming shopify resource missing required 'gid' in optionsPayload")
	}

	// 3. Scan for existing canonical product resource only.
	var existing *content.ResourceNode
	for _, p := range products {
		if gid, ok := p.OptionsPayload["gid"].(string); ok && gid == targetGID {
			existing = p
			break
		}
	}

	// 4. NON-DESTRUCTIVE MERGE: Preserve local-only metadata
	if existing != nil {
		for k, v := range existing.OptionsPayload {
			// If the fresh GraphQL payload doesn't contain a key present in the DB,
			// it is likely a local-only attribute (e.g., 'group', 'serviceBound') and must be kept.
			if _, incomingExists := resource.OptionsPayload[k]; !incomingExists {
				resource.OptionsPayload[k] = v
			}
		}
	}

	// 5. Canonicalize Shopify image warm fields when missing/empty.
	// This keeps manual import payloads aligned with reconcile/webhook payload shape.
	incomingMainURL, _ := resource.OptionsPayload["shopifyImageSourceUrl"].(string)
	incomingVariantRaw, _ := resource.OptionsPayload["shopifyImage"].(string)
	if strings.TrimSpace(incomingMainURL) == "" || !hasUsableShopifyVariantSourceMap(incomingVariantRaw) {
		if rawData, ok := resource.OptionsPayload["shopifyData"].(string); ok && rawData != "" {
			var node map[string]any
			if err := json.Unmarshal([]byte(rawData), &node); err == nil {
				warmMainURL, warmVariantJSON := buildShopifyImageWarmFieldsFromNode(node)
				if strings.TrimSpace(incomingMainURL) == "" && warmMainURL != "" {
					resource.OptionsPayload["shopifyImageSourceUrl"] = warmMainURL
				}
				if !hasUsableShopifyVariantSourceMap(incomingVariantRaw) && warmVariantJSON != "" {
					resource.OptionsPayload["shopifyImage"] = warmVariantJSON
				}
			}
		}
	}

	// 6. Image Processing and Change Detection
	mediaPath := filepath.Join(config.BackendPath, "config", tenantCtx.TenantID, "media")
	processor := media.NewImageProcessor(mediaPath)

	// Process Main Image
	activeMainFileID := ""
	if existing != nil {
		if val, ok := existing.OptionsPayload["image"].(string); ok {
			activeMainFileID = val
		}
	}

	incomingMainURL, _ = resource.OptionsPayload["shopifyImageSourceUrl"].(string)
	storedMainURL := ""
	if existing != nil {
		storedMainURL, _ = existing.OptionsPayload["shopifyImageSourceUrl"].(string)
	}

	var allActiveFileIDs []string
	var filesToDelete []string

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

	// Process Variant Map
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

	if mapData, err := json.Marshal(finalVariantMap); err == nil {
		resource.OptionsPayload["shopifyImage"] = string(mapData)
	}

	// 7. Persistence
	resourceRepo := tenantCtx.ResourceRepo()
	operation := "none"
	productCategory := "product"

	if existing != nil {
		incomingData, _ := resource.OptionsPayload["shopifyData"].(string)
		existingData, _ := existing.OptionsPayload["shopifyData"].(string)

		hasChanged := variantChangeDetected ||
			resource.Title != existing.Title ||
			resource.OneLiner != existing.OneLiner ||
			incomingData != existingData ||
			resource.OptionsPayload["image"] != existing.OptionsPayload["image"]

		if hasChanged {
			resource.ID = existing.ID
			resource.CategorySlug = &productCategory
			if err := resourceRepo.Update(tenantCtx.TenantID, resource, allActiveFileIDs); err != nil {
				return "", fmt.Errorf("failed to update shopify resource %s: %w", resource.ID, err)
			}
			operation = "updated"
		}
	} else {
		if resource.ID == "" {
			resource.ID = security.GenerateULID()
		}
		resource.CategorySlug = &productCategory
		if err := resourceRepo.Store(tenantCtx.TenantID, resource, allActiveFileIDs); err != nil {
			return "", fmt.Errorf("failed to create shopify resource: %w", err)
		}
		operation = "created"
	}

	// 8. Post-Sync Orchestration
	if operation != "none" {
		for _, fileID := range filesToDelete {
			if err := s.imageFileService.Delete(tenantCtx, fileID); err != nil {
				s.logger.Content().Warn("Failed to cleanup replaced shopify image", "error", err, "fileId", fileID)
			}
		}
		tenantCtx.CacheManager.SetResource(tenantCtx.TenantID, resource)
		if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
			s.logger.Content().Error("Failed to refresh content map after shopify upsert", "error", err)
		}
	}

	s.logger.Content().Info("Shopify resource sync complete",
		"tenantId", tenantCtx.TenantID,
		"gid", targetGID,
		"operation", operation,
		"duration", time.Since(start))

	marker.SetSuccess(true)
	return operation, nil
}

func hasUsableShopifyVariantSourceMap(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}

	var variantMap map[string]shopifyVariantImagePayload
	if err := json.Unmarshal([]byte(raw), &variantMap); err != nil {
		return false
	}
	if len(variantMap) == 0 {
		return false
	}

	for _, entry := range variantMap {
		if strings.TrimSpace(entry.SourceURL) != "" {
			return true
		}
	}
	return false
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

	// 2. Collect all canonical products and linked services by GID.
	var productIDs []string
	var linkedServices []*content.ResourceNode
	for _, p := range products {
		if val, ok := p.OptionsPayload["gid"].(string); ok && val == gid {
			productIDs = append(productIDs, p.ID)
		}
	}
	for _, svc := range services {
		if val, ok := svc.OptionsPayload["gid"].(string); ok && val == gid {
			linkedServices = append(linkedServices, svc)
		}
	}

	// 3. Idempotent check: if not found, we are done.
	if len(productIDs) == 0 && len(linkedServices) == 0 {
		s.logger.Content().Debug("Shopify resource not found for deletion; skipping",
			"tenantId", tenantCtx.TenantID,
			"gid", gid)
		marker.SetSuccess(true)
		return "none", nil
	}

	// 4. Delete canonical product rows for this gid.
	for _, productID := range productIDs {
		if err := s.deleteResourceByID(tenantCtx, productID); err != nil {
			return "", fmt.Errorf("failed to delete Shopify-linked product %s: %w", productID, err)
		}
	}

	// 5. Strip Shopify linkage from services and retain service rows.
	for _, svc := range linkedServices {
		next := *svc
		next.OptionsPayload = clearShopifyLinkage(svc.OptionsPayload)
		if err := s.Update(tenantCtx, &next, nil); err != nil {
			return "", fmt.Errorf("failed to strip Shopify linkage from service %s: %w", svc.ID, err)
		}
	}

	s.logger.Content().Info("Shopify resource deleted successfully",
		"tenantId", tenantCtx.TenantID,
		"gid", gid,
		"productCount", len(productIDs),
		"serviceCount", len(linkedServices),
		"duration", time.Since(start))

	marker.SetSuccess(true)
	return "updated", nil
}
