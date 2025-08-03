package services

import (
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

type HourlyActivity map[string]map[string]struct {
	Events     map[string]int `json:"events"`
	VisitorIDs []string       `json:"visitorIds"`
}

type StoryfragmentAnalytics struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	EventCounts int    `json:"eventCounts"`
}

type ContentAnalyticsService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

func NewContentAnalyticsService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ContentAnalyticsService {
	return &ContentAnalyticsService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

func (s *ContentAnalyticsService) GetHourlyNodeActivity(tenantCtx *tenant.Context, epinetID string, startHour, endHour *int) (HourlyActivity, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_hourly_node_activity", tenantCtx.TenantID)
	defer marker.Complete()
	var hourKeys []string
	if startHour != nil && endHour != nil {
		hourKeys = s.getHourKeysForCustomRange(*startHour, *endHour)
	} else {
		hourKeys = s.getHourKeysForTimeRange(168)
	}

	hourlyActivity := make(HourlyActivity)
	for _, hourKey := range hourKeys {
		bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinetID, hourKey)
		if !exists {
			continue
		}

		hourNodeData := make(map[string]struct {
			Events     map[string]int `json:"events"`
			VisitorIDs []string       `json:"visitorIds"`
		})

		for nodeID, stepData := range bin.Data.Steps {
			if len(stepData.Visitors) == 0 {
				continue
			}
			originalNodeID := strings.ReplaceAll(nodeID, "_", "-")
			parts := strings.Split(originalNodeID, "-")

			if len(parts) >= 3 {
				contentID := parts[len(parts)-1]
				verb := parts[len(parts)-2]

				if _, ok := hourNodeData[contentID]; !ok {
					hourNodeData[contentID] = struct {
						Events     map[string]int `json:"events"`
						VisitorIDs []string       `json:"visitorIds"`
					}{Events: make(map[string]int), VisitorIDs: []string{}}
				}

				currentData := hourNodeData[contentID]
				currentData.Events[verb] += len(stepData.Visitors)

				visitorSet := make(map[string]struct{})
				for _, vID := range currentData.VisitorIDs {
					visitorSet[vID] = struct{}{}
				}
				for vID := range stepData.Visitors {
					visitorSet[vID] = struct{}{}
				}

				var newVisitorList []string
				for vID := range visitorSet {
					newVisitorList = append(newVisitorList, vID)
				}
				currentData.VisitorIDs = newVisitorList

				hourNodeData[contentID] = currentData
			}
		}
		if len(hourNodeData) > 0 {
			hourlyActivity[hourKey] = hourNodeData
		}
	}

	s.logger.Analytics().Info("Successfully computed hourly node activity", "tenantId", tenantCtx.TenantID, "epinetId", epinetID, "hourCount", len(hourKeys), "activityHours", len(hourlyActivity), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetHourlyNodeActivity", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return hourlyActivity, nil
}

func (s *ContentAnalyticsService) GetStoryfragmentAnalytics(tenantCtx *tenant.Context, epinetIDs []string, startHour, endHour int) ([]StoryfragmentAnalytics, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_storyfragment_analytics", tenantCtx.TenantID)
	defer marker.Complete()
	hourKeys := s.getHourKeysForCustomRange(startHour, endHour)

	storyFragmentCounts := make(map[string]int)
	storyFragmentTitles := make(map[string]string)

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragments, err := storyFragmentRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, err
	}
	for _, sf := range storyFragments {
		if sf != nil {
			storyFragmentTitles[sf.ID] = sf.Title
		}
	}

	for _, epinetID := range epinetIDs {
		for _, hourKey := range hourKeys {
			bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinetID, hourKey)
			if !exists {
				continue
			}

			for stepID, stepData := range bin.Data.Steps {
				if s.isStoryFragmentStep(stepID) {
					contentID := s.extractContentIDFromNodeID(stepID)
					if contentID != "" && storyFragmentTitles[contentID] != "" {
						storyFragmentCounts[contentID] += len(stepData.Visitors)
					}
				}
			}
		}
	}

	var result []StoryfragmentAnalytics
	for sfID, count := range storyFragmentCounts {
		result = append(result, StoryfragmentAnalytics{
			ID:          sfID,
			Title:       storyFragmentTitles[sfID],
			EventCounts: count,
		})
	}

	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].EventCounts < result[j].EventCounts {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	s.logger.Analytics().Info("Successfully computed storyfragment analytics", "tenantId", tenantCtx.TenantID, "epinetCount", len(epinetIDs), "storyfragmentCount", len(result), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetStoryfragmentAnalytics", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return result, nil
}

func (s *ContentAnalyticsService) extractContentIDFromNodeID(nodeID string) string {
	parts := strings.Split(nodeID, "-")
	if len(parts) >= 1 {
		return parts[len(parts)-1]
	}
	return ""
}

func (s *ContentAnalyticsService) isStoryFragmentStep(stepID string) bool {
	return strings.Contains(stepID, "storyfragment")
}

func (s *ContentAnalyticsService) getHourKeysForTimeRange(hoursBack int) []string {
	hourKeys := make([]string, hoursBack)
	now := time.Now().UTC()

	for i := 0; i < hoursBack; i++ {
		hourTime := now.Add(-time.Duration(i) * time.Hour)
		hourKeys[i] = hourTime.Format("2006-01-02-15")
	}

	return hourKeys
}

func (s *ContentAnalyticsService) getHourKeysForCustomRange(startHour, endHour int) []string {
	if startHour <= endHour {
		return []string{}
	}

	hourKeys := make([]string, startHour-endHour)
	now := time.Now().UTC()

	for i := 0; i < startHour-endHour; i++ {
		hourTime := now.Add(-time.Duration(endHour+i) * time.Hour)
		hourKeys[i] = hourTime.Format("2006-01-02-15")
	}

	return hourKeys
}
