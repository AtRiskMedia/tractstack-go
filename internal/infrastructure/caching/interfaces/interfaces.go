// Package interfaces defines cache operation contracts for multi-tenant content management.
package interfaces

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// ContentCache defines operations for content caching
type ContentCache interface {
	// TractStack operations
	GetTractStack(tenantID, id string) (*content.TractStackNode, bool)
	SetTractStack(tenantID string, tractStack *content.TractStackNode)
	GetAllTractStackIDs(tenantID string) ([]string, bool)

	// StoryFragment operations
	GetStoryFragment(tenantID, id string) (*content.StoryFragmentNode, bool)
	SetStoryFragment(tenantID string, storyFragment *content.StoryFragmentNode)
	GetAllStoryFragmentIDs(tenantID string) ([]string, bool)

	// Pane operations
	GetPane(tenantID, id string) (*content.PaneNode, bool)
	SetPane(tenantID string, pane *content.PaneNode)
	GetAllPaneIDs(tenantID string) ([]string, bool)

	// Menu operations
	GetMenu(tenantID, id string) (*content.MenuNode, bool)
	SetMenu(tenantID string, menu *content.MenuNode)
	GetAllMenuIDs(tenantID string) ([]string, bool)

	// Resource operations
	GetResource(tenantID, id string) (*content.ResourceNode, bool)
	SetResource(tenantID string, resource *content.ResourceNode)
	GetAllResourceIDs(tenantID string) ([]string, bool)

	// Belief operations
	GetBelief(tenantID, id string) (*content.BeliefNode, bool)
	SetBelief(tenantID string, belief *content.BeliefNode)
	GetAllBeliefIDs(tenantID string) ([]string, bool)

	// Epinet operations
	GetEpinet(tenantID, id string) (*content.EpinetNode, bool)
	SetEpinet(tenantID string, epinet *content.EpinetNode)
	GetAllEpinetIDs(tenantID string) ([]string, bool)

	// ImageFile operations
	GetFile(tenantID, id string) (*content.ImageFileNode, bool)
	SetFile(tenantID string, file *content.ImageFileNode)
	GetAllFileIDs(tenantID string) ([]string, bool)

	// Lookup operations
	GetContentBySlug(tenantID, slug string) (string, bool)             // returns ID
	GetResourcesByCategory(tenantID, category string) ([]string, bool) // returns IDs

	// Content map operations
	GetFullContentMap(tenantID string) ([]types.FullContentMapItem, bool)
	SetFullContentMap(tenantID string, contentMap []types.FullContentMapItem)

	// Orphan analysis operations
	GetOrphanAnalysis(tenantID string) (*types.OrphanAnalysisPayload, string, bool) // payload, etag, exists
	SetOrphanAnalysis(tenantID string, payload *types.OrphanAnalysisPayload, etag string)

	// Cache management
	InvalidateContentCache(tenantID string)
}

// UserStateCache defines operations for user state caching
type UserStateCache interface {
	// Visit operations
	GetVisitState(tenantID, visitID string) (*types.VisitState, bool)
	SetVisitState(tenantID string, state *types.VisitState)

	// Fingerprint operations
	GetFingerprintState(tenantID, fingerprintID string) (*types.FingerprintState, bool)
	SetFingerprintState(tenantID string, state *types.FingerprintState)
	IsKnownFingerprint(tenantID, fingerprintID string) bool
	SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool)
	LoadKnownFingerprints(tenantID string, fingerprints map[string]bool)

	// Session operations
	GetSession(tenantID, sessionID string) (*types.SessionData, bool)
	SetSession(tenantID string, sessionData *types.SessionData)

	// Belief registry operations
	GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*types.StoryfragmentBeliefRegistry, bool)
	SetStoryfragmentBeliefRegistry(tenantID string, registry *types.StoryfragmentBeliefRegistry)
	InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string)

	// Session belief context operations
	GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*types.SessionBeliefContext, bool)
	SetSessionBeliefContext(tenantID string, context *types.SessionBeliefContext)
	InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string)

	// Cache management
	InvalidateUserStateCache(tenantID string)
}

// HTMLChunkCache defines operations for HTML fragment caching
type HTMLChunkCache interface {
	// HTML chunk operations
	GetHTMLChunk(tenantID, paneID string, variant types.PaneVariant) (*types.HTMLChunk, bool)
	SetHTMLChunk(tenantID, paneID string, variant types.PaneVariant, html string, dependsOn []string)

	// Dependency tracking
	GetChunkDependencies(tenantID, nodeID string) ([]string, bool)
	InvalidateByDependency(tenantID, nodeID string)

	// Cache management
	InvalidateHTMLChunkCache(tenantID string)
	InvalidateHTMLChunk(tenantID, paneID string, variant types.PaneVariant)
}

// AnalyticsCache defines operations for analytics caching
type AnalyticsCache interface {
	// Epinet analytics operations
	GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*types.HourlyEpinetBin, bool)
	SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *types.HourlyEpinetBin)

	// Content analytics operations
	GetHourlyContentBin(tenantID, contentID, hourKey string) (*types.HourlyContentBin, bool)
	SetHourlyContentBin(tenantID, contentID, hourKey string, bin *types.HourlyContentBin)

	// Site analytics operations
	GetHourlySiteBin(tenantID, hourKey string) (*types.HourlySiteBin, bool)
	SetHourlySiteBin(tenantID, hourKey string, bin *types.HourlySiteBin)

	// Computed metrics operations
	GetLeadMetrics(tenantID string) (*types.LeadMetricsCache, bool)
	SetLeadMetrics(tenantID string, metrics *types.LeadMetricsCache)

	GetDashboardData(tenantID string) (*types.DashboardCache, bool)
	SetDashboardData(tenantID string, data *types.DashboardCache)

	// Batch operations
	GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*types.HourlyEpinetBin, []string)
	PurgeExpiredBins(tenantID string, olderThan string)

	// Cache management
	InvalidateAnalyticsCache(tenantID string)
	UpdateLastFullHour(tenantID, hourKey string)
}

// Cache is the main interface that combines all cache operations
type Cache interface {
	ContentCache
	UserStateCache
	HTMLChunkCache
	AnalyticsCache

	// Tenant management
	InvalidateTenant(tenantID string)
	GetTenantStats(tenantID string) CacheStats

	// Cache management
	GetMemoryStats() map[string]any
	InvalidateAll()
	Health() map[string]any
}

// ReadOnlyAnalyticsCache prevents analytics service from writing to cache
type ReadOnlyAnalyticsCache interface {
	// Epinet analytics operations (read-only)
	GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*types.HourlyEpinetBin, bool)
	GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*types.HourlyEpinetBin, []string)

	// Content analytics operations (read-only)
	GetHourlyContentBin(tenantID, contentID, hourKey string) (*types.HourlyContentBin, bool)

	// Site analytics operations (read-only)
	GetHourlySiteBin(tenantID, hourKey string) (*types.HourlySiteBin, bool)

	// Computed metrics operations (read-only)
	GetLeadMetrics(tenantID string) (*types.LeadMetricsCache, bool)
	GetDashboardData(tenantID string) (*types.DashboardCache, bool)
}

// WriteOnlyAnalyticsCache prevents cache warmer from reading during computation
type WriteOnlyAnalyticsCache interface {
	// Epinet analytics operations (write-only)
	SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *types.HourlyEpinetBin)

	// Content analytics operations (write-only)
	SetHourlyContentBin(tenantID, contentID, hourKey string, bin *types.HourlyContentBin)

	// Site analytics operations (write-only)
	SetHourlySiteBin(tenantID, hourKey string, bin *types.HourlySiteBin)

	// Computed metrics operations (write-only)
	SetLeadMetrics(tenantID string, metrics *types.LeadMetricsCache)
	SetDashboardData(tenantID string, data *types.DashboardCache)

	// Batch operations (write-only)
	PurgeExpiredBins(tenantID string, olderThan string)

	// Utility operations (write-only)
	InvalidateAnalyticsCache(tenantID string)
	UpdateLastFullHour(tenantID, hourKey string)
}

type CacheStats struct {
	Hits   int   `json:"hits"`
	Misses int   `json:"misses"`
	Size   int64 `json:"size"`
}

type CacheTTL time.Duration

// Common TTL values
const (
	TTLNever    CacheTTL = CacheTTL(0)
	TTL1Minute  CacheTTL = CacheTTL(time.Minute)
	TTL5Minutes CacheTTL = CacheTTL(5 * time.Minute)
	TTL1Hour    CacheTTL = CacheTTL(time.Hour)
	TTL24Hours  CacheTTL = CacheTTL(24 * time.Hour)
	TTL7Days    CacheTTL = CacheTTL(7 * 24 * time.Hour)
	TTL28Days   CacheTTL = CacheTTL(28 * 24 * time.Hour)
)
