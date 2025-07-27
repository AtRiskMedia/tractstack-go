// Package content provides resources helpers
package content

import (
	"fmt"
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

// SetResource stores a resource in cache using safe lookup
func (rco *ResourceCacheOperations) SetResource(tenantID string, node *models.ResourceNode) error {
	// Use safe cache lookup instead of ensureTenantCache
	rco.manager.Mu.RLock()
	tenantCache, exists := rco.manager.ContentCache[tenantID]
	rco.manager.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the resource
	tenantCache.Resources[node.ID] = node

	// Update slug lookup with prefix
	tenantCache.SlugToID["resource:"+node.Slug] = node.ID

	// Update category lookup if category is specified
	if node.CategorySlug != nil && *node.CategorySlug != "" {
		if tenantCache.CategoryToIDs[*node.CategorySlug] == nil {
			tenantCache.CategoryToIDs[*node.CategorySlug] = []string{}
		}

		// Check if ID already exists in category
		found := false
		for _, existingID := range tenantCache.CategoryToIDs[*node.CategorySlug] {
			if existingID == node.ID {
				found = true
				break
			}
		}

		// Add to category if not already present
		if !found {
			tenantCache.CategoryToIDs[*node.CategorySlug] = append(tenantCache.CategoryToIDs[*node.CategorySlug], node.ID)
		}
	}

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	rco.manager.Mu.Lock()
	rco.manager.LastAccessed[tenantID] = time.Now().UTC()
	rco.manager.Mu.Unlock()

	return nil
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

	// Get ID from slug lookup with prefix
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

// GetResourcesByCategory retrieves all resources in a category from cache
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
		return []*models.ResourceNode{}, true // Empty slice, not nil
	}

	// Collect resources
	var resources []*models.ResourceNode
	for _, id := range resourceIDs {
		if resource, exists := tenantCache.Resources[id]; exists {
			resources = append(resources, resource)
		}
	}

	// Update last accessed
	rco.manager.Mu.Lock()
	rco.manager.LastAccessed[tenantID] = time.Now().UTC()
	rco.manager.Mu.Unlock()

	return resources, true
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

	// Get resource to remove lookups
	if resource, exists := tenantCache.Resources[id]; exists {
		// Remove slug lookup
		delete(tenantCache.SlugToID, "resource:"+resource.Slug)

		// Remove from category lookup if categorized
		if resource.CategorySlug != nil && *resource.CategorySlug != "" {
			if categoryIDs, exists := tenantCache.CategoryToIDs[*resource.CategorySlug]; exists {
				// Remove ID from category slice
				for i, categoryID := range categoryIDs {
					if categoryID == id {
						tenantCache.CategoryToIDs[*resource.CategorySlug] = append(categoryIDs[:i], categoryIDs[i+1:]...)
						break
					}
				}

				// Remove category entry if empty
				if len(tenantCache.CategoryToIDs[*resource.CategorySlug]) == 0 {
					delete(tenantCache.CategoryToIDs, *resource.CategorySlug)
				}
			}
		}
	}

	// Remove resource
	delete(tenantCache.Resources, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	rco.manager.Mu.Lock()
	rco.manager.LastAccessed[tenantID] = time.Now().UTC()
	rco.manager.Mu.Unlock()
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

	// Clear category lookups for resources
	// Note: This clears ALL category mappings, which may include other content types
	// In a more sophisticated implementation, you'd only clear resource categories
	for category, ids := range tenantCache.CategoryToIDs {
		var filteredIDs []string
		for _, id := range ids {
			// Keep IDs that don't belong to resources
			if _, isResource := tenantCache.Resources[id]; !isResource {
				filteredIDs = append(filteredIDs, id)
			}
		}

		if len(filteredIDs) == 0 {
			delete(tenantCache.CategoryToIDs, category)
		} else {
			tenantCache.CategoryToIDs[category] = filteredIDs
		}
	}

	// Clear resources
	tenantCache.Resources = make(map[string]*models.ResourceNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}
