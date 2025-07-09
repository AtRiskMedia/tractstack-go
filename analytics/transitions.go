// Package analytics provides transition calculation functionality.
package analytics

import (
	"fmt"
	"sort"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// calculateChronologicalTransitions calculates transitions between consecutive steps only (exact V1 pattern)
func calculateChronologicalTransitions(ctx *tenant.Context, epinetID, hourKey string) error {
	cacheManager := cache.GetGlobalManager()

	// Get the epinet bin
	bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey)
	if !exists {
		return fmt.Errorf("epinet bin not found for %s:%s", epinetID, hourKey)
	}

	hourData := bin.Data

	// Create a map of visitors to the nodes they've visited, grouped by step index (exact V1 pattern)
	visitorNodesByStep := make(map[string]map[int][]string)

	// Populate visitor node data, organized by step (exact V1 pattern)
	for nodeID, nodeData := range hourData.Steps {
		for visitorID := range nodeData.Visitors {
			if visitorNodesByStep[visitorID] == nil {
				visitorNodesByStep[visitorID] = make(map[int][]string)
			}

			stepIndex := nodeData.StepIndex
			if visitorNodesByStep[visitorID][stepIndex] == nil {
				visitorNodesByStep[visitorID][stepIndex] = make([]string, 0)
			}

			visitorNodesByStep[visitorID][stepIndex] = append(visitorNodesByStep[visitorID][stepIndex], nodeID)
		}
	}

	// Initialize transitions map
	if hourData.Transitions == nil {
		hourData.Transitions = make(map[string]map[string]*models.HourlyEpinetTransitionData)
	}

	// For each visitor, create transitions only between consecutive steps (exact V1 pattern)
	for visitorID, nodesByStep := range visitorNodesByStep {
		// Get step indices and sort them
		var stepIndices []int
		for stepIndex := range nodesByStep {
			stepIndices = append(stepIndices, stepIndex)
		}
		sort.Ints(stepIndices)

		// Skip if user only visited nodes in one step
		if len(stepIndices) < 2 {
			continue
		}

		// For each consecutive pair of steps (exact V1 pattern)
		for i := 0; i < len(stepIndices)-1; i++ {
			currentStepIndex := stepIndices[i]
			nextStepIndex := stepIndices[i+1]

			// Only create transitions where steps are consecutive (exact V1 pattern)
			if nextStepIndex != currentStepIndex+1 {
				continue
			}

			currentStepNodes := nodesByStep[currentStepIndex]
			nextStepNodes := nodesByStep[nextStepIndex]

			// Create transitions from each node in current step to each node in next step (exact V1 pattern)
			for _, fromNodeID := range currentStepNodes {
				for _, toNodeID := range nextStepNodes {
					addVisitorToTransition(hourData, fromNodeID, toNodeID, visitorID)
				}
			}
		}
	}

	// Save updated data back to cache
	bin.Data = hourData
	bin.ComputedAt = time.Now()
	cacheManager.SetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey, bin)

	return nil
}

// addVisitorToTransition adds a visitor to a transition (exact V1 pattern)
func addVisitorToTransition(hourData *models.HourlyEpinetData, fromNodeID, toNodeID, visitorID string) {
	if hourData.Transitions[fromNodeID] == nil {
		hourData.Transitions[fromNodeID] = make(map[string]*models.HourlyEpinetTransitionData)
	}

	if hourData.Transitions[fromNodeID][toNodeID] == nil {
		hourData.Transitions[fromNodeID][toNodeID] = &models.HourlyEpinetTransitionData{
			Visitors: make(map[string]bool),
		}
	}

	hourData.Transitions[fromNodeID][toNodeID].Visitors[visitorID] = true
}
