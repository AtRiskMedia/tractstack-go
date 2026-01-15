// Package content defines the application's core content-related domain entities.
package content

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
)

// TractStackNode represents the core structure of a TractStack content node, containing metadata like ID, title, and type.
type TractStackNode struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	NodeType        string  `json:"nodeType"`
	Slug            string  `json:"slug"`
	SocialImagePath *string `json:"socialImagePath,omitempty"`
}

// StoryFragmentNode represents a story fragment content node, linking menus and panes.
type StoryFragmentNode struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	NodeType         string            `json:"nodeType"`
	Slug             string            `json:"slug"`
	TractStackID     string            `json:"tractStackId"`
	MenuID           *string           `json:"menuId,omitempty"`
	Menu             *MenuNode         `json:"menu,omitempty"`
	PaneIDs          []string          `json:"paneIds"`
	TailwindBgColour *string           `json:"tailwindBgColour,omitempty"`
	SocialImagePath  *string           `json:"socialImagePath,omitempty"`
	CodeHookTargets  map[string]string `json:"codeHookTargets,omitempty"`
	IsHome           bool              `json:"isHome"`
	Created          time.Time         `json:"created"`
	Changed          *time.Time        `json:"changed,omitempty"`
}

// StoryFragmentCompletePayload represents the full payload for a story fragment, including associated topics and descriptions.
type StoryFragmentCompletePayload struct {
	StoryFragmentNode
	Topics      []string `json:"topics,omitempty"`
	Description *string  `json:"description,omitempty"`
}

// PaneNode represents a reusable content pane, containing HTML AST, styles, and belief logic.
type PaneNode struct {
	ID              string              `json:"id"`
	Title           string              `json:"title"`
	NodeType        string              `json:"nodeType"`
	Slug            string              `json:"slug"`
	IsContextPane   bool                `json:"isContextPane"`
	IsDecorative    bool                `json:"isDecorative"`
	OptionsPayload  map[string]any      `json:"optionsPayload,omitempty"`
	BgColour        *string             `json:"bgColour,omitempty"`
	CodeHookTarget  *string             `json:"codeHookTarget,omitempty"`
	CodeHookPayload map[string]string   `json:"codeHookPayload,omitempty"`
	HTMLAST         *rendering.HTMLAST  `json:"htmlAst,omitempty"`
	HeldBeliefs     map[string][]string `json:"heldBeliefs,omitempty"`
	WithheldBeliefs map[string][]string `json:"withheldBeliefs,omitempty"`
	MarkdownBody    *string             `json:"markdownBody,omitempty"`
	MarkdownID      *string             `json:"markdownId,omitempty"`
	Created         time.Time           `json:"created"`
	Changed         *time.Time          `json:"changed,omitempty"`
}

// MenuNode represents a navigation menu structure.
type MenuNode struct {
	ID             string      `json:"id"`
	Title          string      `json:"title"`
	NodeType       string      `json:"nodeType"`
	Theme          string      `json:"theme"`
	OptionsPayload []*MenuLink `json:"optionsPayload,omitempty"`
}

// MenuLink represents a single link item within a menu.
type MenuLink struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Featured    bool   `json:"featured"`
	ActionLisp  string `json:"actionLisp"`
}

// ResourceNode represents an external or internal resource reference.
type ResourceNode struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	NodeType       string         `json:"nodeType"`
	Slug           string         `json:"slug"`
	CategorySlug   *string        `json:"categorySlug,omitempty"`
	OneLiner       string         `json:"oneliner"`
	ActionLisp     string         `json:"actionLisp"`
	OptionsPayload map[string]any `json:"optionsPayload"`
}

// BeliefNode represents a belief entity used for personalization logic.
type BeliefNode struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	NodeType     string   `json:"nodeType"`
	Slug         string   `json:"slug"`
	Scale        string   `json:"scale"`
	CustomValues []string `json:"customValues,omitempty"`
}

// EpinetNode represents an Epinet structure for interactive storytelling steps.
type EpinetNode struct {
	ID       string        `json:"id"`
	NodeType string        `json:"nodeType"`
	Title    string        `json:"title"`
	Promoted bool          `json:"promoted"`
	Steps    []*EpinetStep `json:"steps"`
}

// EpinetStep represents a single decision or logic step within an Epinet.
type EpinetStep struct {
	GateType   string   `json:"gateType"`
	Title      string   `json:"title"`
	Values     []string `json:"values"`
	ObjectType *string  `json:"objectType,omitempty"`
	ObjectIDs  []string `json:"objectIds,omitempty"`
	BeliefSlug *string  `json:"beliefSlug,omitempty"`
}

// ImageFileNode represents an uploaded image file with metadata and source sets.
type ImageFileNode struct {
	ID             string  `json:"id"`
	Filename       string  `json:"filename"`
	NodeType       string  `json:"nodeType"`
	AltDescription string  `json:"altDescription"`
	Src            string  `json:"src"`
	SrcSet         *string `json:"srcSet,omitempty"`
	Base64Data     *string `json:"base64Data,omitempty"`
}

// ImpressionNode represents a tracked impression or interaction point.
type ImpressionNode struct {
	ID          string `json:"id"`
	NodeType    string `json:"nodeType"`
	TagName     string `json:"tagName"`
	ParentID    string `json:"parentId"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	ButtonText  string `json:"buttonText"`
	ActionsLisp string `json:"actionsLisp"`
}
