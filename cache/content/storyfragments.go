// Package content provides storyfragments helpers
package content

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// StoryFragmentCacheOperations implements storyfragment-specific cache operations
type StoryFragmentCacheOperations struct {
	manager *models.CacheManager
}

// NewStoryFragmentCacheOperations creates a new storyfragment cache operations handler
func NewStoryFragmentCacheOperations(manager *models.CacheManager) *StoryFragmentCacheOperations {
	return &StoryFragmentCacheOperations{manager: manager}
}

// GetStoryFragment retrieves a storyfragment by ID from cache
func (sfco *StoryFragmentCacheOperations) GetStoryFragment(tenantID, id string) (*models.StoryFragmentNode, bool) {
	sfco.manager.Mu.RLock()
	tenantCache, exists := sfco.manager.ContentCache[tenantID]
	sfco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired (24 hours TTL)
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	storyFragment, exists := tenantCache.StoryFragments[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	sfco.manager.Mu.Lock()
	sfco.manager.LastAccessed[tenantID] = time.Now().UTC()
	sfco.manager.Mu.Unlock()

	return storyFragment, true
}

// SetStoryFragment stores a storyfragment in cache using safe lookup
func (sfco *StoryFragmentCacheOperations) SetStoryFragment(tenantID string, node *models.StoryFragmentNode) error {
	// Use safe cache lookup instead of ensureTenantCache
	sfco.manager.Mu.RLock()
	tenantCache, exists := sfco.manager.ContentCache[tenantID]
	sfco.manager.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the storyfragment
	tenantCache.StoryFragments[node.ID] = node

	// Update slug lookup with prefix
	tenantCache.SlugToID["storyfragment:"+node.Slug] = node.ID

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	sfco.manager.Mu.Lock()
	sfco.manager.LastAccessed[tenantID] = time.Now().UTC()
	sfco.manager.Mu.Unlock()

	return nil
}

// GetStoryFragmentBySlug retrieves a storyfragment by slug from cache
func (sfco *StoryFragmentCacheOperations) GetStoryFragmentBySlug(tenantID, slug string) (*models.StoryFragmentNode, bool) {
	sfco.manager.Mu.RLock()
	tenantCache, exists := sfco.manager.ContentCache[tenantID]
	sfco.manager.Mu.RUnlock()

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
	id, exists := tenantCache.SlugToID["storyfragment:"+slug]
	if !exists {
		return nil, false
	}

	// Get storyfragment by ID
	storyFragment, exists := tenantCache.StoryFragments[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	sfco.manager.Mu.Lock()
	sfco.manager.LastAccessed[tenantID] = time.Now().UTC()
	sfco.manager.Mu.Unlock()

	return storyFragment, true
}

// InvalidateStoryFragment removes a specific storyfragment from cache
func (sfco *StoryFragmentCacheOperations) InvalidateStoryFragment(tenantID, id string) {
	sfco.manager.Mu.RLock()
	tenantCache, exists := sfco.manager.ContentCache[tenantID]
	sfco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Get storyfragment to remove slug lookup
	if storyFragment, exists := tenantCache.StoryFragments[id]; exists {
		delete(tenantCache.SlugToID, "storyfragment:"+storyFragment.Slug)
	}

	// Remove storyfragment
	delete(tenantCache.StoryFragments, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	sfco.manager.Mu.Lock()
	sfco.manager.LastAccessed[tenantID] = time.Now().UTC()
	sfco.manager.Mu.Unlock()
}

// InvalidateAllStoryFragments clears all storyfragment cache for a tenant
func (sfco *StoryFragmentCacheOperations) InvalidateAllStoryFragments(tenantID string) {
	sfco.manager.Mu.RLock()
	tenantCache, exists := sfco.manager.ContentCache[tenantID]
	sfco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Remove slug lookups for all storyfragments
	for _, storyFragment := range tenantCache.StoryFragments {
		delete(tenantCache.SlugToID, "storyfragment:"+storyFragment.Slug)
	}

	// Clear storyfragments
	tenantCache.StoryFragments = make(map[string]*models.StoryFragmentNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}
