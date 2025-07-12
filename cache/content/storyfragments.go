// Package content provides storyfragment cache operations
package content

import (
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

// SetStoryFragment stores a storyfragment in cache
func (sfco *StoryFragmentCacheOperations) SetStoryFragment(tenantID string, node *models.StoryFragmentNode) {
	sfco.ensureTenantCache(tenantID)

	sfco.manager.Mu.RLock()
	tenantCache := sfco.manager.ContentCache[tenantID]
	sfco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the storyfragment
	tenantCache.StoryFragments[node.ID] = node

	// Update slug lookup
	tenantCache.SlugToID["storyfragment:"+node.Slug] = node.ID

	// Update tractstack indexing
	tractStackKey := "tractstack:" + node.TractStackID
	if _, exists := tenantCache.CategoryToIDs[tractStackKey]; !exists {
		tenantCache.CategoryToIDs[tractStackKey] = []string{}
	}
	// Add storyfragment ID to tractstack if not already present
	found := false
	for _, existingID := range tenantCache.CategoryToIDs[tractStackKey] {
		if existingID == node.ID {
			found = true
			break
		}
	}
	if !found {
		tenantCache.CategoryToIDs[tractStackKey] = append(tenantCache.CategoryToIDs[tractStackKey], node.ID)
	}

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	sfco.manager.Mu.Lock()
	sfco.manager.LastAccessed[tenantID] = time.Now().UTC()
	sfco.manager.Mu.Unlock()
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

	// Get ID from slug lookup (prefixed to avoid conflicts)
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

// GetStoryFragmentsByTractStack retrieves storyfragments by tractstack ID from cache
func (sfco *StoryFragmentCacheOperations) GetStoryFragmentsByTractStack(tenantID, tractStackID string) ([]*models.StoryFragmentNode, bool) {
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

	// Get storyfragment IDs for tractstack
	tractStackKey := "tractstack:" + tractStackID
	storyFragmentIDs, exists := tenantCache.CategoryToIDs[tractStackKey]
	if !exists || len(storyFragmentIDs) == 0 {
		return nil, false
	}

	// Build result array
	var storyFragments []*models.StoryFragmentNode
	for _, id := range storyFragmentIDs {
		if storyFragment, exists := tenantCache.StoryFragments[id]; exists {
			storyFragments = append(storyFragments, storyFragment)
		}
	}

	if len(storyFragments) == 0 {
		return nil, false
	}

	// Update last accessed
	sfco.manager.Mu.Lock()
	sfco.manager.LastAccessed[tenantID] = time.Now().UTC()
	sfco.manager.Mu.Unlock()

	return storyFragments, true
}

// GetAllStoryFragmentIDs retrieves all storyfragment IDs from cache
func (sfco *StoryFragmentCacheOperations) GetAllStoryFragmentIDs(tenantID string) ([]string, bool) {
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

	// Extract IDs from cached storyfragments
	var ids []string
	for id := range tenantCache.StoryFragments {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, false
	}

	// Update last accessed
	sfco.manager.Mu.Lock()
	sfco.manager.LastAccessed[tenantID] = time.Now().UTC()
	sfco.manager.Mu.Unlock()

	return ids, true
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

	// Get storyfragment to remove slug lookup and tractstack indexing
	if storyFragment, exists := tenantCache.StoryFragments[id]; exists {
		delete(tenantCache.SlugToID, "storyfragment:"+storyFragment.Slug)

		// Remove from tractstack indexing
		tractStackKey := "tractstack:" + storyFragment.TractStackID
		if tractStackIDs, exists := tenantCache.CategoryToIDs[tractStackKey]; exists {
			// Remove storyfragment ID from tractstack
			for i, storyFragmentID := range tractStackIDs {
				if storyFragmentID == id {
					tenantCache.CategoryToIDs[tractStackKey] = append(tractStackIDs[:i], tractStackIDs[i+1:]...)
					break
				}
			}
			// Clean up empty tractstack references
			if len(tenantCache.CategoryToIDs[tractStackKey]) == 0 {
				delete(tenantCache.CategoryToIDs, tractStackKey)
			}
		}
	}

	// Remove storyfragment
	delete(tenantCache.StoryFragments, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
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

	// Remove tractstack indexing for storyfragments
	for key := range tenantCache.CategoryToIDs {
		if len(key) > 11 && key[:11] == "tractstack:" {
			delete(tenantCache.CategoryToIDs, key)
		}
	}

	// Clear storyfragments
	tenantCache.StoryFragments = make(map[string]*models.StoryFragmentNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// ensureTenantCache creates tenant cache if it doesn't exist
func (sfco *StoryFragmentCacheOperations) ensureTenantCache(tenantID string) {
	sfco.manager.Mu.Lock()
	defer sfco.manager.Mu.Unlock()

	if _, exists := sfco.manager.ContentCache[tenantID]; !exists {
		sfco.manager.ContentCache[tenantID] = &models.TenantContentCache{
			TractStacks:    make(map[string]*models.TractStackNode),
			StoryFragments: make(map[string]*models.StoryFragmentNode),
			Panes:          make(map[string]*models.PaneNode),
			Menus:          make(map[string]*models.MenuNode),
			Resources:      make(map[string]*models.ResourceNode),
			Epinets:        make(map[string]*models.EpinetNode),
			Beliefs:        make(map[string]*models.BeliefNode),
			Files:          make(map[string]*models.ImageFileNode),
			SlugToID:       make(map[string]string),
			CategoryToIDs:  make(map[string][]string),
			AllPaneIDs:     []string{},
			LastUpdated:    time.Now().UTC(),
		}
	}

	sfco.manager.LastAccessed[tenantID] = time.Now().UTC()
}
