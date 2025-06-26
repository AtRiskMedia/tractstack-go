// Package models provides HTML rendering data structures for nodes-compositor
package models

// PaneVariant represents different cache variants for belief-based rendering
type PaneVariant string

const (
	PaneVariantDefault PaneVariant = "default"
	PaneVariantHidden  PaneVariant = "hidden"
)

// RenderContext provides the context for HTML rendering operations
type RenderContext struct {
	AllNodes    map[string]*NodeRenderData `json:"allNodes,omitempty"`
	ParentNodes map[string][]string        `json:"parentNodes,omitempty"`
	TenantID    string                     `json:"tenantId,omitempty"`
	UserID      string                     `json:"userId,omitempty"`
}

// CodeHook represents parsed widget parameters from code nodes
type CodeHook struct {
	Hook   string  `json:"hook"`
	Value1 *string `json:"value1,omitempty"`
	Value2 *string `json:"value2,omitempty"`
	Value3 string  `json:"value3,omitempty"`
}

// NodeRenderData represents the complete data needed to render a node
type NodeRenderData struct {
	ID          string               `json:"id"`
	NodeType    string               `json:"nodeType"`
	TagName     *string              `json:"tagName,omitempty"`
	Copy        *string              `json:"copy,omitempty"`
	ElementCSS  *string              `json:"elementCss,omitempty"`
	ParentCSS   []string             `json:"parentCss,omitempty"`
	ParentID    string               `json:"parentId,omitempty"`
	Children    []string             `json:"children,omitempty"`
	PaneData    *PaneRenderData      `json:"paneData,omitempty"`
	BgImageData *BackgroundImageData `json:"bgImageData,omitempty"`

	// Fields for NodeImg template
	ImageURL *string `json:"imageUrl,omitempty"`
	SrcSet   *string `json:"srcSet,omitempty"`
	AltText  *string `json:"altText,omitempty"`

	// Fields for NodeA template
	Href   *string `json:"href,omitempty"`
	Target *string `json:"target,omitempty"`

	// Fields for Markdown nodes
	MarkdownBody *string `json:"markdownBody,omitempty"`

	// Additional extensible data
	CustomData map[string]any `json:"customData,omitempty"`
}

// PaneRenderData represents pane-specific rendering data
type PaneRenderData struct {
	Title           string                 `json:"title"`
	Slug            string                 `json:"slug"`
	IsDecorative    bool                   `json:"isDecorative"`
	BgColour        *string                `json:"bgColour,omitempty"`
	HeldBeliefs     map[string]BeliefValue `json:"heldBeliefs,omitempty"`
	WithheldBeliefs map[string]BeliefValue `json:"withheldBeliefs,omitempty"`
	CodeHookTarget  *string                `json:"codeHookTarget,omitempty"`
	CodeHookPayload map[string]string      `json:"codeHookPayload,omitempty"`
}

// BackgroundNode represents positioned background image data for Markdown layouts
type BackgroundNode struct {
	ID       string `json:"id"`
	Position string `json:"position"` // "left" or "right"
	Size     string `json:"size"`     // "narrow", "wide", or "equal"
}

// BackgroundImageData represents background image metadata for layout decisions
type BackgroundImageData struct {
	Type     string `json:"type"`     // "background-image" or "artpack-image"
	Position string `json:"position"` // "background", "left", "right", "leftBleed", "rightBleed"
	Size     string `json:"size"`     // "narrow", "wide", "equal"
}

// UserState represents user belief state for cache variant determination
type UserState struct {
	UserID   string                 `json:"userId"`
	TenantID string                 `json:"tenantId"`
	Beliefs  map[string]any `json:"beliefs"`
	Badges   []string               `json:"badges"`
}

// HasBadges checks if user has all required badges
func (us *UserState) HasBadges(requiredBadges []string) bool {
	if len(requiredBadges) == 0 {
		return true
	}

	userBadgeSet := make(map[string]bool)
	for _, badge := range us.Badges {
		userBadgeSet[badge] = true
	}

	for _, required := range requiredBadges {
		if !userBadgeSet[required] {
			return false
		}
	}

	return true
}

// MeetsBeliefConditions checks if user meets belief-based visibility conditions
func (us *UserState) MeetsBeliefConditions(conditions map[string]BeliefValue) bool {
	if len(conditions) == 0 {
		return true
	}

	for beliefKey, condition := range conditions {
		userBelief, exists := us.Beliefs[beliefKey]
		if !exists {
			return false
		}

		// Simple string comparison for now - expand based on belief types
		userBeliefStr, ok := userBelief.(string)
		if !ok {
			return false
		}

		switch condition.Verb {
		case "is":
			if condition.Object == nil || userBeliefStr != *condition.Object {
				return false
			}
		case "isNot":
			if condition.Object != nil && userBeliefStr == *condition.Object {
				return false
			}
		default:
			// Unknown verb - fail safe
			return false
		}
	}

	return true
}
