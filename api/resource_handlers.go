// Package api provide resource handlers
package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// ResourceIDsRequest represents the request body for bulk resource loading
type ResourceIDsRequest struct {
	ResourceIDs []string `json:"resourceIds,omitempty"` // Made optional
	Categories  []string `json:"categories,omitempty"`  // New - filter by categories
	Slugs       []string `json:"slugs,omitempty"`       // New - filter by slugs
}

type CreateResourceRequest struct {
	Title          string                 `json:"title" binding:"required"`
	Slug           string                 `json:"slug" binding:"required"`
	CategorySlug   string                 `json:"categorySlug" binding:"required"`
	Oneliner       string                 `json:"oneliner"`
	OptionsPayload map[string]interface{} `json:"optionsPayload"`
	ActionLisp     *string                `json:"actionLisp,omitempty"`
}

type UpdateResourceRequest struct {
	Title          string                 `json:"title" binding:"required"`
	Slug           string                 `json:"slug" binding:"required"`
	CategorySlug   string                 `json:"categorySlug" binding:"required"`
	Oneliner       string                 `json:"oneliner"`
	OptionsPayload map[string]interface{} `json:"optionsPayload"`
	ActionLisp     *string                `json:"actionLisp,omitempty"`
}

// GetResourcesByIDsHandler returns multiple resources by IDs, categories, or slugs using cache-first pattern
func GetResourcesByIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Parse request body
	var req ResourceIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Validate that at least one filtering method is provided
	if len(req.ResourceIDs) == 0 && len(req.Categories) == 0 && len(req.Slugs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one of resourceIds, categories, or slugs must be provided"})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	var allResources []*models.ResourceNode
	resourceMap := make(map[string]*models.ResourceNode) // To avoid duplicates

	// Fetch by IDs if provided
	if len(req.ResourceIDs) > 0 {
		resources, err := resourceService.GetByIDs(req.ResourceIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for _, resource := range resources {
			resourceMap[resource.ID] = resource
		}
	}

	// Fetch by categories if provided
	if len(req.Categories) > 0 {
		for _, category := range req.Categories {
			resources, err := resourceService.GetByCategory(category)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			for _, resource := range resources {
				resourceMap[resource.ID] = resource
			}
		}
	}

	// Fetch by slugs if provided
	if len(req.Slugs) > 0 {
		for _, slug := range req.Slugs {
			resource, err := resourceService.GetBySlug(slug)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if resource != nil {
				resourceMap[resource.ID] = resource
			}
		}
	}

	// Convert map to slice
	for _, resource := range resourceMap {
		allResources = append(allResources, resource)
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": allResources,
		"count":     len(allResources),
	})
}

// GetAllResourceIDsHandler returns all resource IDs using cache-first pattern
func GetAllResourceIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	resourceIDs, err := resourceService.GetAllIDs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resourceIds": resourceIDs,
		"count":       len(resourceIDs),
	})
}

// GetResourceByIDHandler returns a specific resource by ID using cache-first pattern
func GetResourceByIDHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource ID is required"})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	resourceNode, err := resourceService.GetByID(resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if resourceNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	c.JSON(http.StatusOK, resourceNode)
}

// GetResourceBySlugHandler returns a specific resource by slug using cache-first pattern
func GetResourceBySlugHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource slug is required"})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	resourceNode, err := resourceService.GetBySlug(slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if resourceNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	c.JSON(http.StatusOK, resourceNode)
}

// GetResourcesByCategoryHandler returns resources by category using cache-first pattern
func GetResourcesByCategoryHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	category := c.Param("category")
	if category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource category is required"})
		return
	}

	// Use cache-first resource service with global cache manager
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())
	resources, err := resourceService.GetByCategory(category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"category":  category,
		"count":     len(resources),
	})
}

// CreateResourceHandler creates a new resource with authentication and validation
func CreateResourceHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Authentication - Admin OR Editor required
	if !validateAdminOrEditor(c, ctx) {
		return
	}

	// Parse request
	var req CreateResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate resource data against known resources schema
	if err := validateResourceRequest(ctx, req.Title, req.Slug, req.CategorySlug, req.Oneliner, req.OptionsPayload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check for slug uniqueness
	var existingID string
	err = ctx.Database.Conn.QueryRow("SELECT id FROM resources WHERE slug = ?", req.Slug).Scan(&existingID)
	if err != sql.ErrNoRows {
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Resource with this slug already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Generate new ID
	resourceID := ulid.Make().String()

	// Convert options payload to database format
	optionsPayloadJSON, err := json.Marshal(req.OptionsPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize options payload"})
		return
	}

	// Insert into database
	query := `INSERT INTO resources (id, title, slug, category_slug, oneliner, options_payload, action_lisp) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err = ctx.Database.Conn.Exec(query, resourceID, req.Title, req.Slug, req.CategorySlug, req.Oneliner, string(optionsPayloadJSON), req.ActionLisp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create resource"})
		return
	}

	// Create response resource node
	resourceNode := &models.ResourceNode{
		ID:             resourceID,
		Title:          req.Title,
		Slug:           req.Slug,
		CategorySlug:   &req.CategorySlug,
		Oneliner:       req.Oneliner,
		OptionsPayload: req.OptionsPayload,
		ActionLisp:     req.ActionLisp,
	}

	// Cache invalidation cascade
	cache.GetGlobalManager().SetResource(ctx.TenantID, resourceNode)
	cache.GetGlobalManager().InvalidateFullContentMap(ctx.TenantID)
	cache.GetGlobalManager().InvalidateOrphanAnalysis(ctx.TenantID)

	c.JSON(http.StatusCreated, resourceNode)
}

// UpdateResourceHandler updates an existing resource with authentication and validation
func UpdateResourceHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Authentication - Admin OR Editor required
	if !validateAdminOrEditor(c, ctx) {
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Resource ID is required"})
		return
	}

	// Parse request
	var req UpdateResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate resource data against known resources schema
	if err := validateResourceRequest(ctx, req.Title, req.Slug, req.CategorySlug, req.Oneliner, req.OptionsPayload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if resource exists
	var existingID string
	err = ctx.Database.Conn.QueryRow("SELECT id FROM resources WHERE id = ?", resourceID).Scan(&existingID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Resource not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Check for slug uniqueness (excluding current resource)
	var conflictingID string
	err = ctx.Database.Conn.QueryRow("SELECT id FROM resources WHERE slug = ? AND id != ?", req.Slug, resourceID).Scan(&conflictingID)
	if err != sql.ErrNoRows {
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Another resource with this slug already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Convert options payload to database format
	optionsPayloadJSON, err := json.Marshal(req.OptionsPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize options payload"})
		return
	}

	// Update database
	query := `UPDATE resources SET title = ?, slug = ?, category_slug = ?, oneliner = ?, options_payload = ?, action_lisp = ? WHERE id = ?`
	_, err = ctx.Database.Conn.Exec(query, req.Title, req.Slug, req.CategorySlug, req.Oneliner, string(optionsPayloadJSON), req.ActionLisp, resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update resource"})
		return
	}

	// Create response resource node
	resourceNode := &models.ResourceNode{
		ID:             resourceID,
		Title:          req.Title,
		Slug:           req.Slug,
		CategorySlug:   &req.CategorySlug,
		Oneliner:       req.Oneliner,
		OptionsPayload: req.OptionsPayload,
		ActionLisp:     req.ActionLisp,
	}

	// Cache invalidation cascade
	cache.GetGlobalManager().SetResource(ctx.TenantID, resourceNode)
	cache.GetGlobalManager().InvalidateFullContentMap(ctx.TenantID)
	cache.GetGlobalManager().InvalidateOrphanAnalysis(ctx.TenantID)

	c.JSON(http.StatusOK, resourceNode)
}

// Replace the existing DeleteResourceHandler function with this updated version:

// DeleteResourceHandler deletes a resource with authentication and cache-first reference check
func DeleteResourceHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Authentication - Admin OR Editor required
	if !validateAdminOrEditor(c, ctx) {
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Resource ID is required"})
		return
	}

	// Check if resource exists and get its slug
	var existingSlug string
	err = ctx.Database.Conn.QueryRow("SELECT slug FROM resources WHERE id = ?", resourceID).Scan(&existingSlug)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Resource not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Check for resource references using cache-first approach
	usageCount, err := checkResourceReferencesWithCache(ctx, existingSlug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check resource usage"})
		return
	}

	if usageCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":      "Cannot delete resource: it is referenced by other resources",
			"usageCount": usageCount,
		})
		return
	}

	// Delete from database
	_, err = ctx.Database.Conn.Exec("DELETE FROM resources WHERE id = ?", resourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete resource"})
		return
	}

	// Cache invalidation cascade
	cache.GetGlobalManager().InvalidateResource(ctx.TenantID, resourceID)
	cache.GetGlobalManager().InvalidateFullContentMap(ctx.TenantID)
	cache.GetGlobalManager().InvalidateOrphanAnalysis(ctx.TenantID)

	c.JSON(http.StatusOK, gin.H{"message": "Resource deleted successfully"})
}

// checkResourceReferencesWithCache performs cache-first reference validation
func checkResourceReferencesWithCache(ctx *tenant.Context, resourceSlug string) (int, error) {
	var usageCount int

	// Get known resources schema
	if ctx.Config.BrandConfig == nil || ctx.Config.BrandConfig.KnownResources == nil {
		return 0, nil // No schema = no validation
	}
	knownResources := *ctx.Config.BrandConfig.KnownResources

	// Use cache-first resource service to get all resources
	resourceService := content.NewResourceService(ctx, cache.GetGlobalManager())

	// Get all resource IDs (this will warm the cache if needed)
	allIDs, err := resourceService.GetAllIDs()
	if err != nil {
		return 0, fmt.Errorf("failed to get all resource IDs: %w", err)
	}

	// Get all resources (these should now be in cache)
	allResources, err := resourceService.GetByIDs(allIDs)
	if err != nil {
		return 0, fmt.Errorf("failed to get resources: %w", err)
	}

	// Check each resource for references to the target slug
	for _, resource := range allResources {
		if resource.CategorySlug == nil {
			continue
		}

		// Get category definition
		categoryDef, exists := knownResources[*resource.CategorySlug]
		if !exists {
			continue
		}

		// Check only fields with BelongsToCategory relationships
		for fieldName, fieldDef := range categoryDef {
			if fieldDef.BelongsToCategory != "" {
				if fieldValue, exists := resource.OptionsPayload[fieldName]; exists {
					// Handle both string and array of strings
					switch v := fieldValue.(type) {
					case string:
						if v == resourceSlug {
							usageCount++
						}
					case []interface{}:
						// Handle multi-select reference fields
						for _, item := range v {
							if strItem, ok := item.(string); ok && strItem == resourceSlug {
								usageCount++
								break // Count this resource only once even if multiple refs
							}
						}
					}
				}
			}
		}
	}

	return usageCount, nil
}

// validateResourceRequest validates resource creation/update data against known resources schema
func validateResourceRequest(ctx *tenant.Context, title, slug, categorySlug, oneliner string, optionsPayload map[string]interface{}) error {
	// Basic field validation
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title is required")
	}

	if strings.TrimSpace(slug) == "" {
		return fmt.Errorf("slug is required")
	}

	if strings.TrimSpace(categorySlug) == "" {
		return fmt.Errorf("categorySlug is required")
	}

	// Get known resources schema from tenant context
	if ctx.Config.BrandConfig == nil || ctx.Config.BrandConfig.KnownResources == nil {
		return fmt.Errorf("known resources configuration not available")
	}

	knownResources := *ctx.Config.BrandConfig.KnownResources

	// Check if category exists in known resources
	categoryDefinition, exists := knownResources[categorySlug]
	if !exists {
		return fmt.Errorf("category '%s' is not defined in known resources", categorySlug)
	}

	// Validate each field in options payload against category definition
	for fieldName, fieldDef := range categoryDefinition {
		value, hasValue := optionsPayload[fieldName]

		// Check required fields
		if !fieldDef.Optional && !hasValue {
			return fmt.Errorf("field '%s' is required for category '%s'", fieldName, categorySlug)
		}

		// Skip validation if field is optional and not provided
		if !hasValue {
			continue
		}

		// Validate field type and constraints
		if err := validateFieldValue(fieldName, value, fieldDef, knownResources); err != nil {
			return err
		}
	}

	return nil
}

// validateFieldValue validates a single field value against its definition
func validateFieldValue(fieldName string, value interface{}, fieldDef tenant.FieldDefinition, knownResources tenant.KnownResourcesConfig) error {
	switch fieldDef.Type {
	case "string":
		strVal, ok := value.(string)
		if !ok {
			return fmt.Errorf("field '%s' must be a string", fieldName)
		}

		// If this field references another category, validate the reference
		if fieldDef.BelongsToCategory != "" {
			if err := validateCategoryReference(fieldName, strVal, fieldDef.BelongsToCategory, knownResources); err != nil {
				return err
			}
		}

	case "number":
		// Handle both int and float values from JSON
		var numVal float64
		switch v := value.(type) {
		case float64:
			numVal = v
		case int:
			numVal = float64(v)
		case string:
			// Try to parse string as number
			parsed, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("field '%s' must be a number", fieldName)
			}
			numVal = parsed
		default:
			return fmt.Errorf("field '%s' must be a number", fieldName)
		}

		// Check min/max constraints
		if fieldDef.MinNumber != nil && numVal < float64(*fieldDef.MinNumber) {
			return fmt.Errorf("field '%s' must be at least %d", fieldName, *fieldDef.MinNumber)
		}
		if fieldDef.MaxNumber != nil && numVal > float64(*fieldDef.MaxNumber) {
			return fmt.Errorf("field '%s' must be at most %d", fieldName, *fieldDef.MaxNumber)
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field '%s' must be a boolean", fieldName)
		}

	case "multi":
		// Multi fields should be arrays
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("field '%s' must be an array", fieldName)
		}

	case "date":
		// Date fields should be numbers (Unix timestamps)
		switch value.(type) {
		case float64, int:
			// Valid timestamp
		default:
			return fmt.Errorf("field '%s' must be a Unix timestamp", fieldName)
		}

	case "image":
		// Image fields should be strings (URLs or base64)
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field '%s' must be a string", fieldName)
		}

	default:
		return fmt.Errorf("unknown field type '%s' for field '%s'", fieldDef.Type, fieldName)
	}

	return nil
}

// validateCategoryReference validates that a reference to another category is valid
func validateCategoryReference(fieldName, value, referencedCategory string, knownResources tenant.KnownResourcesConfig) error {
	// Check if the referenced category exists
	if _, exists := knownResources[referencedCategory]; !exists {
		return fmt.Errorf("field '%s' references category '%s' which does not exist", fieldName, referencedCategory)
	}

	// Note: We can't validate that the specific resource exists here because
	// this validation runs during creation/update, and we'd need database access.
	// The frontend should handle this validation using the content map.

	return nil
}
