// Package content provides imagefiles helpers
package content

import (
	"fmt"
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

// SetFile stores an imagefile in cache using safe lookup
func (ifco *ImageFileCacheOperations) SetFile(tenantID string, node *models.ImageFileNode) error {
	// Use safe cache lookup instead of ensureTenantCache
	ifco.manager.Mu.RLock()
	tenantCache, exists := ifco.manager.ContentCache[tenantID]
	ifco.manager.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the imagefile
	tenantCache.Files[node.ID] = node

	// Note: ImageFiles don't typically have slugs in the current system,
	// so no slug lookup needed

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	ifco.manager.Mu.Lock()
	ifco.manager.LastAccessed[tenantID] = time.Now().UTC()
	ifco.manager.Mu.Unlock()

	return nil
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

	// Collect all file IDs
	ids := make([]string, 0, len(tenantCache.Files))
	for id := range tenantCache.Files {
		ids = append(ids, id)
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

	// Remove imagefile (no slug lookup to clean up for imagefiles)
	delete(tenantCache.Files, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	ifco.manager.Mu.Lock()
	ifco.manager.LastAccessed[tenantID] = time.Now().UTC()
	ifco.manager.Mu.Unlock()
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

	// Clear imagefiles (no slug lookups to clean up)
	tenantCache.Files = make(map[string]*models.ImageFileNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}
