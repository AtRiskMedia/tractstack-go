// Package content provides epinet cache operations
package content

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// EpinetCacheOperations implements epinet-specific cache operations
type EpinetCacheOperations struct {
	manager *models.CacheManager
}

// NewEpinetCacheOperations creates a new epinet cache operations handler
func NewEpinetCacheOperations(manager *models.CacheManager) *EpinetCacheOperations {
	return &EpinetCacheOperations{manager: manager}
}

// GetEpinet retrieves an epinet by ID from cache
func (eco *EpinetCacheOperations) GetEpinet(tenantID, id string) (*models.EpinetNode, bool) {
	eco.manager.Mu.RLock()
	tenantCache, exists := eco.manager.ContentCache[tenantID]
	eco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired (24 hours TTL)
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	epinet, exists := tenantCache.Epinets[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	eco.manager.Mu.Lock()
	eco.manager.LastAccessed[tenantID] = time.Now().UTC()
	eco.manager.Mu.Unlock()

	return epinet, true
}

// SetEpinet stores an epinet in cache
func (eco *EpinetCacheOperations) SetEpinet(tenantID string, node *models.EpinetNode) {
	eco.ensureTenantCache(tenantID)

	eco.manager.Mu.RLock()
	tenantCache := eco.manager.ContentCache[tenantID]
	eco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the epinet
	tenantCache.Epinets[node.ID] = node

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	eco.manager.Mu.Lock()
	eco.manager.LastAccessed[tenantID] = time.Now().UTC()
	eco.manager.Mu.Unlock()
}

// GetAllEpinetIDs retrieves all epinet IDs from cache
func (eco *EpinetCacheOperations) GetAllEpinetIDs(tenantID string) ([]string, bool) {
	eco.manager.Mu.RLock()
	tenantCache, exists := eco.manager.ContentCache[tenantID]
	eco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	// Extract IDs from cached epinets
	var ids []string
	for id := range tenantCache.Epinets {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, false
	}

	// Update last accessed
	eco.manager.Mu.Lock()
	eco.manager.LastAccessed[tenantID] = time.Now().UTC()
	eco.manager.Mu.Unlock()

	return ids, true
}

// InvalidateEpinet removes a specific epinet from cache
func (eco *EpinetCacheOperations) InvalidateEpinet(tenantID, id string) {
	eco.manager.Mu.RLock()
	tenantCache, exists := eco.manager.ContentCache[tenantID]
	eco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Remove epinet
	delete(tenantCache.Epinets, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// InvalidateAllEpinets clears all epinet cache for a tenant
func (eco *EpinetCacheOperations) InvalidateAllEpinets(tenantID string) {
	eco.manager.Mu.RLock()
	tenantCache, exists := eco.manager.ContentCache[tenantID]
	eco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Clear epinets
	tenantCache.Epinets = make(map[string]*models.EpinetNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// ensureTenantCache creates tenant cache if it doesn't exist
func (eco *EpinetCacheOperations) ensureTenantCache(tenantID string) {
	eco.manager.Mu.Lock()
	defer eco.manager.Mu.Unlock()

	if _, exists := eco.manager.ContentCache[tenantID]; !exists {
		eco.manager.ContentCache[tenantID] = &models.TenantContentCache{
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

	eco.manager.LastAccessed[tenantID] = time.Now().UTC()
}
