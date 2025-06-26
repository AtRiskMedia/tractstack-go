// Package content provides tractstack cache operations
package content

import (
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
	tsco.manager.LastAccessed[tenantID] = time.Now()
	tsco.manager.Mu.Unlock()

	return tractStack, true
}

// SetTractStack stores a tractstack in cache
func (tsco *TractStackCacheOperations) SetTractStack(tenantID string, node *models.TractStackNode) {
	tsco.ensureTenantCache(tenantID)

	tsco.manager.Mu.RLock()
	tenantCache := tsco.manager.ContentCache[tenantID]
	tsco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the tractstack
	tenantCache.TractStacks[node.ID] = node

	// Update slug lookup
	tenantCache.SlugToID["tractstack:"+node.Slug] = node.ID

	// Update last modified
	tenantCache.LastUpdated = time.Now()

	// Update last accessed
	tsco.manager.Mu.Lock()
	tsco.manager.LastAccessed[tenantID] = time.Now()
	tsco.manager.Mu.Unlock()
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

	// Get ID from slug lookup (prefixed to avoid conflicts)
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
	tsco.manager.LastAccessed[tenantID] = time.Now()
	tsco.manager.Mu.Unlock()

	return tractStack, true
}

// GetAllTractStackIDs retrieves all tractstack IDs from cache
func (tsco *TractStackCacheOperations) GetAllTractStackIDs(tenantID string) ([]string, bool) {
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

	// Extract IDs from cached tractstacks
	var ids []string
	for id := range tenantCache.TractStacks {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, false
	}

	// Update last accessed
	tsco.manager.Mu.Lock()
	tsco.manager.LastAccessed[tenantID] = time.Now()
	tsco.manager.Mu.Unlock()

	return ids, true
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
	tenantCache.LastUpdated = time.Now()
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
	tenantCache.LastUpdated = time.Now()
}

// ensureTenantCache creates tenant cache if it doesn't exist
func (tsco *TractStackCacheOperations) ensureTenantCache(tenantID string) {
	tsco.manager.Mu.Lock()
	defer tsco.manager.Mu.Unlock()

	if _, exists := tsco.manager.ContentCache[tenantID]; !exists {
		tsco.manager.ContentCache[tenantID] = &models.TenantContentCache{
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
			LastUpdated:    time.Now(),
		}
	}

	tsco.manager.LastAccessed[tenantID] = time.Now()
}
