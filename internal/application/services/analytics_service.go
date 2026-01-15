// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"sort"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/utilities"
)

// UserCount represents a count of users grouped by a specific dimension.
type UserCount struct {
	ID      string `json:"id"`
	Count   int    `json:"count"`
	IsKnown bool   `json:"isKnown"`
}

// AppliedFilter represents a filter applied to an analytics query.
type AppliedFilter struct {
	BeliefSlug string `json:"beliefSlug"`
	Value      string `json:"value"`
}

// SankeyFilters contains specific filters for generating Sankey diagrams.
type SankeyFilters struct {
	VisitorType    string          `json:"visitorType"`
	SelectedUserID *string         `json:"selectedUserID,omitempty"`
	StartHour      *int            `json:"startHour,omitempty"`
	EndHour        *int            `json:"endHour,omitempty"`
	AppliedFilters []AppliedFilter `json:"appliedFilters,omitempty"`
}

// AnalyticsService handles retrieval and processing of visitor analytics data.
type AnalyticsService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewAnalyticsService creates a new instance of AnalyticsService.
func NewAnalyticsService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *AnalyticsService {
	return &AnalyticsService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// GetFilteredVisitorCounts retrieves visitor counts filtered by time range and other criteria.
func (s *AnalyticsService) GetFilteredVisitorCounts(tenantCtx *tenant.Context, epinetID string, visitorType string, startHour, endHour *int) ([]UserCount, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_filtered_visitor_counts", tenantCtx.TenantID)
	defer marker.Complete()
	var hourKeys []string
	if startHour != nil && endHour != nil {
		hourKeys = utilities.GetHourKeysForCustomRange(*startHour, *endHour)
	} else {
		hourKeys = s.getHourKeysForTimeRange(168)
	}

	visitorEventCount := make(map[string]int)
	for _, hourKey := range hourKeys {
		bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinetID, hourKey)
		if !exists {
			continue
		}
		for _, stepData := range bin.Data.Steps {
			for visitorID := range stepData.Visitors {
				if s.shouldIncludeVisitor(visitorID, &SankeyFilters{VisitorType: visitorType}, stepData) {
					visitorEventCount[visitorID]++
				}
			}
		}
	}

	var userCounts []UserCount
	for id, count := range visitorEventCount {
		// Check if visitor is known by looking in any step data that contains this visitor
		isKnown := false
		for _, hourKey := range hourKeys {
			if bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinetID, hourKey); exists {
				for _, stepData := range bin.Data.Steps {
					if stepData.KnownVisitors[id] {
						isKnown = true
						break
					}
				}
				if isKnown {
					break
				}
			}
		}

		userCounts = append(userCounts, UserCount{
			ID:      id,
			Count:   count,
			IsKnown: isKnown,
		})
	}

	sort.Slice(userCounts, func(i, j int) bool {
		if userCounts[i].Count != userCounts[j].Count {
			return userCounts[i].Count > userCounts[j].Count
		}
		return userCounts[i].ID < userCounts[j].ID
	})

	s.logger.Analytics().Info("Successfully retrieved filtered visitor counts", "tenantId", tenantCtx.TenantID, "epinetId", epinetID, "visitorType", visitorType, "count", len(userCounts), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetFilteredVisitorCounts", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return userCounts, nil
}

func (s *AnalyticsService) shouldIncludeVisitor(visitorID string, filters *SankeyFilters, stepData *types.HourlyEpinetStepData) bool {
	if filters == nil {
		return true
	}

	if filters.SelectedUserID != nil && *filters.SelectedUserID != visitorID {
		return false
	}

	isKnown := stepData.KnownVisitors[visitorID]
	switch filters.VisitorType {
	case "known":
		return isKnown
	case "anonymous":
		return !isKnown
	default:
		return true
	}
}

func (s *AnalyticsService) getHourKeysForTimeRange(hoursBack int) []string {
	hourKeys := make([]string, hoursBack)
	now := time.Now().UTC()

	for i := range hoursBack {
		hourTime := now.Add(-time.Duration(i) * time.Hour)
		hourKeys[i] = hourTime.Format("2006-01-02-15")
	}

	return hourKeys
}
