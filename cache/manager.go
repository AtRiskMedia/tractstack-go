// Package cache provides multi-tenant in-memory caching for content, user state, and analytics.
package cache

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache/content"
	"github.com/AtRiskMedia/tractstack-go/models"
)

var GlobalInstance *Manager

// GetGlobalManager returns the global cache manager instance
func GetGlobalManager() *Manager {
	return GlobalInstance
}

// Manager coordinates all tenant-isolated caches
type Manager struct {
	*models.CacheManager

	// Content operations
	PaneOps *content.PaneCacheOperations
}

// NewManager creates a new cache manager
func NewManager() *Manager {
	cacheManager := &models.CacheManager{
		ContentCache:   make(map[string]*models.TenantContentCache),
		UserStateCache: make(map[string]*models.TenantUserStateCache),
		HTMLChunkCache: make(map[string]*models.TenantHTMLChunkCache),
		AnalyticsCache: make(map[string]*models.TenantAnalyticsCache),
		LastAccessed:   make(map[string]time.Time),
	}

	return &Manager{
		CacheManager: cacheManager,
		PaneOps:      content.NewPaneCacheOperations(cacheManager),
	}
}

// EnsureTenant initializes cache structures for a tenant if they don't exist
func (m *Manager) EnsureTenant(tenantID string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	if _, exists := m.ContentCache[tenantID]; !exists {
		m.ContentCache[tenantID] = &models.TenantContentCache{
			TractStacks:    make(map[string]*models.TractStackNode),
			StoryFragments: make(map[string]*models.StoryFragmentNode),
			Panes:          make(map[string]*models.PaneNode),
			Menus:          make(map[string]*models.MenuNode),
			Resources:      make(map[string]*models.ResourceNode),
			Beliefs:        make(map[string]*models.BeliefNode),
			Files:          make(map[string]*models.ImageFileNode),
			SlugToID:       make(map[string]string),
			CategoryToIDs:  make(map[string][]string),
			AllPaneIDs:     make([]string, 0),
			LastUpdated:    time.Now(),
		}
	}

	if _, exists := m.UserStateCache[tenantID]; !exists {
		m.UserStateCache[tenantID] = &models.TenantUserStateCache{
			FingerprintStates: make(map[string]*models.FingerprintState),
			VisitStates:       make(map[string]*models.VisitState),
			KnownFingerprints: make(map[string]bool),
			LastLoaded:        time.Now(),
		}
	}

	if _, exists := m.HTMLChunkCache[tenantID]; !exists {
		m.HTMLChunkCache[tenantID] = &models.TenantHTMLChunkCache{
			Chunks: make(map[string]*models.HTMLChunk),
			Deps:   make(map[string][]string),
		}
	}

	if _, exists := m.AnalyticsCache[tenantID]; !exists {
		m.AnalyticsCache[tenantID] = &models.TenantAnalyticsCache{
			EpinetBins:  make(map[string]*models.HourlyEpinetBin),
			ContentBins: make(map[string]*models.HourlyContentBin),
			SiteBins:    make(map[string]*models.HourlySiteBin),
			LastUpdated: time.Now(),
		}
	}

	m.LastAccessed[tenantID] = time.Now()
}

// InvalidateTenant removes all cached data for a tenant
func (m *Manager) InvalidateTenant(tenantID string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	delete(m.ContentCache, tenantID)
	delete(m.UserStateCache, tenantID)
	delete(m.HTMLChunkCache, tenantID)
	delete(m.AnalyticsCache, tenantID)
	delete(m.LastAccessed, tenantID)
}

// GetTenantStats returns cache statistics for a tenant
func (m *Manager) GetTenantStats(tenantID string) models.CacheStats {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	stats := models.CacheStats{}

	if contentCache, exists := m.ContentCache[tenantID]; exists {
		contentCache.Mu.RLock()
		stats.Size += int64(len(contentCache.TractStacks))
		stats.Size += int64(len(contentCache.StoryFragments))
		stats.Size += int64(len(contentCache.Panes))
		stats.Size += int64(len(contentCache.Menus))
		stats.Size += int64(len(contentCache.Resources))
		stats.Size += int64(len(contentCache.Beliefs))
		stats.Size += int64(len(contentCache.Files))
		contentCache.Mu.RUnlock()
	}

	if userCache, exists := m.UserStateCache[tenantID]; exists {
		userCache.Mu.RLock()
		stats.Size += int64(len(userCache.FingerprintStates))
		stats.Size += int64(len(userCache.VisitStates))
		userCache.Mu.RUnlock()
	}

	if htmlCache, exists := m.HTMLChunkCache[tenantID]; exists {
		htmlCache.Mu.RLock()
		stats.Size += int64(len(htmlCache.Chunks))
		htmlCache.Mu.RUnlock()
	}

	if analyticsCache, exists := m.AnalyticsCache[tenantID]; exists {
		analyticsCache.Mu.RLock()
		stats.Size += int64(len(analyticsCache.EpinetBins))
		stats.Size += int64(len(analyticsCache.ContentBins))
		stats.Size += int64(len(analyticsCache.SiteBins))
		analyticsCache.Mu.RUnlock()
	}

	return stats
}

// =============================================================================
// Pane Operations
// =============================================================================

func (m *Manager) GetPane(tenantID, id string) (*models.PaneNode, bool) {
	return m.PaneOps.GetPane(tenantID, id)
}

func (m *Manager) SetPane(tenantID string, node *models.PaneNode) {
	m.PaneOps.SetPane(tenantID, node)
}

func (m *Manager) GetPaneBySlug(tenantID, slug string) (*models.PaneNode, bool) {
	return m.PaneOps.GetPaneBySlug(tenantID, slug)
}

func (m *Manager) GetAllPaneIDs(tenantID string) ([]string, bool) {
	return m.PaneOps.GetAllPaneIDs(tenantID)
}

func (m *Manager) SetAllPaneIDs(tenantID string, ids []string) {
	m.PaneOps.SetAllPaneIDs(tenantID, ids)
}

func (m *Manager) InvalidatePane(tenantID, id string) {
	m.PaneOps.InvalidatePane(tenantID, id)
}

func (m *Manager) InvalidateAllPanes(tenantID string) {
	m.PaneOps.InvalidateAllPanes(tenantID)
}

// =============================================================================
// Future Content Operations (Placeholder)
// =============================================================================

func (m *Manager) GetTractStack(tenantID, id string) (*models.TractStackNode, bool) {
	// TODO: Implement
	return nil, false
}

func (m *Manager) GetStoryFragment(tenantID, id string) (*models.StoryFragmentNode, bool) {
	// TODO: Implement
	return nil, false
}

func (m *Manager) GetMenu(tenantID, id string) (*models.MenuNode, bool) {
	// TODO: Implement
	return nil, false
}

func (m *Manager) GetResource(tenantID, id string) (*models.ResourceNode, bool) {
	// TODO: Implement
	return nil, false
}

func (m *Manager) GetBelief(tenantID, id string) (*models.BeliefNode, bool) {
	// TODO: Implement
	return nil, false
}

func (m *Manager) GetFile(tenantID, id string) (*models.ImageFileNode, bool) {
	// TODO: Implement
	return nil, false
}

// =============================================================================
// HTML Chunk Cache Operations (Future)
// =============================================================================

func (m *Manager) GetHTMLChunk(tenantID, paneID string, variant models.PaneVariant) (string, bool) {
	m.EnsureTenant(tenantID)
	cache := m.HTMLChunkCache[tenantID]
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	key := fmt.Sprintf("%s:%s", paneID, variant)
	if chunk, exists := cache.Chunks[key]; exists {
		return chunk.HTML, true
	}
	return "", false
}

func (m *Manager) SetHTMLChunk(tenantID, paneID string, variant models.PaneVariant, html string, dependsOn []string) {
	m.EnsureTenant(tenantID)
	cache := m.HTMLChunkCache[tenantID]
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	key := fmt.Sprintf("%s:%s", paneID, variant)
	cache.Chunks[key] = &models.HTMLChunk{
		HTML:      html,
		CachedAt:  time.Now(),
		DependsOn: dependsOn,
	}
	for _, depID := range dependsOn {
		cache.Deps[depID] = append(cache.Deps[depID], key)
	}
}

// =============================================================================
// User State Cache Operations (Future)
// =============================================================================

func (m *Manager) GetFingerprintState(tenantID, fingerprintID string) (*models.FingerprintState, bool) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	state, exists := cache.FingerprintStates[fingerprintID]
	return state, exists
}

func (m *Manager) SetFingerprintState(tenantID string, state *models.FingerprintState) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.FingerprintStates[state.FingerprintID] = state
}
