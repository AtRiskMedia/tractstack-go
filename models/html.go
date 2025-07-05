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
	AllNodes        map[string]*NodeRenderData `json:"allNodes,omitempty"`
	ParentNodes     map[string][]string        `json:"parentNodes,omitempty"`
	TenantID        string                     `json:"tenantId,omitempty"`
	SessionID       string                     `json:"sessionId,omitempty"`       // ADD
	StoryfragmentID string                     `json:"storyfragmentId,omitempty"` // ADD
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
	Title           string              `json:"title"`
	Slug            string              `json:"slug"`
	IsDecorative    bool                `json:"isDecorative"`
	BgColour        *string             `json:"bgColour,omitempty"`
	HeldBeliefs     map[string][]string `json:"heldBeliefs,omitempty"`
	WithheldBeliefs map[string][]string `json:"withheldBeliefs,omitempty"`
	CodeHookTarget  *string             `json:"codeHookTarget,omitempty"`
	CodeHookPayload map[string]string   `json:"codeHookPayload,omitempty"`
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
