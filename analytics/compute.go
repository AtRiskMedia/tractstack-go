// Package analytics provides analytics computation and aggregation functionality.
package analytics

import (
	"fmt"
	"sort"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// =============================================================================
// Analytics Processing Types (Exact V1 Translation)
// =============================================================================

type SankeyFilters struct {
	VisitorType    string  `json:"visitorType"` // "all", "anonymous", "known"
	SelectedUserID *string `json:"selectedUserId"`
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
	query := `SELECT DISTINCT id FROM fingerprints WHERE lead_id IS NOT NULL`

	rows, err := ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query known fingerprints: %w", err)
	}
	defer rows.Close()

	knownFingerprints := make(map[string]bool)
	for rows.Next() {
		var fingerprintID string
		err := rows.Scan(&fingerprintID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan fingerprint row: %w", err)
		}
		knownFingerprints[fingerprintID] = true
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
