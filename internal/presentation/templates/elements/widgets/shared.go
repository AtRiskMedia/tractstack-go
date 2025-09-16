// Package templates provides shared widget types and utilities
package templates

import (
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

// getUserBeliefs correctly retrieves the user's belief state map from the context.
func getUserBeliefs(ctx *rendering.RenderContext) map[string][]string {
	// Defensive check to prevent nil pointer panics during initial page loads.
	if ctx == nil || ctx.WidgetContext == nil || ctx.WidgetContext.UserBeliefs == nil {
		return make(map[string][]string) // Return an empty map, not nil
	}

	return ctx.WidgetContext.UserBeliefs
}

// getCurrentBeliefState finds the belief state for a given slug from the user's belief map.
func getCurrentBeliefState(userBeliefs map[string][]string, beliefSlug string) *BeliefState {
	if userBeliefs == nil {
		return nil
	}

	beliefValues, exists := userBeliefs[beliefSlug]
	if !exists || len(beliefValues) == 0 {
		return nil
	}

	return &BeliefState{
		ID:   beliefSlug,
		Verb: beliefValues[0],
		Slug: beliefSlug,
	}
}
