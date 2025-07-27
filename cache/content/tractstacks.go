// Package content provides tractstacks helpers
package content

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// TractStackCacheOperations implements tractstack-specific cache operations
type TractStackCacheOperations struct {
	manager *models.CacheManager
}

// NewTractStackCacheOperations creates a new tractstack cache operations handler
func NewTractStackCacheOperations(manager *models.CacheManager) *TractStackCacheOperations {
	return &TractStackCacheOperations{manager: manager}
}

// GetTractStack retrieves a tractstack by ID from cache
func (tsco *TractStackCacheOperations) GetTractStack(tenantID, id string) (*models.TractStackNode, bool) {
	tsco.manager.Mu.RLock()
	tenantCache, exists := tsco.manager.ContentCache[tenantID]
	tsco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired (24 hours TTL)
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	tractStack, exists := tenantCache.TractStacks[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	tsco.manager.Mu.Lock()
	tsco.manager.LastAccessed[tenantID] = time.Now().UTC()
	tsco.manager.Mu.Unlock()

	return tractStack, true
}

// SetTractStack stores a tractstack in cache using safe lookup
func (tsco *TractStackCacheOperations) SetTractStack(tenantID string, node *models.TractStackNode) error {
	// Use safe cache lookup instead of ensureTenantCache
	tsco.manager.Mu.RLock()
	tenantCache, exists := tsco.manager.ContentCache[tenantID]
	tsco.manager.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the tractstack
	tenantCache.TractStacks[node.ID] = node

	// Update slug lookup with prefix
	tenantCache.SlugToID["tractstack:"+node.Slug] = node.ID

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	tsco.manager.Mu.Lock()
	tsco.manager.LastAccessed[tenantID] = time.Now().UTC()
	tsco.manager.Mu.Unlock()

	return nil
}

// GetTractStackBySlug retrieves a tractstack by slug from cache
func (tsco *TractStackCacheOperations) GetTractStackBySlug(tenantID, slug string) (*models.TractStackNode, bool) {
	tsco.manager.Mu.RLock()
	tenantCache, exists := tsco.manager.ContentCache[tenantID]
	tsco.manager.Mu.RUnlock()

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
	id, exists := tenantCache.SlugToID["tractstack:"+slug]
	if !exists {
		return nil, false
	}

	// Get tractstack by ID
	tractStack, exists := tenantCache.TractStacks[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	tsco.manager.Mu.Lock()
	tsco.manager.LastAccessed[tenantID] = time.Now().UTC()
	tsco.manager.Mu.Unlock()

	return tractStack, true
}

// InvalidateTractStack removes a specific tractstack from cache
func (tsco *TractStackCacheOperations) InvalidateTractStack(tenantID, id string) {
	tsco.manager.Mu.RLock()
	tenantCache, exists := tsco.manager.ContentCache[tenantID]
	tsco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Get tractstack to remove slug lookup
	if tractStack, exists := tenantCache.TractStacks[id]; exists {
		delete(tenantCache.SlugToID, "tractstack:"+tractStack.Slug)
	}

	// Remove tractstack
	delete(tenantCache.TractStacks, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	tsco.manager.Mu.Lock()
	tsco.manager.LastAccessed[tenantID] = time.Now().UTC()
	tsco.manager.Mu.Unlock()
}

// InvalidateAllTractStacks clears all tractstack cache for a tenant
func (tsco *TractStackCacheOperations) InvalidateAllTractStacks(tenantID string) {
	tsco.manager.Mu.RLock()
	tenantCache, exists := tsco.manager.ContentCache[tenantID]
	tsco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Remove slug lookups for all tractstacks
	for _, tractStack := range tenantCache.TractStacks {
		delete(tenantCache.SlugToID, "tractstack:"+tractStack.Slug)
	}

	// Clear tractstacks
	tenantCache.TractStacks = make(map[string]*models.TractStackNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}
