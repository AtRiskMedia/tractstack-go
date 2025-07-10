// Package analytics provides node management functionality.
package analytics

import (
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

func addNodeVisitor(ctx *tenant.Context, epinetID, hourKey string, step EpinetStep, contentID, fingerprintID string, stepIndex int, contentItems map[string]ContentItem, matchedVerb string) error {
	nodeID := getStepNodeID(step, contentID, matchedVerb)
	nodeName := getNodeName(step, contentID, contentItems, matchedVerb)
	cacheManager := cache.GetGlobalManager()
	bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey)
	if !exists {
		emptyData := &models.HourlyEpinetData{
			Steps:       make(map[string]*models.HourlyEpinetStepData),
			Transitions: make(map[string]map[string]*models.HourlyEpinetTransitionData),
		}
		bin = &models.HourlyEpinetBin{
			Data:       emptyData,
			ComputedAt: time.Now().UTC(),
			TTL: func() time.Duration {
				now := time.Now().UTC()
				currentHourKey := formatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))
				if hourKey == currentHourKey {
					return config.CurrentHourTTL
				}
				return config.AnalyticsBinTTL
			}(),
		}
	}
	if bin.Data.Steps[nodeID] == nil {
		bin.Data.Steps[nodeID] = &models.HourlyEpinetStepData{
			Visitors:  make(map[string]bool),
			Name:      nodeName,
			StepIndex: stepIndex + 1,
		}
	}
	bin.Data.Steps[nodeID].Visitors[fingerprintID] = true
	cacheManager.SetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey, bin)
	return nil
}

// getStepNodeID generates step node IDs using exact V1 logic
func getStepNodeID(step EpinetStep, contentID string, matchedVerb string) string {
	parts := []string{step.GateType}

	switch step.GateType {
	case "belief", "identifyAs":
		if len(step.Values) > 0 {
			parts = append(parts, step.Values[0])
		}
	case "commitmentAction", "conversionAction":
		parts = append(parts, step.ObjectType)

		if matchedVerb != "" && containsString(step.Values, matchedVerb) {
			parts = append(parts, matchedVerb)
		} else if len(step.Values) > 0 {
			parts = append(parts, step.Values[0])
		}
	}

	parts = append(parts, contentID)
	return strings.Join(parts, "-")
}

// getNodeName generates human-readable node names using exact V1 logic
func getNodeName(step EpinetStep, contentID string, contentItems map[string]ContentItem, matchedVerb string) string {
	content := contentItems[contentID]
	contentTitle := content.Title
	if contentTitle == "" {
		contentTitle = content.Slug
	}
	if contentTitle == "" {
		contentTitle = "Unknown Content"
	}

	switch step.GateType {
	case "belief":
		title := step.Title
		if title == "" && len(step.Values) > 0 {
			title = strings.Join(step.Values, "/")
		}
		return fmt.Sprintf("Believes: %s", title)

	case "identifyAs":
		title := step.Title
		if title == "" && len(step.Values) > 0 {
			title = strings.Join(step.Values, "/")
		}
		return fmt.Sprintf("Identifies as: %s", title)

	case "commitmentAction", "conversionAction":
		actionVerb := matchedVerb
		if actionVerb == "" && len(step.Values) > 0 {
			actionVerb = step.Values[0]
		}
		return fmt.Sprintf("%s: %s", actionVerb, contentTitle)
	}

	return step.Title
}
