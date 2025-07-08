// Package content provides pane helpers
package content

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// PaneCacheOperations implements pane-specific cache operations
type PaneCacheOperations struct {
	manager *models.CacheManager
}

// NewPaneCacheOperations creates a new pane cache operations handler
func NewPaneCacheOperations(manager *models.CacheManager) *PaneCacheOperations {
	return &PaneCacheOperations{manager: manager}
}

// GetPane retrieves a pane by ID from cache
func (pco *PaneCacheOperations) GetPane(tenantID, id string) (*models.PaneNode, bool) {
	pco.manager.Mu.RLock()
	tenantCache, exists := pco.manager.ContentCache[tenantID]
	pco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired (24 hours TTL)
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	pane, exists := tenantCache.Panes[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	pco.manager.Mu.Lock()
	pco.manager.LastAccessed[tenantID] = time.Now().UTC()
	pco.manager.Mu.Unlock()

	return pane, true
}

// SetPane stores a pane in cache
func (pco *PaneCacheOperations) SetPane(tenantID string, node *models.PaneNode) {
	pco.ensureTenantCache(tenantID)

	pco.manager.Mu.RLock()
	tenantCache := pco.manager.ContentCache[tenantID]
	pco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the pane
	tenantCache.Panes[node.ID] = node

	// Update slug lookup
	tenantCache.SlugToID[node.Slug] = node.ID

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	pco.manager.Mu.Lock()
	pco.manager.LastAccessed[tenantID] = time.Now().UTC()
	pco.manager.Mu.Unlock()
}

// GetPaneBySlug retrieves a pane by slug from cache
func (pco *PaneCacheOperations) GetPaneBySlug(tenantID, slug string) (*models.PaneNode, bool) {
	pco.manager.Mu.RLock()
	tenantCache, exists := pco.manager.ContentCache[tenantID]
	pco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	// Get ID from slug lookup
	id, exists := tenantCache.SlugToID[slug]
	if !exists {
		return nil, false
	}

	// Get pane by ID
	pane, exists := tenantCache.Panes[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	pco.manager.Mu.Lock()
	pco.manager.LastAccessed[tenantID] = time.Now().UTC()
	pco.manager.Mu.Unlock()

	return pane, true
}

// GetAllPaneIDs retrieves all pane IDs from cache
func (pco *PaneCacheOperations) GetAllPaneIDs(tenantID string) ([]string, bool) {
	pco.manager.Mu.RLock()
	tenantCache, exists := pco.manager.ContentCache[tenantID]
	pco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	// Check if AllPaneIDs is populated
	if len(tenantCache.AllPaneIDs) == 0 {
		return nil, false
	}

	// Update last accessed
	pco.manager.Mu.Lock()
	pco.manager.LastAccessed[tenantID] = time.Now().UTC()
	pco.manager.Mu.Unlock()

	// Return copy to avoid external mutation
	ids := make([]string, len(tenantCache.AllPaneIDs))
	copy(ids, tenantCache.AllPaneIDs)

	return ids, true
}

// SetAllPaneIDs stores all pane IDs in cache
func (pco *PaneCacheOperations) SetAllPaneIDs(tenantID string, ids []string) {
	pco.ensureTenantCache(tenantID)

	pco.manager.Mu.RLock()
	tenantCache := pco.manager.ContentCache[tenantID]
	pco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store copy to avoid external mutation
	tenantCache.AllPaneIDs = make([]string, len(ids))
	copy(tenantCache.AllPaneIDs, ids)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	pco.manager.Mu.Lock()
	pco.manager.LastAccessed[tenantID] = time.Now().UTC()
	pco.manager.Mu.Unlock()
}

// InvalidatePane removes a specific pane from cache
func (pco *PaneCacheOperations) InvalidatePane(tenantID, id string) {
	pco.manager.Mu.RLock()
	tenantCache, exists := pco.manager.ContentCache[tenantID]
	pco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Get pane to remove slug lookup
	if pane, exists := tenantCache.Panes[id]; exists {
		delete(tenantCache.SlugToID, pane.Slug)
	}

	// Remove pane
	delete(tenantCache.Panes, id)

	// Remove from AllPaneIDs if present
	for i, paneID := range tenantCache.AllPaneIDs {
		if paneID == id {
			tenantCache.AllPaneIDs = append(tenantCache.AllPaneIDs[:i], tenantCache.AllPaneIDs[i+1:]...)
			break
		}
	}

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// InvalidateAllPanes clears all pane cache for a tenant
func (pco *PaneCacheOperations) InvalidateAllPanes(tenantID string) {
	pco.manager.Mu.RLock()
	tenantCache, exists := pco.manager.ContentCache[tenantID]
	pco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Remove slug lookups for all panes
	for _, pane := range tenantCache.Panes {
		delete(tenantCache.SlugToID, pane.Slug)
	}

	// Clear panes
	tenantCache.Panes = make(map[string]*models.PaneNode)
	tenantCache.AllPaneIDs = []string{}

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// ensureTenantCache creates tenant cache if it doesn't exist
func (pco *PaneCacheOperations) ensureTenantCache(tenantID string) {
	pco.manager.Mu.Lock()
	defer pco.manager.Mu.Unlock()

	if _, exists := pco.manager.ContentCache[tenantID]; !exists {
		pco.manager.ContentCache[tenantID] = &models.TenantContentCache{
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

	pco.manager.LastAccessed[tenantID] = time.Now().UTC()
}
