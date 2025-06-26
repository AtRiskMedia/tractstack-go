// Package models defines HTML rendering structures for nodes-compositor
package models

import (
	"time"
)

// RenderContext holds the rendering state and tenant information
type RenderContext struct {
	TenantID    string
	PaneID      string
	UserState   *UserState
	AllNodes    map[string]interface{} // Node storage - will be typed properly later
	ParentNodes map[string][]string    // Parent->Children mapping
}

// PaneVariant represents different rendering variants for belief-based caching
type PaneVariant string

const (
	VariantDefault PaneVariant = "default"
	VariantHidden  PaneVariant = "hidden"
)

// CodeHook represents parsed widget hook data from Node.astro parseCodeHook function
type CodeHook struct {
	Hook   string  // identifyAs, youtube, bunny, toggle, resource, belief, signup
	Value1 *string // First parameter
	Value2 *string // Second parameter
	Value3 string  // Third parameter (always string, may be empty)
}

// NodeRenderData holds the minimal data needed to render any node
type NodeRenderData struct {
	ID           string
	NodeType     string
	TagName      *string           // For FlatNodes with tagName
	Copy         *string           // Text content
	ElementCSS   *string           // CSS classes
	Children     []string          // Child node IDs
	CodeHookData *CodeHook         // Parsed widget data for code nodes
	ButtonData   *ButtonRenderData // Button-specific data
	PaneData     *PaneRenderData   // Pane-specific data
	Href         *string           // For links
	Src          *string           // For images
	Alt          *string           // For images
}

// ButtonRenderData holds button-specific rendering data
type ButtonRenderData struct {
	CallbackPayload interface{} // Lisp payload
	BunnyPayload    interface{} // Video payload
	IsVideo         bool
}

// PaneRenderData holds pane-specific rendering data
type PaneRenderData struct {
	Title           string
	Slug            string
	IsContextPane   bool
	IsDecorative    bool
	BgColour        *string
	CodeHookTarget  *string
	CodeHookPayload map[string]string
	HeldBeliefs     map[string]interface{}
	WithheldBeliefs map[string]interface{}
	OptionsPayload  map[string]interface{} // CSS and styling data
}

// UserState represents user's current belief and badge state for rendering decisions
type UserState struct {
	FingerprintID string
	Beliefs       map[string]interface{}
	Badges        []string
	ConsentGiven  bool
	IsAnonymous   bool
	LastActivity  time.Time
}

// HasBadges checks if user has all required badges
func (us *UserState) HasBadges(required []string) bool {
	if len(required) == 0 {
		return true
	}

	userBadges := make(map[string]bool)
	for _, badge := range us.Badges {
		userBadges[badge] = true
	}

	for _, req := range required {
		if !userBadges[req] {
			return false
		}
	}
	return true
}

// MeetsBeliefConditions checks if user meets belief-based visibility rules
func (us *UserState) MeetsBeliefConditions(conditions map[string]interface{}) bool {
	if len(conditions) == 0 {
		return true
	}

	// Implementation will depend on specific belief condition format
	// For now, return true to allow rendering
	return true
}
