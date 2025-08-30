// Package rendering provides domain entities for HTML rendering operations
package rendering

// CodeHook represents parsed widget parameters from code nodes
type CodeHook struct {
	Hook   string  `json:"hook"`
	Value1 *string `json:"value1,omitempty"`
	Value2 *string `json:"value2,omitempty"`
	Value3 string  `json:"value3,omitempty"`
}

// BackgroundImageData represents background image rendering data
type BackgroundImageData struct {
	URL       string `json:"url"`
	AltText   string `json:"altText"`
	ClassName string `json:"className"`
	Type      string `json:"type"`
	Position  string `json:"position"`
	Size      string `json:"size"`
}

// NodeRenderData represents the complete data needed to render a node
type NodeRenderData struct {
	ID              string               `json:"id"`
	NodeType        string               `json:"nodeType"`
	TagName         *string              `json:"tagName,omitempty"`
	Copy            *string              `json:"copy,omitempty"`
	ElementCSS      *string              `json:"elementCss,omitempty"`
	ParentCSS       []string             `json:"parentCss,omitempty"`
	ParentID        string               `json:"parentId,omitempty"`
	Children        []string             `json:"children,omitempty"`
	PaneData        *PaneRenderData      `json:"paneData,omitempty"`
	BgImageData     *BackgroundImageData `json:"bgImageData,omitempty"`
	VisualBreakData *VisualBreakNode     `json:"visualBreakData,omitempty"`

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

type BackgroundNode struct {
	ID       string
	Position string
	Size     string
}
