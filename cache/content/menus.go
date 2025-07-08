// Package content provides menu cache operations
package content

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// MenuCacheOperations implements menu-specific cache operations
type MenuCacheOperations struct {
	manager *models.CacheManager
}

// NewMenuCacheOperations creates a new menu cache operations handler
func NewMenuCacheOperations(manager *models.CacheManager) *MenuCacheOperations {
	return &MenuCacheOperations{manager: manager}
}

// GetMenu retrieves a menu by ID from cache
func (mco *MenuCacheOperations) GetMenu(tenantID, id string) (*models.MenuNode, bool) {
	mco.manager.Mu.RLock()
	tenantCache, exists := mco.manager.ContentCache[tenantID]
	mco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired (24 hours TTL)
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	menu, exists := tenantCache.Menus[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	mco.manager.Mu.Lock()
	mco.manager.LastAccessed[tenantID] = time.Now().UTC()
	mco.manager.Mu.Unlock()

	return menu, true
}

// SetMenu stores a menu in cache
func (mco *MenuCacheOperations) SetMenu(tenantID string, node *models.MenuNode) {
	mco.ensureTenantCache(tenantID)

	mco.manager.Mu.RLock()
	tenantCache := mco.manager.ContentCache[tenantID]
	mco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the menu
	tenantCache.Menus[node.ID] = node

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	mco.manager.Mu.Lock()
	mco.manager.LastAccessed[tenantID] = time.Now().UTC()
	mco.manager.Mu.Unlock()
}

// GetAllMenuIDs retrieves all menu IDs from cache
func (mco *MenuCacheOperations) GetAllMenuIDs(tenantID string) ([]string, bool) {
	mco.manager.Mu.RLock()
	tenantCache, exists := mco.manager.ContentCache[tenantID]
	mco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	// Extract IDs from cached menus
	var ids []string
	for id := range tenantCache.Menus {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, false
	}

	// Update last accessed
	mco.manager.Mu.Lock()
	mco.manager.LastAccessed[tenantID] = time.Now().UTC()
	mco.manager.Mu.Unlock()

	return ids, true
}

// InvalidateMenu removes a specific menu from cache
func (mco *MenuCacheOperations) InvalidateMenu(tenantID, id string) {
	mco.manager.Mu.RLock()
	tenantCache, exists := mco.manager.ContentCache[tenantID]
	mco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Remove menu
	delete(tenantCache.Menus, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// InvalidateAllMenus clears all menu cache for a tenant
func (mco *MenuCacheOperations) InvalidateAllMenus(tenantID string) {
	mco.manager.Mu.RLock()
	tenantCache, exists := mco.manager.ContentCache[tenantID]
	mco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Clear menus
	tenantCache.Menus = make(map[string]*models.MenuNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// ensureTenantCache creates tenant cache if it doesn't exist
func (mco *MenuCacheOperations) ensureTenantCache(tenantID string) {
	mco.manager.Mu.Lock()
	defer mco.manager.Mu.Unlock()

	if _, exists := mco.manager.ContentCache[tenantID]; !exists {
		mco.manager.ContentCache[tenantID] = &models.TenantContentCache{
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

	mco.manager.LastAccessed[tenantID] = time.Now().UTC()
}
