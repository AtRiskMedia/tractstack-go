// Package analytics provides analytics computation and aggregation functionality.
package analytics

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// =============================================================================
// Analytics Processing Types (Exact V1 Translation)
// =============================================================================

type SankeyFilters struct {
	VisitorType    string  `json:"visitorType"` // "all", "anonymous", "known"
	SelectedUserID *string `json:"userId"`
	StartHour      *int    `json:"startHour"`
	EndHour        *int    `json:"endHour"`
}

type SankeyDiagram struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Nodes []SankeyNode `json:"nodes"`
	Links []SankeyLink `json:"links"`
}

type SankeyNode struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type SankeyLink struct {
	Source int `json:"source"`
	Target int `json:"target"`
	Value  int `json:"value"`
}

const maxNodes = 60

// =============================================================================
// Core Computation Functions (Exact V1 Translation)
// =============================================================================

// ComputeEpinetSankey generates a Sankey diagram from cached epinet data (exact V1 pattern)
func ComputeEpinetSankey(ctx *tenant.Context, epinetID string, filters *SankeyFilters) (*SankeyDiagram, error) {
	// Get hour keys for the time range (exact V1 pattern)
	var hourKeys []string
	if filters != nil && filters.StartHour != nil && filters.EndHour != nil {
		hourKeys = getHourKeysForCustomRange(*filters.StartHour, *filters.EndHour)
	} else {
		hourKeys = getHourKeysForTimeRange(168)
	}

	// Get known fingerprints for visitor filtering (exact V1 pattern)
	knownFingerprints, err := getKnownFingerprints(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get known fingerprints: %w", err)
	}

	// Build visitor count data by node (exact V1 pattern)
	nodeCounts := make(map[string]map[string]bool)                  // nodeID -> visitors set
	nodeNames := make(map[string]string)                            // nodeID -> name
	nodeStepIndices := make(map[string]int)                         // nodeID -> stepIndex
	transitionCounts := make(map[string]map[string]map[string]bool) // fromNode -> toNode -> visitors set

	cacheManager := cache.GetGlobalManager()

	// Process each hour's data (exact V1 pattern)
	for _, hourKey := range hourKeys {
		bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey)
		if !exists {
			continue
		}

		hourData := bin.Data

		// Collect node visitor data (exact V1 pattern)
		for nodeID, stepData := range hourData.Steps {
			if nodeCounts[nodeID] == nil {
				nodeCounts[nodeID] = make(map[string]bool)
				nodeNames[nodeID] = stepData.Name
				nodeStepIndices[nodeID] = stepData.StepIndex
			}

			// Add visitors to node counts, applying filters if specified (exact V1 pattern)
			for visitor := range stepData.Visitors {
				if shouldIncludeVisitor(visitor, filters, knownFingerprints) {
					nodeCounts[nodeID][visitor] = true
				}
			}
		}

		// Collect transition data (exact V1 pattern)
		for fromNode, toNodes := range hourData.Transitions {
			if transitionCounts[fromNode] == nil {
				transitionCounts[fromNode] = make(map[string]map[string]bool)
			}

			for toNode, transitionData := range toNodes {
				if transitionCounts[fromNode][toNode] == nil {
					transitionCounts[fromNode][toNode] = make(map[string]bool)
				}

				// Apply filters to transitions if specified (exact V1 pattern)
				for visitor := range transitionData.Visitors {
					if shouldIncludeVisitor(visitor, filters, knownFingerprints) {
						transitionCounts[fromNode][toNode][visitor] = true
					}
				}
			}
		}
	}

	// Sort nodes by count to keep the most significant ones (exact V1 pattern)
	type nodeInfo struct {
		id    string
		name  string
		count int
	}

	var nodeList []nodeInfo
	for nodeID, visitors := range nodeCounts {
		nodeList = append(nodeList, nodeInfo{
			id:    nodeID,
			name:  nodeNames[nodeID],
			count: len(visitors),
		})
	}

	sort.Slice(nodeList, func(i, j int) bool {
		return nodeList[i].count > nodeList[j].count
	})

	// Limit to maxNodes (exact V1 pattern)
	if len(nodeList) > maxNodes {
		nodeList = nodeList[:maxNodes]
	}

	// Create a map of node IDs to indices in the nodes array (exact V1 pattern)
	nodeIndexMap := make(map[string]int)
	nodes := make([]SankeyNode, len(nodeList))
	for i, node := range nodeList {
		nodeIndexMap[node.id] = i
		nodes[i] = SankeyNode{
			Name: node.name,
			ID:   node.id,
		}
	}

	// Convert transition data to links array (exact V1 pattern)
	var links []SankeyLink
	for fromNode, toNodes := range transitionCounts {
		sourceIndex, sourceExists := nodeIndexMap[fromNode]
		if !sourceExists {
			continue // Skip nodes not in our top nodes
		}

		for toNode, visitors := range toNodes {
			targetIndex, targetExists := nodeIndexMap[toNode]
			if !targetExists {
				continue // Skip nodes not in our top nodes
			}

			// Only include links with a minimum number of visitors for clarity (exact V1 pattern)
			if len(visitors) >= 1 {
				links = append(links, SankeyLink{
					Source: sourceIndex,
					Target: targetIndex,
					Value:  len(visitors),
				})
			}
		}
	}

	// Filter out nodes that don't have any connections (exact V1 pattern)
	connectedNodeIndices := make(map[int]bool)
	for _, link := range links {
		connectedNodeIndices[link.Source] = true
		connectedNodeIndices[link.Target] = true
	}

	// Only keep nodes that appear in at least one link (exact V1 pattern)
	var filteredNodes []SankeyNode
	indexMapping := make(map[int]int)
	newIndex := 0
	for i, node := range nodes {
		if connectedNodeIndices[i] {
			filteredNodes = append(filteredNodes, node)
			indexMapping[i] = newIndex
			newIndex++
		}
	}

	// Remap source and target indices in links (exact V1 pattern)
	for i := range links {
		links[i].Source = indexMapping[links[i].Source]
		links[i].Target = indexMapping[links[i].Target]
	}

	return &SankeyDiagram{
		ID:    epinetID,
		Title: "User Journey Flow",
		Nodes: filteredNodes,
		Links: links,
	}, nil
}

// shouldIncludeVisitor determines if a visitor should be included based on filters (exact V1 pattern)
func shouldIncludeVisitor(visitor string, filters *SankeyFilters, knownFingerprints map[string]bool) bool {
	if filters == nil {
		return true
	}

	// Check specific user filter
	if filters.SelectedUserID != nil && *filters.SelectedUserID != visitor {
		return false
	}

	// Check visitor type filter
	isKnown := knownFingerprints[visitor]
	switch filters.VisitorType {
	case "known":
		return isKnown
	case "anonymous":
		return !isKnown
	case "all":
		return true
	default:
		return true
	}
}

// getKnownFingerprints retrieves known fingerprints from the database (exact V1 pattern)
func getKnownFingerprints(ctx *tenant.Context) (map[string]bool, error) {
	query := `SELECT id, CASE WHEN lead_id IS NOT NULL THEN 1 ELSE 0 END as is_known FROM fingerprints`

	rows, err := ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query fingerprints: %w", err)
	}
	defer rows.Close()

	knownFingerprints := make(map[string]bool)
	for rows.Next() {
		var fingerprintID string
		var isKnown bool
		err := rows.Scan(&fingerprintID, &isKnown)
		if err != nil {
			return nil, fmt.Errorf("failed to scan fingerprint row: %w", err)
		}
		knownFingerprints[fingerprintID] = isKnown
	}

	return knownFingerprints, nil
}

// getHourKeysForCustomRange generates hour keys for a custom range (exact V1 pattern)
func getHourKeysForCustomRange(startHour, endHour int) []string {
	var hourKeys []string

	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)

	// Ensure proper order (min to max)
	minHour := endHour
	maxHour := startHour
	if startHour < endHour {
		minHour = startHour
		maxHour = endHour
	}

	for i := maxHour; i >= minHour; i-- {
		hourTime := currentHour.Add(-time.Duration(i) * time.Hour)
		hourKey := formatHourKey(hourTime)
		hourKeys = append(hourKeys, hourKey)
	}

	return hourKeys
}

// ComputeStoryfragmentAnalytics computes analytics for all storyfragments from cached epinet data
func ComputeStoryfragmentAnalytics(ctx *tenant.Context) ([]StoryfragmentAnalytics, error) {
	// Get all epinets for the tenant (same pattern as dashboard analytics)
	epinets, err := getEpinets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets: %w", err)
	}

	if len(epinets) == 0 {
		return []StoryfragmentAnalytics{}, nil
	}

	// Get hour keys for different time periods
	hours24 := getHourKeysForTimeRange(24)
	hours7d := getHourKeysForTimeRange(168)  // 7 days
	hours28d := getHourKeysForTimeRange(672) // 28 days

	// Get known fingerprints for lead counting
	knownFingerprints, err := getKnownFingerprints(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get known fingerprints: %w", err)
	}

	cacheManager := cache.GetGlobalManager()

	// Map to accumulate data by storyfragment ID
	storyfragmentData := make(map[string]*StoryfragmentAnalytics)

	// Process each epinet and each hour key (same pattern as dashboard analytics)
	for _, epinet := range epinets {
		// Combine all hour keys for processing
		allHourKeys := make(map[string]bool)
		for _, hourKey := range hours24 {
			allHourKeys[hourKey] = true
		}
		for _, hourKey := range hours7d {
			allHourKeys[hourKey] = true
		}
		for _, hourKey := range hours28d {
			allHourKeys[hourKey] = true
		}

		// Process each hour
		for hourKey := range allHourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists || bin == nil || bin.Data == nil {
				continue
			}

			// Determine which time periods this hour belongs to
			isIn24h := containsString(hours24, hourKey)
			isIn7d := containsString(hours7d, hourKey)
			isIn28d := containsString(hours28d, hourKey)

			// Process each step in this hour to extract storyfragment data
			for stepID, stepData := range bin.Data.Steps {
				if stepData == nil {
					continue
				}

				// Parse step ID to extract storyfragment information
				// Expected format: "commitmentAction-StoryFragment-VERB-storyfragmentID"
				stepParts := strings.Split(stepID, "-")
				if len(stepParts) < 4 {
					continue
				}

				// Only process StoryFragment events
				if stepParts[1] != "StoryFragment" {
					continue
				}

				storyfragmentID := stepParts[len(stepParts)-1]

				// Initialize storyfragment data if needed
				if _, exists := storyfragmentData[storyfragmentID]; !exists {
					storyfragmentData[storyfragmentID] = &StoryfragmentAnalytics{
						ID:                    storyfragmentID,
						TotalActions:          0,
						UniqueVisitors:        0,
						Last24hActions:        0,
						Last7dActions:         0,
						Last28dActions:        0,
						Last24hUniqueVisitors: 0,
						Last7dUniqueVisitors:  0,
						Last28dUniqueVisitors: 0,
						TotalLeads:            len(knownFingerprints),
					}
				}

				data := storyfragmentData[storyfragmentID]
				visitorCount := len(stepData.Visitors)

				// Update total actions and visitors
				data.TotalActions += visitorCount

				// Update time-specific metrics
				if isIn24h {
					data.Last24hActions += visitorCount
				}
				if isIn7d {
					data.Last7dActions += visitorCount
				}
				if isIn28d {
					data.Last28dActions += visitorCount
				}
			}
		}
	}

	// Calculate unique visitors for each time period (second pass)
	for storyfragmentID, data := range storyfragmentData {
		uniqueVisitors := make(map[string]bool)
		visitors24h := make(map[string]bool)
		visitors7d := make(map[string]bool)
		visitors28d := make(map[string]bool)

		// Process all epinets and hours again to count unique visitors
		for _, epinet := range epinets {
			allHourKeys := make(map[string]bool)
			for _, hourKey := range hours24 {
				allHourKeys[hourKey] = true
			}
			for _, hourKey := range hours7d {
				allHourKeys[hourKey] = true
			}
			for _, hourKey := range hours28d {
				allHourKeys[hourKey] = true
			}

			for hourKey := range allHourKeys {
				bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
				if !exists || bin == nil || bin.Data == nil {
					continue
				}

				isIn24h := containsString(hours24, hourKey)
				isIn7d := containsString(hours7d, hourKey)
				isIn28d := containsString(hours28d, hourKey)

				// Check each step for this storyfragment
				for stepID, stepData := range bin.Data.Steps {
					stepParts := strings.Split(stepID, "-")
					if len(stepParts) < 4 || stepParts[1] != "StoryFragment" {
						continue
					}

					if stepParts[len(stepParts)-1] == storyfragmentID {
						// Add visitors to appropriate sets
						for visitor := range stepData.Visitors {
							uniqueVisitors[visitor] = true
							if isIn24h {
								visitors24h[visitor] = true
							}
							if isIn7d {
								visitors7d[visitor] = true
							}
							if isIn28d {
								visitors28d[visitor] = true
							}
						}
					}
				}
			}
		}

		// Update visitor counts
		data.UniqueVisitors = len(uniqueVisitors)
		data.Last24hUniqueVisitors = len(visitors24h)
		data.Last7dUniqueVisitors = len(visitors7d)
		data.Last28dUniqueVisitors = len(visitors28d)
	}

	// Get storyfragment metadata (titles and slugs)
	storyFragmentService := content.NewStoryFragmentService(ctx, cacheManager)
	storyfragmentIDs := make([]string, 0, len(storyfragmentData))
	for id := range storyfragmentData {
		storyfragmentIDs = append(storyfragmentIDs, id)
	}

	if len(storyfragmentIDs) > 0 {
		storyfragments, err := storyFragmentService.GetByIDs(storyfragmentIDs)
		if err != nil {
			log.Printf("Warning: failed to get storyfragment metadata: %v", err)
		} else {
			// Update slugs from metadata
			for _, sf := range storyfragments {
				if data, exists := storyfragmentData[sf.ID]; exists {
					data.Slug = sf.Slug
				}
			}
		}
	}

	// Convert map to slice and sort by total actions
	result := make([]StoryfragmentAnalytics, 0, len(storyfragmentData))
	for _, data := range storyfragmentData {
		result = append(result, *data)
	}

	// Sort by total actions descending
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].TotalActions > result[i].TotalActions {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}

// UserCount represents a visitor with their event count and known status
type UserCount struct {
	ID      string `json:"id"`
	Count   int    `json:"count"`
	IsKnown bool   `json:"isKnown"`
}

// HourlyActivity represents hour-by-hour content activity breakdown
type HourlyActivity map[string]map[string]struct {
	Events     map[string]int `json:"events"`
	VisitorIDs []string       `json:"visitorIds"`
}

// GetFilteredVisitorCounts processes hourly epinet data to return visitor counts
func GetFilteredVisitorCounts(
	ctx *tenant.Context,
	epinetID string,
	visitorType string, // "all", "anonymous", "known"
	startHour, endHour *int,
) ([]UserCount, error) {
	cacheManager := cache.GetGlobalManager()
	if cacheManager == nil {
		return []UserCount{}, fmt.Errorf("cache manager not available")
	}

	// Get hour keys based on time range
	var hourKeys []string
	if startHour != nil && endHour != nil {
		hourKeys = getHourKeysForCustomRange(*startHour, *endHour)
	} else {
		hourKeys = getHourKeysForTimeRange(168) // Default 1 week
	}

	// Get known fingerprints for filtering
	knownFingerprints, err := getKnownFingerprints(ctx)
	if err != nil {
		return []UserCount{}, err
	}

	// Collect visitor counts
	visitorMap := make(map[string]struct {
		count   int
		isKnown bool
	})

	// Process each hour
	for _, hourKey := range hourKeys {
		if data, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey); exists {
			// Process each step in this hour's data
			for _, stepData := range data.Data.Steps {
				for visitorID := range stepData.Visitors {
					isKnown := knownFingerprints[visitorID]

					// Update visitor map
					if existing, exists := visitorMap[visitorID]; exists {
						visitorMap[visitorID] = struct {
							count   int
							isKnown bool
						}{
							count:   existing.count + 1,
							isKnown: existing.isKnown || isKnown,
						}
					} else {
						visitorMap[visitorID] = struct {
							count   int
							isKnown bool
						}{
							count:   1,
							isKnown: isKnown,
						}
					}
				}
			}
		}
	}

	// Filter and convert to array
	var userCounts []UserCount
	for visitorID, data := range visitorMap {
		// Apply visitor type filter
		if visitorType == "all" ||
			(visitorType == "known" && data.isKnown) ||
			(visitorType == "anonymous" && !data.isKnown) {
			userCounts = append(userCounts, UserCount{
				ID:      visitorID,
				Count:   data.count,
				IsKnown: data.isKnown,
			})
		}
	}

	// Sort by count (descending) then by ID (ascending)
	sort.Slice(userCounts, func(i, j int) bool {
		if userCounts[i].Count != userCounts[j].Count {
			return userCounts[i].Count > userCounts[j].Count
		}
		return userCounts[i].ID < userCounts[j].ID
	})

	return userCounts, nil
}

// GetHourlyNodeActivity processes hourly epinet data to return hour-by-hour content breakdown
func GetHourlyNodeActivity(
	ctx *tenant.Context,
	epinetID string,
	visitorType string, // "all", "anonymous", "known"
	startHour, endHour *int,
	selectedUserID *string,
) (HourlyActivity, error) {
	cacheManager := cache.GetGlobalManager()
	if cacheManager == nil {
		return HourlyActivity{}, fmt.Errorf("cache manager not available")
	}

	// Get hour keys based on time range
	var hourKeys []string
	if startHour != nil && endHour != nil {
		hourKeys = getHourKeysForCustomRange(*startHour, *endHour)
	} else {
		hourKeys = getHourKeysForTimeRange(168) // Default 1 week
	}

	// Get known fingerprints for filtering
	knownFingerprints, err := getKnownFingerprints(ctx)
	if err != nil {
		return HourlyActivity{}, err
	}

	hourlyActivity := make(HourlyActivity)

	// Process each hour
	for _, hourKey := range hourKeys {
		if data, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey); exists {
			hourData := make(map[string]struct {
				Events     map[string]int `json:"events"`
				VisitorIDs []string       `json:"visitorIds"`
			})

			// Track unique visitors per content item for this hour
			contentVisitors := make(map[string]map[string]bool)

			// Process each step
			for nodeID, stepData := range data.Data.Steps {
				// Filter visitors based on type
				var filteredVisitors []string
				for visitorID := range stepData.Visitors {
					isKnown := knownFingerprints[visitorID]

					// Apply visitor type filter
					if visitorType == "all" ||
						(visitorType == "known" && isKnown) ||
						(visitorType == "anonymous" && !isKnown) {

						// Apply selected user filter if specified
						if selectedUserID == nil || *selectedUserID == visitorID {
							filteredVisitors = append(filteredVisitors, visitorID)
						}
					}
				}

				if len(filteredVisitors) == 0 {
					continue
				}

				// Parse nodeID to extract contentID and verb
				// Expected format: actionType-contentType-VERB-contentID
				parts := strings.Split(nodeID, "-")
				if len(parts) >= 4 {
					contentID := parts[len(parts)-1]
					verb := parts[len(parts)-2]

					// Initialize content data if needed
					if _, exists := hourData[contentID]; !exists {
						hourData[contentID] = struct {
							Events     map[string]int `json:"events"`
							VisitorIDs []string       `json:"visitorIds"`
						}{
							Events:     make(map[string]int),
							VisitorIDs: []string{},
						}
						contentVisitors[contentID] = make(map[string]bool)
					}

					// Update event count
					content := hourData[contentID]
					content.Events[verb] = len(filteredVisitors)

					// Track unique visitors
					for _, visitorID := range filteredVisitors {
						contentVisitors[contentID][visitorID] = true
					}

					hourData[contentID] = content
				}
			}

			// Update visitor IDs for each content
			for contentID, visitors := range contentVisitors {
				if content, exists := hourData[contentID]; exists {
					var visitorIDs []string
					for visitorID := range visitors {
						visitorIDs = append(visitorIDs, visitorID)
					}
					content.VisitorIDs = visitorIDs
					hourData[contentID] = content
				}
			}

			// Only add hour if it has content
			if len(hourData) > 0 {
				hourlyActivity[hourKey] = hourData
			}
		}
	}

	return hourlyActivity, nil
}
