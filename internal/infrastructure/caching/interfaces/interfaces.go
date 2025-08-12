// Package interfaces defines cache operation contracts for multi-tenant content management.
package interfaces

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// ContentCache defines operations for content caching
type ContentCache interface {
	GetTractStack(tenantID, id string) (*content.TractStackNode, bool)
	SetTractStack(tenantID string, tractStack *content.TractStackNode)
	GetAllTractStackIDs(tenantID string) ([]string, bool)
	SetAllTractStackIDs(tenantID string, ids []string)
	GetStoryFragment(tenantID, id string) (*content.StoryFragmentNode, bool)
	SetStoryFragment(tenantID string, storyFragment *content.StoryFragmentNode)
	GetAllStoryFragmentIDs(tenantID string) ([]string, bool)
	SetAllStoryFragmentIDs(tenantID string, ids []string)
	GetPane(tenantID, id string) (*content.PaneNode, bool)
	SetPane(tenantID string, pane *content.PaneNode)
	GetAllPaneIDs(tenantID string) ([]string, bool)
	SetAllPaneIDs(tenantID string, ids []string)
	GetMenu(tenantID, id string) (*content.MenuNode, bool)
	SetMenu(tenantID string, menu *content.MenuNode)
	GetAllMenuIDs(tenantID string) ([]string, bool)
	SetAllMenuIDs(tenantID string, ids []string)
	GetResource(tenantID, id string) (*content.ResourceNode, bool)
	SetResource(tenantID string, resource *content.ResourceNode)
	GetAllResourceIDs(tenantID string) ([]string, bool)
	SetAllResourceIDs(tenantID string, ids []string)
	GetBelief(tenantID, id string) (*content.BeliefNode, bool)
	SetBelief(tenantID string, belief *content.BeliefNode)
	GetAllBeliefIDs(tenantID string) ([]string, bool)
	SetAllBeliefIDs(tenantID string, ids []string)
	GetEpinet(tenantID, id string) (*content.EpinetNode, bool)
	SetEpinet(tenantID string, epinet *content.EpinetNode)
	GetAllEpinetIDs(tenantID string) ([]string, bool)
	SetAllEpinetIDs(tenantID string, ids []string)
	GetFile(tenantID, id string) (*content.ImageFileNode, bool)
	SetFile(tenantID string, file *content.ImageFileNode)
	GetAllFileIDs(tenantID string) ([]string, bool)
	SetAllFileIDs(tenantID string, ids []string)
	GetContentBySlug(tenantID, slug string) (string, bool)
	GetResourcesByCategory(tenantID, category string) ([]string, bool)
	GetFullContentMap(tenantID string) ([]types.FullContentMapItem, bool)
	SetFullContentMap(tenantID string, contentMap []types.FullContentMapItem)
	GetOrphanAnalysis(tenantID string) (*types.OrphanAnalysisPayload, string, bool)
	SetOrphanAnalysis(tenantID string, payload *types.OrphanAnalysisPayload, etag string)
	InvalidateContentCache(tenantID string)
	InvalidateFullContentMap(tenantID string)
	InvalidateResource(tenantID, id string)
	AddResourceID(tenantID, id string)
	RemoveResourceID(tenantID, id string)
	InvalidateTractStack(tenantID, id string)
	AddTractStackID(tenantID, id string)
	RemoveTractStackID(tenantID, id string)
	InvalidateStoryFragment(tenantID, id string)
	AddStoryFragmentID(tenantID, id string)
	RemoveStoryFragmentID(tenantID, id string)
	InvalidatePane(tenantID, id string)
	AddPaneID(tenantID, id string)
	RemovePaneID(tenantID, id string)
	InvalidateMenu(tenantID, id string)
	AddMenuID(tenantID, id string)
	RemoveMenuID(tenantID, id string)
	InvalidateBelief(tenantID, id string)
	AddBeliefID(tenantID, id string)
	RemoveBeliefID(tenantID, id string)
	InvalidateEpinet(tenantID, id string)
	AddEpinetID(tenantID, id string)
	RemoveEpinetID(tenantID, id string)
	InvalidateFile(tenantID, id string)
	AddFileID(tenantID, id string)
	RemoveFileID(tenantID, id string)
}

// UserStateCache defines operations for user state caching
type UserStateCache interface {
	GetVisitState(tenantID, visitID string) (*types.VisitState, bool)
	SetVisitState(tenantID string, state *types.VisitState)
	GetFingerprintState(tenantID, fingerprintID string) (*types.FingerprintState, bool)
	SetFingerprintState(tenantID string, state *types.FingerprintState)
	IsKnownFingerprint(tenantID, fingerprintID string) bool
	SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool)
	LoadKnownFingerprints(tenantID string, fingerprints map[string]bool)
	GetSession(tenantID, sessionID string) (*types.SessionData, bool)
	SetSession(tenantID string, sessionData *types.SessionData)
	RemoveSession(tenantID, sessionID string)
	GetSessionsByFingerprint(tenantID, fingerprintID string) []string
	GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*types.StoryfragmentBeliefRegistry, bool)
	SetStoryfragmentBeliefRegistry(tenantID string, registry *types.StoryfragmentBeliefRegistry)
	InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string)
	GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*types.SessionBeliefContext, bool)
	SetSessionBeliefContext(tenantID string, context *types.SessionBeliefContext)
	InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string)
	InvalidateUserStateCache(tenantID string)
	GetAllSessionIDs(tenantID string) []string
	GetAllFingerprintIDs(tenantID string) []string
	GetAllVisitIDs(tenantID string) []string
	GetAllStoryfragmentBeliefRegistryIDs(tenantID string) []string
}

// HTMLChunkCache defines operations for HTML fragment caching
type HTMLChunkCache interface {
	GetHTMLChunk(tenantID, paneID string, variant types.PaneVariant) (*types.HTMLChunk, bool)
	SetHTMLChunk(tenantID, paneID string, variant types.PaneVariant, html string, dependsOn []string)
	GetChunkDependencies(tenantID, nodeID string) ([]string, bool)
	InvalidateByDependency(tenantID, nodeID string)
	InvalidateHTMLChunkCache(tenantID string)
	InvalidateHTMLChunk(tenantID, paneID string, variant types.PaneVariant)
	GetAllHTMLChunkIDs(tenantID string) []string
}

// AnalyticsCache defines operations for analytics caching
type AnalyticsCache interface {
	GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*types.HourlyEpinetBin, bool)
	SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *types.HourlyEpinetBin)
	GetHourlyContentBin(tenantID, contentID, hourKey string) (*types.HourlyContentBin, bool)
	SetHourlyContentBin(tenantID, contentID, hourKey string, bin *types.HourlyContentBin)
	GetHourlySiteBin(tenantID, hourKey string) (*types.HourlySiteBin, bool)
	SetHourlySiteBin(tenantID, hourKey string, bin *types.HourlySiteBin)
	GetLeadMetricsWithETag(tenantID, cacheKey string) (*types.LeadMetricsData, string, bool)
	SetLeadMetricsWithETag(tenantID, cacheKey string, data *types.LeadMetricsData, etag string)
	GetDashboardDataWithETag(tenantID, cacheKey string) (*types.DashboardData, string, bool)
	SetDashboardDataWithETag(tenantID, cacheKey string, data *types.DashboardData, etag string)
	GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*types.HourlyEpinetBin, []string)
	PurgeExpiredBins(tenantID string, olderThan string)
	InvalidateAnalyticsCache(tenantID string)
	UpdateLastFullHour(tenantID, hourKey string)
}

// Cache is the main interface that combines all cache operations
type Cache interface {
	ContentCache
	UserStateCache
	HTMLChunkCache
	AnalyticsCache
	InvalidateTenant(tenantID string)
	GetTenantStats(tenantID string) CacheStats
	GetMemoryStats() map[string]any
	InvalidateAll()
	Health() map[string]any
}

// ReadOnlyAnalyticsCache prevents analytics service from writing to cache
type ReadOnlyAnalyticsCache interface {
	GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*types.HourlyEpinetBin, bool)
	GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*types.HourlyEpinetBin, []string)
	GetHourlyContentBin(tenantID, contentID, hourKey string) (*types.HourlyContentBin, bool)
	GetHourlySiteBin(tenantID, hourKey string) (*types.HourlySiteBin, bool)
	GetLeadMetrics(tenantID string) (*types.LeadMetricsCache, bool)
	GetDashboardData(tenantID string) (*types.DashboardCache, bool)
}

// WriteOnlyAnalyticsCache prevents cache warmer from reading during computation
type WriteOnlyAnalyticsCache interface {
	SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *types.HourlyEpinetBin)
	SetHourlyContentBin(tenantID, contentID, hourKey string, bin *types.HourlyContentBin)
	SetHourlySiteBin(tenantID, hourKey string, bin *types.HourlySiteBin)
	SetLeadMetrics(tenantID string, metrics *types.LeadMetricsCache)
	SetDashboardData(tenantID string, data *types.DashboardCache)
	PurgeExpiredBins(tenantID string, olderThan string)
	InvalidateAnalyticsCache(tenantID string)
	UpdateLastFullHour(tenantID, hourKey string)
}

type CacheStats struct {
	Hits   int   `json:"hits"`
	Misses int   `json:"misses"`
	Size   int64 `json:"size"`
}

type CacheTTL time.Duration

const (
	TTLNever    CacheTTL = CacheTTL(0)
	TTL1Minute  CacheTTL = CacheTTL(time.Minute)
	TTL5Minutes CacheTTL = CacheTTL(5 * time.Minute)
	TTL1Hour    CacheTTL = CacheTTL(time.Hour)
	TTL24Hours  CacheTTL = CacheTTL(24 * time.Hour)
	TTL7Days    CacheTTL = CacheTTL(7 * 24 * time.Hour)
	TTL28Days   CacheTTL = CacheTTL(28 * 24 * time.Hour)
)
