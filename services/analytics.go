package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// AnalyticsService reads from the cache and computes analytics.
// This is the definitive implementation, designed to be resilient to flawed cached data
// by rebuilding the transition graph from scratch using the correct, idempotent, step-based aggregation logic.
type AnalyticsService struct {
	cache    cache.ReadOnlyAnalyticsCache
	tenantID string
	ctx      *tenant.Context
}

// NewAnalyticsService creates a new instance of the analytics service.
func NewAnalyticsService(cacheInterface cache.ReadOnlyAnalyticsCache, tenantID string) *AnalyticsService {
	return &AnalyticsService{
		cache:    cacheInterface,
		tenantID: tenantID,
		ctx:      nil,
	}
}

// SetContext allows setting the tenant context for database access via content services.
func (as *AnalyticsService) SetContext(ctx *tenant.Context) {
	as.ctx = ctx
}

// =================================================================================
// PRIMARY PUBLIC METHODS
// =================================================================================

// ComputeEpinetSankey generates the Sankey diagram by rebuilding the analytics graph from raw data.
func (as *AnalyticsService) ComputeEpinetSankey(epinetID string, filters *models.SankeyFilters) (*models.SankeyDiagram, error) {
	// 1. Retrieve all necessary data upfront.
	var hourKeys []string
	if filters != nil && filters.StartHour != nil && filters.EndHour != nil {
		hourKeys = utils.GetHourKeysForCustomRange(*filters.StartHour, *filters.EndHour)
	} else {
		hourKeys = utils.GetHourKeysForTimeRange(168)
	}

	knownFingerprints, err := as.getKnownFingerprints()
	if err != nil {
		return nil, err
	}
	contentItems, err := as.getContentItems()
	if err != nil {
		return nil, err
	}

	// 2. Aggregate User Sets for each Node, grouped by Step.
	stepUserSets := make(map[int]map[string]map[string]bool) // map[stepIndex][nodeID][visitorID]

	for _, hourKey := range hourKeys {
		bin, exists := as.cache.GetHourlyEpinetBin(as.tenantID, epinetID, hourKey)
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
				if !as.shouldIncludeVisitor(visitorID, filters, knownFingerprints) {
					continue
				}
				stepUserSets[stepIndex][originalNodeID][visitorID] = true
			}
		}
	}

	// 3. Calculate all POTENTIAL links and their values.
	type potentialLink struct {
		from  string
		to    string
		value int
	}
	var potentialLinks []potentialLink

	// --- Link Step 1 (ENTERED) to Step 2 (PAGEVIEWED) ---
	if stepUserSets[1] != nil && stepUserSets[2] != nil {
		for fromNode, fromVisitors := range stepUserSets[1] {
			for toNode, toVisitors := range stepUserSets[2] {
				intersectionSize := as.calculateIntersection(fromVisitors, toVisitors)
				if intersectionSize > 0 {
					potentialLinks = append(potentialLinks, potentialLink{from: fromNode, to: toNode, value: intersectionSize})
				}
			}
		}
	}
	// --- Link Step 2 (PAGEVIEWED) to Step 3 (Interactions) ---
	if stepUserSets[2] != nil && stepUserSets[3] != nil {
		for fromNode, fromVisitors := range stepUserSets[2] {
			for toNode, toVisitors := range stepUserSets[3] {
				intersectionSize := as.calculateIntersection(fromVisitors, toVisitors)
				if intersectionSize > 0 {
					potentialLinks = append(potentialLinks, potentialLink{from: fromNode, to: toNode, value: intersectionSize})
				}
			}
		}
	}

	// 4. Determine the final set of nodes that are actually connected.
	connectedNodeIDs := make(map[string]bool)
	for _, plink := range potentialLinks {
		connectedNodeIDs[plink.from] = true
		connectedNodeIDs[plink.to] = true
	}

	// 5. Build the final node list and the final index map.
	var finalNodes []models.SankeyNode
	finalNodeIndexMap := make(map[string]int)
	finalIndexMapping := make(map[string]int)
	for nodeID := range connectedNodeIDs {
		finalIndexMapping[nodeID] = len(finalNodes)
		finalNodes = append(finalNodes, models.SankeyNode{
			Name: as.generateOriginalNodeName(nodeID, contentItems),
			ID:   nodeID,
		})
	}

	// Sort final nodes by name for deterministic order, which helps in matching the known good source's indices.
	sort.Slice(finalNodes, func(i, j int) bool {
		return finalNodes[i].Name < finalNodes[j].Name
	})

	// Rebuild the index map based on the sorted order.
	for i, node := range finalNodes {
		finalNodeIndexMap[node.ID] = i
	}

	// 6. Build the final links array using the final node indices.
	var finalLinks []models.SankeyLink
	for _, plink := range potentialLinks {
		sourceIndex, sourceExists := finalNodeIndexMap[plink.from]
		targetIndex, targetExists := finalNodeIndexMap[plink.to]

		if sourceExists && targetExists {
			finalLinks = append(finalLinks, models.SankeyLink{Source: sourceIndex, Target: targetIndex, Value: plink.value})
		}
	}

	return &models.SankeyDiagram{
		ID:    epinetID,
		Title: "User Journey Flow",
		Nodes: finalNodes,
		Links: finalLinks,
	}, nil
}

// GetFilteredVisitorCounts returns a list of users and their event counts.
func (as *AnalyticsService) GetFilteredVisitorCounts(epinetID string, visitorType string, startHour, endHour *int) ([]models.UserCount, error) {
	var hourKeys []string
	if startHour != nil && endHour != nil {
		hourKeys = utils.GetHourKeysForCustomRange(*startHour, *endHour)
	} else {
		hourKeys = utils.GetHourKeysForTimeRange(168)
	}

	knownFingerprints, err := as.getKnownFingerprints()
	if err != nil {
		return nil, err
	}

	visitorEventCount := make(map[string]int)
	for _, hourKey := range hourKeys {
		bin, exists := as.cache.GetHourlyEpinetBin(as.tenantID, epinetID, hourKey)
		if !exists {
			continue
		}
		for _, stepData := range bin.Data.Steps {
			for visitorID := range stepData.Visitors {
				if as.shouldIncludeVisitor(visitorID, &models.SankeyFilters{VisitorType: visitorType}, knownFingerprints) {
					visitorEventCount[visitorID]++
				}
			}
		}
	}

	var userCounts []models.UserCount
	for id, count := range visitorEventCount {
		userCounts = append(userCounts, models.UserCount{
			ID:      id,
			Count:   count,
			IsKnown: knownFingerprints[id],
		})
	}

	sort.Slice(userCounts, func(i, j int) bool {
		if userCounts[i].Count != userCounts[j].Count {
			return userCounts[i].Count > userCounts[j].Count
		}
		return userCounts[i].ID < userCounts[j].ID
	})

	return userCounts, nil
}

// GetHourlyNodeActivity returns a breakdown of activity by content per hour.
func (as *AnalyticsService) GetHourlyNodeActivity(epinetID string, startHour, endHour *int) (models.HourlyActivity, error) {
	var hourKeys []string
	if startHour != nil && endHour != nil {
		hourKeys = utils.GetHourKeysForCustomRange(*startHour, *endHour)
	} else {
		hourKeys = utils.GetHourKeysForTimeRange(168)
	}

	hourlyActivity := make(models.HourlyActivity)
	for _, hourKey := range hourKeys {
		bin, exists := as.cache.GetHourlyEpinetBin(as.tenantID, epinetID, hourKey)
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
	return hourlyActivity, nil
}

// ComputeDashboard computes dashboard analytics from cached data.
func (as *AnalyticsService) ComputeDashboard(startHour, endHour int) (*models.DashboardAnalytics, error) {
	epinets, err := as.getEpinets()
	if err != nil {
		return nil, err
	}
	if len(epinets) == 0 {
		return as.createEmptyDashboardAnalytics(), nil
	}

	hourKeys := utils.GetHourKeysForCustomRange(startHour, endHour)
	dailyHourKeys := utils.GetHourKeysForCustomRange(24, 0)
	weeklyHourKeys := utils.GetHourKeysForCustomRange(168, 0)
	monthlyHourKeys := utils.GetHourKeysForCustomRange(672, 0)

	stats := models.TimeRangeStats{
		Daily:   as.computeAllEvents(epinets, dailyHourKeys),
		Weekly:  as.computeAllEvents(epinets, weeklyHourKeys),
		Monthly: as.computeAllEvents(epinets, monthlyHourKeys),
	}
	return &models.DashboardAnalytics{
		Stats:      stats,
		Line:       as.computeLineData(epinets, hourKeys),
		HotContent: as.computeHotContent(epinets, hourKeys),
	}, nil
}

// ComputeLeadMetrics computes lead metrics from cached data.
func (as *AnalyticsService) ComputeLeadMetrics(startHour, endHour int) (*models.LeadMetrics, error) {
	epinets, err := as.getEpinets()
	if err != nil {
		return nil, err
	}
	if len(epinets) == 0 {
		return as.createEmptyLeadMetrics(), nil
	}

	knownFingerprints, err := as.getKnownFingerprints()
	if err != nil {
		return nil, err
	}

	dailyHourKeys := utils.GetHourKeysForCustomRange(24, 0)
	weeklyHourKeys := utils.GetHourKeysForCustomRange(168, 0)
	monthlyHourKeys := utils.GetHourKeysForCustomRange(672, 0)
	customHourKeys := utils.GetHourKeysForCustomRange(startHour, endHour)

	dailyMetrics := as.aggregateHourlyVisitorMetrics(epinets, dailyHourKeys, knownFingerprints)
	weeklyMetrics := as.aggregateHourlyVisitorMetrics(epinets, weeklyHourKeys, knownFingerprints)
	monthlyMetrics := as.aggregateHourlyVisitorMetrics(epinets, monthlyHourKeys, knownFingerprints)
	customMetrics := as.aggregateHourlyVisitorMetrics(epinets, customHourKeys, knownFingerprints)

	daily24hTotal := len(dailyMetrics.AnonymousVisitors) + len(dailyMetrics.KnownVisitors)
	var firstTime24hPercentage, returning24hPercentage float64
	if daily24hTotal > 0 {
		firstTime24hPercentage = float64(len(dailyMetrics.AnonymousVisitors)) / float64(daily24hTotal) * 100
		returning24hPercentage = float64(len(dailyMetrics.KnownVisitors)) / float64(daily24hTotal) * 100
	}

	weekly7dTotal := len(weeklyMetrics.AnonymousVisitors) + len(weeklyMetrics.KnownVisitors)
	var firstTime7dPercentage, returning7dPercentage float64
	if weekly7dTotal > 0 {
		firstTime7dPercentage = float64(len(weeklyMetrics.AnonymousVisitors)) / float64(weekly7dTotal) * 100
		returning7dPercentage = float64(len(weeklyMetrics.KnownVisitors)) / float64(weekly7dTotal) * 100
	}

	monthly28dTotal := len(monthlyMetrics.AnonymousVisitors) + len(monthlyMetrics.KnownVisitors)
	var firstTime28dPercentage, returning28dPercentage float64
	if monthly28dTotal > 0 {
		firstTime28dPercentage = float64(len(monthlyMetrics.AnonymousVisitors)) / float64(monthly28dTotal) * 100
		returning28dPercentage = float64(len(monthlyMetrics.KnownVisitors)) / float64(monthly28dTotal) * 100
	}

	return &models.LeadMetrics{
		TotalVisits:            len(customMetrics.AnonymousVisitors) + len(customMetrics.KnownVisitors),
		LastActivity:           as.getLastActivityTime(),
		FirstTime24h:           len(dailyMetrics.AnonymousVisitors),
		Returning24h:           len(dailyMetrics.KnownVisitors),
		FirstTime7d:            len(weeklyMetrics.AnonymousVisitors),
		Returning7d:            len(weeklyMetrics.KnownVisitors),
		FirstTime28d:           len(monthlyMetrics.AnonymousVisitors),
		Returning28d:           len(monthlyMetrics.KnownVisitors),
		FirstTime24hPercentage: firstTime24hPercentage,
		Returning24hPercentage: returning24hPercentage,
		FirstTime7dPercentage:  firstTime7dPercentage,
		Returning7dPercentage:  returning7dPercentage,
		FirstTime28dPercentage: firstTime28dPercentage,
		Returning28dPercentage: returning28dPercentage,
		TotalLeads:             as.getTotalLeads(),
	}, nil
}

// =================================================================================
// INTERNAL HELPERS
// =================================================================================

// calculateIntersection finds the number of common elements between two sets.
func (as *AnalyticsService) calculateIntersection(setA, setB map[string]bool) int {
	count := 0
	if len(setA) > len(setB) {
		setA, setB = setB, setA // Iterate over the smaller set for efficiency
	}
	for key := range setA {
		if setB[key] {
			count++
		}
	}
	return count
}

// getNodeIndex gets or creates a unique integer index for a node ID.
func (as *AnalyticsService) getNodeIndex(nodeID string, indexMap *map[string]int) int {
	if idx, exists := (*indexMap)[nodeID]; exists {
		return idx
	}
	newIndex := len(*indexMap)
	(*indexMap)[nodeID] = newIndex
	return newIndex
}

// generateOriginalNodeName correctly generates node names with proper fallbacks and specific overrides.
func (as *AnalyticsService) generateOriginalNodeName(nodeID string, contentItems map[string]models.ContentItem) string {
	parts := strings.Split(nodeID, "-")
	if len(parts) < 2 {
		return "Unknown Node"
	}

	contentID := parts[len(parts)-1]
	content, contentExists := contentItems[contentID]
	contentTitle := ""
	if contentExists {
		contentTitle = content.Title
		if contentTitle == "" {
			contentTitle = content.Slug
		}
	}
	if contentTitle == "" {
		contentTitle = "Unknown Content"
	}

	gateType := parts[0]
	verb := ""
	if len(parts) > 2 {
		verb = strings.ToUpper(parts[len(parts)-2])
	} else if len(parts) > 1 {
		verb = strings.ToUpper(parts[0])
	}

	switch gateType {
	case "commitmentAction", "conversionAction":
		return fmt.Sprintf("%s: %s", verb, contentTitle)
	case "belief":
		return fmt.Sprintf("Believes: %s", parts[1])
	case "identifyAs":
		return fmt.Sprintf("Identifies as: %s", parts[1])
	}
	return contentTitle
}

func (as *AnalyticsService) shouldIncludeVisitor(visitor string, filters *models.SankeyFilters, knownFingerprints map[string]bool) bool {
	if filters == nil {
		return true
	}
	if filters.SelectedUserID != nil && *filters.SelectedUserID != visitor {
		return false
	}
	isKnown := knownFingerprints[visitor]
	switch filters.VisitorType {
	case "known":
		return isKnown
	case "anonymous":
		return !isKnown
	default:
		return true
	}
}

func (as *AnalyticsService) getEpinets() ([]models.EpinetConfig, error) {
	if as.ctx == nil {
		return nil, fmt.Errorf("tenant context not set")
	}
	epinetService := content.NewEpinetService(as.ctx, cache.GetGlobalManager())
	epinetIDs, err := epinetService.GetAllIDs()
	if err != nil {
		return nil, err
	}
	epinetNodes, err := epinetService.GetByIDs(epinetIDs)
	if err != nil {
		return nil, err
	}
	var epinets []models.EpinetConfig
	for _, node := range epinetNodes {
		if node != nil {
			var steps []models.EpinetStep
			for _, nodeStep := range node.Steps {
				step := models.EpinetStep{GateType: nodeStep.GateType, Title: nodeStep.Title, Values: nodeStep.Values, ObjectIds: nodeStep.ObjectIDs}
				if nodeStep.ObjectType != nil {
					step.ObjectType = *nodeStep.ObjectType
				}
				steps = append(steps, step)
			}
			epinets = append(epinets, models.EpinetConfig{ID: node.ID, Title: node.Title, Steps: steps})
		}
	}
	return epinets, nil
}

func (as *AnalyticsService) getKnownFingerprints() (map[string]bool, error) {
	if as.ctx == nil {
		return nil, fmt.Errorf("tenant context not set")
	}
	query := `SELECT id, CASE WHEN lead_id IS NOT NULL THEN 1 ELSE 0 END as is_known FROM fingerprints`
	rows, err := as.ctx.Database.Conn.Query(query)
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

func (as *AnalyticsService) getContentItems() (map[string]models.ContentItem, error) {
	if as.ctx == nil {
		return nil, fmt.Errorf("tenant context not set")
	}
	contentItems := make(map[string]models.ContentItem)
	storyFragmentService := content.NewStoryFragmentService(as.ctx, cache.GetGlobalManager())
	storyFragmentIDs, err := storyFragmentService.GetAllIDs()
	if err != nil {
		return nil, err
	}
	storyFragments, err := storyFragmentService.GetByIDs(storyFragmentIDs)
	if err != nil {
		return nil, err
	}
	for _, sf := range storyFragments {
		if sf != nil {
			contentItems[sf.ID] = models.ContentItem{Title: sf.Title, Slug: sf.Slug}
		}
	}
	paneService := content.NewPaneService(as.ctx, cache.GetGlobalManager())
	paneIDs, err := paneService.GetAllIDs()
	if err != nil {
		return nil, err
	}
	panes, err := paneService.GetByIDs(paneIDs)
	if err != nil {
		return nil, err
	}
	for _, pane := range panes {
		if pane != nil {
			contentItems[pane.ID] = models.ContentItem{Title: pane.Title, Slug: pane.Slug}
		}
	}
	return contentItems, nil
}

func (as *AnalyticsService) createEmptyDashboardAnalytics() *models.DashboardAnalytics {
	return &models.DashboardAnalytics{
		Stats:      models.TimeRangeStats{Daily: 0, Weekly: 0, Monthly: 0},
		Line:       []models.LineDataSeries{},
		HotContent: []models.HotItem{},
	}
}

func (as *AnalyticsService) createEmptyLeadMetrics() *models.LeadMetrics {
	return &models.LeadMetrics{}
}

func (as *AnalyticsService) computeAllEvents(epinets []models.EpinetConfig, hourKeys []string) int {
	total := 0
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			if bin, exists := as.cache.GetHourlyEpinetBin(as.tenantID, epinet.ID, hourKey); exists {
				for _, stepData := range bin.Data.Steps {
					total += len(stepData.Visitors)
				}
			}
		}
	}
	return total
}

func (as *AnalyticsService) computeLineData(epinets []models.EpinetConfig, hourKeys []string) []models.LineDataSeries {
	eventsByHour := make(map[string]int)
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			if bin, exists := as.cache.GetHourlyEpinetBin(as.tenantID, epinet.ID, hourKey); exists {
				for _, stepData := range bin.Data.Steps {
					eventsByHour[hourKey] += len(stepData.Visitors)
				}
			}
		}
	}
	var lineData []models.LineDataPoint
	for _, hourKey := range hourKeys {
		lineData = append(lineData, models.LineDataPoint{X: hourKey, Y: eventsByHour[hourKey]})
	}
	return []models.LineDataSeries{{ID: "events", Data: lineData}}
}

func (as *AnalyticsService) computeHotContent(epinets []models.EpinetConfig, hourKeys []string) []models.HotItem {
	contentCounts := make(map[string]int)

	// Aggregate content activity across all epinets and hours
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			if bin, exists := as.cache.GetHourlyEpinetBin(as.tenantID, epinet.ID, hourKey); exists {
				// Count events by content ID
				for stepID, stepData := range bin.Data.Steps {
					contentID := as.extractContentIDFromNodeID(stepID)
					if contentID != "" && as.isStoryFragmentStep(stepID) {
						contentCounts[contentID] += len(stepData.Visitors)
					}
				}
			}
		}
	}

	// Format and sort the result
	var sortedContent []models.HotItem
	for id, totalEvents := range contentCounts {
		sortedContent = append(sortedContent, models.HotItem{
			ID:          id,
			TotalEvents: totalEvents,
		})
	}

	// Sort by total events descending
	for i := 0; i < len(sortedContent)-1; i++ {
		for j := i + 1; j < len(sortedContent); j++ {
			if sortedContent[j].TotalEvents > sortedContent[i].TotalEvents {
				sortedContent[i], sortedContent[j] = sortedContent[j], sortedContent[i]
			}
		}
	}

	// Limit to top 10
	maxItems := 10
	if len(sortedContent) > maxItems {
		sortedContent = sortedContent[:maxItems]
	}

	return sortedContent
}

// extractContentIDFromNodeID extracts the content ID from a node ID
func (as *AnalyticsService) extractContentIDFromNodeID(nodeID string) string {
	parts := strings.Split(nodeID, "_") // ← CHANGE FROM "-" TO "_"
	if len(parts) < 2 {
		return ""
	}
	// Content ID is always the last part
	return parts[len(parts)-1]
}

// isStoryFragmentStep checks if the stepID represents a StoryFragment event
func (as *AnalyticsService) isStoryFragmentStep(stepID string) bool {
	return strings.Contains(stepID, "_StoryFragment_") // ← CHANGE TO UNDERSCORE
}

func (as *AnalyticsService) aggregateHourlyVisitorMetrics(epinets []models.EpinetConfig, hourKeys []string, knownFingerprints map[string]bool) struct {
	AnonymousVisitors map[string]bool
	KnownVisitors     map[string]bool
} {
	result := struct {
		AnonymousVisitors map[string]bool
		KnownVisitors     map[string]bool
	}{
		AnonymousVisitors: make(map[string]bool),
		KnownVisitors:     make(map[string]bool),
	}
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			if bin, exists := as.cache.GetHourlyEpinetBin(as.tenantID, epinet.ID, hourKey); exists {
				for _, stepData := range bin.Data.Steps {
					for visitor := range stepData.Visitors {
						if knownFingerprints[visitor] {
							result.KnownVisitors[visitor] = true
						} else {
							result.AnonymousVisitors[visitor] = true
						}
					}
				}
			}
		}
	}
	return result
}

func (as *AnalyticsService) getLastActivityTime() string {
	return time.Now().Format("2006-01-02T15:04:05Z")
}

func (as *AnalyticsService) getTotalLeads() int {
	if as.ctx == nil {
		return 0
	}
	var count int
	query := `SELECT COUNT(*) FROM leads`
	if err := as.ctx.Database.Conn.QueryRow(query).Scan(&count); err != nil {
		return 0
	}
	return count
}
