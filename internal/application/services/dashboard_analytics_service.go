package services

import (
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

type TimeRangeStats struct {
	Daily   int `json:"daily"`
	Weekly  int `json:"weekly"`
	Monthly int `json:"monthly"`
}

type LineDataPoint struct {
	X string `json:"x"`
	Y int    `json:"y"`
}

type LineDataSeries struct {
	ID   string          `json:"id"`
	Data []LineDataPoint `json:"data"`
}

type HotItem struct {
	ID          string `json:"id"`
	TotalEvents int    `json:"totalEvents"`
}

type DashboardAnalytics struct {
	Stats      TimeRangeStats   `json:"stats"`
	Line       []LineDataSeries `json:"line"`
	HotContent []HotItem        `json:"hotContent"`
}

type EpinetConfig struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type DashboardAnalyticsService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

func NewDashboardAnalyticsService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *DashboardAnalyticsService {
	return &DashboardAnalyticsService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

func (s *DashboardAnalyticsService) ComputeDashboard(tenantCtx *tenant.Context, startHour, endHour int) (*DashboardAnalytics, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("compute_dashboard", tenantCtx.TenantID)
	defer marker.Complete()
	epinets, err := s.getEpinets(tenantCtx)
	if err != nil {
		return nil, err
	}
	if len(epinets) == 0 {
		return s.createEmptyDashboardAnalytics(), nil
	}

	hourKeys := s.getHourKeysForCustomRange(startHour, endHour)
	dailyHourKeys := s.getHourKeysForCustomRange(24, 0)
	weeklyHourKeys := s.getHourKeysForCustomRange(168, 0)
	monthlyHourKeys := s.getHourKeysForCustomRange(672, 0)

	stats := TimeRangeStats{
		Daily:   s.computeAllEvents(tenantCtx, epinets, dailyHourKeys),
		Weekly:  s.computeAllEvents(tenantCtx, epinets, weeklyHourKeys),
		Monthly: s.computeAllEvents(tenantCtx, epinets, monthlyHourKeys),
	}

	s.logger.Analytics().Info("Successfully computed dashboard analytics", "tenantId", tenantCtx.TenantID, "startHour", startHour, "endHour", endHour, "epinetCount", len(epinets), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for ComputeDashboard", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return &DashboardAnalytics{
		Stats:      stats,
		Line:       s.computeLineData(tenantCtx, epinets, hourKeys),
		HotContent: s.computeHotContent(tenantCtx, epinets, hourKeys),
	}, nil
}

func (s *DashboardAnalyticsService) getEpinets(tenantCtx *tenant.Context) ([]EpinetConfig, error) {
	epinetRepo := tenantCtx.EpinetRepo()
	epinets, err := epinetRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, err
	}

	configs := make([]EpinetConfig, 0, len(epinets))
	for _, epinet := range epinets {
		if epinet != nil {
			configs = append(configs, EpinetConfig{
				ID:    epinet.ID,
				Title: epinet.Title,
			})
		}
	}
	return configs, nil
}

func (s *DashboardAnalyticsService) computeAllEvents(tenantCtx *tenant.Context, epinets []EpinetConfig, hourKeys []string) int {
	total := 0
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			if bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinet.ID, hourKey); exists {
				for _, stepData := range bin.Data.Steps {
					total += len(stepData.Visitors)
				}
			}
		}
	}
	return total
}

func (s *DashboardAnalyticsService) computeLineData(tenantCtx *tenant.Context, epinets []EpinetConfig, hourKeys []string) []LineDataSeries {
	// Track events by verb type and hour
	eventsByVerbAndHour := make(map[string]map[string]int)

	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			if bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinet.ID, hourKey); exists {
				for nodeID, stepData := range bin.Data.Steps {
					// Parse nodeID to extract verb
					// Format: "commitmentAction_StoryFragment_VERB_contentID"
					parts := strings.Split(nodeID, "_")
					if len(parts) >= 3 {
						verb := parts[len(parts)-2]

						// Initialize maps if needed
						if eventsByVerbAndHour[verb] == nil {
							eventsByVerbAndHour[verb] = make(map[string]int)
						}

						// Add visitor count to this verb for this hour
						eventsByVerbAndHour[verb][hourKey] += len(stepData.Visitors)
					}
				}
			}
		}
	}

	// Convert to LineDataSeries format
	var lineSeriesList []LineDataSeries
	for verb, hourData := range eventsByVerbAndHour {
		var lineData []LineDataPoint
		for _, hourKey := range hourKeys {
			count := hourData[hourKey] // Will be 0 if key doesn't exist
			lineData = append(lineData, LineDataPoint{X: hourKey, Y: count})
		}

		if len(lineData) > 0 {
			lineSeriesList = append(lineSeriesList, LineDataSeries{
				ID:   verb,
				Data: lineData,
			})
		}
	}

	// If no verb-specific data found, return empty instead of single "events" series
	if len(lineSeriesList) == 0 {
		return []LineDataSeries{}
	}

	return lineSeriesList
}

func (s *DashboardAnalyticsService) computeHotContent(tenantCtx *tenant.Context, epinets []EpinetConfig, hourKeys []string) []HotItem {
	contentCounts := make(map[string]int)

	// ADD: Debug logging for input parameters
	s.logger.Analytics().Debug("computeHotContent started",
		"tenantId", tenantCtx.TenantID,
		"epinetCount", len(epinets),
		"hourKeyCount", len(hourKeys),
		"hourKeys", hourKeys)

	totalStepsProcessed := 0
	storyFragmentStepsFound := 0
	validContentIDs := 0

	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			if bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinet.ID, hourKey); exists {

				s.logger.Analytics().Debug("Processing epinet bin",
					"epinetId", epinet.ID,
					"hourKey", hourKey,
					"stepCount", len(bin.Data.Steps))

				for stepID, stepData := range bin.Data.Steps {
					totalStepsProcessed++

					s.logger.Analytics().Debug("Processing step",
						"stepID", stepID,
						"visitorCount", len(stepData.Visitors))

					contentID := s.extractContentIDFromNodeID(stepID)
					isStoryFragmentStep := s.isStoryFragmentStep(stepID)

					s.logger.Analytics().Debug("Step extraction results",
						"stepID", stepID,
						"extractedContentID", contentID,
						"isStoryFragmentStep", isStoryFragmentStep)

					if contentID != "" && isStoryFragmentStep {
						storyFragmentStepsFound++
						validContentIDs++
						contentCounts[contentID] += len(stepData.Visitors)

						s.logger.Analytics().Debug("Valid StoryFragment step found",
							"stepID", stepID,
							"contentID", contentID,
							"visitorCount", len(stepData.Visitors),
							"runningTotal", contentCounts[contentID])
					}
				}
			} else {
				s.logger.Analytics().Debug("No epinet bin found",
					"epinetId", epinet.ID,
					"hourKey", hourKey)
			}
		}
	}

	s.logger.Analytics().Debug("computeHotContent processing summary",
		"totalStepsProcessed", totalStepsProcessed,
		"storyFragmentStepsFound", storyFragmentStepsFound,
		"validContentIDs", validContentIDs,
		"uniqueContentItems", len(contentCounts),
		"contentCounts", contentCounts)

	var sortedContent []HotItem
	for id, totalEvents := range contentCounts {
		sortedContent = append(sortedContent, HotItem{
			ID:          id,
			TotalEvents: totalEvents,
		})
	}

	for i := 0; i < len(sortedContent)-1; i++ {
		for j := i + 1; j < len(sortedContent); j++ {
			if sortedContent[i].TotalEvents < sortedContent[j].TotalEvents {
				sortedContent[i], sortedContent[j] = sortedContent[j], sortedContent[i]
			}
		}
	}

	if len(sortedContent) > 10 {
		sortedContent = sortedContent[:10]
	}

	s.logger.Analytics().Debug("computeHotContent final result",
		"resultCount", len(sortedContent),
		"result", sortedContent)

	return sortedContent
}

func (s *DashboardAnalyticsService) extractContentIDFromNodeID(nodeID string) string {
	parts := strings.Split(nodeID, "_")
	if len(parts) >= 1 {
		return parts[len(parts)-1]
	}
	return ""
}

func (s *DashboardAnalyticsService) isStoryFragmentStep(stepID string) bool {
	return strings.Contains(stepID, "StoryFragment")
}

func (s *DashboardAnalyticsService) createEmptyDashboardAnalytics() *DashboardAnalytics {
	return &DashboardAnalytics{
		Stats:      TimeRangeStats{Daily: 0, Weekly: 0, Monthly: 0},
		Line:       []LineDataSeries{},
		HotContent: []HotItem{},
	}
}

func (s *DashboardAnalyticsService) getHourKeysForCustomRange(startHour, endHour int) []string {
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
