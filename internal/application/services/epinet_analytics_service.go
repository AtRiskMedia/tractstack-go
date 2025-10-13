package services

import (
	"slices"
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

	epinetConfig, err := s.getEpinetConfig(tenantCtx, epinetID)
	if err != nil {
		s.logger.Analytics().Error("Failed to get epinet config", "epinetId", epinetID, "error", err)
		return nil, err
	}

	stepUserSets := make(map[int]map[string]map[string]bool)
	nodeNames := make(map[string]string)

	for _, hourKey := range hourKeys {
		bin, exists := tenantCtx.CacheManager.GetHourlyEpinetBin(tenantCtx.TenantID, epinetID, hourKey)
		if !exists {
			continue
		}
		for nodeID, stepData := range bin.Data.Steps {
			mangledID := strings.ReplaceAll(nodeID, "_", "-")
			stepIndex := stepData.StepIndex

			if stepUserSets[stepIndex] == nil {
				stepUserSets[stepIndex] = make(map[string]map[string]bool)
			}
			if stepUserSets[stepIndex][mangledID] == nil {
				stepUserSets[stepIndex][mangledID] = make(map[string]bool)
			}

			nodeNames[mangledID] = stepData.Name

			for visitorID := range stepData.Visitors {
				if !s.shouldIncludeVisitor(visitorID, filters, stepData) {
					continue
				}
				stepUserSets[stepIndex][mangledID][visitorID] = true
			}
		}
	}

	stepOrder := make([]int, 0, len(stepUserSets))
	for stepIndex := range stepUserSets {
		stepOrder = append(stepOrder, stepIndex)
	}
	sort.Ints(stepOrder)
	if len(stepOrder) == 0 {
		return &SankeyDiagram{
			ID: epinetID, Title: "User Journey Flow", Nodes: []SankeyNode{}, Links: []SankeyLink{},
		}, nil
	}

	getUsersInStep := func(stepIndex int) map[string]bool {
		users := make(map[string]bool)
		if nodes, ok := stepUserSets[stepIndex]; ok {
			for _, visitors := range nodes {
				for visitorID := range visitors {
					users[visitorID] = true
				}
			}
		}
		return users
	}

	if epinetConfig != nil {
		for i := 0; i < len(stepOrder)-1; i++ {
			currentStepIndex := stepOrder[i]
			nextStepIndex := stepOrder[i+1]

			var currentStepDef *types.EpinetStep
			for j := range epinetConfig.Steps {
				if epinetConfig.Steps[j].StepIndex == currentStepIndex {
					currentStepDef = &epinetConfig.Steps[j]
					break
				}
			}

			if currentStepDef == nil {
				continue
			}

			usersInCurrentStep := getUsersInStep(currentStepIndex)
			usersInNextStep := getUsersInStep(nextStepIndex)

			if currentStepDef.GateType == "commitmentAction" && slices.Contains(currentStepDef.Values, "ENTERED") {
				previouslyEntered := make(map[string]bool)
				for visitorID := range usersInNextStep {
					if !usersInCurrentStep[visitorID] {
						previouslyEntered[visitorID] = true
					}
				}

				if len(previouslyEntered) > 0 {
					nodeID := "commitmentAction-Previously-Entered"
					if stepUserSets[currentStepIndex] == nil {
						stepUserSets[currentStepIndex] = make(map[string]map[string]bool)
					}
					stepUserSets[currentStepIndex][nodeID] = previouslyEntered
					nodeNames[nodeID] = "Previously Entered"
				}
			}

			if currentStepDef.GateType == "identifyAs" || currentStepDef.GateType == "belief" {
				anonymous := make(map[string]bool)
				for visitorID := range usersInNextStep {
					if !usersInCurrentStep[visitorID] {
						anonymous[visitorID] = true
					}
				}

				if len(anonymous) > 0 {
					nodeID := "identifyAs-Anonymous-Traffic"
					if stepUserSets[currentStepIndex] == nil {
						stepUserSets[currentStepIndex] = make(map[string]map[string]bool)
					}
					stepUserSets[currentStepIndex][nodeID] = anonymous
					nodeNames[nodeID] = "Anonymous Traffic"
				}
			}
		}
	}

	visitorsWhoReachedStep := make(map[int]map[string]bool)
	firstStepIndex := stepOrder[0]
	visitorsWhoReachedStep[firstStepIndex] = make(map[string]bool)
	if nodes, ok := stepUserSets[firstStepIndex]; ok {
		for _, nodeVisitors := range nodes {
			for visitorID := range nodeVisitors {
				visitorsWhoReachedStep[firstStepIndex][visitorID] = true
			}
		}
	}

	potentialLinks := make([]potentialLink, 0)
	for i := 0; i < len(stepOrder)-1; i++ {
		sourceStepIndex := stepOrder[i]
		targetStepIndex := stepOrder[i+1]

		validPathVisitors := visitorsWhoReachedStep[sourceStepIndex]
		if len(validPathVisitors) == 0 {
			break
		}

		visitorsWhoReachedStep[targetStepIndex] = make(map[string]bool)

		for sourceNode, sourceNodeVisitors := range stepUserSets[sourceStepIndex] {
			qualifiedVisitorsAtSource := s.intersectVisitors(sourceNodeVisitors, validPathVisitors)
			if len(qualifiedVisitorsAtSource) == 0 {
				continue
			}

			for targetNode, targetNodeVisitors := range stepUserSets[targetStepIndex] {
				intersection := s.intersectVisitors(qualifiedVisitorsAtSource, targetNodeVisitors)

				if len(intersection) > 0 {
					potentialLinks = append(potentialLinks, potentialLink{
						from:  sourceNode,
						to:    targetNode,
						value: len(intersection),
					})
					for visitorID := range intersection {
						visitorsWhoReachedStep[targetStepIndex][visitorID] = true
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
		var title string
		if name, ok := nodeNames[nodeID]; ok {
			title = name
		} else {
			contentID := s.extractContentIDFromNodeID(nodeID)
			if item, exists := contentItems[contentID]; exists {
				title = item.Title
			} else {
				title = "[deleted content]"
			}
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

	return &SankeyDiagram{
		ID:    epinetID,
		Title: "User Journey Flow",
		Nodes: finalNodes,
		Links: finalLinks,
	}, nil
}

func (s *EpinetAnalyticsService) getEpinetConfig(tenantCtx *tenant.Context, epinetID string) (*types.EpinetConfig, error) {
	epinetRepo := tenantCtx.EpinetRepo()
	allEpinets, err := epinetRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, err
	}

	for _, node := range allEpinets {
		if node != nil && node.ID == epinetID {
			var steps []types.EpinetStep
			for i, nodeStep := range node.Steps {
				step := types.EpinetStep{
					GateType:  nodeStep.GateType,
					Title:     nodeStep.Title,
					Values:    nodeStep.Values,
					ObjectIds: nodeStep.ObjectIDs,
					StepIndex: i + 1,
				}
				if nodeStep.ObjectType != nil {
					step.ObjectType = *nodeStep.ObjectType
				}
				if nodeStep.BeliefSlug != nil {
					step.BeliefSlug = *nodeStep.BeliefSlug
				}
				steps = append(steps, step)
			}

			return &types.EpinetConfig{
				ID:    node.ID,
				Title: node.Title,
				Steps: steps,
			}, nil
		}
	}

	return nil, nil
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
