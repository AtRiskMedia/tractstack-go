// Package content provides menus helpers
package content

import (
	"fmt"
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

// SetMenu stores a menu in cache using safe lookup
func (mco *MenuCacheOperations) SetMenu(tenantID string, node *models.MenuNode) error {
	// Use safe cache lookup instead of ensureTenantCache
	mco.manager.Mu.RLock()
	tenantCache, exists := mco.manager.ContentCache[tenantID]
	mco.manager.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the menu
	tenantCache.Menus[node.ID] = node

	// Note: Menus don't typically have slugs in the current system,
	// so no slug lookup needed

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	mco.manager.Mu.Lock()
	mco.manager.LastAccessed[tenantID] = time.Now().UTC()
	mco.manager.Mu.Unlock()

	return nil
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

	// Remove menu (no slug lookup to clean up for menus)
	delete(tenantCache.Menus, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	mco.manager.Mu.Lock()
	mco.manager.LastAccessed[tenantID] = time.Now().UTC()
	mco.manager.Mu.Unlock()
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

	// Clear menus (no slug lookups to clean up)
	tenantCache.Menus = make(map[string]*models.MenuNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}
