// Package cache provides comprehensive multi-tenant cache management
package cache

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/models"
)

var globalManager *Manager

// Manager provides centralized cache operations with proper tenant isolation
type Manager struct {
	// Tenant-isolated caches
	ContentCache   map[string]*models.TenantContentCache
	UserStateCache map[string]*models.TenantUserStateCache
	HTMLChunkCache map[string]*models.TenantHTMLChunkCache
	AnalyticsCache map[string]*models.TenantAnalyticsCache

	// Cache metadata
	Mu           sync.RWMutex
	LastAccessed map[string]time.Time
}

// NewManager creates a new cache manager instance
func NewManager() *Manager {
	return &Manager{
		ContentCache:   make(map[string]*models.TenantContentCache),
		UserStateCache: make(map[string]*models.TenantUserStateCache),
		HTMLChunkCache: make(map[string]*models.TenantHTMLChunkCache),
		AnalyticsCache: make(map[string]*models.TenantAnalyticsCache),
		LastAccessed:   make(map[string]time.Time),
	}
}

// =============================================================================
// Global Manager Functions
// =============================================================================

// SetGlobalManager sets the global cache manager instance
func SetGlobalManager(manager *Manager) {
	globalManager = manager
}

// GetGlobalManager returns the global cache manager instance
func GetGlobalManager() *Manager {
	return globalManager
}

// =============================================================================
// Safe Cache Lookup Methods
// =============================================================================

// GetTenantContentCache safely retrieves a tenant's content cache
func (m *Manager) GetTenantContentCache(tenantID string) (*models.TenantContentCache, error) {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	cache, exists := m.ContentCache[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}
	return cache, nil
}

// GetTenantUserStateCache safely retrieves a tenant's user state cache
func (m *Manager) GetTenantUserStateCache(tenantID string) (*models.TenantUserStateCache, error) {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	cache, exists := m.UserStateCache[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}
	return cache, nil
}

// GetTenantAnalyticsCache safely retrieves a tenant's analytics cache
func (m *Manager) GetTenantAnalyticsCache(tenantID string) (*models.TenantAnalyticsCache, error) {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	cache, exists := m.AnalyticsCache[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}
	return cache, nil
}

// GetTenantHTMLChunkCache safely retrieves a tenant's HTML chunk cache
func (m *Manager) GetTenantHTMLChunkCache(tenantID string) (*models.TenantHTMLChunkCache, error) {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	cache, exists := m.HTMLChunkCache[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not initialized - server startup issue", tenantID)
	}
	return cache, nil
}

// updateTenantAccessTime centralizes the safe update of LastAccessed to prevent deadlocks.
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
		m.ContentCache[tenantID] = &models.TenantContentCache{
			TractStacks:           make(map[string]*models.TractStackNode),
			StoryFragments:        make(map[string]*models.StoryFragmentNode),
			Panes:                 make(map[string]*models.PaneNode),
			Menus:                 make(map[string]*models.MenuNode),
			Resources:             make(map[string]*models.ResourceNode),
			Epinets:               make(map[string]*models.EpinetNode),
			Beliefs:               make(map[string]*models.BeliefNode),
			Files:                 make(map[string]*models.ImageFileNode),
			SlugToID:              make(map[string]string),
			CategoryToIDs:         make(map[string][]string),
			AllPaneIDs:            make([]string, 0),
			FullContentMap:        make([]models.FullContentMapItem, 0),
			ContentMapLastUpdated: time.Now().UTC(),
			LastUpdated:           time.Now().UTC(),
			Mu:                    sync.RWMutex{},
			OrphanAnalysis:        nil,
		}
	}

	// Initialize user state cache
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

	// Initialize HTML chunk cache
	if m.HTMLChunkCache[tenantID] == nil {
		m.HTMLChunkCache[tenantID] = &models.TenantHTMLChunkCache{
			Chunks: make(map[string]*models.HTMLChunk),
			Deps:   make(map[string][]string),
			Mu:     sync.RWMutex{},
		}
	}

	// Initialize analytics cache
	if m.AnalyticsCache[tenantID] == nil {
		m.AnalyticsCache[tenantID] = &models.TenantAnalyticsCache{
			EpinetBins:   make(map[string]*models.HourlyEpinetBin),
			ContentBins:  make(map[string]*models.HourlyContentBin),
			SiteBins:     make(map[string]*models.HourlySiteBin),
			LastUpdated:  time.Now().UTC(),
			Mu:           sync.RWMutex{},
			LastFullHour: "",
		}
	}

	// Update last accessed
	m.LastAccessed[tenantID] = time.Now().UTC()
}

// =============================================================================
// User State Cache Operations
// =============================================================================

func (m *Manager) GetVisitState(tenantID, visitID string) (*models.VisitState, bool) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	state, exists := cache.VisitStates[visitID]
	return state, exists
}

func (m *Manager) SetVisitState(tenantID string, state *models.VisitState) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.VisitStates[state.VisitID] = state
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) IsKnownFingerprint(tenantID, fingerprintID string) bool {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	return cache.KnownFingerprints[fingerprintID]
}

func (m *Manager) SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.KnownFingerprints[fingerprintID] = isKnown
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) LoadKnownFingerprints(tenantID string, fingerprints map[string]bool) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	for fpID, isKnown := range fingerprints {
		cache.KnownFingerprints[fpID] = isKnown
	}
	cache.LastLoaded = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetFingerprintState(tenantID, fingerprintID string) (*models.FingerprintState, bool) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	state, exists := cache.FingerprintStates[fingerprintID]
	return state, exists
}

func (m *Manager) SetFingerprintState(tenantID string, state *models.FingerprintState) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.FingerprintStates[state.FingerprintID] = state
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*models.StoryfragmentBeliefRegistry, bool) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	registry, exists := cache.StoryfragmentBeliefRegistries[storyfragmentID]
	return registry, exists
}

func (m *Manager) SetStoryfragmentBeliefRegistry(tenantID string, registry *models.StoryfragmentBeliefRegistry) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	if cache.StoryfragmentBeliefRegistries == nil {
		cache.StoryfragmentBeliefRegistries = make(map[string]*models.StoryfragmentBeliefRegistry)
	}
	cache.StoryfragmentBeliefRegistries[registry.StoryfragmentID] = registry
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	delete(cache.StoryfragmentBeliefRegistries, storyfragmentID)
}

func (m *Manager) GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*models.SessionBeliefContext, bool) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	key := fmt.Sprintf("%s:%s", sessionID, storyfragmentID)
	context, exists := cache.SessionBeliefContexts[key]
	return context, exists
}

func (m *Manager) SetSessionBeliefContext(tenantID string, context *models.SessionBeliefContext) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	if cache.SessionBeliefContexts == nil {
		cache.SessionBeliefContexts = make(map[string]*models.SessionBeliefContext)
	}
	key := fmt.Sprintf("%s:%s", context.SessionID, context.StoryfragmentID)
	cache.SessionBeliefContexts[key] = context
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	key := fmt.Sprintf("%s:%s", sessionID, storyfragmentID)
	delete(cache.SessionBeliefContexts, key)
}

func (m *Manager) SetSession(tenantID string, sessionData *models.SessionData) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	// Check if the session already exists. If so, we're just updating it.
	if _, exists := cache.SessionStates[sessionData.SessionID]; exists {
		cache.SessionStates[sessionData.SessionID] = sessionData
		m.updateTenantAccessTime(tenantID)
		return
	}

	// This is a new session. Check if we are at capacity.
	if len(cache.SessionStates) >= config.MaxSessionsPerTenant {
		// Instead of immediately rejecting, first try to evict expired sessions to make room.
		now := time.Now().UTC()
		evictedCount := 0
		for sessionID, session := range cache.SessionStates {
			if session.IsExpired() || now.Sub(session.LastActivity) > config.UserStateTTL {
				delete(cache.SessionStates, sessionID)
				evictedCount++
			}
		}

		if evictedCount > 0 {
			log.Printf("INFO: Session cache for tenant %s was full. Evicted %d expired sessions to make space.", tenantID, evictedCount)
		}

		// After eviction, check the limit again.
		if len(cache.SessionStates) >= config.MaxSessionsPerTenant {
			log.Printf("WARN: Session cache for tenant %s is still full after cleanup (limit: %d). Rejecting new session.", tenantID, config.MaxSessionsPerTenant)
			return // Reject the new session
		}
	}

	// Add the new session.
	cache.SessionStates[sessionData.SessionID] = sessionData
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetSession(tenantID, sessionID string) (*models.SessionData, bool) {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	session, exists := cache.SessionStates[sessionID]
	return session, exists
}

func (m *Manager) GetAllStoryfragmentBeliefRegistryIDs(tenantID string) []string {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
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

// =============================================================================
// HTML Chunk Cache Operations
// =============================================================================

func (m *Manager) GetHTMLChunk(tenantID, paneID string, variant models.PaneVariant) (string, bool) {
	cache, err := m.GetTenantHTMLChunkCache(tenantID)
	if err != nil {
		return "", false
	}

	cache.Mu.RLock()
	key := fmt.Sprintf("%s:%s", paneID, variant)
	chunk, exists := cache.Chunks[key]
	cache.Mu.RUnlock()

	if exists {
		m.updateTenantAccessTime(tenantID)
		return chunk.HTML, true
	}

	return "", false
}

func (m *Manager) SetHTMLChunk(tenantID, paneID string, variant models.PaneVariant, html string, dependsOn []string) {
	cache, err := m.GetTenantHTMLChunkCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	key := fmt.Sprintf("%s:%s", paneID, variant)
	cache.Chunks[key] = &models.HTMLChunk{
		HTML:      html,
		CachedAt:  time.Now().UTC(),
		DependsOn: dependsOn,
	}
	for _, depID := range dependsOn {
		cache.Deps[depID] = append(cache.Deps[depID], key)
	}
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateHTMLChunk(tenantID, nodeID string) {
	cache, err := m.GetTenantHTMLChunkCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	if dependentKeys, exists := cache.Deps[nodeID]; exists {
		for _, key := range dependentKeys {
			delete(cache.Chunks, key)
		}
		delete(cache.Deps, nodeID)
	}
}

func (m *Manager) InvalidatePattern(tenantID, pattern string) {
	cache, err := m.GetTenantHTMLChunkCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	keysToDelete := []string{}
	for key := range cache.Chunks {
		// Simple pattern matching - extend as needed
		if pattern == "*" || key == pattern {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(cache.Chunks, key)
	}

	// Clean up dependency mappings
	for depID, keys := range cache.Deps {
		filteredKeys := []string{}
		for _, key := range keys {
			found := false
			for _, deletedKey := range keysToDelete {
				if key == deletedKey {
					found = true
					break
				}
			}
			if !found {
				filteredKeys = append(filteredKeys, key)
			}
		}

		if len(filteredKeys) == 0 {
			delete(cache.Deps, depID)
		} else {
			cache.Deps[depID] = filteredKeys
		}
	}
}

// =============================================================================
// Analytics Cache Operations
// =============================================================================

func (m *Manager) GetAnalyticsSummary(tenantID string) map[string]interface{} {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return map[string]interface{}{
			"exists": false,
			"error":  err.Error(),
		}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	return map[string]interface{}{
		"exists":         true,
		"episetBins":     len(cache.EpinetBins),
		"contentBins":    len(cache.ContentBins),
		"siteBins":       len(cache.SiteBins),
		"hasLeadMetrics": cache.LeadMetrics != nil,
		"hasDashboard":   cache.DashboardData != nil,
		"lastFullHour":   cache.LastFullHour,
		"lastUpdated":    cache.LastUpdated,
	}
}

func (m *Manager) InvalidateAnalyticsCache(tenantID string) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.LeadMetrics = nil
	cache.DashboardData = nil
	cache.LastFullHour = ""
	cache.LastUpdated = time.Now()
}

func (m *Manager) UpdateLastFullHour(tenantID, hourKey string) {
	cache, err := m.GetTenantAnalyticsCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.LastFullHour = hourKey
	cache.LastUpdated = time.Now()
}

// =============================================================================
// Content Map Operations
// =============================================================================

func (m *Manager) GetFullContentMap(tenantID string) ([]models.FullContentMapItem, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if len(cache.FullContentMap) == 0 {
		return nil, false
	}

	return cache.FullContentMap, true
}

func (m *Manager) SetFullContentMap(tenantID string, contentMap []models.FullContentMapItem) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.FullContentMap = contentMap
	cache.ContentMapLastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateFullContentMap(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.FullContentMap = nil
	cache.ContentMapLastUpdated = time.Time{}
	cache.LastUpdated = time.Now().UTC()
}

func (m *Manager) SetOrphanAnalysis(tenantID string, payload *models.OrphanAnalysisPayload, etag string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.OrphanAnalysis = &models.OrphanAnalysisCache{
		Data:        payload,
		ETag:        etag,
		LastUpdated: time.Now().UTC(),
	}
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetOrphanAnalysis(tenantID string) (*models.OrphanAnalysisPayload, string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, "", false
	}

	cache.Mu.RLock()
	analysisCache := cache.OrphanAnalysis
	cache.Mu.RUnlock()

	if analysisCache == nil {
		return nil, "", false
	}

	if time.Since(analysisCache.LastUpdated) > models.TTL24Hours.Duration() {
		return nil, "", false
	}

	m.updateTenantAccessTime(tenantID)

	return analysisCache.Data, analysisCache.ETag, true
}

func (m *Manager) InvalidateOrphanAnalysis(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.OrphanAnalysis = nil
}

// =============================================================================
// Content Operations - Belief
// =============================================================================

func (m *Manager) GetBelief(tenantID, id string) (*models.BeliefNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	belief, exists := cache.Beliefs[id]
	cache.Mu.RUnlock()

	if isExpired || !exists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return belief, true
}

func (m *Manager) SetBelief(tenantID string, node *models.BeliefNode) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.Beliefs[node.ID] = node
	cache.SlugToID["belief:"+node.Slug] = node.ID
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetBeliefBySlug(tenantID, slug string) (*models.BeliefNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	id, idExists := cache.SlugToID["belief:"+slug]
	var belief *models.BeliefNode
	var beliefExists bool
	if idExists {
		belief, beliefExists = cache.Beliefs[id]
	}
	cache.Mu.RUnlock()

	if isExpired || !beliefExists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return belief, true
}

func (m *Manager) GetBeliefIDBySlug(tenantID, slug string) (string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return "", false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	id, exists := cache.SlugToID["belief:"+slug]
	if exists {
		_, exists = cache.Beliefs[id]
	}
	cache.Mu.RUnlock()

	if isExpired || !exists {
		return "", false
	}

	m.updateTenantAccessTime(tenantID)
	return id, true
}

func (m *Manager) GetAllBeliefIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	ids := make([]string, 0, len(cache.Beliefs))
	for id := range cache.Beliefs {
		ids = append(ids, id)
	}
	cache.Mu.RUnlock()

	if isExpired {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return ids, true
}

func (m *Manager) InvalidateBelief(tenantID, id string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	if belief, exists := cache.Beliefs[id]; exists {
		delete(cache.SlugToID, "belief:"+belief.Slug)
	}
	delete(cache.Beliefs, id)
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateAllBeliefs(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	for _, belief := range cache.Beliefs {
		delete(cache.SlugToID, "belief:"+belief.Slug)
	}

	cache.Beliefs = make(map[string]*models.BeliefNode)
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Content Operations - Epinet
// =============================================================================

func (m *Manager) GetEpinet(tenantID, id string) (*models.EpinetNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	epinet, exists := cache.Epinets[id]
	cache.Mu.RUnlock()

	if isExpired || !exists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return epinet, true
}

func (m *Manager) SetEpinet(tenantID string, node *models.EpinetNode) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.Epinets[node.ID] = node
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllEpinetIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	ids := make([]string, 0, len(cache.Epinets))
	for id := range cache.Epinets {
		ids = append(ids, id)
	}
	cache.Mu.RUnlock()

	if isExpired {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return ids, true
}

func (m *Manager) InvalidateEpinet(tenantID, id string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	delete(cache.Epinets, id)
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateAllEpinets(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Epinets = make(map[string]*models.EpinetNode)
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Content Operations - Pane
// =============================================================================

func (m *Manager) GetPane(tenantID, id string) (*models.PaneNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	pane, exists := cache.Panes[id]
	cache.Mu.RUnlock()

	if isExpired || !exists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return pane, true
}

func (m *Manager) SetPane(tenantID string, node *models.PaneNode) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.Panes[node.ID] = node
	cache.SlugToID[node.Slug] = node.ID
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetPaneBySlug(tenantID, slug string) (*models.PaneNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	id, idExists := cache.SlugToID[slug]
	var pane *models.PaneNode
	var paneExists bool
	if idExists {
		pane, paneExists = cache.Panes[id]
	}
	cache.Mu.RUnlock()

	if isExpired || !paneExists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return pane, true
}

func (m *Manager) GetAllPaneIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	if len(cache.AllPaneIDs) == 0 {
		cache.Mu.RUnlock()
		return nil, false
	}
	ids := make([]string, len(cache.AllPaneIDs))
	copy(ids, cache.AllPaneIDs)
	cache.Mu.RUnlock()

	if isExpired {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return ids, true
}

func (m *Manager) SetAllPaneIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.AllPaneIDs = make([]string, len(ids))
	copy(cache.AllPaneIDs, ids)
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidatePane(tenantID, id string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	if pane, exists := cache.Panes[id]; exists {
		delete(cache.SlugToID, pane.Slug)
	}
	delete(cache.Panes, id)
	for i, paneID := range cache.AllPaneIDs {
		if paneID == id {
			cache.AllPaneIDs = append(cache.AllPaneIDs[:i], cache.AllPaneIDs[i+1:]...)
			break
		}
	}
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateAllPanes(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	for _, pane := range cache.Panes {
		delete(cache.SlugToID, pane.Slug)
	}

	cache.Panes = make(map[string]*models.PaneNode)
	cache.AllPaneIDs = []string{}
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Content Operations - StoryFragment
// =============================================================================

func (m *Manager) GetStoryFragment(tenantID, id string) (*models.StoryFragmentNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	storyFragment, exists := cache.StoryFragments[id]
	cache.Mu.RUnlock()

	if isExpired || !exists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return storyFragment, true
}

func (m *Manager) SetStoryFragment(tenantID string, node *models.StoryFragmentNode) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.StoryFragments[node.ID] = node
	cache.SlugToID["storyfragment:"+node.Slug] = node.ID
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetStoryFragmentBySlug(tenantID, slug string) (*models.StoryFragmentNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	id, idExists := cache.SlugToID["storyfragment:"+slug]
	var storyFragment *models.StoryFragmentNode
	var storyFragmentExists bool
	if idExists {
		storyFragment, storyFragmentExists = cache.StoryFragments[id]
	}
	cache.Mu.RUnlock()

	if isExpired || !storyFragmentExists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return storyFragment, true
}

func (m *Manager) GetAllStoryFragmentIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	ids := make([]string, 0, len(cache.StoryFragments))
	for id := range cache.StoryFragments {
		ids = append(ids, id)
	}
	cache.Mu.RUnlock()

	if isExpired {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return ids, true
}

func (m *Manager) InvalidateStoryFragment(tenantID, id string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	if storyFragment, exists := cache.StoryFragments[id]; exists {
		delete(cache.SlugToID, "storyfragment:"+storyFragment.Slug)
	}
	delete(cache.StoryFragments, id)
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateAllStoryFragments(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	for _, storyFragment := range cache.StoryFragments {
		delete(cache.SlugToID, "storyfragment:"+storyFragment.Slug)
	}

	cache.StoryFragments = make(map[string]*models.StoryFragmentNode)
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Content Operations - TractStack
// =============================================================================

func (m *Manager) GetTractStack(tenantID, id string) (*models.TractStackNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	tractStack, exists := cache.TractStacks[id]
	cache.Mu.RUnlock()

	if isExpired || !exists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return tractStack, true
}

func (m *Manager) SetTractStack(tenantID string, node *models.TractStackNode) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.TractStacks[node.ID] = node
	cache.SlugToID["tractstack:"+node.Slug] = node.ID
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetTractStackBySlug(tenantID, slug string) (*models.TractStackNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	id, idExists := cache.SlugToID["tractstack:"+slug]
	var tractStack *models.TractStackNode
	var tractStackExists bool
	if idExists {
		tractStack, tractStackExists = cache.TractStacks[id]
	}
	cache.Mu.RUnlock()

	if isExpired || !tractStackExists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return tractStack, true
}

func (m *Manager) GetAllTractStackIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	ids := make([]string, 0, len(cache.TractStacks))
	for id := range cache.TractStacks {
		ids = append(ids, id)
	}
	cache.Mu.RUnlock()

	if isExpired {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return ids, true
}

func (m *Manager) InvalidateTractStack(tenantID, id string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	if tractStack, exists := cache.TractStacks[id]; exists {
		delete(cache.SlugToID, "tractstack:"+tractStack.Slug)
	}
	delete(cache.TractStacks, id)
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateAllTractStacks(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	for _, tractStack := range cache.TractStacks {
		delete(cache.SlugToID, "tractstack:"+tractStack.Slug)
	}

	cache.TractStacks = make(map[string]*models.TractStackNode)
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Content Operations - Menu
// =============================================================================

func (m *Manager) GetMenu(tenantID, id string) (*models.MenuNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	menu, exists := cache.Menus[id]
	cache.Mu.RUnlock()

	if isExpired || !exists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return menu, true
}

func (m *Manager) SetMenu(tenantID string, node *models.MenuNode) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.Menus[node.ID] = node
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllMenuIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	ids := make([]string, 0, len(cache.Menus))
	for id := range cache.Menus {
		ids = append(ids, id)
	}
	cache.Mu.RUnlock()

	if isExpired {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return ids, true
}

func (m *Manager) InvalidateMenu(tenantID, id string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	delete(cache.Menus, id)
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateAllMenus(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Menus = make(map[string]*models.MenuNode)
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Content Operations - Resource
// =============================================================================

func (m *Manager) GetResource(tenantID, id string) (*models.ResourceNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	resource, exists := cache.Resources[id]
	cache.Mu.RUnlock()

	if isExpired || !exists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return resource, true
}

func (m *Manager) SetResource(tenantID string, node *models.ResourceNode) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.Resources[node.ID] = node
	cache.SlugToID["resource:"+node.Slug] = node.ID

	// Update category indexing
	if node.CategorySlug != nil {
		category := *node.CategorySlug
		if cache.CategoryToIDs[category] == nil {
			cache.CategoryToIDs[category] = []string{}
		}
		found := false
		for _, existingID := range cache.CategoryToIDs[category] {
			if existingID == node.ID {
				found = true
				break
			}
		}
		if !found {
			cache.CategoryToIDs[category] = append(cache.CategoryToIDs[category], node.ID)
		}
	}
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetResourceBySlug(tenantID, slug string) (*models.ResourceNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	id, idExists := cache.SlugToID["resource:"+slug]
	var resource *models.ResourceNode
	var resourceExists bool
	if idExists {
		resource, resourceExists = cache.Resources[id]
	}
	cache.Mu.RUnlock()

	if isExpired || !resourceExists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return resource, true
}

func (m *Manager) GetAllResourceIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	ids := make([]string, 0, len(cache.Resources))
	for id := range cache.Resources {
		ids = append(ids, id)
	}
	cache.Mu.RUnlock()

	if isExpired {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return ids, true
}

func (m *Manager) GetResourcesByCategory(tenantID, category string) ([]*models.ResourceNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	resourceIDs, exists := cache.CategoryToIDs[category]
	if !exists || len(resourceIDs) == 0 {
		cache.Mu.RUnlock()
		return []*models.ResourceNode{}, !isExpired
	}
	var resources []*models.ResourceNode
	for _, id := range resourceIDs {
		if resource, exists := cache.Resources[id]; exists {
			resources = append(resources, resource)
		}
	}
	cache.Mu.RUnlock()

	if isExpired {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return resources, true
}

func (m *Manager) InvalidateResource(tenantID, id string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	if resource, exists := cache.Resources[id]; exists {
		delete(cache.SlugToID, "resource:"+resource.Slug)

		// Remove from category lookup if categorized
		if resource.CategorySlug != nil && *resource.CategorySlug != "" {
			if categoryIDs, exists := cache.CategoryToIDs[*resource.CategorySlug]; exists {
				for i, categoryID := range categoryIDs {
					if categoryID == id {
						cache.CategoryToIDs[*resource.CategorySlug] = append(categoryIDs[:i], categoryIDs[i+1:]...)
						break
					}
				}

				if len(cache.CategoryToIDs[*resource.CategorySlug]) == 0 {
					delete(cache.CategoryToIDs, *resource.CategorySlug)
				}
			}
		}
	}
	delete(cache.Resources, id)
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateAllResources(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	for _, resource := range cache.Resources {
		delete(cache.SlugToID, "resource:"+resource.Slug)
	}

	// Clear category lookups for resources
	for category, ids := range cache.CategoryToIDs {
		var filteredIDs []string
		for _, id := range ids {
			if _, isResource := cache.Resources[id]; !isResource {
				filteredIDs = append(filteredIDs, id)
			}
		}

		if len(filteredIDs) == 0 {
			delete(cache.CategoryToIDs, category)
		} else {
			cache.CategoryToIDs[category] = filteredIDs
		}
	}

	cache.Resources = make(map[string]*models.ResourceNode)
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Content Operations - ImageFile
// =============================================================================

func (m *Manager) GetFile(tenantID, id string) (*models.ImageFileNode, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	file, exists := cache.Files[id]
	cache.Mu.RUnlock()

	if isExpired || !exists {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return file, true
}

func (m *Manager) SetFile(tenantID string, node *models.ImageFileNode) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	cache.Files[node.ID] = node
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllFileIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}

	cache.Mu.RLock()
	isExpired := time.Since(cache.LastUpdated) > models.TTL24Hours.Duration()
	ids := make([]string, 0, len(cache.Files))
	for id := range cache.Files {
		ids = append(ids, id)
	}
	cache.Mu.RUnlock()

	if isExpired {
		return nil, false
	}

	m.updateTenantAccessTime(tenantID)
	return ids, true
}

func (m *Manager) InvalidateFile(tenantID, id string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	delete(cache.Files, id)
	cache.LastUpdated = time.Now().UTC()
	cache.Mu.Unlock()

	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateAllFiles(tenantID string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Files = make(map[string]*models.ImageFileNode)
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Cache Cleanup and Maintenance
// =============================================================================

func (m *Manager) CleanupExpiredCaches() {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-24 * time.Hour)

	for tenantID, lastAccessed := range m.LastAccessed {
		if lastAccessed.Before(cutoff) {
			delete(m.ContentCache, tenantID)
			delete(m.UserStateCache, tenantID)
			delete(m.HTMLChunkCache, tenantID)
			delete(m.AnalyticsCache, tenantID)
			delete(m.LastAccessed, tenantID)
		}
	}
}

func (m *Manager) GetCacheStatus() map[string]interface{} {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	return map[string]interface{}{
		"tenantCount":     len(m.ContentCache),
		"lastCleanup":     time.Now().UTC(),
		"memoryOptimized": true,
	}
}
