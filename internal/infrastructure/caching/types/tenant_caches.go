// Package types defines cache data structures for multi-tenant content and analytics.
package types

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
)

// TenantContentCache holds all content nodes for a single tenant
type TenantContentCache struct {
	TractStacks    map[string]*content.TractStackNode    // id -> node
	StoryFragments map[string]*content.StoryFragmentNode // id -> node
	Panes          map[string]*content.PaneNode          // id -> node
	Menus          map[string]*content.MenuNode          // id -> node
	Resources      map[string]*content.ResourceNode      // id -> node
	Epinets        map[string]*content.EpinetNode        // id -> node
	Beliefs        map[string]*content.BeliefNode        // id -> node
	Files          map[string]*content.ImageFileNode     // id -> node

	StoryfragmentBeliefRegistries map[string]*StoryfragmentBeliefRegistry // storyfragmentId -> belief registry

	// Lookup indices
	SlugToID      map[string]string   // slug -> id
	CategoryToIDs map[string][]string // category -> []id
	AllPaneIDs    []string            // cached list of all pane IDs

	// Content map cache
	FullContentMap        []FullContentMapItem `json:"fullContentMap,omitempty"`
	ContentMapLastUpdated time.Time            `json:"contentMapLastUpdated"`

	// Orphan analysis
	OrphanAnalysis *OrphanAnalysisCache `json:"orphanAnalysis"`

	// Cache metadata
	LastUpdated time.Time
	Mu          sync.RWMutex // Exported for access
}

// TenantHTMLChunkCache holds HTML fragment cache for a single tenant
type TenantHTMLChunkCache struct {
	Chunks map[string]*HTMLChunk // "paneId:variant" -> chunk
	Deps   map[string][]string   // nodeId -> []cacheKeys
	Mu     sync.RWMutex          // Exported for access
}

// PaneVariant represents different rendering variants for personalization
type PaneVariant struct {
	BeliefMode      string   `json:"beliefMode"`      // "default", "personalized", etc.
	HeldBeliefs     []string `json:"heldBeliefs"`     // Beliefs user holds
	WithheldBeliefs []string `json:"withheldBeliefs"` // Beliefs user doesn't hold
}

// HTMLChunk represents cached HTML content with dependencies
type HTMLChunk struct {
	HTML        string      `json:"html"`
	PaneID      string      `json:"paneId"`
	Variant     PaneVariant `json:"variant"`
	DependsOn   []string    `json:"dependsOn"`
	LastUpdated time.Time   `json:"lastUpdated"`
}

// TenantAnalyticsCache holds analytics data for a single tenant
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

type FullContentMapItem struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

type OrphanAnalysisCache struct {
	Data        *OrphanAnalysisPayload `json:"data"`
	ETag        string                 `json:"etag"`
	LastUpdated time.Time              `json:"lastUpdated"`
}

type OrphanAnalysisPayload struct {
	StoryFragments map[string][]string `json:"storyFragments"`
	Panes          map[string][]string `json:"panes"`
	Menus          map[string][]string `json:"menus"`
	Files          map[string][]string `json:"files"`
	Beliefs        map[string][]string `json:"beliefs"`
	Status         string              `json:"status"`
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
