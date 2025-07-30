// Package stores provides concrete cache store implementations
package stores

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// FragmentsStore implements HTML fragment caching operations with tenant isolation
type FragmentsStore struct {
	tenantCaches map[string]*types.TenantHTMLChunkCache
	mu           sync.RWMutex
}

// NewFragmentsStore creates a new fragments cache store
func NewFragmentsStore() *FragmentsStore {
	return &FragmentsStore{
		tenantCaches: make(map[string]*types.TenantHTMLChunkCache),
	}
}

// InitializeTenant creates cache structures for a tenant
func (fs *FragmentsStore) InitializeTenant(tenantID string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.tenantCaches[tenantID] == nil {
		fs.tenantCaches[tenantID] = &types.TenantHTMLChunkCache{
			Chunks: make(map[string]*types.HTMLChunk),
			Deps:   make(map[string][]string),
		}
	}
}

// GetTenantCache safely retrieves a tenant's HTML chunk cache
func (fs *FragmentsStore) GetTenantCache(tenantID string) (*types.TenantHTMLChunkCache, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	cache, exists := fs.tenantCaches[tenantID]
	return cache, exists
}

// =============================================================================
// HTML Chunk Operations
// =============================================================================

// GetHTMLChunk retrieves an HTML chunk with pane variant
func (fs *FragmentsStore) GetHTMLChunk(tenantID, paneID string, variant types.PaneVariant) (*types.HTMLChunk, bool) {
	cache, exists := fs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	// Create chunk key from pane ID and variant
	chunkKey := fs.BuildChunkKey(paneID, variant)
	chunk, exists := cache.Chunks[chunkKey]

	if !exists {
		return nil, false
	}

	// Check if chunk is expired (1 hour TTL for HTML fragments)
	if time.Since(chunk.LastUpdated) > time.Hour {
		return nil, false
	}

	return chunk, true
}

// SetHTMLChunk stores an HTML chunk with dependencies
func (fs *FragmentsStore) SetHTMLChunk(tenantID, paneID string, variant types.PaneVariant, html string, dependsOn []string) {
	cache, exists := fs.GetTenantCache(tenantID)
	if !exists {
		fs.InitializeTenant(tenantID)
		cache, _ = fs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	chunkKey := fs.BuildChunkKey(paneID, variant)

	// Create HTML chunk
	chunk := &types.HTMLChunk{
		HTML:        html,
		PaneID:      paneID,
		Variant:     variant,
		DependsOn:   dependsOn,
		LastUpdated: time.Now().UTC(),
	}

	// Store chunk
	cache.Chunks[chunkKey] = chunk

	// Update dependency mappings
	fs.updateDependencies(cache, chunkKey, dependsOn)
}

// BuildChunkKey creates a unique key for HTML chunks based on pane ID and variant
func (fs *FragmentsStore) BuildChunkKey(paneID string, variant types.PaneVariant) string {
	if variant.BeliefMode == "" {
		return paneID + ":default"
	}

	// Include belief context in key for personalized variants
	key := paneID + ":" + variant.BeliefMode

	if len(variant.HeldBeliefs) > 0 {
		key += ":held"
		for _, belief := range variant.HeldBeliefs {
			key += "-" + belief
		}
	}

	if len(variant.WithheldBeliefs) > 0 {
		key += ":withheld"
		for _, belief := range variant.WithheldBeliefs {
			key += "-" + belief
		}
	}

	return key
}

// updateDependencies updates the dependency mappings for invalidation
func (fs *FragmentsStore) updateDependencies(cache *types.TenantHTMLChunkCache, chunkKey string, dependsOn []string) {
	// For each dependency, add this chunk key to its dependents list
	for _, depID := range dependsOn {
		if cache.Deps[depID] == nil {
			cache.Deps[depID] = make([]string, 0)
		}

		// Check if chunk key already exists in dependents
		found := false
		for _, existingKey := range cache.Deps[depID] {
			if existingKey == chunkKey {
				found = true
				break
			}
		}

		// Add if not found
		if !found {
			cache.Deps[depID] = append(cache.Deps[depID], chunkKey)
		}
	}
}

// =============================================================================
// Dependency-Based Invalidation Operations
// =============================================================================

// InvalidateByDependency invalidates all HTML chunks that depend on a specific content ID
func (fs *FragmentsStore) InvalidateByDependency(tenantID, dependencyID string) {
	cache, exists := fs.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	// Get all chunk keys that depend on this dependency
	dependentKeys, exists := cache.Deps[dependencyID]
	if !exists {
		return
	}

	// Remove all dependent chunks
	for _, chunkKey := range dependentKeys {
		delete(cache.Chunks, chunkKey)
	}

	// Clean up dependency mapping
	delete(cache.Deps, dependencyID)

	// Also clean up any dependency mappings that reference the deleted chunks
	fs.cleanupOrphanedDependencies(cache, dependentKeys)
}

// cleanupOrphanedDependencies removes chunk references from dependency mappings when chunks are deleted
func (fs *FragmentsStore) cleanupOrphanedDependencies(cache *types.TenantHTMLChunkCache, deletedChunkKeys []string) {
	for depID, chunkKeys := range cache.Deps {
		filteredKeys := make([]string, 0)

		for _, chunkKey := range chunkKeys {
			// Check if this chunk key was deleted
			wasDeleted := false
			for _, deletedKey := range deletedChunkKeys {
				if chunkKey == deletedKey {
					wasDeleted = true
					break
				}
			}

			// Keep chunk key if it wasn't deleted
			if !wasDeleted {
				filteredKeys = append(filteredKeys, chunkKey)
			}
		}

		// Update or remove dependency mapping
		if len(filteredKeys) == 0 {
			delete(cache.Deps, depID)
		} else {
			cache.Deps[depID] = filteredKeys
		}
	}
}

// InvalidateByPattern invalidates HTML chunks matching a pattern
func (fs *FragmentsStore) InvalidateByPattern(tenantID, pattern string) {
	cache, exists := fs.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	keysToDelete := make([]string, 0)

	// Find chunks matching pattern
	for chunkKey := range cache.Chunks {
		if fs.matchesPattern(chunkKey, pattern) {
			keysToDelete = append(keysToDelete, chunkKey)
		}
	}

	// Delete matching chunks
	for _, chunkKey := range keysToDelete {
		delete(cache.Chunks, chunkKey)
	}

	// Clean up dependency mappings
	fs.cleanupOrphanedDependencies(cache, keysToDelete)
}

// matchesPattern checks if a chunk key matches the given pattern
func (fs *FragmentsStore) matchesPattern(chunkKey, pattern string) bool {
	// Simple pattern matching - extend as needed
	if pattern == "*" {
		return true
	}

	// Pattern like "paneID:*" matches all variants of a pane
	if len(pattern) > 2 && pattern[len(pattern)-2:] == ":*" {
		panePrefix := pattern[:len(pattern)-1] // Remove the "*"
		return len(chunkKey) >= len(panePrefix) && chunkKey[:len(panePrefix)] == panePrefix
	}

	// Exact match
	return chunkKey == pattern
}

// =============================================================================
// Cache Management Operations
// =============================================================================

// InvalidateHTMLChunkCache clears all HTML chunk cache for a tenant
func (fs *FragmentsStore) InvalidateHTMLChunkCache(tenantID string) {
	cache, exists := fs.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	// Clear all chunks and dependencies
	cache.Chunks = make(map[string]*types.HTMLChunk)
	cache.Deps = make(map[string][]string)
}

// GetChunksByPaneID retrieves all cached variants for a specific pane
func (fs *FragmentsStore) GetChunksByPaneID(tenantID, paneID string) map[string]*types.HTMLChunk {
	cache, exists := fs.GetTenantCache(tenantID)
	if !exists {
		return make(map[string]*types.HTMLChunk)
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	result := make(map[string]*types.HTMLChunk)
	panePrefix := paneID + ":"

	for chunkKey, chunk := range cache.Chunks {
		if len(chunkKey) >= len(panePrefix) && chunkKey[:len(panePrefix)] == panePrefix {
			// Check if chunk is not expired
			if time.Since(chunk.LastUpdated) <= time.Hour {
				result[chunkKey] = chunk
			}
		}
	}

	return result
}

// GetHTMLChunkSummary returns cache status summary for debugging
func (fs *FragmentsStore) GetHTMLChunkSummary(tenantID string) map[string]any {
	cache, exists := fs.GetTenantCache(tenantID)
	if !exists {
		return map[string]any{
			"exists": false,
		}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	// Count expired vs active chunks
	activeChunks := 0
	expiredChunks := 0
	now := time.Now().UTC()

	for _, chunk := range cache.Chunks {
		if time.Since(chunk.LastUpdated) <= time.Hour {
			activeChunks++
		} else {
			expiredChunks++
		}
	}

	return map[string]any{
		"exists":        true,
		"totalChunks":   len(cache.Chunks),
		"activeChunks":  activeChunks,
		"expiredChunks": expiredChunks,
		"dependencies":  len(cache.Deps),
		"currentTime":   now,
	}
}

// PurgeExpiredChunks removes expired HTML chunks
func (fs *FragmentsStore) PurgeExpiredChunks(tenantID string) int {
	cache, exists := fs.GetTenantCache(tenantID)
	if !exists {
		return 0
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	expiredKeys := make([]string, 0)

	// Find expired chunks
	for chunkKey, chunk := range cache.Chunks {
		if time.Since(chunk.LastUpdated) > time.Hour {
			expiredKeys = append(expiredKeys, chunkKey)
		}
	}

	// Remove expired chunks
	for _, chunkKey := range expiredKeys {
		delete(cache.Chunks, chunkKey)
	}

	// Clean up dependency mappings
	fs.cleanupOrphanedDependencies(cache, expiredKeys)

	return len(expiredKeys)
}
