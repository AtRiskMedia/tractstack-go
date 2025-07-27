// Package content provides epinets helpers
package content

import (
	"fmt"
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

// SetEpinet stores an epinet in cache using safe lookup
func (eco *EpinetCacheOperations) SetEpinet(tenantID string, node *models.EpinetNode) error {
	// Use safe cache lookup instead of ensureTenantCache
	eco.manager.Mu.RLock()
	tenantCache, exists := eco.manager.ContentCache[tenantID]
	eco.manager.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Store the epinet
	tenantCache.Epinets[node.ID] = node

	// Note: Epinets don't typically have slugs in the current system,
	// so no slug lookup needed

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	eco.manager.Mu.Lock()
	eco.manager.LastAccessed[tenantID] = time.Now().UTC()
	eco.manager.Mu.Unlock()

	return nil
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

	// Collect all epinet IDs
	ids := make([]string, 0, len(tenantCache.Epinets))
	for id := range tenantCache.Epinets {
		ids = append(ids, id)
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

	// Remove epinet (no slug lookup to clean up for epinets)
	delete(tenantCache.Epinets, id)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()

	// Update last accessed
	eco.manager.Mu.Lock()
	eco.manager.LastAccessed[tenantID] = time.Now().UTC()
	eco.manager.Mu.Unlock()
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

	// Clear epinets (no slug lookups to clean up)
	tenantCache.Epinets = make(map[string]*models.EpinetNode)

	// Update last modified
	tenantCache.LastUpdated = time.Now().UTC()
}
