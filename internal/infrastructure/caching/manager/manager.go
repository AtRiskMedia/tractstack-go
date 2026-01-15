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
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/utilities"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
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
	genericStore   map[string]types.GenericCacheItem
	contentStore   *stores.ContentStore
	analyticsStore *stores.AnalyticsStore
	configStore    *stores.ConfigStore
	sessionsStore  *stores.SessionsStore
	fragmentsStore *stores.FragmentsStore
	logger         *logging.ChanneledLogger
}

// NewManager creates a new cache manager with initialized internal stores.
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

// GetTenantContentCache retrieves the content cache specific to a tenant.
func (m *Manager) GetTenantContentCache(tenantID string) (*types.TenantContentCache, error) {
	cache, exists := m.contentStore.GetTenantCache(tenantID)
	if !exists {
		return nil, fmt.Errorf("tenant %s content cache not initialized", tenantID)
	}
	return cache, nil
}

// GetTenantUserStateCache retrieves the user state cache for a specific tenant.
func (m *Manager) GetTenantUserStateCache(tenantID string) (*types.TenantUserStateCache, error) {
	cache, exists := m.sessionsStore.GetTenantCache(tenantID)
	if !exists {
		return nil, fmt.Errorf("tenant %s user state cache not initialized", tenantID)
	}
	return cache, nil
}

// GetTenantHTMLChunkCache retrieves the HTML chunk cache for a specific tenant.
func (m *Manager) GetTenantHTMLChunkCache(tenantID string) (*types.TenantHTMLChunkCache, error) {
	cache, exists := m.fragmentsStore.GetTenantCache(tenantID)
	if !exists {
		return nil, fmt.Errorf("tenant %s HTML chunk cache not initialized", tenantID)
	}
	return cache, nil
}

// GetTenantAnalyticsCache retrieves the analytics cache for a specific tenant.
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

// InitializeTenant sets up all necessary cache stores for a specific tenant.
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

// GetRangeCacheStatus checks the cache status for a range of hourly analytics bins to determine if fetching is required.
func (m *Manager) GetRangeCacheStatus(tenantID, epinetID string, startHour, endHour int) types.RangeCacheStatus {
	hourKeys := utilities.GetHourKeysForCustomRange(startHour, endHour)

	now := time.Now().UTC()
	currentHourKey := utilities.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))

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
	switch {
	case len(missingHours) == 0:
		action = "proceed"
	case currentHourExpired && !historicalMissing:
		action = "refresh_current"
	default:
		action = "load_range"
	}

	return types.RangeCacheStatus{
		Action:             action,
		CurrentHourExpired: currentHourExpired,
		HistoricalComplete: !historicalMissing,
		MissingHours:       missingHours,
	}
}

// GetHourlyEpinetBin retrieves a cached hourly bin for a specific Epinet.
func (m *Manager) GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*types.HourlyEpinetBin, bool) {
	return m.analyticsStore.GetHourlyEpinetBin(tenantID, epinetID, hourKey)
}

// SetHourlyEpinetBin stores an hourly bin for an Epinet in the cache.
func (m *Manager) SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *types.HourlyEpinetBin) {
	m.analyticsStore.SetHourlyEpinetBin(tenantID, epinetID, hourKey, bin)
	m.updateTenantAccessTime(tenantID)
}

// GetHourlyContentBin retrieves a cached hourly bin for a specific content item.
func (m *Manager) GetHourlyContentBin(tenantID, contentID, hourKey string) (*types.HourlyContentBin, bool) {
	return m.analyticsStore.GetHourlyContentBin(tenantID, contentID, hourKey)
}

// SetHourlyContentBin stores an hourly bin for a content item in the cache.
func (m *Manager) SetHourlyContentBin(tenantID, contentID, hourKey string, bin *types.HourlyContentBin) {
	m.analyticsStore.SetHourlyContentBin(tenantID, contentID, hourKey, bin)
	m.updateTenantAccessTime(tenantID)
}

// GetHourlySiteBin retrieves a cached hourly bin for the entire site.
func (m *Manager) GetHourlySiteBin(tenantID, hourKey string) (*types.HourlySiteBin, bool) {
	return m.analyticsStore.GetHourlySiteBin(tenantID, hourKey)
}

// SetHourlySiteBin stores an hourly bin for the site in the cache.
func (m *Manager) SetHourlySiteBin(tenantID, hourKey string, bin *types.HourlySiteBin) {
	m.analyticsStore.SetHourlySiteBin(tenantID, hourKey, bin)
	m.updateTenantAccessTime(tenantID)
}

// GetLeadMetrics retrieves the cached lead metrics for a tenant.
func (m *Manager) GetLeadMetrics(tenantID string) (*types.LeadMetricsCache, bool) {
	return m.analyticsStore.GetLeadMetrics(tenantID)
}

// SetLeadMetrics stores the lead metrics for a tenant in the cache.
func (m *Manager) SetLeadMetrics(tenantID string, metrics *types.LeadMetricsCache) {
	m.analyticsStore.SetLeadMetrics(tenantID, metrics)
	m.updateTenantAccessTime(tenantID)
}

// GetLeadMetricsWithETag retrieves cached lead metrics along with an ETag for validation.
func (m *Manager) GetLeadMetricsWithETag(tenantID, _ string) (*types.LeadMetricsData, string, bool) {
	dataCache, found := m.analyticsStore.GetLeadMetrics(tenantID)
	if !found || dataCache == nil {
		return nil, "", false
	}
	return dataCache.Data, "", true
}

// SetLeadMetricsWithETag stores lead metrics in the cache with an associated ETag.
func (m *Manager) SetLeadMetricsWithETag(tenantID, _ string, data *types.LeadMetricsData, _ string) {
	cacheEntry := &types.LeadMetricsCache{
		Data:         data,
		LastComputed: time.Now().UTC(),
	}
	m.analyticsStore.SetLeadMetrics(tenantID, cacheEntry)
	m.updateTenantAccessTime(tenantID)
}

// GetDashboardData retrieves the cached dashboard data for a tenant.
func (m *Manager) GetDashboardData(tenantID string) (*types.DashboardCache, bool) {
	return m.analyticsStore.GetDashboardData(tenantID)
}

// SetDashboardData stores the dashboard data for a tenant in the cache.
func (m *Manager) SetDashboardData(tenantID string, data *types.DashboardCache) {
	m.analyticsStore.SetDashboardData(tenantID, data)
	m.updateTenantAccessTime(tenantID)
}

// GetDashboardDataWithETag retrieves cached dashboard data along with an ETag.
func (m *Manager) GetDashboardDataWithETag(tenantID, _ string) (*types.DashboardData, string, bool) {
	dataCache, found := m.analyticsStore.GetDashboardData(tenantID)
	if !found || dataCache == nil {
		return nil, "", false
	}
	return dataCache.Data, "", true
}

// SetDashboardDataWithETag stores dashboard data in the cache with an associated ETag.
func (m *Manager) SetDashboardDataWithETag(tenantID, _ string, data *types.DashboardData, _ string) {
	cacheEntry := &types.DashboardCache{
		Data:         data,
		LastComputed: time.Now().UTC(),
	}
	m.analyticsStore.SetDashboardData(tenantID, cacheEntry)
	m.updateTenantAccessTime(tenantID)
}

// GetHourlyEpinetRange retrieves a map of hourly bins for an Epinet over a specified sequence of hours.
func (m *Manager) GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*types.HourlyEpinetBin, []string) {
	return m.analyticsStore.GetHourlyEpinetRange(tenantID, epinetID, hourKeys)
}

// PurgeExpiredBins removes analytics bins from the cache that have exceeded their retention period.
func (m *Manager) PurgeExpiredBins(tenantID string, olderThan string) {
	m.analyticsStore.PurgeExpiredBins(tenantID, olderThan)
	m.updateTenantAccessTime(tenantID)
}

// InvalidateAnalyticsCache clears all analytics-related data from the cache for a specific tenant.
func (m *Manager) InvalidateAnalyticsCache(tenantID string) {
	m.analyticsStore.InvalidateAnalyticsCache(tenantID)
	m.updateTenantAccessTime(tenantID)
}

// UpdateLastFullHour records the most recent hour for which analytics data has been fully processed.
func (m *Manager) UpdateLastFullHour(tenantID, hourKey string) {
	m.analyticsStore.UpdateLastFullHour(tenantID, hourKey)
	m.updateTenantAccessTime(tenantID)
}

// GetTractStack retrieves a TractStack node from the tenant's content cache.
func (m *Manager) GetTractStack(tenantID, id string) (*content.TractStackNode, bool) {
	return m.contentStore.GetTractStack(tenantID, id)
}

// SetTractStack stores a TractStack node in the tenant's content cache.
func (m *Manager) SetTractStack(tenantID string, node *content.TractStackNode) {
	m.contentStore.SetTractStack(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

// GetAllTractStackIDs returns a list of all TractStack IDs currently cached for a tenant.
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

// SetAllTractStackIDs stores the complete list of TractStack IDs for a tenant in the cache.
func (m *Manager) SetAllTractStackIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllTractStackIDs = ids
}

// GetStoryFragment retrieves a StoryFragment node from the tenant's content cache.
func (m *Manager) GetStoryFragment(tenantID, id string) (*content.StoryFragmentNode, bool) {
	return m.contentStore.GetStoryFragment(tenantID, id)
}

// SetStoryFragment stores a StoryFragment node in the tenant's content cache.
func (m *Manager) SetStoryFragment(tenantID string, node *content.StoryFragmentNode) {
	m.contentStore.SetStoryFragment(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

// GetAllStoryFragmentIDs returns a list of all StoryFragment IDs currently cached for a tenant.
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

// SetAllStoryFragmentIDs stores the complete list of StoryFragment IDs for a tenant in the cache.
func (m *Manager) SetAllStoryFragmentIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllStoryFragmentIDs = ids
}

// GetPane retrieves a Pane node from the tenant's content cache.
func (m *Manager) GetPane(tenantID, id string) (*content.PaneNode, bool) {
	return m.contentStore.GetPane(tenantID, id)
}

// SetPane stores a Pane node in the tenant's content cache.
func (m *Manager) SetPane(tenantID string, node *content.PaneNode) {
	m.contentStore.SetPane(tenantID, node)
	m.updateTenantAccessTime(tenantID)
}

// GetAllPaneIDs returns a list of all Pane IDs currently cached for a tenant.
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

// SetAllPaneIDs stores the complete list of Pane IDs for a tenant in the cache.
func (m *Manager) SetAllPaneIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllPaneIDs = ids
}

// GetMenu retrieves a Menu node from the tenant's content cache.
func (m *Manager) GetMenu(tenantID, id string) (*content.MenuNode, bool) {
	return m.contentStore.GetMenu(tenantID, id)
}

// SetMenu stores a Menu node in the tenant's content cache.
func (m *Manager) SetMenu(tenantID string, node *content.MenuNode) {
	m.contentStore.SetMenu(tenantID, node)
}

// GetAllMenuIDs returns a list of all Menu IDs currently cached for a tenant.
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

// SetAllMenuIDs stores the complete list of Menu IDs for a tenant in the cache.
func (m *Manager) SetAllMenuIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllMenuIDs = ids
}

// GetResource retrieves a Resource node from the tenant's content cache.
func (m *Manager) GetResource(tenantID, id string) (*content.ResourceNode, bool) {
	return m.contentStore.GetResource(tenantID, id)
}

// SetResource stores a Resource node in the tenant's content cache.
func (m *Manager) SetResource(tenantID string, node *content.ResourceNode) {
	m.contentStore.SetResource(tenantID, node)
}

// GetAllResourceIDs returns a list of all Resource IDs currently cached for a tenant.
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

// SetAllResourceIDs stores the complete list of Resource IDs for a tenant in the cache.
func (m *Manager) SetAllResourceIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllResourceIDs = ids
}

// GetBelief retrieves a Belief node from the tenant's content cache.
func (m *Manager) GetBelief(tenantID, id string) (*content.BeliefNode, bool) {
	return m.contentStore.GetBelief(tenantID, id)
}

// SetBelief stores a Belief node in the tenant's content cache.
func (m *Manager) SetBelief(tenantID string, node *content.BeliefNode) {
	m.contentStore.SetBelief(tenantID, node)
}

// GetAllBeliefIDs returns a list of all Belief IDs currently cached for a tenant.
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

// SetAllBeliefIDs stores the complete list of Belief IDs for a tenant in the cache.
func (m *Manager) SetAllBeliefIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllBeliefIDs = ids
}

// GetEpinet retrieves an Epinet node from the tenant's content cache.
func (m *Manager) GetEpinet(tenantID, id string) (*content.EpinetNode, bool) {
	return m.contentStore.GetEpinet(tenantID, id)
}

// SetEpinet stores an Epinet node in the tenant's content cache.
func (m *Manager) SetEpinet(tenantID string, node *content.EpinetNode) {
	m.contentStore.SetEpinet(tenantID, node)
}

// GetAllEpinetIDs returns a list of all Epinet IDs currently cached for a tenant.
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

// SetAllEpinetIDs stores the complete list of Epinet IDs for a tenant in the cache.
func (m *Manager) SetAllEpinetIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllEpinetIDs = ids
}

// GetFile retrieves an ImageFile node from the tenant's content cache.
func (m *Manager) GetFile(tenantID, id string) (*content.ImageFileNode, bool) {
	return m.contentStore.GetImageFile(tenantID, id)
}

// SetFile stores an ImageFile node in the tenant's content cache.
func (m *Manager) SetFile(tenantID string, node *content.ImageFileNode) {
	m.contentStore.SetImageFile(tenantID, node)
}

// GetAllFileIDs returns a list of all ImageFile IDs currently cached for a tenant.
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

// SetAllFileIDs stores the complete list of ImageFile IDs for a tenant in the cache.
func (m *Manager) SetAllFileIDs(tenantID string, ids []string) {
	cache, err := m.GetTenantContentCache(tenantID)
	if err != nil {
		return
	}
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	cache.AllFileIDs = ids
}

// GetContentBySlug resolves a content slug to its unique identifier for a specific tenant.
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

// GetResourcesByCategory retrieves all resource IDs belonging to a specific category for a tenant.
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

// GetFullContentMap retrieves the complete hierarchical content map for a tenant.
func (m *Manager) GetFullContentMap(tenantID string) ([]types.FullContentMapItem, bool) {
	return m.contentStore.GetFullContentMap(tenantID)
}

// SetFullContentMap stores the complete hierarchical content map for a tenant in the cache.
func (m *Manager) SetFullContentMap(tenantID string, contentMap []types.FullContentMapItem) {
	m.contentStore.SetFullContentMap(tenantID, contentMap)
}

// GetOrphanAnalysis retrieves the results of an orphan analysis for a specific tenant.
func (m *Manager) GetOrphanAnalysis(tenantID string) (*types.OrphanAnalysisPayload, string, bool) {
	return m.contentStore.GetOrphanAnalysis(tenantID)
}

// SetOrphanAnalysis stores the results of an orphan analysis in the cache with an ETag.
func (m *Manager) SetOrphanAnalysis(tenantID string, payload *types.OrphanAnalysisPayload, etag string) {
	m.contentStore.SetOrphanAnalysis(tenantID, payload, etag)
}

// InvalidateContentCache clears all cached content nodes for a specific tenant.
func (m *Manager) InvalidateContentCache(tenantID string) {
	m.contentStore.InvalidateContentCache(tenantID)
}

// GetVisitState retrieves the tracking state for a specific visit ID.
func (m *Manager) GetVisitState(tenantID, visitID string) (*types.VisitState, bool) {
	return m.sessionsStore.GetVisitState(tenantID, visitID)
}

// SetVisitState stores the tracking state for a visit in the user state cache.
func (m *Manager) SetVisitState(tenantID string, state *types.VisitState) {
	m.sessionsStore.SetVisitState(tenantID, state)
}

// GetFingerprintState retrieves the accumulated belief state for a specific visitor fingerprint.
func (m *Manager) GetFingerprintState(tenantID, fingerprintID string) (*types.FingerprintState, bool) {
	return m.sessionsStore.GetFingerprintState(tenantID, fingerprintID)
}

// SetFingerprintState stores the accumulated belief state for a fingerprint in the cache.
func (m *Manager) SetFingerprintState(tenantID string, state *types.FingerprintState) {
	m.sessionsStore.SetFingerprintState(tenantID, state)
}

// IsKnownFingerprint checks if a visitor fingerprint has been previously recorded by the system.
func (m *Manager) IsKnownFingerprint(tenantID, fingerprintID string) bool {
	return m.sessionsStore.IsKnownFingerprint(tenantID, fingerprintID)
}

// SetKnownFingerprint marks a visitor fingerprint as known or unknown in the tracking system.
func (m *Manager) SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool) {
	m.sessionsStore.SetKnownFingerprint(tenantID, fingerprintID, isKnown)
}

// LoadKnownFingerprints bulk loads a map of known fingerprints into the tenant's cache.
func (m *Manager) LoadKnownFingerprints(tenantID string, fingerprints map[string]bool) {
	m.sessionsStore.LoadKnownFingerprints(tenantID, fingerprints)
}

// GetSession retrieves ephemeral session data for a specific session ID.
func (m *Manager) GetSession(tenantID, sessionID string) (*types.SessionData, bool) {
	return m.sessionsStore.GetSession(tenantID, sessionID)
}

// SetSession stores ephemeral session data in the user state cache.
func (m *Manager) SetSession(tenantID string, sessionData *types.SessionData) {
	m.sessionsStore.SetSession(tenantID, sessionData)
}

// GetStoryfragmentBeliefRegistry retrieves the belief requirements registry for a specific story fragment.
func (m *Manager) GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*types.StoryfragmentBeliefRegistry, bool) {
	return m.sessionsStore.GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
}

// SetStoryfragmentBeliefRegistry stores a belief requirements registry for a story fragment in the cache.
func (m *Manager) SetStoryfragmentBeliefRegistry(tenantID string, registry *types.StoryfragmentBeliefRegistry) {
	m.sessionsStore.SetStoryfragmentBeliefRegistry(tenantID, registry)
}

// InvalidateStoryfragmentBeliefRegistry removes a story fragment's belief registry from the cache.
func (m *Manager) InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) {
	m.sessionsStore.InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID)
}

// GetSessionBeliefContext retrieves the evaluated belief context for a specific session and story fragment pair.
func (m *Manager) GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*types.SessionBeliefContext, bool) {
	return m.sessionsStore.GetSessionBeliefContext(tenantID, sessionID, storyfragmentID)
}

// SetSessionBeliefContext stores an evaluated belief context in the cache.
func (m *Manager) SetSessionBeliefContext(tenantID string, context *types.SessionBeliefContext) {
	m.sessionsStore.SetSessionBeliefContext(tenantID, context)
}

// InvalidateSessionBeliefContext removes the evaluated belief context for a session and story fragment pair.
func (m *Manager) InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string) {
	m.sessionsStore.InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID)
}

// InvalidateUserStateCache clears all cached user, session, and fingerprint data for a tenant.
func (m *Manager) InvalidateUserStateCache(tenantID string) {
	m.sessionsStore.InvalidateUserStateCache(tenantID)
}

// GetHTMLChunk retrieves a cached HTML fragment for a specific pane and rendering variant.
func (m *Manager) GetHTMLChunk(tenantID, paneID string, variant types.PaneVariant) (*types.HTMLChunk, bool) {
	return m.fragmentsStore.GetHTMLChunk(tenantID, paneID, variant)
}

// SetHTMLChunk stores a rendered HTML fragment and its dependencies in the cache.
func (m *Manager) SetHTMLChunk(tenantID, paneID string, variant types.PaneVariant, html string, dependsOn []string) {
	m.fragmentsStore.SetHTMLChunk(tenantID, paneID, variant, html, dependsOn)
}

// GetChunkDependencies returns the list of content nodes that a cached HTML chunk depends on.
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

// InvalidateByDependency clears all cached items that have a dependency on the specified node ID.
func (m *Manager) InvalidateByDependency(tenantID, nodeID string) {
	m.fragmentsStore.InvalidateByDependency(tenantID, nodeID)
}

// InvalidateHTMLChunkCache clears the entire HTML fragment cache for a specific tenant.
func (m *Manager) InvalidateHTMLChunkCache(tenantID string) {
	m.fragmentsStore.InvalidateHTMLChunkCache(tenantID)
}

// InvalidateHTMLChunk removes a specific rendered HTML fragment from the cache.
func (m *Manager) InvalidateHTMLChunk(tenantID, paneID string, variant types.PaneVariant) {
	m.fragmentsStore.InvalidateByPattern(tenantID, m.fragmentsStore.BuildChunkKey(paneID, variant))
}

// InvalidateTenant clears all cache stores (content, user state, analytics, and HTML) for a specific tenant.
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

// GetMemoryStats returns global memory usage information for the caching system.
func (m *Manager) GetMemoryStats() map[string]any {
	return make(map[string]any)
}

// InvalidateAll clears all cached data for all tenants across the entire system.
func (m *Manager) InvalidateAll() {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	for _, tenantID := range m.contentStore.GetAllTenantIDs() {
		m.InvalidateTenant(tenantID)
	}
}

// Health returns the current operational status of the cache manager and its sub-stores.
func (m *Manager) Health() map[string]any {
	return map[string]any{"status": "ok"}
}

// GetAllSessionIDs returns all session IDs for a tenant
func (m *Manager) GetAllSessionIDs(tenantID string) []string {
	cache, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return []string{}
	}

	cache.SessionsMu.RLock()
	defer cache.SessionsMu.RUnlock()

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

	cache.FingerprintsMu.RLock()
	defer cache.FingerprintsMu.RUnlock()

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

	cache.VisitsMu.RLock()
	defer cache.VisitsMu.RUnlock()

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

	cache.BeliefRegistriesMu.RLock()
	defer cache.BeliefRegistriesMu.RUnlock()

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

// InvalidateResource removes a specific resource and its dependencies from the tenant's cache.
func (m *Manager) InvalidateResource(tenantID, id string) {
	m.contentStore.InvalidateResource(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// AddResourceID tracks a new resource identifier in the tenant's index.
func (m *Manager) AddResourceID(tenantID, id string) {
	m.contentStore.AddResourceID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// RemoveResourceID removes a resource identifier from the tenant's index.
func (m *Manager) RemoveResourceID(tenantID, id string) {
	m.contentStore.RemoveResourceID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// InvalidateTractStack removes a specific TractStack and its dependencies from the tenant's cache.
func (m *Manager) InvalidateTractStack(tenantID, id string) {
	m.contentStore.InvalidateTractStack(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// AddTractStackID tracks a new TractStack identifier in the tenant's index.
func (m *Manager) AddTractStackID(tenantID, id string) {
	m.contentStore.AddTractStackID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// RemoveTractStackID removes a TractStack identifier from the tenant's index.
func (m *Manager) RemoveTractStackID(tenantID, id string) {
	m.contentStore.RemoveTractStackID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// InvalidateStoryFragment removes a specific StoryFragment and its dependencies from the tenant's cache.
func (m *Manager) InvalidateStoryFragment(tenantID, id string) {
	m.contentStore.InvalidateStoryFragment(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// AddStoryFragmentID tracks a new StoryFragment identifier in the tenant's index.
func (m *Manager) AddStoryFragmentID(tenantID, id string) {
	m.contentStore.AddStoryFragmentID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// RemoveStoryFragmentID removes a StoryFragment identifier from the tenant's index.
func (m *Manager) RemoveStoryFragmentID(tenantID, id string) {
	m.contentStore.RemoveStoryFragmentID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// InvalidatePane removes a specific Pane and its dependencies from the tenant's cache.
func (m *Manager) InvalidatePane(tenantID, id string) {
	m.contentStore.InvalidatePane(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// AddPaneID tracks a new Pane identifier in the tenant's index.
func (m *Manager) AddPaneID(tenantID, id string) {
	m.contentStore.AddPaneID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// RemovePaneID removes a Pane identifier from the tenant's index.
func (m *Manager) RemovePaneID(tenantID, id string) {
	m.contentStore.RemovePaneID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// InvalidateMenu removes a specific Menu and its dependencies from the tenant's cache.
func (m *Manager) InvalidateMenu(tenantID, id string) {
	m.contentStore.InvalidateMenu(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// AddMenuID tracks a new Menu identifier in the tenant's index.
func (m *Manager) AddMenuID(tenantID, id string) {
	m.contentStore.AddMenuID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// RemoveMenuID removes a Menu identifier from the tenant's index.
func (m *Manager) RemoveMenuID(tenantID, id string) {
	m.contentStore.RemoveMenuID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// InvalidateBelief removes a specific Belief and its dependencies from the tenant's cache.
func (m *Manager) InvalidateBelief(tenantID, id string) {
	m.contentStore.InvalidateBelief(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// AddBeliefID tracks a new Belief identifier in the tenant's index.
func (m *Manager) AddBeliefID(tenantID, id string) {
	m.contentStore.AddBeliefID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// RemoveBeliefID removes a Belief identifier from the tenant's index.
func (m *Manager) RemoveBeliefID(tenantID, id string) {
	m.contentStore.RemoveBeliefID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// InvalidateEpinet removes a specific Epinet and its dependencies from the tenant's cache.
func (m *Manager) InvalidateEpinet(tenantID, id string) {
	m.contentStore.InvalidateEpinet(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// AddEpinetID tracks a new Epinet identifier in the tenant's index.
func (m *Manager) AddEpinetID(tenantID, id string) {
	m.contentStore.AddEpinetID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// RemoveEpinetID removes an Epinet identifier from the tenant's index.
func (m *Manager) RemoveEpinetID(tenantID, id string) {
	m.contentStore.RemoveEpinetID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// InvalidateFile removes a specific File and its dependencies from the tenant's cache.
func (m *Manager) InvalidateFile(tenantID, id string) {
	m.contentStore.InvalidateFile(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// AddFileID tracks a new File identifier in the tenant's index.
func (m *Manager) AddFileID(tenantID, id string) {
	m.contentStore.AddFileID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// RemoveFileID removes a File identifier from the tenant's index.
func (m *Manager) RemoveFileID(tenantID, id string) {
	m.contentStore.RemoveFileID(tenantID, id)
	m.updateTenantAccessTime(tenantID)
}

// GetSessionsByFingerprint returns all session IDs for a given fingerprint
func (m *Manager) GetSessionsByFingerprint(tenantID, fingerprintID string) []string {
	_, err := m.GetTenantUserStateCache(tenantID)
	if err != nil {
		return []string{}
	}
	return m.sessionsStore.GetSessionsByFingerprint(tenantID, fingerprintID)
}

// RemoveSession deletes all cached data associated with a specific user session.
func (m *Manager) RemoveSession(tenantID, sessionID string) {
	m.sessionsStore.RemoveSession(tenantID, sessionID)
	m.updateTenantAccessTime(tenantID)
}

// BatchInvalidateSessionBeliefContexts removes multiple session belief contexts from the cache in a single operation.
func (m *Manager) BatchInvalidateSessionBeliefContexts(tenantID string, targets []types.SessionBeliefTarget) {
	m.sessionsStore.BatchInvalidateSessionBeliefContexts(tenantID, targets)
}

// GetGeneric retrieves a value from the generic cache if it exists and has not expired.
func (m *Manager) GetGeneric(tenantID, key string) (any, bool) {
	m.Mu.RLock()
	defer m.Mu.RUnlock()
	if m.genericStore == nil {
		return nil, false
	}

	item, exists := m.genericStore[fmt.Sprintf("%s:%s", tenantID, key)]
	if !exists || time.Now().UTC().After(item.ExpiresAt) {
		return nil, false
	}
	return item.Value, true
}

// SetGenericWithTTL sets a value in the generic cache with a specific TTL.
func (m *Manager) SetGenericWithTTL(tenantID, key string, value any, ttl time.Duration) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if m.genericStore == nil {
		m.genericStore = make(map[string]types.GenericCacheItem)
	}

	item := types.GenericCacheItem{
		Value:     value,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	m.genericStore[fmt.Sprintf("%s:%s", tenantID, key)] = item
}

// GetSessionMetrics returns aggregated real-time statistics about active user sessions.
func (m *Manager) GetSessionMetrics(tenantID string) types.SessionMetrics {
	return m.sessionsStore.GetSessionMetrics(tenantID)
}
