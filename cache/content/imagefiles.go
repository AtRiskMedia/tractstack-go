// Package content provides imagefile cache operations
package content

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// ImageFileCacheOperations implements imagefile-specific cache operations
type ImageFileCacheOperations struct {
	manager *models.CacheManager
}

// NewImageFileCacheOperations creates a new imagefile cache operations handler
func NewImageFileCacheOperations(manager *models.CacheManager) *ImageFileCacheOperations {
	return &ImageFileCacheOperations{manager: manager}
}

// GetFile retrieves an imagefile by ID from cache
func (ifco *ImageFileCacheOperations) GetFile(tenantID, id string) (*models.ImageFileNode, bool) {
	ifco.manager.Mu.RLock()
	tenantCache, exists := ifco.manager.ContentCache[tenantID]
	ifco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired (24 hours TTL)
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	file, exists := tenantCache.Files[id]
	if !exists {
		return nil, false
	}

	// Update last accessed
	ifco.manager.Mu.Lock()
	ifco.manager.LastAccessed[tenantID] = time.Now().UTC()
	ifco.manager.Mu.Unlock()

	return file, true
}

// SetFile stores an imagefile in cache
func (ifco *ImageFileCacheOperations) SetFile(tenantID string, node *models.ImageFileNode) {
	ifco.ensureTenantCache(tenantID)

	ifco.manager.Mu.RLock()
	tenantCache := ifco.manager.ContentCache[tenantID]
	ifco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the imagefile
	tenantCache.Files[node.ID] = node

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	ifco.manager.Mu.Lock()
	ifco.manager.LastAccessed[tenantID] = time.Now().UTC()
	ifco.manager.Mu.Unlock()
}

// GetAllFileIDs retrieves all imagefile IDs from cache
func (ifco *ImageFileCacheOperations) GetAllFileIDs(tenantID string) ([]string, bool) {
	ifco.manager.Mu.RLock()
	tenantCache, exists := ifco.manager.ContentCache[tenantID]
	ifco.manager.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	// Check if cache is expired
	if time.Since(tenantCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	// Extract IDs from cached files
	var ids []string
	for id := range tenantCache.Files {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, false
	}

	// Update last accessed
	ifco.manager.Mu.Lock()
	ifco.manager.LastAccessed[tenantID] = time.Now().UTC()
	ifco.manager.Mu.Unlock()

	return ids, true
}

// InvalidateFile removes a specific imagefile from cache
func (ifco *ImageFileCacheOperations) InvalidateFile(tenantID, id string) {
	ifco.manager.Mu.RLock()
	tenantCache, exists := ifco.manager.ContentCache[tenantID]
	ifco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Remove imagefile
	delete(tenantCache.Files, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// InvalidateAllFiles clears all imagefile cache for a tenant
func (ifco *ImageFileCacheOperations) InvalidateAllFiles(tenantID string) {
	ifco.manager.Mu.RLock()
	tenantCache, exists := ifco.manager.ContentCache[tenantID]
	ifco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Clear imagefiles
	tenantCache.Files = make(map[string]*models.ImageFileNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}

// ensureTenantCache creates tenant cache if it doesn't exist
func (ifco *ImageFileCacheOperations) ensureTenantCache(tenantID string) {
	ifco.manager.Mu.Lock()
	defer ifco.manager.Mu.Unlock()

	if _, exists := ifco.manager.ContentCache[tenantID]; !exists {
		ifco.manager.ContentCache[tenantID] = &models.TenantContentCache{
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

	ifco.manager.LastAccessed[tenantID] = time.Now().UTC()
}
