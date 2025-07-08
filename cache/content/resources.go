// Package content provides resource cache operations
package content

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// ResourceCacheOperations implements resource-specific cache operations
type ResourceCacheOperations struct {
	manager *models.CacheManager
}

// NewResourceCacheOperations creates a new resource cache operations handler
func NewResourceCacheOperations(manager *models.CacheManager) *ResourceCacheOperations {
	return &ResourceCacheOperations{manager: manager}
}

// GetResource retrieves a resource by ID from cache
func (rco *ResourceCacheOperations) GetResource(tenantID, id string) (*models.ResourceNode, bool) {
	rco.manager.Mu.RLock()
	tenantCache, exists := rco.manager.ContentCache[tenantID]
	rco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired (24 hours TTL)
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	resource, exists := tenantCache.Resources[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	rco.manager.Mu.Lock()
	rco.manager.LastAccessed[tenantID] = time.Now().UTC()
	rco.manager.Mu.Unlock()

	return resource, true
}

// SetResource stores a resource in cache
func (rco *ResourceCacheOperations) SetResource(tenantID string, node *models.ResourceNode) {
	rco.ensureTenantCache(tenantID)

	rco.manager.Mu.RLock()
	tenantCache := rco.manager.ContentCache[tenantID]
	rco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the resource
	tenantCache.Resources[node.ID] = node

	// Update slug lookup
	tenantCache.SlugToID["resource:"+node.Slug] = node.ID

	// Update category indexing
	if node.CategorySlug != nil {
		category := *node.CategorySlug
		// Initialize category list if it doesn't exist
		if _, exists := tenantCache.CategoryToIDs[category]; !exists {
			tenantCache.CategoryToIDs[category] = []string{}
		}
		// Add resource ID to category if not already present
		found := false
		for _, existingID := range tenantCache.CategoryToIDs[category] {
			if existingID == node.ID {
				found = true
				break
			}
		}
		if !found {
			tenantCache.CategoryToIDs[category] = append(tenantCache.CategoryToIDs[category], node.ID)
		}
	}

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	rco.manager.Mu.Lock()
	rco.manager.LastAccessed[tenantID] = time.Now().UTC()
	rco.manager.Mu.Unlock()
}

// GetResourceBySlug retrieves a resource by slug from cache
func (rco *ResourceCacheOperations) GetResourceBySlug(tenantID, slug string) (*models.ResourceNode, bool) {
	rco.manager.Mu.RLock()
	tenantCache, exists := rco.manager.ContentCache[tenantID]
	rco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	// Get ID from slug lookup (prefixed to avoid conflicts)
	id, exists := tenantCache.SlugToID["resource:"+slug]
	if !exists {
		return nil, false
	}

	// Get resource by ID
	resource, exists := tenantCache.Resources[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	rco.manager.Mu.Lock()
	rco.manager.LastAccessed[tenantID] = time.Now().UTC()
	rco.manager.Mu.Unlock()

	return resource, true
}

// GetResourcesByCategory retrieves resources by category from cache
func (rco *ResourceCacheOperations) GetResourcesByCategory(tenantID, category string) ([]*models.ResourceNode, bool) {
	rco.manager.Mu.RLock()
	tenantCache, exists := rco.manager.ContentCache[tenantID]
	rco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	// Get resource IDs for category
	resourceIDs, exists := tenantCache.CategoryToIDs[category]
	if !exists || len(resourceIDs) == 0 {
		return nil, false
	}

	// Build result array
	var resources []*models.ResourceNode
	for _, id := range resourceIDs {
		if resource, exists := tenantCache.Resources[id]; exists {
			resources = append(resources, resource)
		}
	}

	if len(resources) == 0 {
		return nil, false
	}

	// Update last accessed
	rco.manager.Mu.Lock()
	rco.manager.LastAccessed[tenantID] = time.Now().UTC()
	rco.manager.Mu.Unlock()

	return resources, true
}

// GetAllResourceIDs retrieves all resource IDs from cache
func (rco *ResourceCacheOperations) GetAllResourceIDs(tenantID string) ([]string, bool) {
	rco.manager.Mu.RLock()
	tenantCache, exists := rco.manager.ContentCache[tenantID]
	rco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	// Extract IDs from cached resources
	var ids []string
	for id := range tenantCache.Resources {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, false
	}

	// Update last accessed
	rco.manager.Mu.Lock()
	rco.manager.LastAccessed[tenantID] = time.Now().UTC()
	rco.manager.Mu.Unlock()

	return ids, true
}

// InvalidateResource removes a specific resource from cache
func (rco *ResourceCacheOperations) InvalidateResource(tenantID, id string) {
	rco.manager.Mu.RLock()
	tenantCache, exists := rco.manager.ContentCache[tenantID]
	rco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Get resource to remove slug lookup and category indexing
	if resource, exists := tenantCache.Resources[id]; exists {
		delete(tenantCache.SlugToID, "resource:"+resource.Slug)

		// Remove from category indexing
		if resource.CategorySlug != nil {
			category := *resource.CategorySlug
			if categoryIDs, exists := tenantCache.CategoryToIDs[category]; exists {
				// Remove resource ID from category
				for i, resourceID := range categoryIDs {
					if resourceID == id {
						tenantCache.CategoryToIDs[category] = append(categoryIDs[:i], categoryIDs[i+1:]...)
						break
					}
				}
				// Clean up empty categories
				if len(tenantCache.CategoryToIDs[category]) == 0 {
					delete(tenantCache.CategoryToIDs, category)
				}
			}
		}
	}

	// Remove resource
	delete(tenantCache.Resources, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// InvalidateAllResources clears all resource cache for a tenant
func (rco *ResourceCacheOperations) InvalidateAllResources(tenantID string) {
	rco.manager.Mu.RLock()
	tenantCache, exists := rco.manager.ContentCache[tenantID]
	rco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Remove slug lookups for all resources
	for _, resource := range tenantCache.Resources {
		delete(tenantCache.SlugToID, "resource:"+resource.Slug)
	}

	// Clear category indexing for resources
	tenantCache.CategoryToIDs = make(map[string][]string)

	// Clear resources
	tenantCache.Resources = make(map[string]*models.ResourceNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// ensureTenantCache creates tenant cache if it doesn't exist
func (rco *ResourceCacheOperations) ensureTenantCache(tenantID string) {
	rco.manager.Mu.Lock()
	defer rco.manager.Mu.Unlock()

	if _, exists := rco.manager.ContentCache[tenantID]; !exists {
		rco.manager.ContentCache[tenantID] = &models.TenantContentCache{
			TractStacks:    make(map[string]*models.TractStackNode),
			StoryFragments: make(map[string]*models.StoryFragmentNode),
			Panes:          make(map[string]*models.PaneNode),
			Menus:          make(map[string]*models.MenuNode),
			Resources:      make(map[string]*models.ResourceNode),
			Beliefs:        make(map[string]*models.BeliefNode),
			Files:          make(map[string]*models.ImageFileNode),
			SlugToID:       make(map[string]string),
			CategoryToIDs:  make(map[string][]string),
			AllPaneIDs:     []string{},
			LastUpdated:    time.Now().UTC(),
		}
	}

	rco.manager.LastAccessed[tenantID] = time.Now().UTC()
}
