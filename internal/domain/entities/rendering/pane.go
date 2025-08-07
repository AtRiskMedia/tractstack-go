// Package rendering provides domain entities for HTML rendering operations
package rendering

import "time"

// PaneVariant represents different cache variants for belief-based rendering
type PaneVariant struct {
	BeliefMode      string   `json:"beliefMode"`      // "default", "hidden", "personalized"
	HeldBeliefs     []string `json:"heldBeliefs"`     // User's held beliefs
	WithheldBeliefs []string `json:"withheldBeliefs"` // User's withheld beliefs
}

// DefaultPaneVariant returns the default variant for non-personalized rendering
func DefaultPaneVariant() PaneVariant {
	return PaneVariant{
		BeliefMode:      "default",
		HeldBeliefs:     []string{},
		WithheldBeliefs: []string{},
	}
}

// HiddenPaneVariant returns the hidden variant for belief-filtered content
func HiddenPaneVariant() PaneVariant {
	return PaneVariant{
		BeliefMode:      "hidden",
		HeldBeliefs:     []string{},
		WithheldBeliefs: []string{},
	}
}

// PersonalizedPaneVariant creates a variant for specific user beliefs
func PersonalizedPaneVariant(heldBeliefs, withheldBeliefs []string) PaneVariant {
	return PaneVariant{
		BeliefMode:      "personalized",
		HeldBeliefs:     heldBeliefs,
		WithheldBeliefs: withheldBeliefs,
	}
}

// PaneRenderData represents pane-specific rendering data
type PaneRenderData struct {
	Title           string         `json:"title"`
	Slug            string         `json:"slug"`
	IsDecorative    bool           `json:"isDecorative"`
	BgColour        *string        `json:"bgColour,omitempty"`
	HeldBeliefs     map[string]any `json:"heldBeliefs,omitempty"`
	WithheldBeliefs map[string]any `json:"withheldBeliefs,omitempty"`
	CodeHookTarget  *string        `json:"codeHookTarget,omitempty"`
	CodeHookPayload map[string]any `json:"codeHookPayload,omitempty"`
	Created         time.Time      `json:"created"`
	Changed         *time.Time     `json:"changed,omitempty"`
}
