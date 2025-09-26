package services

import (
	"sort"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/utilities"
)

type SankeyNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SankeyLink struct {
	Source int `json:"source"`
	Target int `json:"target"`
	Value  int `json:"value"`
}

type SankeyDiagram struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Nodes []SankeyNode `json:"nodes"`
	Links []SankeyLink `json:"links"`
}

type ContentItem struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

type potentialLink struct {
	from  string
	to    string
	value int
}

type EpinetAnalyticsService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

func NewEpinetAnalyticsService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *EpinetAnalyticsService {
	return &EpinetAnalyticsService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

func (s *EpinetAnalyticsService) ComputeEpinetSankey(tenantCtx *tenant.Context, epinetID string, filters *SankeyFilters) (*SankeyDiagram, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("compute_epinet_sankey", tenantCtx.TenantID)
	defer marker.Complete()
	var hourKeys []string
	if filters != nil && filters.StartHour != nil && filters.EndHour != nil {
		hourKeys = utilities.GetHourKeysForCustomRange(*filters.StartHour, *filters.EndHour)
	} else {
		hourKeys = s.getHourKeysForTimeRange(168)
	}

	contentItems, err := s.getContentItems(tenantCtx)
	if err != nil {
		return nil, err
	}

	stepUserSets := make(map[int]map[string]map[string]bool)

	for _, hourKey := range hourKeys {
		bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinetID, hourKey)
		if !exists {
			continue
		}
		for nodeID, stepData := range bin.Data.Steps {
			originalNodeID := strings.ReplaceAll(nodeID, "_", "-")
			stepIndex := stepData.StepIndex

			if stepUserSets[stepIndex] == nil {
				stepUserSets[stepIndex] = make(map[string]map[string]bool)
			}
			if stepUserSets[stepIndex][originalNodeID] == nil {
				stepUserSets[stepIndex][originalNodeID] = make(map[string]bool)
			}

			for visitorID := range stepData.Visitors {
				if !s.shouldIncludeVisitor(visitorID, filters, stepData) {
					continue
				}
				stepUserSets[stepIndex][originalNodeID][visitorID] = true
			}
		}
	}

	// Initialize slices to be non-nil.
	potentialLinks := make([]potentialLink, 0)
	stepOrder := make([]int, 0, len(stepUserSets))

	for stepIndex := range stepUserSets {
		stepOrder = append(stepOrder, stepIndex)
	}
	sort.Ints(stepOrder)

	// If there are no steps with visitor data for the given time range, return an empty diagram.
	if len(stepOrder) == 0 {
		s.logger.Analytics().Info("No epinet steps with data in range, returning empty sankey.", "tenantId", tenantCtx.TenantID, "epinetId", epinetID, "duration", time.Since(start))
		return &SankeyDiagram{
			ID:    epinetID,
			Title: "User Journey Flow",
			Nodes: []SankeyNode{},
			Links: []SankeyLink{},
		}, nil
	}

	// 1. Identify all visitors who appear in the first step of the funnel. These are the only journeys we will trace.
	firstStepIndex := stepOrder[0]
	startingVisitors := make(map[string]bool)
	if nodes, ok := stepUserSets[firstStepIndex]; ok {
		for _, nodeVisitors := range nodes {
			for visitorID := range nodeVisitors {
				startingVisitors[visitorID] = true
			}
		}
	}

	// 2. Create links only between an epinet step and its immediate successor, and only for visitors who started their journey at Step 1.
	for i := 0; i < len(stepOrder)-1; i++ {
		sourceStep := stepOrder[i]
		targetStep := stepOrder[i+1]

		for sourceNode, sourceNodeVisitors := range stepUserSets[sourceStep] {
			// From the current source node, find which visitors also started their journey at the first step.
			validSourceVisitors := s.intersectVisitors(sourceNodeVisitors, startingVisitors)
			if len(validSourceVisitors) == 0 {
				continue // No one in this source node began their journey at the start of the funnel, so skip.
			}

			for targetNode, targetNodeVisitors := range stepUserSets[targetStep] {
				// Find the intersection between the valid source visitors and the visitors of the target node.
				intersection := s.intersectVisitors(validSourceVisitors, targetNodeVisitors)

				if len(intersection) > 0 {
					potentialLinks = append(potentialLinks, potentialLink{
						from:  sourceNode,
						to:    targetNode,
						value: len(intersection),
					})
				}
			}
		}
	}

	nodeSet := make(map[string]bool)
	for _, plink := range potentialLinks {
		nodeSet[plink.from] = true
		nodeSet[plink.to] = true
	}

	var finalNodes []SankeyNode
	finalNodeIndexMap := make(map[string]int)
	for nodeID := range nodeSet {
		var title string
		contentID := s.extractContentIDFromNodeID(nodeID)
		if item, exists := contentItems[contentID]; exists {
			// If the content ID is found in the map, use its title.
			title = item.Title
		} else {
			// If the lookup fails (due to deleted content, etc.), use a redacted placeholder.
			title = "[deleted content]"
		}
		finalNodeIndexMap[nodeID] = len(finalNodes)
		finalNodes = append(finalNodes, SankeyNode{ID: nodeID, Name: title})
	}

	var finalLinks []SankeyLink
	for _, plink := range potentialLinks {
		sourceIndex, sourceExists := finalNodeIndexMap[plink.from]
		targetIndex, targetExists := finalNodeIndexMap[plink.to]

		if sourceExists && targetExists {
			finalLinks = append(finalLinks, SankeyLink{Source: sourceIndex, Target: targetIndex, Value: plink.value})
		}
	}

	s.logger.Analytics().Info("Successfully computed epinet sankey", "tenantId", tenantCtx.TenantID, "epinetId", epinetID, "nodeCount", len(finalNodes), "linkCount", len(finalLinks), "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for ComputeEpinetSankey", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return &SankeyDiagram{
		ID:    epinetID,
		Title: "User Journey Flow",
		Nodes: finalNodes,
		Links: finalLinks,
	}, nil
}

func (s *EpinetAnalyticsService) intersectVisitors(set1, set2 map[string]bool) map[string]bool {
	intersection := make(map[string]bool)
	for visitor := range set1 {
		if set2[visitor] {
			intersection[visitor] = true
		}
	}
	return intersection
}

func (s *EpinetAnalyticsService) shouldIncludeVisitor(visitorID string, filters *SankeyFilters, stepData *types.HourlyEpinetStepData) bool {
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

func (s *EpinetAnalyticsService) getContentItems(tenantCtx *tenant.Context) (map[string]ContentItem, error) {
	contentItems := make(map[string]ContentItem)

	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	storyFragments, err := storyFragmentRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, err
	}
	for _, sf := range storyFragments {
		if sf != nil {
			contentItems[sf.ID] = ContentItem{Title: sf.Title, Slug: sf.Slug}
		}
	}

	paneRepo := tenantCtx.PaneRepo()
	panes, err := paneRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, err
	}
	for _, pane := range panes {
		if pane != nil {
			contentItems[pane.ID] = ContentItem{Title: pane.Title, Slug: pane.Slug}
		}
	}

	return contentItems, nil
}

func (s *EpinetAnalyticsService) extractContentIDFromNodeID(nodeID string) string {
	parts := strings.Split(nodeID, "-")
	if len(parts) >= 1 {
		return parts[len(parts)-1]
	}
	return ""
}

func (s *EpinetAnalyticsService) getHourKeysForTimeRange(hoursBack int) []string {
	hourKeys := make([]string, hoursBack)
	now := time.Now().UTC()

	for i := range hoursBack {
		hourTime := now.Add(-time.Duration(i) * time.Hour)
		hourKeys[i] = hourTime.Format("2006-01-02-15")
	}

	return hourKeys
}
