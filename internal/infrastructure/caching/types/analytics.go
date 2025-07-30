// Package types defines analytics data structures for multi-tenant analytics processing.
package types

import (
	"time"
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
