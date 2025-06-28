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
	PaneOps          *content.PaneCacheOperations
	TractStackOps    *content.TractStackCacheOperations
	StoryFragmentOps *content.StoryFragmentCacheOperations
	MenuOps          *content.MenuCacheOperations
	ResourceOps      *content.ResourceCacheOperations
	BeliefOps        *content.BeliefCacheOperations
	ImageFileOps     *content.ImageFileCacheOperations
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
		CacheManager:     cacheManager,
		PaneOps:          content.NewPaneCacheOperations(cacheManager),
		TractStackOps:    content.NewTractStackCacheOperations(cacheManager),
		StoryFragmentOps: content.NewStoryFragmentCacheOperations(cacheManager),
		MenuOps:          content.NewMenuCacheOperations(cacheManager),
		ResourceOps:      content.NewResourceCacheOperations(cacheManager),
		BeliefOps:        content.NewBeliefCacheOperations(cacheManager),
		ImageFileOps:     content.NewImageFileCacheOperations(cacheManager),
	}
}

// GetSession retrieves session data from cache
func (m *Manager) GetSession(tenantID, sessionID string) (*models.SessionData, bool) {
	m.Mu.RLock()
	tenant, exists := m.UserStateCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenant.Mu.RLock()
	defer tenant.Mu.RUnlock()

	if tenant.SessionStates == nil {
		return nil, false
	}

	session, found := tenant.SessionStates[sessionID]
	if !found || session.IsExpired() {
		return nil, false
	}

	session.UpdateActivity()
	return session, true
}

// SetSession stores session data in cache
func (m *Manager) SetSession(tenantID string, session *models.SessionData) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	tenant := m.UserStateCache[tenantID]
	m.Mu.RUnlock()

	tenant.Mu.Lock()
	defer tenant.Mu.Unlock()

	if tenant.SessionStates == nil {
		tenant.SessionStates = make(map[string]*models.SessionData)
	}

	tenant.SessionStates[session.SessionID] = session
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
			SessionStates:     make(map[string]*models.SessionData),
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
// TractStack Operations
// =============================================================================

func (m *Manager) GetTractStack(tenantID, id string) (*models.TractStackNode, bool) {
	return m.TractStackOps.GetTractStack(tenantID, id)
}

func (m *Manager) SetTractStack(tenantID string, node *models.TractStackNode) {
	m.TractStackOps.SetTractStack(tenantID, node)
}

func (m *Manager) GetTractStackBySlug(tenantID, slug string) (*models.TractStackNode, bool) {
	return m.TractStackOps.GetTractStackBySlug(tenantID, slug)
}

func (m *Manager) GetAllTractStackIDs(tenantID string) ([]string, bool) {
	return m.TractStackOps.GetAllTractStackIDs(tenantID)
}

func (m *Manager) InvalidateTractStack(tenantID, id string) {
	m.TractStackOps.InvalidateTractStack(tenantID, id)
}

func (m *Manager) InvalidateAllTractStacks(tenantID string) {
	m.TractStackOps.InvalidateAllTractStacks(tenantID)
}

// =============================================================================
// StoryFragment Operations
// =============================================================================

func (m *Manager) GetStoryFragment(tenantID, id string) (*models.StoryFragmentNode, bool) {
	return m.StoryFragmentOps.GetStoryFragment(tenantID, id)
}

func (m *Manager) SetStoryFragment(tenantID string, node *models.StoryFragmentNode) {
	m.StoryFragmentOps.SetStoryFragment(tenantID, node)
}

func (m *Manager) GetStoryFragmentBySlug(tenantID, slug string) (*models.StoryFragmentNode, bool) {
	return m.StoryFragmentOps.GetStoryFragmentBySlug(tenantID, slug)
}

func (m *Manager) GetAllStoryFragmentIDs(tenantID string) ([]string, bool) {
	return m.StoryFragmentOps.GetAllStoryFragmentIDs(tenantID)
}

func (m *Manager) InvalidateStoryFragment(tenantID, id string) {
	m.StoryFragmentOps.InvalidateStoryFragment(tenantID, id)
}

func (m *Manager) InvalidateAllStoryFragments(tenantID string) {
	m.StoryFragmentOps.InvalidateAllStoryFragments(tenantID)
}

// =============================================================================
// Menu Operations
// =============================================================================

func (m *Manager) GetMenu(tenantID, id string) (*models.MenuNode, bool) {
	return m.MenuOps.GetMenu(tenantID, id)
}

func (m *Manager) SetMenu(tenantID string, node *models.MenuNode) {
	m.MenuOps.SetMenu(tenantID, node)
}

func (m *Manager) GetAllMenuIDs(tenantID string) ([]string, bool) {
	return m.MenuOps.GetAllMenuIDs(tenantID)
}

func (m *Manager) InvalidateMenu(tenantID, id string) {
	m.MenuOps.InvalidateMenu(tenantID, id)
}

func (m *Manager) InvalidateAllMenus(tenantID string) {
	m.MenuOps.InvalidateAllMenus(tenantID)
}

// =============================================================================
// Resource Operations
// =============================================================================

func (m *Manager) GetResource(tenantID, id string) (*models.ResourceNode, bool) {
	return m.ResourceOps.GetResource(tenantID, id)
}

func (m *Manager) SetResource(tenantID string, node *models.ResourceNode) {
	m.ResourceOps.SetResource(tenantID, node)
}

func (m *Manager) GetResourceBySlug(tenantID, slug string) (*models.ResourceNode, bool) {
	return m.ResourceOps.GetResourceBySlug(tenantID, slug)
}

func (m *Manager) GetResourcesByCategory(tenantID, category string) ([]*models.ResourceNode, bool) {
	return m.ResourceOps.GetResourcesByCategory(tenantID, category)
}

func (m *Manager) GetAllResourceIDs(tenantID string) ([]string, bool) {
	return m.ResourceOps.GetAllResourceIDs(tenantID)
}

func (m *Manager) InvalidateResource(tenantID, id string) {
	m.ResourceOps.InvalidateResource(tenantID, id)
}

func (m *Manager) InvalidateAllResources(tenantID string) {
	m.ResourceOps.InvalidateAllResources(tenantID)
}

// =============================================================================
// Belief Operations
// =============================================================================

func (m *Manager) GetBelief(tenantID, id string) (*models.BeliefNode, bool) {
	return m.BeliefOps.GetBelief(tenantID, id)
}

func (m *Manager) SetBelief(tenantID string, node *models.BeliefNode) {
	m.BeliefOps.SetBelief(tenantID, node)
}

func (m *Manager) GetBeliefBySlug(tenantID, slug string) (*models.BeliefNode, bool) {
	return m.BeliefOps.GetBeliefBySlug(tenantID, slug)
}

func (m *Manager) GetAllBeliefIDs(tenantID string) ([]string, bool) {
	return m.BeliefOps.GetAllBeliefIDs(tenantID)
}

func (m *Manager) InvalidateBelief(tenantID, id string) {
	m.BeliefOps.InvalidateBelief(tenantID, id)
}

func (m *Manager) InvalidateAllBeliefs(tenantID string) {
	m.BeliefOps.InvalidateAllBeliefs(tenantID)
}

// =============================================================================
// ImageFile Operations
// =============================================================================

func (m *Manager) GetFile(tenantID, id string) (*models.ImageFileNode, bool) {
	return m.ImageFileOps.GetFile(tenantID, id)
}

func (m *Manager) SetFile(tenantID string, node *models.ImageFileNode) {
	m.ImageFileOps.SetFile(tenantID, node)
}

func (m *Manager) GetAllFileIDs(tenantID string) ([]string, bool) {
	return m.ImageFileOps.GetAllFileIDs(tenantID)
}

func (m *Manager) InvalidateFile(tenantID, id string) {
	m.ImageFileOps.InvalidateFile(tenantID, id)
}

func (m *Manager) InvalidateAllFiles(tenantID string) {
	m.ImageFileOps.InvalidateAllFiles(tenantID)
}

// =============================================================================
// HTML Chunk Cache Operations
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
// User State Cache Operations - Visit State (ADDED FOR PHASE 1)
// =============================================================================

func (m *Manager) GetVisitState(tenantID, visitID string) (*models.VisitState, bool) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	state, exists := cache.VisitStates[visitID]
	return state, exists
}

func (m *Manager) SetVisitState(tenantID string, state *models.VisitState) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.VisitStates[state.VisitID] = state
}

func (m *Manager) IsKnownFingerprint(tenantID, fingerprintID string) bool {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	isKnown, exists := cache.KnownFingerprints[fingerprintID]
	return exists && isKnown
}

func (m *Manager) SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.KnownFingerprints[fingerprintID] = isKnown
}

func (m *Manager) LoadKnownFingerprints(tenantID string, fingerprints map[string]bool) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	for fpID, isKnown := range fingerprints {
		cache.KnownFingerprints[fpID] = isKnown
	}
}

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

func init() {
	GlobalInstance = NewManager()
}
