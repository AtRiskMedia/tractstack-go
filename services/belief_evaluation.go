// Package services provides belief evaluation logic ported from V1 UseFilterPane.ts
package services

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
)

// BeliefEvaluationEngine handles pane visibility evaluation based on user beliefs
type BeliefEvaluationEngine struct{}

// NewBeliefEvaluationEngine creates a new belief evaluation engine
func NewBeliefEvaluationEngine() *BeliefEvaluationEngine {
	return &BeliefEvaluationEngine{}
}

// EvaluatePaneVisibility is the main entry point for belief evaluation
// Returns: "visible", "hidden", or "empty" based on belief requirements
func (bee *BeliefEvaluationEngine) EvaluatePaneVisibility(
	paneBeliefs models.PaneBeliefData,
	userBeliefs map[string][]string,
) string {
	heldResult := bee.processHeldBeliefs(paneBeliefs, userBeliefs)
	withheldResult := bee.processWithheldBeliefs(paneBeliefs, userBeliefs)
	visibility := bee.calculateVisibility(paneBeliefs, heldResult, withheldResult)
	return visibility
}

// processHeldBeliefs handles belief requirements for showing content
func (bee *BeliefEvaluationEngine) processHeldBeliefs(
	paneBeliefs models.PaneBeliefData,
	userBeliefs map[string][]string,
) bool {
	// If no held beliefs required, allow visibility
	if len(paneBeliefs.HeldBeliefs) == 0 {
		return true
	}

	// Extract match-across keys from MatchAcross array (equivalent to V1's filter["MATCH-ACROSS"])
	matchAcrossKeys := paneBeliefs.MatchAcross // This is []string

	matchAcrossFilter := make(map[string][]string)
	regularFilter := make(map[string][]string)

	// Categorize keys into match-across and regular filters, skip LINKED-BELIEFS
	for key, values := range paneBeliefs.HeldBeliefs {
		// Skip null/empty values
		if len(values) == 0 {
			continue
		}

		// Check if this key is in match-across list
		isMatchAcross := false
		for _, matchKey := range matchAcrossKeys {
			if key == matchKey {
				isMatchAcross = true
				break
			}
		}

		if isMatchAcross {
			matchAcrossFilter[key] = values
		} else {
			regularFilter[key] = values
		}
	}

	// Evaluate match-across filter (OR logic - some() in V1)
	var matchAcrossResult bool
	if len(matchAcrossFilter) == 0 {
		matchAcrossResult = true
	} else {
		matchAcrossResult = false
		for key, valueOrValues := range matchAcrossFilter {
			if bee.hasMatchingBelief(userBeliefs, key, valueOrValues) {
				matchAcrossResult = true
				break // OR logic - one match is enough
			}
		}
	}

	// Evaluate regular filter (AND logic - every() in V1)
	var regularResult bool
	if len(regularFilter) == 0 {
		regularResult = true
	} else {
		regularResult = true
		for key, valueOrValues := range regularFilter {
			if !bee.hasMatchingBelief(userBeliefs, key, valueOrValues) {
				regularResult = false
				break // AND logic - one failure fails all
			}
		}
	}

	// Both filters must pass (V1: return matchAcrossResult && regularResult)
	return matchAcrossResult && regularResult
}

// processWithheldBeliefs handles belief requirements for hiding content
// Returns true if content should be shown (user doesn't have withheld beliefs)
// Returns false if content should be hidden (user has withheld beliefs)
func (bee *BeliefEvaluationEngine) processWithheldBeliefs(
	paneBeliefs models.PaneBeliefData,
	userBeliefs map[string][]string,
) bool {
	// If no withheld beliefs specified, allow visibility
	if len(paneBeliefs.WithheldBeliefs) == 0 {
		return true
	}

	// Check if user has ANY of the withheld beliefs
	// If they do, content should be hidden (return false)
	for key, prohibitedValues := range paneBeliefs.WithheldBeliefs {
		if bee.hasMatchingBelief(userBeliefs, key, prohibitedValues) {
			return false // User has a prohibited belief, hide content
		}
	}

	// User doesn't have any prohibited beliefs, show content
	return true
}

// calculateVisibility determines final visibility state
// Returns "visible", "hidden", or "empty"
func (bee *BeliefEvaluationEngine) calculateVisibility(
	paneBeliefs models.PaneBeliefData,
	heldResult bool,
	withheldResult bool,
) string {
	// Check if pane has any belief requirements
	hasHeldRequirements := len(paneBeliefs.HeldBeliefs) > 0 || len(paneBeliefs.MatchAcross) > 0
	hasWithheldRequirements := len(paneBeliefs.WithheldBeliefs) > 0

	// If pane has held belief requirements, user must satisfy them to be visible
	if hasHeldRequirements {
		if !heldResult {
			return "hidden" // User doesn't satisfy held requirements = hidden
		}
	}

	// If pane has withheld belief requirements, user must NOT have prohibited beliefs
	if hasWithheldRequirements {
		if !withheldResult {
			return "hidden" // User has prohibited beliefs = hidden
		}
	}

	// If we reach here, either:
	// 1. Pane has no belief requirements (always visible)
	// 2. Pane has requirements and user satisfies all of them
	return "visible"
}

// hasMatchingBelief checks if user has any of the required belief values for a given key
// Implements the V1 matchesBelief and hasMatchingBelief logic
func (bee *BeliefEvaluationEngine) hasMatchingBelief(
	userBeliefs map[string][]string,
	key string,
	requiredValues []string,
) bool {
	// Get user's beliefs for this key
	userValues, exists := userBeliefs[key]
	if !exists {
		return false
	}

	// Check if any user value matches any required value
	for _, userValue := range userValues {
		for _, requiredValue := range requiredValues {
			if bee.matchesBelief(userValue, requiredValue) {
				return true
			}
		}
	}

	return false
}

// matchesBelief implements the V1 belief matching logic
// Handles special cases like "*" wildcard and IDENTIFY_AS verb matching
func (bee *BeliefEvaluationEngine) matchesBelief(userValue, requiredValue string) bool {
	// Wildcard match
	if requiredValue == "*" {
		return true
	}

	// Exact match
	if userValue == requiredValue {
		return true
	}

	// For IDENTIFY_AS beliefs, we might need additional logic here
	// The V1 code suggests verb matching, but in the Go model we're working with
	// belief values directly. This can be extended if needed.

	return false
}

// ApplyVisibilityWrapper wraps HTML content based on visibility state
func (bee *BeliefEvaluationEngine) ApplyVisibilityWrapper(htmlContent, visibility string) string {
	// fmt.Printf("🔧 WRAPPER DEBUG: visibility=%s, content length=%d\n", visibility, len(htmlContent))

	switch visibility {
	case "visible":
		// fmt.Printf("🔧 WRAPPER DEBUG: Returning visible content unchanged\n")
		return htmlContent
	case "hidden":
		result := `<div style="display:none !important;">` + htmlContent + `</div>`
		// fmt.Printf("🔧 WRAPPER DEBUG: Applied hidden wrapper, new length=%d\n", len(result))
		return result
	case "empty":
		// fmt.Printf("🔧 WRAPPER DEBUG: Returning empty div\n")
		return `<div style="display:none !important;"></div>`
	default:
		// fmt.Printf("🔧 WRAPPER DEBUG: Unknown visibility '%s', returning unchanged\n", visibility)
		return htmlContent
	}
}

// GetUserBeliefsFromFingerprint retrieves user beliefs from fingerprint state
// Helper function to extract beliefs from cache for evaluation
func (bee *BeliefEvaluationEngine) GetUserBeliefsFromFingerprint(
	tenantID, fingerprintID string,
	cacheManager interface {
		GetFingerprintState(tenantID, fingerprintID string) (*models.FingerprintState, bool)
	},
) map[string][]string {
	if fingerprintState, exists := cacheManager.GetFingerprintState(tenantID, fingerprintID); exists {
		return fingerprintState.HeldBeliefs
	}

	// Return empty beliefs if fingerprint not found
	return make(map[string][]string)
}

// GetUserBeliefsFromSession retrieves user beliefs from session context
// Alternative helper for session-based belief retrieval
func (bee *BeliefEvaluationEngine) GetUserBeliefsFromSession(
	tenantID, sessionID string,
	cacheManager interface {
		GetSession(tenantID, sessionID string) (*models.SessionData, bool)
		GetFingerprintState(tenantID, fingerprintID string) (*models.FingerprintState, bool)
	},
) map[string][]string {
	if sessionData, exists := cacheManager.GetSession(tenantID, sessionID); exists {
		// Get fingerprint state from session's fingerprint ID
		return bee.GetUserBeliefsFromFingerprint(tenantID, sessionData.FingerprintID, cacheManager)
	}

	// Return empty beliefs if session not found
	return make(map[string][]string)
}

// InjectFilterButton adds the filter button to the HTML content
func (bee *BeliefEvaluationEngine) InjectFilterButton(htmlContent, filterButtonHTML string) string {
	// Find the opening pane div and inject the button after it
	panePattern := `(<div[^>]*id="pane-[^"]*"[^>]*>)`
	re := regexp.MustCompile(panePattern)

	return re.ReplaceAllString(htmlContent, "$1"+filterButtonHTML)
}

// RenderFilterButton generates the Filter back button HTML
func (bee *BeliefEvaluationEngine) RenderFilterButton(paneID string, effectiveFilter map[string]interface{}, gotoPaneID string, paneBeliefs models.PaneBeliefData) string {
	// Extract beliefs to unset using v1 logic
	var beliefsToUnset []string

	// Get all keys except MATCH-ACROSS and LINKED-BELIEFS from effective filter (user's actual beliefs)
	for key := range effectiveFilter {
		if key != "MATCH-ACROSS" && key != "LINKED-BELIEFS" {
			beliefsToUnset = append(beliefsToUnset, key)
		}
	}

	// Add LINKED-BELIEFS if present in effectiveFilter (matching V1 logic)
	if linkedBeliefsValue, exists := effectiveFilter["LINKED-BELIEFS"]; exists {
		// log.Printf("🔧 RenderFilterButton: linkedBeliefsValue type=%T, value=%v", linkedBeliefsValue, linkedBeliefsValue)
		if linkedArray, ok := linkedBeliefsValue.([]string); ok {
			for _, linkedStr := range linkedArray {
				// Add to beliefsToUnset if not already present
				found := false
				for _, existing := range beliefsToUnset {
					if existing == linkedStr {
						found = true
						break
					}
				}
				if !found {
					beliefsToUnset = append(beliefsToUnset, linkedStr)
				}
			}
		}
	}

	if len(beliefsToUnset) == 0 {
		return ""
	}

	// Create comma-separated list
	unsetBeliefIds := strings.Join(beliefsToUnset, ",")

	// Build hx-vals with gotoPaneID if available
	var hxVals string
	if gotoPaneID != "" {
		hxVals = fmt.Sprintf(`{"unsetBeliefIds": %q, "paneId": %q, "gotoPaneID": %q}`, unsetBeliefIds, paneID, gotoPaneID)
	} else {
		hxVals = fmt.Sprintf(`{"unsetBeliefIds": %q, "paneId": %q}`, unsetBeliefIds, paneID)
	}

	return fmt.Sprintf(`
		<button
			type="button"
			class="z-10 absolute top-2 right-2 p-1.5 bg-white rounded-full hover:bg-black text-mydarkgrey hover:text-white"
			title="Go Back"
			hx-post="/api/v1/state"
			hx-trigger="click"
			hx-swap="none"
			hx-vals='%s'
			hx-preserve="true"
		>
			<svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" d="M9 15L3 9m0 0l6-6M3 9h12a6 6 0 010 12h-3" />
			</svg>
		</button>
	`, hxVals)
}

// CalculateEffectiveFilter returns the intersection of user beliefs and pane requirements for unset logic
func (bee *BeliefEvaluationEngine) CalculateEffectiveFilter(
	paneBeliefs models.PaneBeliefData,
	userBeliefs map[string][]string,
) map[string]interface{} {
	effectiveFilter := make(map[string]interface{})

	// Add user beliefs that match pane's held beliefs requirements
	for beliefSlug, requiredValues := range paneBeliefs.HeldBeliefs {
		if bee.hasMatchingBelief(userBeliefs, beliefSlug, requiredValues) {
			if userValues, exists := userBeliefs[beliefSlug]; exists {
				effectiveFilter[beliefSlug] = userValues
			}
		}
	}

	// Add user beliefs that match pane's match-across requirements (OR logic)
	for _, beliefSlug := range paneBeliefs.MatchAcross {
		if userValues, exists := userBeliefs[beliefSlug]; exists {
			effectiveFilter[beliefSlug] = userValues
		}
	}

	// Add LINKED-BELIEFS if present in pane configuration
	if len(paneBeliefs.LinkedBeliefs) > 0 {
		effectiveFilter["LINKED-BELIEFS"] = paneBeliefs.LinkedBeliefs
	}

	// Add MATCH-ACROSS if there are match-across beliefs (for metadata, not unset logic)
	if len(paneBeliefs.MatchAcross) > 0 {
		effectiveFilter["MATCH-ACROSS"] = paneBeliefs.MatchAcross
	}

	return effectiveFilter
}
