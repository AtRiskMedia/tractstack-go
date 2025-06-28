// Package models defines cache data structures for multi-tenant content and analytics.
package models

import (
	"sync"
	"time"
)

// CacheManager coordinates all tenant-isolated caches
type CacheManager struct {
	// All caches are tenant-isolated
	ContentCache   map[string]*TenantContentCache   // tenantId -> content
	UserStateCache map[string]*TenantUserStateCache // tenantId -> user states
	HTMLChunkCache map[string]*TenantHTMLChunkCache // tenantId -> html chunks
	AnalyticsCache map[string]*TenantAnalyticsCache // tenantId -> analytics

	// Cache metadata
	Mu           sync.RWMutex         // Exported for access
	LastAccessed map[string]time.Time // tenantId -> last access
}

// =============================================================================
// Content Cache Types
// =============================================================================

type TenantContentCache struct {
	// Core content nodes
	TractStacks    map[string]*TractStackNode    // id -> node
	StoryFragments map[string]*StoryFragmentNode // id -> node
	Panes          map[string]*PaneNode          // id -> node
	Menus          map[string]*MenuNode          // id -> node
	Resources      map[string]*ResourceNode      // id -> node
	Beliefs        map[string]*BeliefNode        // id -> node
	Files          map[string]*ImageFileNode     // id -> node

	// Lookup indices
	SlugToID      map[string]string   // slug -> id
	CategoryToIDs map[string][]string // category -> []id

	// ADD THIS LINE:
	AllPaneIDs []string // cached list of all pane IDs

	// Cache metadata
	LastUpdated time.Time
	Mu          sync.RWMutex // Exported for access
}

type TractStackNode struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Slug            string  `json:"slug"`
	SocialImagePath *string `json:"socialImagePath,omitempty"`
}

type StoryFragmentNode struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Slug             string     `json:"slug"`
	TractStackID     string     `json:"tractStackId"`
	MenuID           *string    `json:"menuId,omitempty"`
	PaneIDs          []string   `json:"paneIds"`
	TailwindBgColour *string    `json:"tailwindBgColour,omitempty"`
	SocialImagePath  *string    `json:"socialImagePath,omitempty"`
	Created          time.Time  `json:"created"`
	Changed          *time.Time `json:"changed,omitempty"`
}

type PaneNode struct {
	ID              string                 `json:"id"`
	Title           string                 `json:"title"`
	Slug            string                 `json:"slug"`
	IsContextPane   bool                   `json:"isContextPane"`
	IsDecorative    bool                   `json:"isDecorative"`
	OptionsPayload  map[string]any         `json:"optionsPayload,omitempty"`
	BgColour        *string                `json:"bgColour,omitempty"`
	CodeHookTarget  *string                `json:"codeHookTarget,omitempty"`
	CodeHookPayload map[string]string      `json:"codeHookPayload,omitempty"`
	HeldBeliefs     map[string]BeliefValue `json:"heldBeliefs,omitempty"`
	WithheldBeliefs map[string]BeliefValue `json:"withheldBeliefs,omitempty"`
	Created         time.Time              `json:"created"`
	Changed         *time.Time             `json:"changed,omitempty"`
}

type MenuNode struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Theme          string     `json:"theme"`
	OptionsPayload []MenuLink `json:"optionsPayload"`
}

type ResourceNode struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Slug           string         `json:"slug"`
	CategorySlug   *string        `json:"categorySlug,omitempty"`
	Oneliner       string         `json:"oneliner"`
	ActionLisp     *string        `json:"actionLisp,omitempty"`
	OptionsPayload map[string]any `json:"optionsPayload"`
}

type BeliefNode struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Slug         string   `json:"slug"`
	Scale        string   `json:"scale"`
	CustomValues []string `json:"customValues,omitempty"`
}

type ImageFileNode struct {
	ID             string  `json:"id"`
	Filename       string  `json:"filename"`
	AltDescription string  `json:"altDescription"`
	URL            string  `json:"url"`
	SrcSet         *string `json:"srcSet,omitempty"`
}

type BeliefValue struct {
	Verb   string  `json:"verb"`   // BELIEVES_YES, BELIEVES_NO, IDENTIFY_AS
	Object *string `json:"object"` // only used when verb=IDENTIFY_AS
}

type MenuLink struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Featured    bool   `json:"featured"`
	ActionLisp  string `json:"actionLisp"`
}

// =============================================================================
// User State Cache Types
// =============================================================================

type TenantUserStateCache struct {
	// Persistent user state by fingerprint
	FingerprintStates map[string]*FingerprintState // fingerprintId -> state

	// Current visit state by visit ID
	VisitStates map[string]*VisitState // visitId -> state

	// Known fingerprints (have lead_id)
	KnownFingerprints map[string]bool // fingerprintId -> isKnown

	// Session state cache (ephemeral)
	SessionStates map[string]*SessionData // sessionId -> session data

	// Cache metadata
	LastLoaded time.Time
	Mu         sync.RWMutex // Exported for access
}

type FingerprintState struct {
	FingerprintID string                 `json:"fingerprintId"`
	HeldBeliefs   map[string]BeliefValue `json:"heldBeliefs"` // beliefSlug -> value
	HeldBadges    map[string]string      `json:"heldBadges"`  // badgeSlug -> value
	LastActivity  time.Time              `json:"lastActivity"`
}

type VisitState struct {
	VisitID       string    `json:"visitId"`
	FingerprintID string    `json:"fingerprintId"`
	StartTime     time.Time `json:"startTime"`
	LastActivity  time.Time `json:"lastActivity"`
	CurrentPage   string    `json:"currentPage"`
	Referrer      *Referrer `json:"referrer,omitempty"`
}

type Referrer struct {
	HTTPReferrer *string `json:"httpReferrer,omitempty"`
	UTMSource    *string `json:"utmSource,omitempty"`
	UTMMedium    *string `json:"utmMedium,omitempty"`
	UTMCampaign  *string `json:"utmCampaign,omitempty"`
	UTMTerm      *string `json:"utmTerm,omitempty"`
	UTMContent   *string `json:"utmContent,omitempty"`
}

// =============================================================================
// HTML Chunk Cache Types
// =============================================================================

type TenantHTMLChunkCache struct {
	Chunks map[string]*HTMLChunk // "paneId:variant" -> chunk
	Deps   map[string][]string   // nodeId -> []cacheKeys
	Mu     sync.RWMutex          // Exported for access
}

type HTMLChunk struct {
	HTML      string    `json:"html"`
	CachedAt  time.Time `json:"cachedAt"`
	DependsOn []string  `json:"dependsOn"` // Content IDs this chunk depends on
}

const EmptyPaneHTML = `<div class="pane-empty"></div>`

// =============================================================================
// Analytics Cache Types
// =============================================================================

type TenantAnalyticsCache struct {
	// User journey analysis (epinets)
	EpinetBins map[string]*HourlyEpinetBin // "epinetId:hourKey" -> bin

	// Content performance analytics
	ContentBins map[string]*HourlyContentBin // "contentId:hourKey" -> bin

	// Site-wide analytics
	SiteBins map[string]*HourlySiteBin // "hourKey" -> bin

	// Computed metrics (shorter TTL)
	LeadMetrics   *LeadMetricsCache
	DashboardData *DashboardCache

	// Cache metadata
	LastFullHour string // Last processed hour key
	LastUpdated  time.Time
	Mu           sync.RWMutex // Exported for access
}

type HourlyEpinetBin struct {
	Data       *HourlyEpinetData `json:"data"`
	ComputedAt time.Time         `json:"computedAt"`
	TTL        time.Duration     `json:"ttl"`
}

type HourlyEpinetData struct {
	Steps       map[string]*HourlyEpinetStepData                  `json:"steps"`
	Transitions map[string]map[string]*HourlyEpinetTransitionData `json:"transitions"`
}

type HourlyEpinetStepData struct {
	Visitors  map[string]bool `json:"visitors"` // Set of visitor IDs
	Name      string          `json:"name"`
	StepIndex int             `json:"stepIndex"`
}

type HourlyEpinetTransitionData struct {
	Visitors map[string]bool `json:"visitors"` // Set of visitor IDs
}

type HourlyContentBin struct {
	Data       *HourlyContentData `json:"data"`
	ComputedAt time.Time          `json:"computedAt"`
	TTL        time.Duration      `json:"ttl"`
}

type HourlyContentData struct {
	UniqueVisitors    map[string]bool `json:"uniqueVisitors"`    // Set of visitor IDs
	KnownVisitors     map[string]bool `json:"knownVisitors"`     // Set of known visitor IDs
	AnonymousVisitors map[string]bool `json:"anonymousVisitors"` // Set of anonymous visitor IDs
	Actions           int             `json:"actions"`
	EventCounts       map[string]int  `json:"eventCounts"` // eventType -> count
}

type HourlySiteBin struct {
	Data       *HourlySiteData `json:"data"`
	ComputedAt time.Time       `json:"computedAt"`
	TTL        time.Duration   `json:"ttl"`
}

type HourlySiteData struct {
	TotalVisits       int             `json:"totalVisits"`
	KnownVisitors     map[string]bool `json:"knownVisitors"`     // Set of known visitor IDs
	AnonymousVisitors map[string]bool `json:"anonymousVisitors"` // Set of anonymous visitor IDs
	EventCounts       map[string]int  `json:"eventCounts"`       // eventType -> count
}

type LeadMetricsCache struct {
	Data       *LeadMetrics  `json:"data"`
	ComputedAt time.Time     `json:"computedAt"`
	TTL        time.Duration `json:"ttl"`
}

type LeadMetrics struct {
	TotalVisits            int     `json:"totalVisits"`
	LastActivity           string  `json:"lastActivity"`
	FirstTime24h           int     `json:"firstTime24h"`
	Returning24h           int     `json:"returning24h"`
	FirstTime7d            int     `json:"firstTime7d"`
	Returning7d            int     `json:"returning7d"`
	FirstTime28d           int     `json:"firstTime28d"`
	Returning28d           int     `json:"returning28d"`
	FirstTime24hPercentage float64 `json:"firstTime24hPercentage"`
	Returning24hPercentage float64 `json:"returning24hPercentage"`
	FirstTime7dPercentage  float64 `json:"firstTime7dPercentage"`
	Returning7dPercentage  float64 `json:"returning7dPercentage"`
	FirstTime28dPercentage float64 `json:"firstTime28dPercentage"`
	Returning28dPercentage float64 `json:"returning28dPercentage"`
	TotalLeads             int     `json:"totalLeads"`
}

type DashboardCache struct {
	Data       *DashboardAnalytics `json:"data"`
	ComputedAt time.Time           `json:"computedAt"`
	TTL        time.Duration       `json:"ttl"`
}

type DashboardAnalytics struct {
	Stats      TimeRangeStats   `json:"stats"`
	Line       []LineDataSeries `json:"line"`
	HotContent []HotItem        `json:"hotContent"`
}

type TimeRangeStats struct {
	Daily   int `json:"daily"`
	Weekly  int `json:"weekly"`
	Monthly int `json:"monthly"`
}

type LineDataSeries struct {
	ID   string          `json:"id"`
	Data []LineDataPoint `json:"data"`
}

type LineDataPoint struct {
	X any `json:"x"` // string or number
	Y int `json:"y"`
}

type HotItem struct {
	ID          string `json:"id"`
	TotalEvents int    `json:"totalEvents"`
}

// =============================================================================
// Cache Helper Types
// =============================================================================

type CacheKey struct {
	TenantID string
	Type     string
	ID       string
	Variant  string
}

type CacheStats struct {
	Hits   int   `json:"hits"`
	Misses int   `json:"misses"`
	Size   int64 `json:"size"`
}

type CacheLock struct {
	AcquiredAt time.Time
	ExpiresAt  time.Time
	TenantID   string
	Key        string
}

// CacheTTL represents a cache time-to-live duration
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

// Duration returns the TTL as a time.Duration
func (ttl CacheTTL) Duration() time.Duration {
	return time.Duration(ttl)
}

// String returns a human-readable representation of the TTL
func (ttl CacheTTL) String() string {
	return time.Duration(ttl).String()
}
