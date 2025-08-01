// Package types defines analytics data structures for multi-tenant analytics processing.
package types

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/analytics"
)

// HourlyEpinetBin contains analytics data for an epinet in a specific hour
type HourlyEpinetBin struct {
	Data       *HourlyEpinetData `json:"data"`
	ComputedAt time.Time         `json:"computedAt"`
	TTL        time.Duration     `json:"ttl"`
}

// HourlyEpinetData contains the core epinet analytics data
type HourlyEpinetData struct {
	Steps       map[string]*HourlyEpinetStepData                  `json:"steps"`
	Transitions map[string]map[string]*HourlyEpinetTransitionData `json:"transitions"`
}

// HourlyEpinetStepData contains visitor data for a specific epinet step
type HourlyEpinetStepData struct {
	Visitors  map[string]bool `json:"visitors"` // Set of visitor IDs
	Name      string          `json:"name"`
	StepIndex int             `json:"stepIndex"`
}

// HourlyEpinetTransitionData contains visitor transition data between steps
type HourlyEpinetTransitionData struct {
	Visitors map[string]bool `json:"visitors"` // Set of visitor IDs
}

// HourlyContentBin contains analytics data for content in a specific hour
type HourlyContentBin struct {
	Data       *HourlyContentData `json:"data"`
	ComputedAt time.Time          `json:"computedAt"`
	TTL        time.Duration      `json:"ttl"`
}

// HourlyContentData contains the core content analytics data
type HourlyContentData struct {
	UniqueVisitors    map[string]bool `json:"uniqueVisitors"`    // Set of visitor IDs
	KnownVisitors     map[string]bool `json:"knownVisitors"`     // Set of known visitor IDs
	AnonymousVisitors map[string]bool `json:"anonymousVisitors"` // Set of anonymous visitor IDs
	Actions           int             `json:"actions"`
	EventCounts       map[string]int  `json:"eventCounts"`
}

// HourlySiteBin contains site-wide analytics data for a specific hour
type HourlySiteBin struct {
	Data       *HourlySiteData `json:"data"`
	ComputedAt time.Time       `json:"computedAt"`
	TTL        time.Duration   `json:"ttl"`
}

// HourlySiteData contains the core site analytics data.
type HourlySiteData struct {
	UniqueVisitors    map[string]bool `json:"uniqueVisitors"`    // Set of visitor IDs
	KnownVisitors     map[string]bool `json:"knownVisitors"`     // Set of known visitor IDs
	AnonymousVisitors map[string]bool `json:"anonymousVisitors"` // Set of anonymous visitor IDs
	PageViews         int             `json:"pageViews"`
	Sessions          int             `json:"sessions"`
}

// LeadMetricsCache contains computed lead metrics
type LeadMetricsCache struct {
	Data         *LeadMetricsData `json:"data"`
	LastComputed time.Time        `json:"computedAt"`
	TTL          time.Duration    `json:"ttl"`
}

// LeadMetricsData contains lead conversion and attribution data.
type LeadMetricsData struct {
	Status           string         `json:"status,omitempty"` // "loading" | "complete"
	TotalLeads       int            `json:"totalLeads"`
	NewLeads         int            `json:"newLeads"`
	LeadSources      map[string]int `json:"leadSources"`
	LeadsByTimeframe map[string]int `json:"leadsByTimeframe"`
}

// DashboardCache contains computed dashboard metrics
type DashboardCache struct {
	Data         *DashboardData `json:"data"`
	LastComputed time.Time      `json:"computedAt"`
	TTL          time.Duration  `json:"ttl"`
}

// DashboardData contains high-level dashboard metrics.
type DashboardData struct {
	Status   string           `json:"status,omitempty"` // "loading" | "complete"
	Overview *OverviewMetrics `json:"overview"`
	Traffic  *TrafficMetrics  `json:"traffic"`
}

// OverviewMetrics contains the raw aggregates needed for an overview dashboard.
type OverviewMetrics struct {
	UniqueVisitors int `json:"uniqueVisitors"`
	PageViews      int `json:"pageViews"`
	Sessions       int `json:"sessions"`
}

// TrafficMetrics contains raw traffic source aggregations.
type TrafficMetrics struct {
	Sources   map[string]int `json:"sources"`
	Mediums   map[string]int `json:"mediums"`
	Campaigns map[string]int `json:"campaigns"`
	Referrers map[string]int `json:"referrers"`
	TopPages  map[string]int `json:"topPages"`
}

// SankeyDiagram represents the data structure for a Sankey diagram, including its status.
type SankeyDiagram struct {
	Status string       `json:"status,omitempty"` // "loading" | "complete"
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	Nodes  []SankeyNode `json:"nodes"`
	Links  []SankeyLink `json:"links"`
}

// SankeyNode represents a node in a Sankey diagram.
type SankeyNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// SankeyLink represents a link between nodes in a Sankey diagram.
type SankeyLink struct {
	Source int `json:"source"`
	Target int `json:"target"`
	Value  int `json:"value"`
}

// RangeCacheStatus communicates the state of a requested range of hourly bins.
type RangeCacheStatus struct {
	Action             string // "proceed", "refresh_current", "load_range"
	CurrentHourExpired bool
	HistoricalComplete bool
	MissingHours       []string
}

// EpinetConfig represents a simplified epinet structure for analytics processing.
type EpinetConfig struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Steps []EpinetStep `json:"steps"`
}

// EpinetStep represents a step within an EpinetConfig.
type EpinetStep struct {
	GateType   string   `json:"gateType"`
	Values     []string `json:"values"`
	ObjectType string   `json:"objectType"`
	ObjectIds  []string `json:"objectIds"`
	Title      string   `json:"title"`
}

// ContentItem holds a simplified view of a content node for analytics naming.
type ContentItem struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

// The ActionEvent and BeliefEvent structs are defined in the domain/analytics package
// and are imported where needed, so they are not duplicated here.
type (
	ActionEvent = analytics.ActionEvent
	BeliefEvent = analytics.BeliefEvent
)
