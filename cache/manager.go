// Package cache provides multi-tenant in-memory caching for content, user state, and analytics.
package cache

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache/content"
	"github.com/AtRiskMedia/tractstack-go/models"
)

/*
=============================================================================
CRITICAL LOCKING HIERARCHY DOCUMENTATION
=============================================================================
*/

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
	EpinetOps        *content.EpinetCacheOperations
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
		Mu:             sync.RWMutex{},
	}

	return &Manager{
		CacheManager:     cacheManager,
		PaneOps:          content.NewPaneCacheOperations(cacheManager),
		TractStackOps:    content.NewTractStackCacheOperations(cacheManager),
		StoryFragmentOps: content.NewStoryFragmentCacheOperations(cacheManager),
		MenuOps:          content.NewMenuCacheOperations(cacheManager),
		ResourceOps:      content.NewResourceCacheOperations(cacheManager),
		EpinetOps:        content.NewEpinetCacheOperations(cacheManager),
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

func (m *Manager) SetSession(tenantID string, session *models.SessionData) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	tenant := m.UserStateCache[tenantID]
	m.Mu.RUnlock()

	tenant.Mu.Lock()
	if len(tenant.SessionStates) >= MaxSessionsPerTenant {
		m.evictOldestSessions(tenantID, MaxSessionsPerTenant*8/10)
	}
	tenant.SessionStates[session.SessionID] = session
	tenant.Mu.Unlock()
}

// EnsureTenant initializes cache structures for a tenant if they don't exist.
func (m *Manager) EnsureTenant(tenantID string) {
	m.Mu.RLock()
	_, analyticsExists := m.AnalyticsCache[tenantID]
	_, contentExists := m.ContentCache[tenantID]
	_, userStateExists := m.UserStateCache[tenantID]
	_, htmlExists := m.HTMLChunkCache[tenantID]
	if analyticsExists && contentExists && userStateExists && htmlExists {
		m.Mu.RUnlock()
		return
	}
	m.Mu.RUnlock()

	m.Mu.Lock()
	defer m.Mu.Unlock()

	if m.ContentCache[tenantID] == nil {
		m.ContentCache[tenantID] = &models.TenantContentCache{
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
			AllPaneIDs:     make([]string, 0),
			LastUpdated:    time.Now().UTC(),
			Mu:             sync.RWMutex{},
			OrphanAnalysis: nil,
		}
	}

	if m.UserStateCache[tenantID] == nil {
		m.UserStateCache[tenantID] = &models.TenantUserStateCache{
			FingerprintStates:             make(map[string]*models.FingerprintState),
			VisitStates:                   make(map[string]*models.VisitState),
			KnownFingerprints:             make(map[string]bool),
			SessionStates:                 make(map[string]*models.SessionData),
			StoryfragmentBeliefRegistries: make(map[string]*models.StoryfragmentBeliefRegistry),
			SessionBeliefContexts:         make(map[string]*models.SessionBeliefContext),
			LastLoaded:                    time.Now().UTC(),
			Mu:                            sync.RWMutex{},
		}
	}

	if m.HTMLChunkCache[tenantID] == nil {
		m.HTMLChunkCache[tenantID] = &models.TenantHTMLChunkCache{
			Chunks: make(map[string]*models.HTMLChunk),
			Deps:   make(map[string][]string),
			Mu:     sync.RWMutex{},
		}
	}

	if m.AnalyticsCache[tenantID] == nil {
		m.AnalyticsCache[tenantID] = &models.TenantAnalyticsCache{
			EpinetBins:  make(map[string]*models.HourlyEpinetBin),
			ContentBins: make(map[string]*models.HourlyContentBin),
			SiteBins:    make(map[string]*models.HourlySiteBin),
			LastUpdated: time.Now().UTC(),
			Mu:          sync.RWMutex{},
		}
	}

	m.LastAccessed[tenantID] = time.Now().UTC()
}

// cleanupOldestTenantsUnsafe removes the oldest accessed tenants to make room for new ones
// INTERNAL USE ONLY: Assumes caller already holds m.Mu.Lock()
func (m *Manager) cleanupOldestTenantsUnsafe() {
	var oldestTenant string
	oldestTime := time.Now().UTC()

	for tenantID, lastAccessed := range m.LastAccessed {
		if lastAccessed.Before(oldestTime) {
			oldestTime = lastAccessed
			oldestTenant = tenantID
		}
	}

	if oldestTenant != "" {
		delete(m.ContentCache, oldestTenant)
		delete(m.UserStateCache, oldestTenant)
		delete(m.HTMLChunkCache, oldestTenant)
		delete(m.AnalyticsCache, oldestTenant)
		delete(m.LastAccessed, oldestTenant)
		fmt.Printf("Evicted oldest tenant: %s (last accessed: %v)\n", oldestTenant, oldestTime)
	}
}

// InvalidateTenant removes all cached data for a tenant
func (m *Manager) InvalidateTenant(tenantID string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	delete(m.ContentCache, tenantID)
	delete(m.UserStateCache, tenantID)
	delete(m.HTMLChunkCache, tenantID)
	// CRITICAL FIX: Do not invalidate the analytics cache during a general tenant invalidation.
	// This prevents the StateHandler from destructively interfering with a running cache warmer process.
	// delete(m.AnalyticsCache, tenantID)
	delete(m.LastAccessed, tenantID)
}

// getTenantStatsUnsafe returns cache statistics for a tenant
// INTERNAL USE ONLY: Assumes caller already holds m.Mu.RLock() or m.Mu.Lock()
func (m *Manager) getTenantStatsUnsafe(tenantID string) models.CacheStats {
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

// GetTenantStats returns cache statistics for a tenant
// PUBLIC METHOD: Acquires its own lock for external callers
func (m *Manager) GetTenantStats(tenantID string) models.CacheStats {
	m.Mu.RLock()
	defer m.Mu.RUnlock()
	return m.getTenantStatsUnsafe(tenantID)
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

func (m *Manager) GetBeliefIDBySlug(tenantID, slug string) (string, bool) {
	return m.BeliefOps.GetBeliefIDBySlug(tenantID, slug)
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
		CachedAt:  time.Now().UTC(),
		DependsOn: dependsOn,
	}
	for _, depID := range dependsOn {
		cache.Deps[depID] = append(cache.Deps[depID], key)
	}
}

// =============================================================================
// User State Cache Operations - Visit State (SINGLE ACTIVE VISIT PATTERN)
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
	if cache == nil {
		log.Printf("ERROR: UserStateCache[%s] is nil after EnsureTenant", tenantID)
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	for visitID, existingState := range cache.VisitStates {
		if existingState.FingerprintID == state.FingerprintID && visitID != state.VisitID {
			delete(cache.VisitStates, visitID)
		}
	}
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
	m.Mu.RLock()
	cache := m.UserStateCache[tenantID]
	m.Mu.RUnlock()

	if cache == nil {
		log.Printf("ERROR SetKnownFingerprint: Tenant was deleted between EnsureTenant and here")
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.KnownFingerprints[fingerprintID] = isKnown
	log.Printf("DEBUG: Completing tenant cache initialization (triggered by UserState request)")
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
	if cache == nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.FingerprintStates[state.FingerprintID] = state
}

func (m *Manager) evictOldestSessions(tenantID string, keepCount int) {
	cache := m.UserStateCache[tenantID]

	type sessionAge struct {
		id       string
		lastUsed time.Time
	}

	var sessions []sessionAge
	for id, session := range cache.SessionStates {
		sessions = append(sessions, sessionAge{id: id, lastUsed: session.LastActivity})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].lastUsed.Before(sessions[j].lastUsed)
	})

	toRemove := len(sessions) - keepCount
	for i := 0; i < toRemove && i < len(sessions); i++ {
		delete(cache.SessionStates, sessions[i].id)
	}
}

// =============================================================================
// Belief Registry Cache Operations
// =============================================================================

func (m *Manager) GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*models.StoryfragmentBeliefRegistry, bool) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	registry, exists := cache.StoryfragmentBeliefRegistries[storyfragmentID]
	return registry, exists
}

func (m *Manager) SetStoryfragmentBeliefRegistry(tenantID string, registry *models.StoryfragmentBeliefRegistry) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	if cache == nil {
		log.Printf("ERROR: UserStateCache[%s] is nil after EnsureTenant", tenantID)
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	if cache.StoryfragmentBeliefRegistries == nil {
		cache.StoryfragmentBeliefRegistries = make(map[string]*models.StoryfragmentBeliefRegistry)
	}
	cache.StoryfragmentBeliefRegistries[registry.StoryfragmentID] = registry
}

func (m *Manager) InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	delete(cache.StoryfragmentBeliefRegistries, storyfragmentID)
}

func (m *Manager) GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*models.SessionBeliefContext, bool) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	key := fmt.Sprintf("%s:%s", sessionID, storyfragmentID)
	context, exists := cache.SessionBeliefContexts[key]
	return context, exists
}

func (m *Manager) SetSessionBeliefContext(tenantID string, context *models.SessionBeliefContext) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	if cache == nil {
		log.Printf("ERROR: UserStateCache[%s] is nil after EnsureTenant", tenantID)
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	if cache.SessionBeliefContexts == nil {
		cache.SessionBeliefContexts = make(map[string]*models.SessionBeliefContext)
	}
	key := fmt.Sprintf("%s:%s", context.SessionID, context.StoryfragmentID)
	cache.SessionBeliefContexts[key] = context
}

func (m *Manager) InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string) {
	m.EnsureTenant(tenantID)
	cache := m.UserStateCache[tenantID]
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	key := fmt.Sprintf("%s:%s", sessionID, storyfragmentID)
	delete(cache.SessionBeliefContexts, key)
}

func (m *Manager) GetAllStoryfragmentBeliefRegistryIDs(tenantID string) []string {
	m.Mu.RLock()
	cache, exists := m.UserStateCache[tenantID]
	m.Mu.RUnlock()
	if !exists {
		return []string{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.StoryfragmentBeliefRegistries == nil {
		return []string{}
	}

	storyfragmentIDs := make([]string, 0, len(cache.StoryfragmentBeliefRegistries))
	for storyfragmentID := range cache.StoryfragmentBeliefRegistries {
		storyfragmentIDs = append(storyfragmentIDs, storyfragmentID)
	}

	return storyfragmentIDs
}

func (m *Manager) GetEpinet(tenantID, id string) (*models.EpinetNode, bool) {
	return m.EpinetOps.GetEpinet(tenantID, id)
}

func (m *Manager) SetEpinet(tenantID string, node *models.EpinetNode) {
	m.EpinetOps.SetEpinet(tenantID, node)
}

func (m *Manager) GetAllEpinetIDs(tenantID string) ([]string, bool) {
	return m.EpinetOps.GetAllEpinetIDs(tenantID)
}

// =============================================================================
// Content Map Operations
// =============================================================================

func (m *Manager) GetFullContentMap(tenantID string) ([]models.FullContentMapItem, bool) {
	m.Mu.RLock()
	tenantCache, exists := m.ContentCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return nil, false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	if tenantCache.FullContentMap == nil ||
		time.Since(tenantCache.ContentMapLastUpdated) > models.TTL24Hours.Duration() {
		return nil, false
	}

	m.Mu.Lock()
	m.LastAccessed[tenantID] = time.Now().UTC()
	m.Mu.Unlock()

	return tenantCache.FullContentMap, true
}

func (m *Manager) SetFullContentMap(tenantID string, contentMap []models.FullContentMapItem) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	tenantCache := m.ContentCache[tenantID]
	m.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	tenantCache.FullContentMap = contentMap
	tenantCache.ContentMapLastUpdated = time.Now().UTC()
	tenantCache.LastUpdated = time.Now().UTC()

	m.Mu.Lock()
	m.LastAccessed[tenantID] = time.Now().UTC()
	m.Mu.Unlock()
}

func (m *Manager) InvalidateFullContentMap(tenantID string) {
	m.Mu.RLock()
	tenantCache, exists := m.ContentCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	tenantCache.FullContentMap = nil
	tenantCache.ContentMapLastUpdated = time.Time{}

	tenantCache.LastUpdated = time.Now().UTC()
}

func (m *Manager) GetOrphanAnalysis(tenantID string) (*models.OrphanAnalysisPayload, string, bool) {
	m.Mu.RLock()
	tenantCache, exists := m.ContentCache[tenantID]
	m.Mu.RUnlock()

	if !exists || tenantCache.OrphanAnalysis == nil {
		return nil, "", false
	}

	tenantCache.Mu.RLock()
	defer tenantCache.Mu.RUnlock()

	if time.Since(tenantCache.OrphanAnalysis.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, "", false
	}

	m.Mu.Lock()
	m.LastAccessed[tenantID] = time.Now().UTC()
	m.Mu.Unlock()

	return tenantCache.OrphanAnalysis.Data, tenantCache.OrphanAnalysis.ETag, true
}

func (m *Manager) SetOrphanAnalysis(tenantID string, payload *models.OrphanAnalysisPayload, etag string) {
	m.EnsureTenant(tenantID)

	m.Mu.RLock()
	tenantCache := m.ContentCache[tenantID]
	m.Mu.RUnlock()

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	tenantCache.OrphanAnalysis = &models.OrphanAnalysisCache{
		Data:        payload,
		ETag:        etag,
		LastUpdated: time.Now().UTC(),
	}

	m.Mu.Lock()
	m.LastAccessed[tenantID] = time.Now().UTC()
	m.Mu.Unlock()
}

func (m *Manager) InvalidateOrphanAnalysis(tenantID string) {
	m.Mu.RLock()
	tenantCache, exists := m.ContentCache[tenantID]
	m.Mu.RUnlock()

	if !exists {
		return
	}

	tenantCache.Mu.Lock()
	defer tenantCache.Mu.Unlock()

	tenantCache.OrphanAnalysis = nil
}
