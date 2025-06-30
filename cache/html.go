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
		hco.manager.LastAccessed[tenantID] = time.Now()
		hco.manager.Mu.Unlock()

		return chunk.HTML, true
	}

	return "", false
}

// SetHTMLChunk stores HTML chunk with dependencies and variant
func (hco *HTMLCacheOperations) SetHTMLChunk(tenantID, paneID string, variant models.PaneVariant, html string, dependsOn []string) {
	hco.ensureTenantCache(tenantID)

	hco.manager.Mu.RLock()
	tenantCache := hco.manager.HTMLChunkCache[tenantID]
	hco.manager.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	key := fmt.Sprintf("%s:%s", paneID, variant)
	tenantCache.Chunks[key] = &models.HTMLChunk{
		HTML:      html,
		CachedAt:  time.Now(),
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
	hco.manager.LastAccessed[tenantID] = time.Now()
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
func (hco *HTMLCacheOperations) DetermineVariant(userState *models.FingerprintState, paneBeliefs map[string]any) models.PaneVariant {
	// If no beliefs are set on the pane, use default variant
	if len(paneBeliefs) == 0 {
		return models.PaneVariantDefault
	}

	// If user has no state, use default variant
	if userState == nil {
		return models.PaneVariantDefault
	}

	// Check if user's beliefs match pane's belief requirements
	// This is a simplified implementation - extend based on actual belief logic
	for beliefSlug, requirement := range paneBeliefs {
		if userBelief, exists := userState.HeldBeliefs[beliefSlug]; exists {
			// If user has conflicting belief, use hidden variant
			if !hco.beliefsMatch(userBelief, requirement) {
				return models.PaneVariantHidden
			}
		} else {
			// If user doesn't have required belief, use hidden variant
			return models.PaneVariantHidden
		}
	}

	return models.PaneVariantDefault
}

// beliefsMatch checks if user belief matches pane requirement
func (hco *HTMLCacheOperations) beliefsMatch(userBelief models.BeliefValue, requirement any) bool {
	// Simplified belief matching logic - extend based on actual requirements
	// This would need to be expanded based on the actual belief system
	return true // For Stage 4, always return true - implement actual logic later
}

// ensureTenantCache creates tenant HTML cache if it doesn't exist
func (hco *HTMLCacheOperations) ensureTenantCache(tenantID string) {
	hco.manager.Mu.Lock()
	defer hco.manager.Mu.Unlock()

	if _, exists := hco.manager.HTMLChunkCache[tenantID]; !exists {
		hco.manager.HTMLChunkCache[tenantID] = &models.TenantHTMLChunkCache{
			Chunks: make(map[string]*models.HTMLChunk),
			Deps:   make(map[string][]string),
		}
	}

	hco.manager.LastAccessed[tenantID] = time.Now()
}
