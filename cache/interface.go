// Package cache defines interfaces for cache operations across different cache types.
package cache

import (
	"github.com/AtRiskMedia/tractstack-go/models"
)

// ContentCache defines operations for content caching
type ContentCache interface {
	// TractStack operations
	GetTractStack(tenantID, id string) (*models.TractStackNode, bool)
	SetTractStack(tenantID string, node *models.TractStackNode)
	GetTractStackBySlug(tenantID, slug string) (*models.TractStackNode, bool)

	// StoryFragment operations
	GetStoryFragment(tenantID, id string) (*models.StoryFragmentNode, bool)
	SetStoryFragment(tenantID string, node *models.StoryFragmentNode)
	GetStoryFragmentBySlug(tenantID, slug string) (*models.StoryFragmentNode, bool)

	// Session belief context operations
	GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*models.SessionBeliefContext, bool)
	SetSessionBeliefContext(tenantID string, context *models.SessionBeliefContext)
	InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string)

	// Menu operations
	GetMenu(tenantID, id string) (*models.MenuNode, bool)
	SetMenu(tenantID string, node *models.MenuNode)

	// Resource operations
	GetResource(tenantID, id string) (*models.ResourceNode, bool)
	SetResource(tenantID string, node *models.ResourceNode)
	GetResourceBySlug(tenantID, slug string) (*models.ResourceNode, bool)
	GetResourcesByCategory(tenantID, category string) ([]*models.ResourceNode, bool)

	// Epinet operations
	GetEpinet(tenantID, id string) (*models.EpinetNode, bool)
	SetEpinet(tenantID string, node *models.EpinetNode)
	GetAllEpinetIDs(tenantID string) ([]string, bool)

	// Belief operations
	GetBelief(tenantID, id string) (*models.BeliefNode, bool)
	SetBelief(tenantID string, node *models.BeliefNode)
	GetBeliefBySlug(tenantID, slug string) (*models.BeliefNode, bool)
	GetBeliefIDBySlug(tenantID, slug string) (string, bool)

	// File operations
	GetFile(tenantID, id string) (*models.ImageFileNode, bool)
	SetFile(tenantID string, node *models.ImageFileNode)

	// Pane operations
	GetPane(tenantID, id string) (*models.PaneNode, bool)
	SetPane(tenantID string, node *models.PaneNode)
	GetPaneBySlug(tenantID, slug string) (*models.PaneNode, bool)
	GetAllPaneIDs(tenantID string) ([]string, bool)
	SetAllPaneIDs(tenantID string, ids []string)
	InvalidatePane(tenantID, id string) // For when pane is altered
	InvalidateAllPanes(tenantID string) // For bulk invalidation
}

// UserStateCache defines operations for user state caching
type UserStateCache interface {
	// Fingerprint state operations
	GetFingerprintState(tenantID, fingerprintID string) (*models.FingerprintState, bool)
	SetFingerprintState(tenantID string, state *models.FingerprintState)

	// Visit state operations
	GetVisitState(tenantID, visitID string) (*models.VisitState, bool)
	SetVisitState(tenantID string, state *models.VisitState)

	// Known fingerprint operations
	IsKnownFingerprint(tenantID, fingerprintID string) bool
	SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool)

	// Batch operations
	LoadKnownFingerprints(tenantID string, fingerprints map[string]bool)

	// Belief registry operations
	GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*models.StoryfragmentBeliefRegistry, bool)
	SetStoryfragmentBeliefRegistry(tenantID string, registry *models.StoryfragmentBeliefRegistry)
	InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string)

	// GetAllStoryfragmentBeliefRegistryIDs returns all storyfragment IDs that have cached belief registries
	GetAllStoryfragmentBeliefRegistryIDs(tenantID string) []string
}

// HTMLChunkCache defines operations for HTML chunk caching
type HTMLChunkCache interface {
	// HTML chunk operations
	GetHTMLChunk(tenantID, paneID string, variant models.PaneVariant) (string, bool)
	SetHTMLChunk(tenantID, paneID string, variant models.PaneVariant, html string, dependsOn []string)

	// Invalidation operations
	InvalidateHTMLChunk(tenantID, nodeID string)
	InvalidatePattern(tenantID, pattern string)
}

// AnalyticsCache defines operations for analytics caching
type AnalyticsCache interface {
	// Epinet analytics operations
	GetHourlyEpinetBin(tenantID, epinetID, hourKey string) (*models.HourlyEpinetBin, bool)
	SetHourlyEpinetBin(tenantID, epinetID, hourKey string, bin *models.HourlyEpinetBin)

	// Content analytics operations
	GetHourlyContentBin(tenantID, contentID, hourKey string) (*models.HourlyContentBin, bool)
	SetHourlyContentBin(tenantID, contentID, hourKey string, bin *models.HourlyContentBin)

	// Site analytics operations
	GetHourlySiteBin(tenantID, hourKey string) (*models.HourlySiteBin, bool)
	SetHourlySiteBin(tenantID, hourKey string, bin *models.HourlySiteBin)

	// Computed metrics operations
	GetLeadMetrics(tenantID string) (*models.LeadMetricsCache, bool)
	SetLeadMetrics(tenantID string, metrics *models.LeadMetricsCache)

	GetDashboardData(tenantID string) (*models.DashboardCache, bool)
	SetDashboardData(tenantID string, data *models.DashboardCache)

	// Batch operations
	GetHourlyEpinetRange(tenantID, epinetID string, hourKeys []string) (map[string]*models.HourlyEpinetBin, []string)
	PurgeExpiredBins(tenantID string, olderThan string)
}

// Cache is the main interface that combines all cache operations
type Cache interface {
	ContentCache
	UserStateCache
	HTMLChunkCache
	AnalyticsCache

	// Tenant management
	EnsureTenant(tenantID string)
	InvalidateTenant(tenantID string)
	GetTenantStats(tenantID string) models.CacheStats

	// Cache management
	GetMemoryStats() map[string]any
	InvalidateAll()
	Health() map[string]any
}

// CacheProvider defines the interface for cache implementations
type CacheProvider interface {
	// Core cache instance
	GetCache() Cache

	// Lifecycle management
	Start() error
	Stop() error
	Health() map[string]any

	// Configuration
	Configure(config map[string]any) error
}

// CacheMiddleware defines the interface for cache middleware
type CacheMiddleware interface {
	// Request interception
	BeforeGet(tenantID, key string) bool
	AfterGet(tenantID, key string, hit bool)
	BeforeSet(tenantID, key string, data any) bool
	AfterSet(tenantID, key string, success bool)

	// Statistics and monitoring
	GetStats() map[string]any
	Reset()
}

// CacheObserver defines the interface for cache event observation
type CacheObserver interface {
	OnCacheHit(tenantID, cacheType, key string)
	OnCacheMiss(tenantID, cacheType, key string)
	OnCacheSet(tenantID, cacheType, key string)
	OnCacheInvalidate(tenantID, cacheType, key string)
	OnTenantCreate(tenantID string)
	OnTenantRemove(tenantID string)
}

// Validator defines the interface for cache data validation
type Validator interface {
	ValidateContent(content any) error
	ValidateUserState(state any) error
	ValidateAnalytics(analytics any) error
	ValidateTenantID(tenantID string) error
}

// Serializer defines the interface for cache serialization
type Serializer interface {
	Serialize(data any) ([]byte, error)
	Deserialize(data []byte, target any) error
	ContentType() string
}

// CacheStrategy defines different caching strategies
type CacheStrategy interface {
	ShouldCache(tenantID, key string, data any) bool
	GetTTL(tenantID, key string, data any) models.CacheTTL
	GetEvictionPolicy() EvictionPolicy
}

// EvictionPolicy defines cache eviction policies
type EvictionPolicy interface {
	ShouldEvict(tenantID string, stats models.CacheStats) []string
	GetMaxSize() int64
	GetMaxAge() models.CacheTTL
}

// CacheConfig holds configuration for cache implementations
type CacheConfig struct {
	// Size limits
	MaxTenants    int   `json:"maxTenants"`
	MaxMemoryMB   int64 `json:"maxMemoryMB"`
	MaxItemsTotal int64 `json:"maxItemsTotal"`

	// TTL settings
	DefaultTTL   models.CacheTTL `json:"defaultTTL"`
	ContentTTL   models.CacheTTL `json:"contentTTL"`
	UserStateTTL models.CacheTTL `json:"userStateTTL"`
	AnalyticsTTL models.CacheTTL `json:"analyticsTTL"`

	// Cleanup settings
	CleanupInterval models.CacheTTL `json:"cleanupInterval"`
	TenantTimeout   models.CacheTTL `json:"tenantTimeout"`

	// Performance settings
	EnableCompression bool `json:"enableCompression"`
	EnableMetrics     bool `json:"enableMetrics"`
	EnableObservers   bool `json:"enableObservers"`

	// Strategy settings
	EvictionPolicy string `json:"evictionPolicy"` // "lru", "lfu", "ttl"
	CacheStrategy  string `json:"cacheStrategy"`  // "aggressive", "conservative", "adaptive"
}
