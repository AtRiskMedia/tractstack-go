// Package content provides belief cache operations
package content

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// BeliefCacheOperations implements belief-specific cache operations
type BeliefCacheOperations struct {
	manager *models.CacheManager
}

// NewBeliefCacheOperations creates a new belief cache operations handler
func NewBeliefCacheOperations(manager *models.CacheManager) *BeliefCacheOperations {
	return &BeliefCacheOperations{manager: manager}
}

// GetBelief retrieves a belief by ID from cache
func (bco *BeliefCacheOperations) GetBelief(tenantID, id string) (*models.BeliefNode, bool) {
	bco.manager.Mu.RLock()
	tenantCache, exists := bco.manager.ContentCache[tenantID]
	bco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired (24 hours TTL)
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	belief, exists := tenantCache.Beliefs[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	bco.manager.Mu.Lock()
	bco.manager.LastAccessed[tenantID] = time.Now().UTC()
	bco.manager.Mu.Unlock()

	return belief, true
}

// SetBelief stores a belief in cache
func (bco *BeliefCacheOperations) SetBelief(tenantID string, node *models.BeliefNode) {
	bco.ensureTenantCache(tenantID)

	bco.manager.Mu.RLock()
	tenantCache := bco.manager.ContentCache[tenantID]
	bco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the belief
	tenantCache.Beliefs[node.ID] = node

	// Update slug lookup
	tenantCache.SlugToID["belief:"+node.Slug] = node.ID

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	bco.manager.Mu.Lock()
	bco.manager.LastAccessed[tenantID] = time.Now().UTC()
	bco.manager.Mu.Unlock()
}

// GetBeliefBySlug retrieves a belief by slug from cache
func (bco *BeliefCacheOperations) GetBeliefBySlug(tenantID, slug string) (*models.BeliefNode, bool) {
	bco.manager.Mu.RLock()
	tenantCache, exists := bco.manager.ContentCache[tenantID]
	bco.manager.Mu.RUnlock()

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
	id, exists := tenantCache.SlugToID["belief:"+slug]
	if !exists {
		return nil, false
	}

	// Get belief by ID
	belief, exists := tenantCache.Beliefs[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	bco.manager.Mu.Lock()
	bco.manager.LastAccessed[tenantID] = time.Now().UTC()
	bco.manager.Mu.Unlock()

	return belief, true
}

// GetAllBeliefIDs retrieves all belief IDs from cache
func (bco *BeliefCacheOperations) GetAllBeliefIDs(tenantID string) ([]string, bool) {
	bco.manager.Mu.RLock()
	tenantCache, exists := bco.manager.ContentCache[tenantID]
	bco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	// Extract IDs from cached beliefs
	var ids []string
	for id := range tenantCache.Beliefs {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, false
	}

	// Update last accessed
	bco.manager.Mu.Lock()
	bco.manager.LastAccessed[tenantID] = time.Now().UTC()
	bco.manager.Mu.Unlock()

	return ids, true
}

// InvalidateBelief removes a specific belief from cache
func (bco *BeliefCacheOperations) InvalidateBelief(tenantID, id string) {
	bco.manager.Mu.RLock()
	tenantCache, exists := bco.manager.ContentCache[tenantID]
	bco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Get belief to remove slug lookup
	if belief, exists := tenantCache.Beliefs[id]; exists {
		delete(tenantCache.SlugToID, "belief:"+belief.Slug)
	}

	// Remove belief
	delete(tenantCache.Beliefs, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// InvalidateAllBeliefs clears all belief cache for a tenant
func (bco *BeliefCacheOperations) InvalidateAllBeliefs(tenantID string) {
	bco.manager.Mu.RLock()
	tenantCache, exists := bco.manager.ContentCache[tenantID]
	bco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Remove slug lookups for all beliefs
	for _, belief := range tenantCache.Beliefs {
		delete(tenantCache.SlugToID, "belief:"+belief.Slug)
	}

	// Clear beliefs
	tenantCache.Beliefs = make(map[string]*models.BeliefNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// ensureTenantCache creates tenant cache if it doesn't exist
func (bco *BeliefCacheOperations) ensureTenantCache(tenantID string) {
	bco.manager.Mu.Lock()
	defer bco.manager.Mu.Unlock()

	if _, exists := bco.manager.ContentCache[tenantID]; !exists {
		bco.manager.ContentCache[tenantID] = &models.TenantContentCache{
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

	bco.manager.LastAccessed[tenantID] = time.Now().UTC()
}

// GetBeliefIDBySlug retrieves only the belief ID by slug from cache
func (bco *BeliefCacheOperations) GetBeliefIDBySlug(tenantID, slug string) (string, bool) {
	bco.manager.Mu.RLock()
	tenantCache, exists := bco.manager.ContentCache[tenantID]
	bco.manager.Mu.RUnlock()

	if !exists {
		return "", false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return "", false
	}

	// Get ID directly from slug lookup (prefixed to avoid conflicts)
	id, exists := tenantCache.SlugToID["belief:"+slug]
	if exists {
		// Update last accessed
		bco.manager.Mu.Lock()
		bco.manager.LastAccessed[tenantID] = time.Now().UTC()
		bco.manager.Mu.Unlock()
	}

	return id, exists
}
