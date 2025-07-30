// Package manager provides centralized cache operations with proper tenant isolation
package manager

import (
	"fmt"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/stores"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// Interface assertions to ensure Manager implements all required interfaces.
// This is a compile-time check that validates the contract between the manager and services.
var (
	_ interfaces.Cache          = (*Manager)(nil)
	_ interfaces.ContentCache   = (*Manager)(nil)
	_ interfaces.UserStateCache = (*Manager)(nil)
	_ interfaces.HTMLChunkCache = (*Manager)(nil)
	_ interfaces.AnalyticsCache = (*Manager)(nil)
)

// Manager provides centralized cache operations with proper tenant isolation
type Manager struct {
	// Tenant-isolated caches
	ContentCache   map[string]*types.TenantContentCache
	UserStateCache map[string]*types.TenantUserStateCache
	HTMLChunkCache map[string]*types.TenantHTMLChunkCache
	AnalyticsCache map[string]*types.TenantAnalyticsCache

	// Cache metadata
	Mu           sync.RWMutex
	LastAccessed map[string]time.Time

	// Store instances
	contentStore   *stores.ContentStore
	analyticsStore *stores.AnalyticsStore
	configStore    *stores.ConfigStore
	sessionsStore  *stores.SessionsStore
	fragmentsStore *stores.FragmentsStore
}

// NewManager creates a new cache manager instance
func NewManager() *Manager {
	return &Manager{
		ContentCache:   make(map[string]*types.TenantContentCache),
		UserStateCache: make(map[string]*types.TenantUserStateCache),
		HTMLChunkCache: make(map[string]*types.TenantHTMLChunkCache),
		AnalyticsCache: make(map[string]*types.TenantAnalyticsCache),
		LastAccessed:   make(map[string]time.Time),

		contentStore:   stores.NewContentStore(),
		analyticsStore: stores.NewAnalyticsStore(),
		configStore:    stores.NewConfigStore(),
		sessionsStore:  stores.NewSessionsStore(),
		fragmentsStore: stores.NewFragmentsStore(),
	}
}

// =============================================================================
// Safe Cache Lookup Methods
// =============================================================================

// GetTenantContentCache safely retrieves a tenant's content cache
func (m *Manager) GetTenantContentCache(tenantID string) (*types.TenantContentCache, error) {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	cache, exists := m.ContentCache[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}
	return cache, nil
}

// GetTenantUserStateCache safely retrieves a tenant's user state cache
func (m *Manager) GetTenantUserStateCache(tenantID string) (*types.TenantUserStateCache, error) {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	cache, exists := m.UserStateCache[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}
	return cache, nil
}

// GetTenantHTMLChunkCache safely retrieves a tenant's HTML chunk cache
func (m *Manager) GetTenantHTMLChunkCache(tenantID string) (*types.TenantHTMLChunkCache, error) {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	cache, exists := m.HTMLChunkCache[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}
	return cache, nil
}

// GetTenantAnalyticsCache safely retrieves a tenant's analytics cache
func (m *Manager) GetTenantAnalyticsCache(tenantID string) (*types.TenantAnalyticsCache, error) {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	cache, exists := m.AnalyticsCache[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}
	return cache, nil
}

// updateTenantAccessTime updates the last accessed time for a tenant
// It must be called *after* any tenant-specific locks are released.
func (m *Manager) updateTenantAccessTime(tenantID string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.LastAccessed[tenantID] = time.Now().UTC()
}

// =============================================================================
// Tenant Cache Initialization (Startup Only)
// =============================================================================

// InitializeTenant creates all cache structures for a tenant (called during startup only)
func (m *Manager) InitializeTenant(tenantID string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	// Initialize content cache
	if m.ContentCache[tenantID] == nil {
		m.ContentCache[tenantID] = &types.TenantContentCache{
			TractStacks:                   make(map[string]*content.TractStackNode),
			StoryFragments:                make(map[string]*content.StoryFragmentNode),
			Panes:                         make(map[string]*content.PaneNode),
			Menus:                         make(map[string]*content.MenuNode),
			Resources:                     make(map[string]*content.ResourceNode),
			Epinets:                       make(map[string]*content.EpinetNode),
			Beliefs:                       make(map[string]*content.BeliefNode),
			Files:                         make(map[string]*content.ImageFileNode),
			StoryfragmentBeliefRegistries: make(map[string]*types.StoryfragmentBeliefRegistry),
			SlugToID:                      make(map[string]string),
			CategoryToIDs:                 make(map[string][]string),
			AllPaneIDs:                    make([]string, 0),
			FullContentMap:                make([]types.FullContentMapItem, 0),
			ContentMapLastUpdated:         time.Now().UTC(),
			LastUpdated:                   time.Now().UTC(),
			Mu:                            sync.RWMutex{},
			OrphanAnalysis:                nil,
		}
	}

	// Initialize user state cache
	if m.UserStateCache[tenantID] == nil {
		m.UserStateCache[tenantID] = &types.TenantUserStateCache{
			FingerprintStates:     make(map[string]*types.FingerprintState),
			VisitStates:           make(map[string]*types.VisitState),
			KnownFingerprints:     make(map[string]bool),
			SessionStates:         make(map[string]*types.SessionData),
			SessionBeliefContexts: make(map[string]*types.SessionBeliefContext),
			LastLoaded:            time.Now().UTC(),
			Mu:                    sync.RWMutex{},
		}
	}

	// Initialize HTML chunk cache
	if m.HTMLChunkCache[tenantID] == nil {
		m.HTMLChunkCache[tenantID] = &types.TenantHTMLChunkCache{
			Chunks: make(map[string]*types.HTMLChunk),
			Deps:   make(map[string][]string),
			Mu:     sync.RWMutex{},
		}
	}

	// Initialize analytics cache
	if m.AnalyticsCache[tenantID] == nil {
		m.AnalyticsCache[tenantID] = &types.TenantAnalyticsCache{
			EpinetBins:    make(map[string]*types.HourlyEpinetBin),
			ContentBins:   make(map[string]*types.HourlyContentBin),
			SiteBins:      make(map[string]*types.HourlySiteBin),
			LeadMetrics:   nil,
			DashboardData: nil,
			LastFullHour:  "",
			LastUpdated:   time.Now().UTC(),
			Mu:            sync.RWMutex{},
		}
	}

	// Initialize stores for tenant
	m.contentStore.InitializeTenant(tenantID)
	m.analyticsStore.InitializeTenant(tenantID)
	m.configStore.InitializeTenant(tenantID)
	m.sessionsStore.InitializeTenant(tenantID)
	m.fragmentsStore.InitializeTenant(tenantID)

	m.LastAccessed[tenantID] = time.Now().UTC()
}

// =============================================================================
// ContentCache Interface Implementation
// =============================================================================

func (m *Manager) GetTractStack(tenantID, id string) (*content.TractStackNode, bool) {
	return m.contentStore.GetTractStack(tenantID, id)
}

func (m *Manager) SetTractStack(tenantID string, node *content.TractStackNode) {
	m.contentStore.SetTractStack(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllTractStackIDs(tenantID string) ([]string, bool) {
	// This method was missing from the store, so we implement it on the manager directly for now.
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	ids := make([]string, 0, len(cache.TractStacks))
	for id := range cache.TractStacks {
		ids = append(ids, id)
	}
	return ids, true
}

func (m *Manager) GetStoryFragment(tenantID, id string) (*content.StoryFragmentNode, bool) {
	return m.contentStore.GetStoryFragment(tenantID, id)
}

func (m *Manager) SetStoryFragment(tenantID string, node *content.StoryFragmentNode) {
	m.contentStore.SetStoryFragment(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllStoryFragmentIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	ids := make([]string, 0, len(cache.StoryFragments))
	for id := range cache.StoryFragments {
		ids = append(ids, id)
	}
	return ids, true
}

func (m *Manager) GetPane(tenantID, id string) (*content.PaneNode, bool) {
	return m.contentStore.GetPane(tenantID, id)
}

func (m *Manager) SetPane(tenantID string, node *content.PaneNode) {
	m.contentStore.SetPane(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllPaneIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	ids := make([]string, 0, len(cache.Panes))
	for id := range cache.Panes {
		ids = append(ids, id)
	}
	return ids, true
}

func (m *Manager) GetMenu(tenantID, id string) (*content.MenuNode, bool) {
	return m.contentStore.GetMenu(tenantID, id)
}

func (m *Manager) SetMenu(tenantID string, node *content.MenuNode) {
	m.contentStore.SetMenu(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllMenuIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	ids := make([]string, 0, len(cache.Menus))
	for id := range cache.Menus {
		ids = append(ids, id)
	}
	return ids, true
}

func (m *Manager) GetResource(tenantID, id string) (*content.ResourceNode, bool) {
	return m.contentStore.GetResource(tenantID, id)
}

func (m *Manager) SetResource(tenantID string, node *content.ResourceNode) {
	m.contentStore.SetResource(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllResourceIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	ids := make([]string, 0, len(cache.Resources))
	for id := range cache.Resources {
		ids = append(ids, id)
	}
	return ids, true
}

func (m *Manager) GetBelief(tenantID, id string) (*content.BeliefNode, bool) {
	return m.contentStore.GetBelief(tenantID, id)
}

func (m *Manager) SetBelief(tenantID string, node *content.BeliefNode) {
	m.contentStore.SetBelief(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllBeliefIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	ids := make([]string, 0, len(cache.Beliefs))
	for id := range cache.Beliefs {
		ids = append(ids, id)
	}
	return ids, true
}

func (m *Manager) GetEpinet(tenantID, id string) (*content.EpinetNode, bool) {
	return m.contentStore.GetEpinet(tenantID, id)
}

func (m *Manager) SetEpinet(tenantID string, node *content.EpinetNode) {
	m.contentStore.SetEpinet(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllEpinetIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	ids := make([]string, 0, len(cache.Epinets))
	for id := range cache.Epinets {
		ids = append(ids, id)
	}
	return ids, true
}

func (m *Manager) GetFile(tenantID, id string) (*content.ImageFileNode, bool) {
	return m.contentStore.GetImageFile(tenantID, id)
}

func (m *Manager) SetFile(tenantID string, node *content.ImageFileNode) {
	m.contentStore.SetImageFile(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllFileIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	ids := make([]string, 0, len(cache.Files))
	for id := range cache.Files {
		ids = append(ids, id)
	}
	return ids, true
}

func (m *Manager) GetContentBySlug(tenantID, slug string) (string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return "", false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	id, exists := cache.SlugToID[slug]
	return id, exists
}

func (m *Manager) GetResourcesByCategory(tenantID, category string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	ids, exists := cache.CategoryToIDs[category]
	return ids, exists
}

func (m *Manager) GetFullContentMap(tenantID string) ([]types.FullContentMapItem, bool) {
	return m.contentStore.GetFullContentMap(tenantID)
}

func (m *Manager) SetFullContentMap(tenantID string, contentMap []types.FullContentMapItem) {
	m.contentStore.SetFullContentMap(tenantID, contentMap)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetOrphanAnalysis(tenantID string) (*types.OrphanAnalysisPayload, string, bool) {
	return m.contentStore.GetOrphanAnalysis(tenantID)
}

func (m *Manager) SetOrphanAnalysis(tenantID string, payload *types.OrphanAnalysisPayload, etag string) {
	m.contentStore.SetOrphanAnalysis(tenantID, payload, etag)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateContentCache(tenantID string) {
	m.contentStore.InvalidateContentCache(tenantID)
	m.updateTenantAccessTime(tenantID)
}

// =============================================================================
// UserStateCache Interface Implementation
// =============================================================================

func (m *Manager) GetVisitState(tenantID, visitID string) (*types.VisitState, bool) {
	return m.sessionsStore.GetVisitState(tenantID, visitID)
}

func (m *Manager) SetVisitState(tenantID string, state *types.VisitState) {
	m.sessionsStore.SetVisitState(tenantID, state)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetFingerprintState(tenantID, fingerprintID string) (*types.FingerprintState, bool) {
	return m.sessionsStore.GetFingerprintState(tenantID, fingerprintID)
}

func (m *Manager) SetFingerprintState(tenantID string, state *types.FingerprintState) {
	m.sessionsStore.SetFingerprintState(tenantID, state)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) IsKnownFingerprint(tenantID, fingerprintID string) bool {
	return m.sessionsStore.IsKnownFingerprint(tenantID, fingerprintID)
}

func (m *Manager) SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool) {
	m.sessionsStore.SetKnownFingerprint(tenantID, fingerprintID, isKnown)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) LoadKnownFingerprints(tenantID string, fingerprints map[string]bool) {
	m.sessionsStore.LoadKnownFingerprints(tenantID, fingerprints)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetSession(tenantID, sessionID string) (*types.SessionData, bool) {
	return m.sessionsStore.GetSession(tenantID, sessionID)
}

func (m *Manager) SetSession(tenantID string, sessionData *types.SessionData) {
	m.sessionsStore.SetSession(tenantID, sessionData)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*types.StoryfragmentBeliefRegistry, bool) {
	return m.sessionsStore.GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
}

func (m *Manager) SetStoryfragmentBeliefRegistry(tenantID string, registry *types.StoryfragmentBeliefRegistry) {
	m.sessionsStore.SetStoryfragmentBeliefRegistry(tenantID, registry)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) {
	m.sessionsStore.InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*types.SessionBeliefContext, bool) {
	return m.sessionsStore.GetSessionBeliefContext(tenantID, sessionID, storyfragmentID)
}

func (m *Manager) SetSessionBeliefContext(tenantID string, context *types.SessionBeliefContext) {
	m.sessionsStore.SetSessionBeliefContext(tenantID, context)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string) {
	m.sessionsStore.InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateUserStateCache(tenantID string) {
	m.sessionsStore.InvalidateUserStateCache(tenantID)
	m.updateTenantAccessTime(tenantID)
}

// =============================================================================
// HTMLChunkCache Interface Implementation
// =============================================================================

func (m *Manager) GetHTMLChunk(tenantID, paneID string, variant types.PaneVariant) (*types.HTMLChunk, bool) {
	return m.fragmentsStore.GetHTMLChunk(tenantID, paneID, variant)
}

func (m *Manager) SetHTMLChunk(tenantID, paneID string, variant types.PaneVariant, html string, dependsOn []string) {
	m.fragmentsStore.SetHTMLChunk(tenantID, paneID, variant, html, dependsOn)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetChunkDependencies(tenantID, nodeID string) ([]string, bool) {
	cache, err := m.GetTenantHTMLChunkCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	deps, exists := cache.Deps[nodeID]
	return deps, exists
}

func (m *Manager) InvalidateByDependency(tenantID, dependencyID string) {
	m.fragmentsStore.InvalidateByDependency(tenantID, dependencyID)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateHTMLChunkCache(tenantID string) {
	m.fragmentsStore.InvalidateHTMLChunkCache(tenantID)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateHTMLChunk(tenantID, paneID string, variant types.PaneVariant) {
	m.fragmentsStore.InvalidateByPattern(tenantID, m.fragmentsStore.BuildChunkKey(paneID, variant))
	m.updateTenantAccessTime(tenantID)
}

// =============================================================================
// AnalyticsCache Interface Implementation
// =============================================================================

func (m *Manager) GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*types.HourlyEpinetBin, bool) {
	return m.analyticsStore.GetHourlyEpinetBin(tenantID, epinetID, hourKey)
}

func (m *Manager) SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *types.HourlyEpinetBin) {
	m.analyticsStore.SetHourlyEpinetBin(tenantID, epinetID, hourKey, bin)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetHourlyContentBin(tenantID, contentID, hourKey string) (*types.HourlyContentBin, bool) {
	return m.analyticsStore.GetHourlyContentBin(tenantID, contentID, hourKey)
}

func (m *Manager) SetHourlyContentBin(tenantID, contentID, hourKey string, bin *types.HourlyContentBin) {
	m.analyticsStore.SetHourlyContentBin(tenantID, contentID, hourKey, bin)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetHourlySiteBin(tenantID, hourKey string) (*types.HourlySiteBin, bool) {
	return m.analyticsStore.GetHourlySiteBin(tenantID, hourKey)
}

func (m *Manager) SetHourlySiteBin(tenantID, hourKey string, bin *types.HourlySiteBin) {
	m.analyticsStore.SetHourlySiteBin(tenantID, hourKey, bin)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetLeadMetrics(tenantID string) (*types.LeadMetricsCache, bool) {
	return m.analyticsStore.GetLeadMetrics(tenantID)
}

func (m *Manager) SetLeadMetrics(tenantID string, metrics *types.LeadMetricsCache) {
	m.analyticsStore.SetLeadMetrics(tenantID, metrics)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetDashboardData(tenantID string) (*types.DashboardCache, bool) {
	return m.analyticsStore.GetDashboardData(tenantID)
}

func (m *Manager) SetDashboardData(tenantID string, data *types.DashboardCache) {
	m.analyticsStore.SetDashboardData(tenantID, data)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*types.HourlyEpinetBin, []string) {
	return m.analyticsStore.GetHourlyEpinetRange(tenantID, epinetID, hourKeys)
}

func (m *Manager) PurgeExpiredBins(tenantID string, olderThan string) {
	m.analyticsStore.PurgeExpiredBins(tenantID, olderThan)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateAnalyticsCache(tenantID string) {
	m.analyticsStore.InvalidateAnalyticsCache(tenantID)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) UpdateLastFullHour(tenantID, hourKey string) {
	m.analyticsStore.UpdateLastFullHour(tenantID, hourKey)
	m.updateTenantAccessTime(tenantID)
}

// =============================================================================
// Cache Interface Implementation
// =============================================================================

func (m *Manager) InvalidateTenant(tenantID string) {
	m.contentStore.InvalidateContentCache(tenantID)
	m.sessionsStore.InvalidateUserStateCache(tenantID)
	m.fragmentsStore.InvalidateHTMLChunkCache(tenantID)
	m.analyticsStore.InvalidateAnalyticsCache(tenantID)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetTenantStats(tenantID string) interfaces.CacheStats {
	// Dummy implementation for now
	return interfaces.CacheStats{}
}

func (m *Manager) GetMemoryStats() map[string]any {
	// Dummy implementation for now
	return make(map[string]any)
}

func (m *Manager) InvalidateAll() {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	for tenantID := range m.ContentCache {
		m.InvalidateTenant(tenantID)
	}
}

func (m *Manager) Health() map[string]any {
	// Dummy implementation for now
	return map[string]any{"status": "ok"}
}
