// Package services provides pure domain services for business logic
package services

import (
	"slices"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// BeliefEvaluationService handles pane visibility evaluation based on user beliefs.
// This is a pure domain service with no infrastructure dependencies.
type BeliefEvaluationService struct{}

// NewBeliefEvaluationService creates a new belief evaluation engine.
func NewBeliefEvaluationService() *BeliefEvaluationService {
	return &BeliefEvaluationService{}
}

// EvaluatePaneVisibility is the main entry point for belief evaluation.
// Returns "visible" or "hidden" based on belief requirements.
func (s *BeliefEvaluationService) EvaluatePaneVisibility(
	paneBeliefs types.PaneBeliefData,
	userBeliefs map[string][]string,
) string {
	heldResult := s.processHeldBeliefs(paneBeliefs, userBeliefs)
	withheldResult := s.processWithheldBeliefs(paneBeliefs, userBeliefs)

	if !heldResult || !withheldResult {
		return "hidden"
	}

	return "visible"
}

// processHeldBeliefs handles requirements for showing content.
func (s *BeliefEvaluationService) processHeldBeliefs(
	paneBeliefs types.PaneBeliefData,
	userBeliefs map[string][]string,
) bool {
	if len(paneBeliefs.HeldBeliefs) == 0 {
		return true
	}

	matchAcrossKeys := paneBeliefs.MatchAcross
	matchAcrossFilter := make(map[string][]string)
	regularFilter := make(map[string][]string)

	for key, values := range paneBeliefs.HeldBeliefs {
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

	matchAcrossResult := true
	if len(matchAcrossFilter) > 0 {
		matchAcrossResult = false
		for key, valueOrValues := range matchAcrossFilter {
			if s.hasMatchingBelief(userBeliefs, key, valueOrValues) {
				matchAcrossResult = true
				break
			}
		}
	}

	regularResult := true
	if len(regularFilter) > 0 {
		for key, valueOrValues := range regularFilter {
			if !s.hasMatchingBelief(userBeliefs, key, valueOrValues) {
				regularResult = false
				break
			}
		}
	}

	return matchAcrossResult && regularResult
}

// processWithheldBeliefs handles requirements for hiding content.
func (s *BeliefEvaluationService) processWithheldBeliefs(
	paneBeliefs types.PaneBeliefData,
	userBeliefs map[string][]string,
) bool {
	if len(paneBeliefs.WithheldBeliefs) == 0 {
		return true
	}

	for key, prohibitedValues := range paneBeliefs.WithheldBeliefs {
		if s.hasMatchingBelief(userBeliefs, key, prohibitedValues) {
			return false
		}
	}

	return true
}

// hasMatchingBelief checks if user has any of the required belief values for a given key.
func (s *BeliefEvaluationService) hasMatchingBelief(
	userBeliefs map[string][]string,
	key string,
	requiredValues []string,
) bool {
	userValues, exists := userBeliefs[key]
	if !exists {
		return false
	}

	for _, userValue := range userValues {
		for _, requiredValue := range requiredValues {
			if userValue == requiredValue || requiredValue == "*" {
				return true
			}
		}
	}

	return false
}

// CalculateEffectiveFilter returns the intersection of user beliefs and pane requirements for unset logic
func (s *BeliefEvaluationService) CalculateEffectiveFilter(
	paneBeliefs types.PaneBeliefData,
	userBeliefs map[string][]string,
) map[string]any {
	effectiveFilter := make(map[string]any)

	for beliefSlug, requiredValues := range paneBeliefs.HeldBeliefs {
		if s.hasMatchingBelief(userBeliefs, beliefSlug, requiredValues) {
			if userValues, exists := userBeliefs[beliefSlug]; exists {
				effectiveFilter[beliefSlug] = userValues
			}
		}
	}

	for _, beliefSlug := range paneBeliefs.MatchAcross {
		if userValues, exists := userBeliefs[beliefSlug]; exists {
			effectiveFilter[beliefSlug] = userValues
		}
	}

	if len(paneBeliefs.LinkedBeliefs) > 0 {
		effectiveFilter["LINKED-BELIEFS"] = paneBeliefs.LinkedBeliefs
	}

	if len(paneBeliefs.MatchAcross) > 0 {
		effectiveFilter["MATCH-ACROSS"] = paneBeliefs.MatchAcross
	}

	return effectiveFilter
}

// ExtractBeliefsToUnset extracts beliefs to unset from effectiveFilter (legacy logic)
func (s *BeliefEvaluationService) ExtractBeliefsToUnset(
	effectiveFilter map[string]any,
) []string {
	var beliefsToUnset []string

	// Get all keys except MATCH-ACROSS and LINKED-BELIEFS (user's actual beliefs)
	for key := range effectiveFilter {
		if key != "MATCH-ACROSS" && key != "LINKED-BELIEFS" {
			beliefsToUnset = append(beliefsToUnset, key)
		}
	}

	// Add LINKED-BELIEFS if present (cascade unset)
	if linkedBeliefsValue, exists := effectiveFilter["LINKED-BELIEFS"]; exists {
		if linkedArray, ok := linkedBeliefsValue.([]string); ok {
			for _, linkedStr := range linkedArray {
				// Add to beliefsToUnset if not already present
				if !slices.Contains(beliefsToUnset, linkedStr) {
					beliefsToUnset = append(beliefsToUnset, linkedStr)
				}
			}
		}
	}

	return beliefsToUnset
}
