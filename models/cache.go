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
	ID              string              `json:"id"`
	Title           string              `json:"title"`
	Slug            string              `json:"slug"`
	IsContextPane   bool                `json:"isContextPane"`
	IsDecorative    bool                `json:"isDecorative"`
	OptionsPayload  map[string]any      `json:"optionsPayload,omitempty"`
	BgColour        *string             `json:"bgColour,omitempty"`
	CodeHookTarget  *string             `json:"codeHookTarget,omitempty"`
	CodeHookPayload map[string]string   `json:"codeHookPayload,omitempty"`
	HeldBeliefs     map[string][]string `json:"heldBeliefs,omitempty"`     // CHANGED FROM BeliefValue
	WithheldBeliefs map[string][]string `json:"withheldBeliefs,omitempty"` // CHANGED FROM BeliefValue
	Created         time.Time           `json:"created"`
	Changed         *time.Time          `json:"changed,omitempty"`
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
	SessionStates                 map[string]*SessionData                 // sessionId -> session data
	StoryfragmentBeliefRegistries map[string]*StoryfragmentBeliefRegistry // storyfragmentId -> belief registry
	SessionBeliefContexts         map[string]*SessionBeliefContext        // "sessionId:storyfragmentId" -> context

	// Cache metadata
	LastLoaded time.Time
	Mu         sync.RWMutex // Exported for access
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

// StoryfragmentBeliefRegistry stores extracted belief requirements per storyfragment
type StoryfragmentBeliefRegistry struct {
	StoryfragmentID    string                    `json:"storyfragmentId"`
	PaneBeliefPayloads map[string]PaneBeliefData `json:"paneBeliefPayloads"` // paneId -> belief data
	RequiredBeliefs    map[string]bool           `json:"requiredBeliefs"`    // flat list for lookup
	RequiredBadges     []string                  `json:"requiredBadges"`     // badge requirements
	PaneWidgetBeliefs  map[string][]string       `json:"paneWidgetBeliefs"`  // paneId -> belief slugs used by widgets
	AllWidgetBeliefs   map[string]bool           `json:"allWidgetBeliefs"`   // flat lookup for all widget beliefs
	LastUpdated        time.Time                 `json:"lastUpdated"`
}

// PaneBeliefData represents extracted belief data from a single pane
type PaneBeliefData struct {
	HeldBeliefs     map[string][]string `json:"heldBeliefs"`     // standard belief matching
	WithheldBeliefs map[string][]string `json:"withheldBeliefs"` // standard belief matching
	MatchAcross     []string            `json:"matchAcross"`     // OR logic - separate processing
	LinkedBeliefs   []string            `json:"linkedBeliefs"`   // cascade unset - separate processing
	HeldBadges      []string            `json:"heldBadges"`      // if implemented
}

// =========================================================================
// SESSION BELIEF CONTEXT MODEL
// =========================================================================

// SessionBeliefContext tracks session-specific belief state for personalization
type SessionBeliefContext struct {
	TenantID        string              `json:"tenantId"`
	SessionID       string              `json:"sessionId"`
	StoryfragmentID string              `json:"storyfragmentId"`
	UserBeliefs     map[string][]string `json:"userBeliefs"`
	LastEvaluation  time.Time           `json:"lastEvaluation"`
}
