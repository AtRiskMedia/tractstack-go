// Package models defines analytics data structures for multi-tenant analytics processing.
package models

import (
	"sync"
	"time"
)

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
// API Response Types (From The Plan)
// =============================================================================

type SankeyDiagram struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Nodes []SankeyNode `json:"nodes"`
	Links []SankeyLink `json:"links"`
}

type SankeyNode struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type SankeyLink struct {
	Source int `json:"source"`
	Target int `json:"target"`
	Value  int `json:"value"`
}

// =============================================================================
// Database Query Types (From The Plan)
// =============================================================================

type EpinetConfig struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Steps []EpinetStep `json:"steps"`
}

type EpinetStep struct {
	GateType   string   `json:"gateType"`   // "belief", "identifyAs", "commitmentAction", "conversionAction"
	Values     []string `json:"values"`     // Verbs or objects to match
	ObjectType string   `json:"objectType"` // "StoryFragment", "Pane"
	ObjectIds  []string `json:"objectIds"`  // Specific content IDs
	Title      string   `json:"title"`
}

type ActionEvent struct {
	ObjectID      string    `json:"objectId"`
	ObjectType    string    `json:"objectType"`
	Verb          string    `json:"verb"`
	FingerprintID string    `json:"fingerprintId"`
	CreatedAt     time.Time `json:"createdAt"`
}

type BeliefEvent struct {
	BeliefID      string    `json:"beliefId"`
	FingerprintID string    `json:"fingerprintId"`
	Verb          string    `json:"verb"`
	Object        *string   `json:"object"` // For identifyAs events
	UpdatedAt     time.Time `json:"updatedAt"`
}

// =============================================================================
// Analytics Processing Types (From The Plan)
// =============================================================================

type SankeyFilters struct {
	VisitorType    string  `json:"visitorType"` // "all", "anonymous", "known"
	SelectedUserID *string `json:"userId"`
	StartHour      *int    `json:"startHour"`
	EndHour        *int    `json:"endHour"`
}

type ContentItem struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
}
