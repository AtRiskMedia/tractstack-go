// Package templates provides shared widget types and utilities
package templates

import (
	"encoding/json"
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
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

func getUserBeliefs(ctx *models.RenderContext) map[string][]string {
	if ctx.SessionID == "" || ctx.StoryfragmentID == "" {
		return nil
	}

	sessionContext, exists := cache.GetGlobalManager().GetSessionBeliefContext(
		ctx.TenantID,
		ctx.SessionID,
		ctx.StoryfragmentID,
	)
	if !exists {
		return nil
	}

	return sessionContext.UserBeliefs
}

func getCurrentBeliefState(userBeliefs map[string][]string, beliefSlug string) *BeliefState {
	if userBeliefs == nil {
		return nil
	}

	beliefStrings, exists := userBeliefs[beliefSlug]
	if !exists || len(beliefStrings) == 0 {
		return nil
	}

	// Parse the first belief JSON string
	var belief BeliefState
	if err := json.Unmarshal([]byte(beliefStrings[0]), &belief); err != nil {
		return nil
	}

	return &belief
}

func getSessionStrategy(ctx *models.RenderContext) string {
	if ctx.SessionID != "" {
		return fmt.Sprintf(`hx-headers='{"X-TractStack-Session-ID": "%s"}'`, ctx.SessionID)
	}
	return `hx-headers='{"X-TractStack-Session-ID": localStorage.getItem("tractstack_session_id")}'`
}
