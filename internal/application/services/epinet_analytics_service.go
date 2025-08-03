package services

import (
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

type SankeyNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
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
	logger *logging.ChanneledLogger
}

func NewEpinetAnalyticsService(logger *logging.ChanneledLogger) *EpinetAnalyticsService {
	return &EpinetAnalyticsService{
		logger: logger,
	}
}

func (s *EpinetAnalyticsService) ComputeEpinetSankey(tenantCtx *tenant.Context, epinetID string, filters *SankeyFilters) (*SankeyDiagram, error) {
	start := time.Now()
	var hourKeys []string
	if filters != nil && filters.StartHour != nil && filters.EndHour != nil {
		hourKeys = s.getHourKeysForCustomRange(*filters.StartHour, *filters.EndHour)
	} else {
		hourKeys = s.getHourKeysForTimeRange(168)
	}

	knownFingerprints, err := s.getKnownFingerprints(tenantCtx)
	if err != nil {
		return nil, err
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
				if !s.shouldIncludeVisitor(visitorID, filters, knownFingerprints) {
					continue
				}
				stepUserSets[stepIndex][originalNodeID][visitorID] = true
			}
		}
	}

	var potentialLinks []potentialLink
	var stepOrder []int
	for stepIndex := range stepUserSets {
		stepOrder = append(stepOrder, stepIndex)
	}

	for i := 0; i < len(stepOrder); i++ {
		for j := i + 1; j < len(stepOrder); j++ {
			sourceStep := stepOrder[i]
			targetStep := stepOrder[j]

			for sourceNode := range stepUserSets[sourceStep] {
				for targetNode := range stepUserSets[targetStep] {
					intersection := s.intersectVisitors(
						stepUserSets[sourceStep][sourceNode],
						stepUserSets[targetStep][targetNode],
					)
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
	}

	nodeSet := make(map[string]bool)
	for _, plink := range potentialLinks {
		nodeSet[plink.from] = true
		nodeSet[plink.to] = true
	}

	var finalNodes []SankeyNode
	finalNodeIndexMap := make(map[string]int)
	for nodeID := range nodeSet {
		title := nodeID
		if item, exists := contentItems[s.extractContentIDFromNodeID(nodeID)]; exists {
			title = item.Title
		}
		finalNodeIndexMap[nodeID] = len(finalNodes)
		finalNodes = append(finalNodes, SankeyNode{ID: nodeID, Title: title})
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

func (s *EpinetAnalyticsService) shouldIncludeVisitor(visitorID string, filters *SankeyFilters, knownFingerprints map[string]bool) bool {
	if filters == nil {
		return true
	}

	if filters.SelectedUserID != nil && *filters.SelectedUserID != visitorID {
		return false
	}

	isKnown := knownFingerprints[visitorID]
	switch filters.VisitorType {
	case "known":
		return isKnown
	case "anonymous":
		return !isKnown
	default:
		return true
	}
}

func (s *EpinetAnalyticsService) getKnownFingerprints(tenantCtx *tenant.Context) (map[string]bool, error) {
	query := `SELECT id, CASE WHEN lead_id IS NOT NULL THEN 1 ELSE 0 END as is_known FROM fingerprints`
	rows, err := tenantCtx.Database.Conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	knownFingerprints := make(map[string]bool)
	for rows.Next() {
		var fingerprintID string
		var isKnown bool
		if err := rows.Scan(&fingerprintID, &isKnown); err != nil {
			return nil, err
		}
		knownFingerprints[fingerprintID] = isKnown
	}
	return knownFingerprints, nil
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

	for i := 0; i < hoursBack; i++ {
		hourTime := now.Add(-time.Duration(i) * time.Hour)
		hourKeys[i] = hourTime.Format("2006-01-02-15")
	}

	return hourKeys
}

func (s *EpinetAnalyticsService) getHourKeysForCustomRange(startHour, endHour int) []string {
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
