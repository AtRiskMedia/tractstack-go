// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// ResourceService orchestrates resource operations with cache-first repository pattern
type ResourceService struct {
	// No stored dependencies - all passed via tenant context
}

// NewResourceService creates a new resource service singleton
func NewResourceService() *ResourceService {
	return &ResourceService{}
}

// GetAllIDs returns all resource IDs for a tenant (cache-first)
func (s *ResourceService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	resourceRepo := tenantCtx.ResourceRepo()

	resources, err := resourceRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all resources: %w", err)
	}

	ids := make([]string, len(resources))
	for i, resource := range resources {
		ids[i] = resource.ID
	}

	return ids, nil
}

// GetByID returns a resource by ID (cache-first)
func (s *ResourceService) GetByID(tenantCtx *tenant.Context, id string) (*content.ResourceNode, error) {
	if id == "" {
		return nil, fmt.Errorf("resource ID cannot be empty")
	}

	resourceRepo := tenantCtx.ResourceRepo()
	resource, err := resourceRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource %s: %w", id, err)
	}

	return resource, nil
}

// GetByIDs returns multiple resources by IDs (cache-first with bulk loading)
func (s *ResourceService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.ResourceNode, error) {
	if len(ids) == 0 {
		return []*content.ResourceNode{}, nil
	}

	resourceRepo := tenantCtx.ResourceRepo()
	resources, err := resourceRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources by IDs: %w", err)
	}

	return resources, nil
}

// GetBySlug returns a resource by slug (cache-first)
func (s *ResourceService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.ResourceNode, error) {
	if slug == "" {
		return nil, fmt.Errorf("resource slug cannot be empty")
	}

	resourceRepo := tenantCtx.ResourceRepo()
	resource, err := resourceRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource by slug %s: %w", slug, err)
	}

	return resource, nil
}

// GetByFilters returns resources by multiple filter criteria (cache-first)
func (s *ResourceService) GetByFilters(tenantCtx *tenant.Context, ids []string, categories []string, slugs []string) ([]*content.ResourceNode, error) {
	// If no filters provided, return empty result
	if len(ids) == 0 && len(categories) == 0 && len(slugs) == 0 {
		return []*content.ResourceNode{}, nil
	}

	resourceRepo := tenantCtx.ResourceRepo()
	resources, err := resourceRepo.FindByFilters(tenantCtx.TenantID, ids, categories, slugs)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources by filters: %w", err)
	}

	return resources, nil
}

// Create creates a new resource
func (s *ResourceService) Create(tenantCtx *tenant.Context, resource *content.ResourceNode) error {
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
	err := resourceRepo.Store(tenantCtx.TenantID, resource)
	if err != nil {
		return fmt.Errorf("failed to create resource %s: %w", resource.ID, err)
	}

	return nil
}

// Update updates an existing resource
func (s *ResourceService) Update(tenantCtx *tenant.Context, resource *content.ResourceNode) error {
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

	// Verify resource exists before updating
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

	return nil
}

// Delete deletes a resource
func (s *ResourceService) Delete(tenantCtx *tenant.Context, id string) error {
	if id == "" {
		return fmt.Errorf("resource ID cannot be empty")
	}

	resourceRepo := tenantCtx.ResourceRepo()

	// Verify resource exists before deleting
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

	return nil
}
