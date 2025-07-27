// Package cache provides HTML chunk caching with belief-based variants
package cache

import (
	"fmt"
	"slices"
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// HTMLCacheOperations provides HTML chunk cache operations
type HTMLCacheOperations struct {
	manager *models.CacheManager
}

// NewHTMLCacheOperations creates a new HTML cache operations instance
func NewHTMLCacheOperations(manager *models.CacheManager) *HTMLCacheOperations {
	return &HTMLCacheOperations{manager: manager}
}

// GetHTMLChunk retrieves cached HTML for a pane with belief-based variant
func (hco *HTMLCacheOperations) GetHTMLChunk(tenantID, paneID string, variant models.PaneVariant) (string, bool) {
	hco.manager.Mu.RLock()
	tenantCache, exists := hco.manager.HTMLChunkCache[tenantID]
	hco.manager.Mu.RUnlock()

	if !exists {
		return "", false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	key := fmt.Sprintf("%s:%s", paneID, variant)
	if chunk, exists := tenantCache.Chunks[key]; exists {
		// Update last accessed
		hco.manager.Mu.Lock()
		hco.manager.LastAccessed[tenantID] = time.Now().UTC()
		hco.manager.Mu.Unlock()

		return chunk.HTML, true
	}

	return "", false
}

// SetHTMLChunk stores HTML chunk with dependencies and variant
func (hco *HTMLCacheOperations) SetHTMLChunk(tenantID, paneID string, variant models.PaneVariant, html string, dependsOn []string) {
	hco.manager.Mu.RLock()
	tenantCache := hco.manager.HTMLChunkCache[tenantID]
	hco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	key := fmt.Sprintf("%s:%s", paneID, variant)
	tenantCache.Chunks[key] = &models.HTMLChunk{
		HTML:      html,
		CachedAt:  time.Now().UTC(),
		DependsOn: dependsOn,
	}

	// Update dependency mapping for cache invalidation
	for _, depID := range dependsOn {
		if tenantCache.Deps[depID] == nil {
			tenantCache.Deps[depID] = []string{}
		}
		tenantCache.Deps[depID] = append(tenantCache.Deps[depID], key)
	}

	// Update last accessed
	hco.manager.Mu.Lock()
	hco.manager.LastAccessed[tenantID] = time.Now().UTC()
	hco.manager.Mu.Unlock()
}

// InvalidateHTMLChunk removes cached HTML chunks that depend on a specific node
func (hco *HTMLCacheOperations) InvalidateHTMLChunk(tenantID, nodeID string) {
	hco.manager.Mu.RLock()
	tenantCache, exists := hco.manager.HTMLChunkCache[tenantID]
	hco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Find all chunks that depend on this node
	if dependentKeys, exists := tenantCache.Deps[nodeID]; exists {
		for _, key := range dependentKeys {
			delete(tenantCache.Chunks, key)
		}
		delete(tenantCache.Deps, nodeID)
	}
}

// InvalidatePattern removes cached HTML chunks matching a pattern
func (hco *HTMLCacheOperations) InvalidatePattern(tenantID, pattern string) {
	hco.manager.Mu.RLock()
	tenantCache, exists := hco.manager.HTMLChunkCache[tenantID]
	hco.manager.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	// Remove chunks matching pattern (e.g., "paneID:*" for all variants)
	keysToDelete := []string{}
	for key := range tenantCache.Chunks {
		// Simple pattern matching - extend as needed
		if pattern == "*" || key == pattern {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(tenantCache.Chunks, key)
	}

	// Clean up dependency mappings
	for depID, keys := range tenantCache.Deps {
		filteredKeys := slices.DeleteFunc(keys, func(key string) bool {
			return slices.Contains(keysToDelete, key)
		})

		if len(filteredKeys) == 0 {
			delete(tenantCache.Deps, depID)
		} else {
			tenantCache.Deps[depID] = filteredKeys
		}
	}
}

// GetCacheStats returns HTML cache statistics for a tenant
func (hco *HTMLCacheOperations) GetCacheStats(tenantID string) map[string]any {
	hco.manager.Mu.RLock()
	tenantCache, exists := hco.manager.HTMLChunkCache[tenantID]
	hco.manager.Mu.RUnlock()

	if !exists {
		return map[string]any{
			"chunks":       0,
			"dependencies": 0,
		}
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	return map[string]any{
		"chunks":       len(tenantCache.Chunks),
		"dependencies": len(tenantCache.Deps),
	}
}

// DetermineVariant determines the appropriate cache variant based on user state and pane beliefs
func (hco *HTMLCacheOperations) DetermineVariant(userState *models.FingerprintState, nodeID string) models.PaneVariant {
	// Get separated belief data from renderer
	heldBeliefs, withheldBeliefs := hco.getPaneBeliefs(nodeID)

	// If no beliefs are set on the pane, use default variant
	if len(heldBeliefs) == 0 && len(withheldBeliefs) == 0 {
		return models.PaneVariantDefault
	}

	// If user has no state, use default variant
	if userState == nil {
		return models.PaneVariantDefault
	}

	// Check held beliefs - user must have all required held beliefs
	for beliefSlug, requirement := range heldBeliefs {
		if userBelief, exists := userState.HeldBeliefs[beliefSlug]; exists {
			if !hco.beliefsMatch(userBelief, requirement) {
				return models.PaneVariantHidden
			}
		} else {
			// User doesn't have required held belief
			return models.PaneVariantHidden
		}
	}

	// Check withheld beliefs - user must NOT have any of the withheld beliefs
	for beliefSlug, requirement := range withheldBeliefs {
		if userBelief, exists := userState.HeldBeliefs[beliefSlug]; exists {
			if hco.beliefsMatch(userBelief, requirement) {
				// User has a belief they should not have
				return models.PaneVariantHidden
			}
		}
	}

	return models.PaneVariantDefault
}

// beliefsMatch checks if user belief intersects with pane requirement
func (hco *HTMLCacheOperations) beliefsMatch(userBelief []string, requirement []string) bool {
	// Check if any user belief values match any required values
	for _, userVal := range userBelief {
		for _, reqVal := range requirement {
			if userVal == reqVal {
				return true
			}
		}
	}
	return false
}

// getPaneBeliefs gets separated belief data from renderer context
func (hco *HTMLCacheOperations) getPaneBeliefs(nodeID string) (map[string][]string, map[string][]string) {
	return make(map[string][]string), make(map[string][]string)
}
