// Package templates provides shared widget types and utilities
package templates

import (
	"encoding/json"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// BeliefState represents a parsed belief JSON object
type BeliefState struct {
	ID     string `json:"id"`
	Verb   string `json:"verb"`
	Slug   string `json:"slug"`
	Object string `json:"object,omitempty"`
}

// ScaleOption represents an option in a belief scale
type ScaleOption struct {
	ID    int    `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Shared utility functions for all widgets

// getUserBeliefs now uses pre-resolved widget context instead of direct cache calls
func getUserBeliefs(ctx *rendering.RenderContext) map[string][]string {
	if ctx.WidgetContext == nil {
		return nil
	}

	return ctx.WidgetContext.UserBeliefs
}

func getCurrentBeliefState(userBeliefs map[string][]string, beliefSlug string) *BeliefState {
	if userBeliefs == nil {
		return nil
	}

	beliefStrings, exists := userBeliefs[beliefSlug]
	if !exists || len(beliefStrings) == 0 {
		return nil
	}

	rawValue := beliefStrings[0]

	// Try to parse as JSON first (for legacy/structured beliefs)
	var belief BeliefState
	if err := json.Unmarshal([]byte(rawValue), &belief); err == nil {
		return &belief
	}

	// If JSON parsing fails, treat as raw verb string
	result := &BeliefState{
		ID:     beliefSlug,
		Verb:   rawValue,
		Slug:   beliefSlug,
		Object: "",
	}
	return result
}
