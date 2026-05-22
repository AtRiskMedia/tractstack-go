// Package handlers provides HTTP handlers for resource endpoints
package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/media"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// ResourceIDsRequest represents the request body for bulk resource loading
type ResourceIDsRequest struct {
	ResourceIDs []string `json:"resourceIds,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	Slugs       []string `json:"slugs,omitempty"`
}

// ResourceHandlers contains all resource-related HTTP handlers
type ResourceHandlers struct {
	resourceService  *services.ResourceService
	imageFileService *services.ImageFileService
	logger           *logging.ChanneledLogger
	perfTracker      *performance.Tracker
}

// NewResourceHandlers creates resource handlers with injected dependencies
func NewResourceHandlers(resourceService *services.ResourceService, imageFileService *services.ImageFileService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ResourceHandlers {
	return &ResourceHandlers{
		resourceService:  resourceService,
		imageFileService: imageFileService,
		logger:           logger,
		perfTracker:      perfTracker,
	}
}

// GetAllResourceIDs returns all resource IDs using cache-first pattern
func (h *ResourceHandlers) GetAllResourceIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_all_resource_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get all resource IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	resourceIDs, err := h.resourceService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get all resource IDs request completed", "count", len(resourceIDs), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetAllResourceIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"resourceIds": resourceIDs,
		"count":       len(resourceIDs),
	})
}

// GetResourcesByIDs returns multiple resources by IDs/filters using cache-first pattern
func (h *ResourceHandlers) GetResourcesByIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_resources_by_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get resources by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req ResourceIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Handle different request patterns
	var resources []*content.ResourceNode
	var err error

	if len(req.ResourceIDs) > 0 || len(req.Categories) > 0 || len(req.Slugs) > 0 {
		// Multi-filter request
		resources, err = h.resourceService.GetByFilters(tenantCtx, req.ResourceIDs, req.Categories, req.Slugs)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one filter (resourceIds, categories, or slugs) must be provided"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get resources by IDs request completed", "requestedCount", len(req.ResourceIDs), "foundCount", len(resources), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetResourcesByIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(req.ResourceIDs))

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"count":     len(resources),
	})
}

// GetResourceByID returns a specific resource by ID using cache-first pattern
func (h *ResourceHandlers) GetResourceByID(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_resource_by_id_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get resource by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "resourceId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource ID is required"})
		return
	}

	resourceNode, err := h.resourceService.GetByID(tenantCtx, resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if resourceNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	h.logger.Content().Info("Get resource by ID request completed", "resourceId", resourceID, "found", resourceNode != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetResourceByID request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", resourceID)

	c.JSON(http.StatusOK, resourceNode)
}

// GetResourceBySlug returns a specific resource by slug using cache-first pattern
func (h *ResourceHandlers) GetResourceBySlug(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_resource_by_slug_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get resource by slug request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource slug is required"})
		return
	}

	resourceNode, err := h.resourceService.GetBySlug(tenantCtx, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if resourceNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	h.logger.Content().Info("Get resource by slug request completed", "slug", slug, "found", resourceNode != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetResourceBySlug request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	c.JSON(http.StatusOK, resourceNode)
}

// CreateResource creates a new resource, handling image uploads within the payload.
func (h *ResourceHandlers) CreateResource(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	marker := h.perfTracker.StartOperation("create_resource_request", tenantCtx.TenantID)
	defer marker.Complete()
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var resource content.ResourceNode
	if err := c.ShouldBindJSON(&resource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if gid, ok := resource.OptionsPayload["gid"].(string); ok && strings.HasPrefix(gid, "gid://shopify/") {
		category := ""
		if resource.CategorySlug != nil {
			category = *resource.CategorySlug
		}

		if category == "service" {
			products, err := h.resourceService.GetByCategory(tenantCtx, "product")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check canonical product", "details": err.Error()})
				return
			}

			hasCanonicalProduct := false
			for _, p := range products {
				if existingGID, _ := p.OptionsPayload["gid"].(string); existingGID == gid {
					hasCanonicalProduct = true
					break
				}
			}

			if !hasCanonicalProduct {
				rawShopifyData, _ := resource.OptionsPayload["shopifyData"].(string)
				if strings.TrimSpace(rawShopifyData) == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "missing canonical product for gid-linked service import"})
					return
				}

				productCategory := "product"
				canonical := content.ResourceNode{
					Title:        resource.Title,
					OneLiner:     resource.OneLiner,
					Slug:         strings.Replace(resource.Slug, "service-", "product-", 1),
					CategorySlug: &productCategory,
					NodeType:     "Resource",
					OptionsPayload: map[string]any{
						"gid":         gid,
						"shopifyData": rawShopifyData,
					},
				}
				if _, err := h.resourceService.UpsertShopifyResource(tenantCtx, &canonical); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create canonical product", "details": err.Error()})
					return
				}
			}

			if resource.OptionsPayload == nil {
				resource.OptionsPayload = map[string]any{}
			}
			delete(resource.OptionsPayload, "shopifyData")
			delete(resource.OptionsPayload, "shopifyImage")
			delete(resource.OptionsPayload, "shopifyImageSourceUrl")

			fileIDs, err := h.processResourceImages(tenantCtx, &resource)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process resource images", "details": err.Error()})
				return
			}

			if err := h.resourceService.Create(tenantCtx, &resource, fileIDs); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			marker.SetSuccess(true)
			c.JSON(http.StatusCreated, resource)
			return
		}

		_, err := h.resourceService.UpsertShopifyResource(tenantCtx, &resource)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync shopify resource", "details": err.Error()})
			return
		}

		marker.SetSuccess(true)
		c.JSON(http.StatusCreated, resource)
		return
	}

	fileIDs, err := h.processResourceImages(tenantCtx, &resource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process resource images", "details": err.Error()})
		return
	}

	err = h.resourceService.Create(tenantCtx, &resource, fileIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for CreateResource request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", resource.ID)

	c.JSON(http.StatusCreated, resource)
}

// findOrphanedFileIDs compares an existing resource with an incoming update payload
// to identify file IDs that are being replaced and will become orphans.
func (h *ResourceHandlers) findOrphanedFileIDs(existingResource *content.ResourceNode, newResource *content.ResourceNode) []string {
	var orphanedIDs []string
	if existingResource.OptionsPayload == nil || newResource.OptionsPayload == nil {
		return orphanedIDs
	}

	// Iterate through the fields of the incoming payload.
	for key, newValue := range newResource.OptionsPayload {
		// A base64 string indicates a new file is being uploaded for this field.
		if newStringValue, ok := newValue.(string); ok && strings.HasPrefix(newStringValue, "data:image/") {
			// Check if the existing resource had a file for this field.
			if existingValue, exists := existingResource.OptionsPayload[key]; exists {
				// The existing value should be a map if it's a processed image.
				if existingMap, ok := existingValue.(map[string]any); ok {
					// If it has a fileId, it's an orphan.
					if fileID, ok := existingMap["fileId"].(string); ok && fileID != "" {
						orphanedIDs = append(orphanedIDs, fileID)
						h.logger.Content().Debug("Identified orphaned file for replacement", "field", key, "orphanedFileId", fileID)
					}
				}
			}
		}
	}

	return orphanedIDs
}

// UpdateResource updates an existing resource, handling the replacement and cleanup of images.
func (h *ResourceHandlers) UpdateResource(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	marker := h.perfTracker.StartOperation("update_resource_request", tenantCtx.TenantID)
	defer marker.Complete()
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource ID is required"})
		return
	}

	// Step 1: Fetch the current state of the resource before binding the new data.
	existingResource, err := h.resourceService.GetByID(tenantCtx, resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve existing resource", "details": err.Error()})
		return
	}
	if existingResource == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	// Step 2: Bind the incoming update payload.
	var updatedResource content.ResourceNode
	if err := c.ShouldBindJSON(&updatedResource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}
	updatedResource.ID = resourceID // Ensure ID is set correctly.

	// Step 3: Compare payloads to find files that are being replaced.
	orphanedFileIDs := h.findOrphanedFileIDs(existingResource, &updatedResource)

	// Step 4: Process any new Base64 images in the payload.
	newFileIDs, err := h.processResourceImages(tenantCtx, &updatedResource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process resource images", "details": err.Error()})
		return
	}

	// Step 5: Update the resource in the database with the new data.
	err = h.resourceService.Update(tenantCtx, &updatedResource, newFileIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Step 6: On successful update, clean up the orphaned files in the background.
	if len(orphanedFileIDs) > 0 {
		go func() {
			h.logger.Content().Info("Starting background cleanup of orphaned resource files", "count", len(orphanedFileIDs))
			for _, orphanID := range orphanedFileIDs {
				if err := h.imageFileService.Delete(tenantCtx, orphanID); err != nil {
					h.logger.Content().Error("Failed to delete orphaned file during background cleanup", "error", err, "fileId", orphanID)
				}
			}
			h.logger.Content().Info("Background cleanup of orphaned resource files completed.")
		}()
	}

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for UpdateResource request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", updatedResource.ID)

	// Return the mutated resource so the frontend has the resolved image paths.
	c.JSON(http.StatusOK, updatedResource)
}

// DeleteResource deletes a resource
func (h *ResourceHandlers) DeleteResource(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	marker := h.perfTracker.StartOperation("delete_resource_request", tenantCtx.TenantID)
	defer marker.Complete()
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource ID is required"})
		return
	}

	err := h.resourceService.Delete(tenantCtx, resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for DeleteResource request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "resourceId", resourceID)

	c.JSON(http.StatusOK, gin.H{
		"message":    "resource deleted successfully",
		"resourceId": resourceID,
	})
}

// processResourceImages scans a resource's optionsPayload for Base64 image data,
// processes each image, creates an ImageFileNode record, and mutates the payload
// to replace the Base64 data with the final image details.
// It returns a slice of the newly created file IDs.
func (h *ResourceHandlers) processResourceImages(tenantCtx *tenant.Context, resource *content.ResourceNode) ([]string, error) {
	var createdFileIDs []string
	if resource.OptionsPayload == nil {
		return createdFileIDs, nil
	}

	mediaPath := filepath.Join(config.BackendPath, "config", tenantCtx.TenantID, "media")
	processor := media.NewImageProcessor(mediaPath)

	for key, value := range resource.OptionsPayload {
		base64Data, ok := value.(string)
		if !ok || !strings.HasPrefix(base64Data, "data:image/") {
			continue
		}

		fileID := ulid.Make().String()
		h.logger.Content().Debug("Processing new resource image", "field", key, "assignedFileId", fileID)

		src, srcSet, err := processor.ProcessResourceImageWithSizes(base64Data, fileID)
		if err != nil {
			h.logger.Content().Error("Failed to process resource image", "error", err, "field", key)
			return nil, fmt.Errorf("failed to process image for field '%s': %w", key, err)
		}

		filename := filepath.Base(src)
		imageFile := content.ImageFileNode{
			ID:             fileID,
			NodeType:       "File",
			Filename:       filename,
			AltDescription: "Image requiring description",
			Src:            src,
			SrcSet:         srcSet,
		}

		if err := h.imageFileService.Create(tenantCtx, &imageFile); err != nil {
			h.logger.Content().Error("Failed to create image file record", "error", err, "fileID", fileID)
			// Note: Could add cleanup logic here to delete the physically saved files if the DB record fails.
			return nil, fmt.Errorf("failed to create database record for image file '%s': %w", fileID, err)
		}

		createdFileIDs = append(createdFileIDs, fileID)

		// Mutate the payload: replace base64 with a structured object.
		// This keeps the payload consistent and provides the frontend with necessary data.
		imagePayload := map[string]any{
			"fileId": fileID,
			"src":    src,
		}
		if srcSet != nil {
			imagePayload["srcSet"] = *srcSet
		}
		resource.OptionsPayload[key] = imagePayload
	}

	return createdFileIDs, nil
}
