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
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// Interface assertions to ensure Manager implements all required interfaces.
var (
	_ interfaces.Cache                   = (*Manager)(nil)
	_ interfaces.WriteOnlyAnalyticsCache = (*Manager)(nil)
	_ interfaces.ReadOnlyAnalyticsCache  = (*Manager)(nil)
)

// Manager provides centralized cache operations with proper tenant isolation by delegating to specialized stores.
type Manager struct {
	Mu             sync.RWMutex
	LastAccessed   map[string]time.Time
	contentStore   *stores.ContentStore
	analyticsStore *stores.AnalyticsStore
	configStore    *stores.ConfigStore
	sessionsStore  *stores.SessionsStore
	fragmentsStore *stores.FragmentsStore
	logger         *logging.ChanneledLogger
}

func NewManager(logger *logging.ChanneledLogger) *Manager {
	if logger != nil {
		logger.Cache().Info("Initializing cache manager", "stores", []string{"content", "analytics", "config", "sessions", "fragments"})
	}

	return &Manager{
		LastAccessed:   make(map[string]time.Time),
		contentStore:   stores.NewContentStore(logger),
		analyticsStore: stores.NewAnalyticsStore(logger),
		configStore:    stores.NewConfigStore(logger),
		sessionsStore:  stores.NewSessionsStore(logger),
		fragmentsStore: stores.NewFragmentsStore(logger),
		logger:         logger,
	}
}

func (m *Manager) GetTenantContentCache(tenantID string) (*types.TenantContentCache, error) {
	cache, exists := m.contentStore.GetTenantCache(tenantID)
	if !exists {
		return nil, fmt.Errorf("tenant %s content cache not initialized", tenantID)
	}
	return cache, nil
}

func (m *Manager) GetTenantUserStateCache(tenantID string) (*types.TenantUserStateCache, error) {
	cache, exists := m.sessionsStore.GetTenantCache(tenantID)
	if !exists {
		return nil, fmt.Errorf("tenant %s user state cache not initialized", tenantID)
	}
	return cache, nil
}

func (m *Manager) GetTenantHTMLChunkCache(tenantID string) (*types.TenantHTMLChunkCache, error) {
	cache, exists := m.fragmentsStore.GetTenantCache(tenantID)
	if !exists {
		return nil, fmt.Errorf("tenant %s HTML chunk cache not initialized", tenantID)
	}
	return cache, nil
}

func (m *Manager) GetTenantAnalyticsCache(tenantID string) (*types.TenantAnalyticsCache, error) {
	cache, exists := m.analyticsStore.GetTenantCache(tenantID)
	if !exists {
		return nil, fmt.Errorf("tenant %s analytics cache not initialized", tenantID)
	}
	return cache, nil
}

func (m *Manager) updateTenantAccessTime(tenantID string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.LastAccessed[tenantID] = time.Now().UTC()
}

func (m *Manager) InitializeTenant(tenantID string) {
	start := time.Now()
	if m.logger != nil {
		m.logger.Cache().Debug("Initializing tenant cache", "tenantId", tenantID)
	}

	m.contentStore.InitializeTenant(tenantID)
	m.analyticsStore.InitializeTenant(tenantID)
	m.configStore.InitializeTenant(tenantID)
	m.sessionsStore.InitializeTenant(tenantID)
	m.fragmentsStore.InitializeTenant(tenantID)
	m.updateTenantAccessTime(tenantID)

	if m.logger != nil {
		m.logger.Cache().Info("Tenant cache initialized", "tenantId", tenantID, "duration", time.Since(start))
	}
}

func (m *Manager) GetRangeCacheStatus(tenantID, epinetID string, startHour, endHour int) types.RangeCacheStatus {
	hourKeys := utils.GetHourKeysForCustomRange(startHour, endHour)

	now := time.Now().UTC()
	currentHourKey := utils.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))

	var missingHours []string
	currentHourExpired := false
	historicalMissing := false

	foundBins, missingKeys := m.GetHourlyEpinetRange(tenantID, epinetID, hourKeys)

	for _, missingKey := range missingKeys {
		missingHours = append(missingHours, missingKey)
		if missingKey == currentHourKey {
			currentHourExpired = true
		} else {
			historicalMissing = true
		}
	}

	for hourKey, bin := range foundBins {
		isExpired := false
		ttl := config.AnalyticsBinTTL
		if hourKey == currentHourKey {
			ttl = config.CurrentHourTTL
		}
		if time.Since(bin.ComputedAt) > ttl {
			isExpired = true
		}

		if isExpired {
			missingHours = append(missingHours, hourKey)
			if hourKey == currentHourKey {
				currentHourExpired = true
			} else {
				historicalMissing = true
			}
		}
	}

	var action string
	if len(missingHours) == 0 {
		action = "proceed"
	} else if currentHourExpired && !historicalMissing {
		action = "refresh_current"
	} else {
		action = "load_range"
	}

	return types.RangeCacheStatus{
		Action:             action,
		CurrentHourExpired: currentHourExpired,
		HistoricalComplete: !historicalMissing,
		MissingHours:       missingHours,
	}
}

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

func (m *Manager) GetLeadMetricsWithETag(tenantID, cacheKey string) (*types.LeadMetricsData, string, bool) {
	dataCache, found := m.analyticsStore.GetLeadMetrics(tenantID)
	if !found || dataCache == nil {
		return nil, "", false
	}
	return dataCache.Data, "", true
}

func (m *Manager) SetLeadMetricsWithETag(tenantID, cacheKey string, data *types.LeadMetricsData, etag string) {
	cacheEntry := &types.LeadMetricsCache{
		Data:         data,
		LastComputed: time.Now().UTC(),
	}
	m.analyticsStore.SetLeadMetrics(tenantID, cacheEntry)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetDashboardData(tenantID string) (*types.DashboardCache, bool) {
	return m.analyticsStore.GetDashboardData(tenantID)
}

func (m *Manager) SetDashboardData(tenantID string, data *types.DashboardCache) {
	m.analyticsStore.SetDashboardData(tenantID, data)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetDashboardDataWithETag(tenantID, cacheKey string) (*types.DashboardData, string, bool) {
	dataCache, found := m.analyticsStore.GetDashboardData(tenantID)
	if !found || dataCache == nil {
		return nil, "", false
	}
	return dataCache.Data, "", true
}

func (m *Manager) SetDashboardDataWithETag(tenantID, cacheKey string, data *types.DashboardData, etag string) {
	cacheEntry := &types.DashboardCache{
		Data:         data,
		LastComputed: time.Now().UTC(),
	}
	m.analyticsStore.SetDashboardData(tenantID, cacheEntry)
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

func (m *Manager) GetTractStack(tenantID, id string) (*content.TractStackNode, bool) {
	return m.contentStore.GetTractStack(tenantID, id)
}

func (m *Manager) SetTractStack(tenantID string, node *content.TractStackNode) {
	m.contentStore.SetTractStack(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) GetAllTractStackIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	// The key change: check the dedicated slice.
	// If this slice is nil or empty, it's a cache miss.
	if len(cache.AllTractStackIDs) == 0 {
		return nil, false
	}
	ids := make([]string, len(cache.AllTractStackIDs))
	copy(ids, cache.AllTractStackIDs)
	return ids, true
}

func (m *Manager) SetAllTractStackIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllTractStackIDs = ids
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
	if len(cache.AllStoryFragmentIDs) == 0 {
		return nil, false
	}
	ids := make([]string, len(cache.AllStoryFragmentIDs))
	copy(ids, cache.AllStoryFragmentIDs)
	return ids, true
}

func (m *Manager) SetAllStoryFragmentIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllStoryFragmentIDs = ids
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
	if len(cache.AllPaneIDs) == 0 {
		return nil, false
	}
	ids := make([]string, len(cache.AllPaneIDs))
	copy(ids, cache.AllPaneIDs)
	return ids, true
}

func (m *Manager) SetAllPaneIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllPaneIDs = ids
}

func (m *Manager) GetMenu(tenantID, id string) (*content.MenuNode, bool) {
	return m.contentStore.GetMenu(tenantID, id)
}

func (m *Manager) SetMenu(tenantID string, node *content.MenuNode) {
	m.contentStore.SetMenu(tenantID, node)
}

func (m *Manager) GetAllMenuIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	if len(cache.AllMenuIDs) == 0 {
		return nil, false
	}
	ids := make([]string, len(cache.AllMenuIDs))
	copy(ids, cache.AllMenuIDs)
	return ids, true
}

func (m *Manager) SetAllMenuIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllMenuIDs = ids
}

func (m *Manager) GetResource(tenantID, id string) (*content.ResourceNode, bool) {
	return m.contentStore.GetResource(tenantID, id)
}

func (m *Manager) SetResource(tenantID string, node *content.ResourceNode) {
	m.contentStore.SetResource(tenantID, node)
}

func (m *Manager) GetAllResourceIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	if len(cache.AllResourceIDs) == 0 {
		return nil, false
	}
	ids := make([]string, len(cache.AllResourceIDs))
	copy(ids, cache.AllResourceIDs)
	return ids, true
}

func (m *Manager) SetAllResourceIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllResourceIDs = ids
}

func (m *Manager) GetBelief(tenantID, id string) (*content.BeliefNode, bool) {
	return m.contentStore.GetBelief(tenantID, id)
}

func (m *Manager) SetBelief(tenantID string, node *content.BeliefNode) {
	m.contentStore.SetBelief(tenantID, node)
}

func (m *Manager) GetAllBeliefIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	if len(cache.AllBeliefIDs) == 0 {
		return nil, false
	}
	ids := make([]string, len(cache.AllBeliefIDs))
	copy(ids, cache.AllBeliefIDs)
	return ids, true
}

func (m *Manager) SetAllBeliefIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllBeliefIDs = ids
}

func (m *Manager) GetEpinet(tenantID, id string) (*content.EpinetNode, bool) {
	return m.contentStore.GetEpinet(tenantID, id)
}

func (m *Manager) SetEpinet(tenantID string, node *content.EpinetNode) {
	m.contentStore.SetEpinet(tenantID, node)
}

func (m *Manager) GetAllEpinetIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	if len(cache.AllEpinetIDs) == 0 {
		return nil, false
	}
	ids := make([]string, len(cache.AllEpinetIDs))
	copy(ids, cache.AllEpinetIDs)
	return ids, true
}

func (m *Manager) SetAllEpinetIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllEpinetIDs = ids
}

func (m *Manager) GetFile(tenantID, id string) (*content.ImageFileNode, bool) {
	return m.contentStore.GetImageFile(tenantID, id)
}

func (m *Manager) SetFile(tenantID string, node *content.ImageFileNode) {
	m.contentStore.SetImageFile(tenantID, node)
}

func (m *Manager) GetAllFileIDs(tenantID string) ([]string, bool) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return nil, false
	}
	cache.Mu.RLock()
	defer cache.Mu.RUnlock()
	if len(cache.AllFileIDs) == 0 {
		return nil, false
	}
	ids := make([]string, len(cache.AllFileIDs))
	copy(ids, cache.AllFileIDs)
	return ids, true
}

func (m *Manager) SetAllFileIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllFileIDs = ids
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
}

func (m *Manager) GetOrphanAnalysis(tenantID string) (*types.OrphanAnalysisPayload, string, bool) {
	return m.contentStore.GetOrphanAnalysis(tenantID)
}

func (m *Manager) SetOrphanAnalysis(tenantID string, payload *types.OrphanAnalysisPayload, etag string) {
	m.contentStore.SetOrphanAnalysis(tenantID, payload, etag)
}

func (m *Manager) InvalidateContentCache(tenantID string) {
	m.contentStore.InvalidateContentCache(tenantID)
}

func (m *Manager) GetVisitState(tenantID, visitID string) (*types.VisitState, bool) {
	return m.sessionsStore.GetVisitState(tenantID, visitID)
}

func (m *Manager) SetVisitState(tenantID string, state *types.VisitState) {
	m.sessionsStore.SetVisitState(tenantID, state)
}

func (m *Manager) GetFingerprintState(tenantID, fingerprintID string) (*types.FingerprintState, bool) {
	return m.sessionsStore.GetFingerprintState(tenantID, fingerprintID)
}

func (m *Manager) SetFingerprintState(tenantID string, state *types.FingerprintState) {
	m.sessionsStore.SetFingerprintState(tenantID, state)
}

func (m *Manager) IsKnownFingerprint(tenantID, fingerprintID string) bool {
	return m.sessionsStore.IsKnownFingerprint(tenantID, fingerprintID)
}

func (m *Manager) SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool) {
	m.sessionsStore.SetKnownFingerprint(tenantID, fingerprintID, isKnown)
}

func (m *Manager) LoadKnownFingerprints(tenantID string, fingerprints map[string]bool) {
	m.sessionsStore.LoadKnownFingerprints(tenantID, fingerprints)
}

func (m *Manager) GetSession(tenantID, sessionID string) (*types.SessionData, bool) {
	return m.sessionsStore.GetSession(tenantID, sessionID)
}

func (m *Manager) SetSession(tenantID string, sessionData *types.SessionData) {
	m.sessionsStore.SetSession(tenantID, sessionData)
}

func (m *Manager) GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*types.StoryfragmentBeliefRegistry, bool) {
	return m.sessionsStore.GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
}

func (m *Manager) SetStoryfragmentBeliefRegistry(tenantID string, registry *types.StoryfragmentBeliefRegistry) {
	m.sessionsStore.SetStoryfragmentBeliefRegistry(tenantID, registry)
}

func (m *Manager) InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) {
	m.sessionsStore.InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
}

func (m *Manager) GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*types.SessionBeliefContext, bool) {
	return m.sessionsStore.GetSessionBeliefContext(tenantID, sessionID, storyfragmentID)
}

func (m *Manager) SetSessionBeliefContext(tenantID string, context *types.SessionBeliefContext) {
	m.sessionsStore.SetSessionBeliefContext(tenantID, context)
}

func (m *Manager) InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string) {
	m.sessionsStore.InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID)
}

func (m *Manager) InvalidateUserStateCache(tenantID string) {
	m.sessionsStore.InvalidateUserStateCache(tenantID)
}

func (m *Manager) GetHTMLChunk(tenantID, paneID string, variant types.PaneVariant) (*types.HTMLChunk, bool) {
	return m.fragmentsStore.GetHTMLChunk(tenantID, paneID, variant)
}

func (m *Manager) SetHTMLChunk(tenantID, paneID string, variant types.PaneVariant, html string, dependsOn []string) {
	m.fragmentsStore.SetHTMLChunk(tenantID, paneID, variant, html, dependsOn)
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

func (m *Manager) InvalidateByDependency(tenantID, nodeID string) {
	m.fragmentsStore.InvalidateByDependency(tenantID, nodeID)
}

func (m *Manager) InvalidateHTMLChunkCache(tenantID string) {
	m.fragmentsStore.InvalidateHTMLChunkCache(tenantID)
}

func (m *Manager) InvalidateHTMLChunk(tenantID, paneID string, variant types.PaneVariant) {
	m.fragmentsStore.InvalidateByPattern(tenantID, m.fragmentsStore.BuildChunkKey(paneID, variant))
}

func (m *Manager) InvalidateTenant(tenantID string) {
	start := time.Now()
	if m.logger != nil {
		m.logger.Cache().Debug("Invalidating tenant cache", "tenantId", tenantID)
	}

	m.contentStore.InvalidateContentCache(tenantID)
	m.sessionsStore.InvalidateUserStateCache(tenantID)
	m.fragmentsStore.InvalidateHTMLChunkCache(tenantID)
	m.analyticsStore.InvalidateAnalyticsCache(tenantID)
	m.updateTenantAccessTime(tenantID)

	if m.logger != nil {
		m.logger.Cache().Info("Tenant cache invalidated", "tenantId", tenantID, "duration", time.Since(start))
	}
}

func (m *Manager) GetTenantStats(tenantID string) interfaces.CacheStats {
	return interfaces.CacheStats{}
}

func (m *Manager) GetMemoryStats() map[string]any {
	return make(map[string]any)
}

func (m *Manager) InvalidateAll() {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	for _, tenantID := range m.contentStore.GetAllTenantIDs() {
		m.InvalidateTenant(tenantID)
	}
}

func (m *Manager) Health() map[string]any {
	return map[string]any{"status": "ok"}
}

// GetAllSessionIDs returns all session IDs for a tenant
func (m *Manager) GetAllSessionIDs(tenantID string) []string {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return []string{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	sessionIDs := make([]string, 0, len(cache.SessionStates))
	for sessionID := range cache.SessionStates {
		sessionIDs = append(sessionIDs, sessionID)
	}
	return sessionIDs
}

// GetAllFingerprintIDs returns all fingerprint IDs for a tenant
func (m *Manager) GetAllFingerprintIDs(tenantID string) []string {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return []string{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	fingerprintIDs := make([]string, 0, len(cache.FingerprintStates))
	for fingerprintID := range cache.FingerprintStates {
		fingerprintIDs = append(fingerprintIDs, fingerprintID)
	}
	return fingerprintIDs
}

// GetAllVisitIDs returns all visit IDs for a tenant
func (m *Manager) GetAllVisitIDs(tenantID string) []string {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return []string{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	visitIDs := make([]string, 0, len(cache.VisitStates))
	for visitID := range cache.VisitStates {
		visitIDs = append(visitIDs, visitID)
	}
	return visitIDs
}

// GetAllHTMLChunkIDs returns all HTML chunk keys for a tenant
func (m *Manager) GetAllHTMLChunkIDs(tenantID string) []string {
	cache, err := m.GetTenantHTMLChunkCache(tenantID)
	if err != nil {
		return []string{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	chunkIDs := make([]string, 0, len(cache.Chunks))
	for chunkID := range cache.Chunks {
		chunkIDs = append(chunkIDs, chunkID)
	}
	return chunkIDs
}

// GetAllStoryfragmentBeliefRegistryIDs returns all storyfragment IDs that have cached belief registries
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

// InvalidateFullContentMap clears the content map with thundering herd protection
func (m *Manager) InvalidateFullContentMap(tenantID string) {
	start := time.Now()

	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		if m.logger != nil {
			m.logger.Cache().Error("Failed to get tenant cache for content map invalidation",
				"tenantId", tenantID, "error", err, "duration", time.Since(start))
		}
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.FullContentMap = make([]types.FullContentMapItem, 0)
	cache.ContentMapLastUpdated = time.Time{}
	cache.LastUpdated = time.Now().UTC()

	if m.logger != nil {
		m.logger.Cache().Info("Content map invalidated", "tenantId", tenantID, "duration", time.Since(start))
	}
}

func (m *Manager) InvalidateResource(tenantID, id string) {
	m.contentStore.InvalidateResource(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) AddResourceID(tenantID, id string) {
	m.contentStore.AddResourceID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) RemoveResourceID(tenantID, id string) {
	m.contentStore.RemoveResourceID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateTractStack(tenantID, id string) {
	m.contentStore.InvalidateTractStack(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) AddTractStackID(tenantID, id string) {
	m.contentStore.AddTractStackID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) RemoveTractStackID(tenantID, id string) {
	m.contentStore.RemoveTractStackID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateStoryFragment(tenantID, id string) {
	m.contentStore.InvalidateStoryFragment(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) AddStoryFragmentID(tenantID, id string) {
	m.contentStore.AddStoryFragmentID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) RemoveStoryFragmentID(tenantID, id string) {
	m.contentStore.RemoveStoryFragmentID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidatePane(tenantID, id string) {
	m.contentStore.InvalidatePane(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) AddPaneID(tenantID, id string) {
	m.contentStore.AddPaneID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) RemovePaneID(tenantID, id string) {
	m.contentStore.RemovePaneID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateMenu(tenantID, id string) {
	m.contentStore.InvalidateMenu(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) AddMenuID(tenantID, id string) {
	m.contentStore.AddMenuID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) RemoveMenuID(tenantID, id string) {
	m.contentStore.RemoveMenuID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateBelief(tenantID, id string) {
	m.contentStore.InvalidateBelief(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) AddBeliefID(tenantID, id string) {
	m.contentStore.AddBeliefID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) RemoveBeliefID(tenantID, id string) {
	m.contentStore.RemoveBeliefID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateEpinet(tenantID, id string) {
	m.contentStore.InvalidateEpinet(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) AddEpinetID(tenantID, id string) {
	m.contentStore.AddEpinetID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) RemoveEpinetID(tenantID, id string) {
	m.contentStore.RemoveEpinetID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) InvalidateFile(tenantID, id string) {
	m.contentStore.InvalidateFile(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) AddFileID(tenantID, id string) {
	m.contentStore.AddFileID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

func (m *Manager) RemoveFileID(tenantID, id string) {
	m.contentStore.RemoveFileID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}
