// Package content provides beliefs helpers
package content

import (
	"fmt"
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

// SetBelief stores a belief in cache using safe lookup
func (bco *BeliefCacheOperations) SetBelief(tenantID string, node *models.BeliefNode) error {
	// Use safe cache lookup instead of ensureTenantCache
	bco.manager.Mu.RLock()
	tenantCache, exists := bco.manager.ContentCache[tenantID]
	bco.manager.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the belief
	tenantCache.Beliefs[node.ID] = node

	// Update slug lookup with prefix
	tenantCache.SlugToID["belief:"+node.Slug] = node.ID

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	bco.manager.Mu.Lock()
	bco.manager.LastAccessed[tenantID] = time.Now().UTC()
	bco.manager.Mu.Unlock()

	return nil
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

	// Get ID from slug lookup with prefix
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

// GetBeliefIDBySlug retrieves a belief ID by slug from cache
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

	// Get ID from slug lookup with prefix
	id, exists := tenantCache.SlugToID["belief:"+slug]
	if !exists {
		return "", false
	}

	// Verify belief still exists
	if _, exists := tenantCache.Beliefs[id]; !exists {
		return "", false
	}

	// Update last accessed
	bco.manager.Mu.Lock()
	bco.manager.LastAccessed[tenantID] = time.Now().UTC()
	bco.manager.Mu.Unlock()

	return id, true
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

	// Collect all belief IDs
	ids := make([]string, 0, len(tenantCache.Beliefs))
	for id := range tenantCache.Beliefs {
		ids = append(ids, id)
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

	// Update last accessed
	bco.manager.Mu.Lock()
	bco.manager.LastAccessed[tenantID] = time.Now().UTC()
	bco.manager.Mu.Unlock()
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
