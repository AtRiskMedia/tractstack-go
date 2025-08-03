package services

import (
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
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
	logger *logging.ChanneledLogger
}

func NewDashboardAnalyticsService(logger *logging.ChanneledLogger) *DashboardAnalyticsService {
	return &DashboardAnalyticsService{
		logger: logger,
	}
}

func (s *DashboardAnalyticsService) ComputeDashboard(tenantCtx *tenant.Context, startHour, endHour int) (*DashboardAnalytics, error) {
	start := time.Now()
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
	eventsByHour := make(map[string]int)
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			if bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinet.ID, hourKey); exists {
				for _, stepData := range bin.Data.Steps {
					eventsByHour[hourKey] += len(stepData.Visitors)
				}
			}
		}
	}

	var lineData []LineDataPoint
	for _, hourKey := range hourKeys {
		lineData = append(lineData, LineDataPoint{X: hourKey, Y: eventsByHour[hourKey]})
	}

	return []LineDataSeries{{ID: "events", Data: lineData}}
}

func (s *DashboardAnalyticsService) computeHotContent(tenantCtx *tenant.Context, epinets []EpinetConfig, hourKeys []string) []HotItem {
	contentCounts := make(map[string]int)

	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			if bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinet.ID, hourKey); exists {
				for stepID, stepData := range bin.Data.Steps {
					contentID := s.extractContentIDFromNodeID(stepID)
					if contentID != "" && s.isStoryFragmentStep(stepID) {
						contentCounts[contentID] += len(stepData.Visitors)
					}
				}
			}
		}
	}

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

	return sortedContent
}

func (s *DashboardAnalyticsService) extractContentIDFromNodeID(nodeID string) string {
	parts := strings.Split(nodeID, "-")
	if len(parts) >= 1 {
		return parts[len(parts)-1]
	}
	return ""
}

func (s *DashboardAnalyticsService) isStoryFragmentStep(stepID string) bool {
	return strings.Contains(stepID, "storyfragment")
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
