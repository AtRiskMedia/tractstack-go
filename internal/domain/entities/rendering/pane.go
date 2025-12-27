// Package rendering provides domain entities for HTML rendering operations
package rendering

import "time"

type HTMLASTNode struct {
	Tag      string            `json:"tag"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Children []HTMLASTNode     `json:"children,omitempty"`
	Text     string            `json:"text,omitempty"`
	ID       string            `json:"id,omitempty"`
}

type ViewportCSS struct {
	XS string `json:"xs"`
	MD string `json:"md"`
	XL string `json:"xl"`
}

type HTMLAST struct {
	CSS         string        `json:"css"`
	ViewportCSS ViewportCSS   `json:"viewportCss"`
	Tree        []HTMLASTNode `json:"tree"`
}

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
	HTMLAST         *HTMLAST       `json:"htmlAst,omitempty"`
	Created         time.Time      `json:"created"`
	Changed         *time.Time     `json:"changed,omitempty"`
}
