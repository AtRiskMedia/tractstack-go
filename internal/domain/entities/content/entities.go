// Package content defines the application's core content-related domain entities.
package content

import "time"

type TractStackNode struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	NodeType        string  `json:"nodeType"`
	Slug            string  `json:"slug"`
	SocialImagePath *string `json:"socialImagePath,omitempty"`
}

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
	HeldBeliefs     map[string][]string `json:"heldBeliefs,omitempty"`
	WithheldBeliefs map[string][]string `json:"withheldBeliefs,omitempty"`
	MarkdownBody    *string             `json:"markdownBody,omitempty"`
	MarkdownID      *string             `json:"markdownId,omitempty"`
	Created         time.Time           `json:"created"`
	Changed         *time.Time          `json:"changed,omitempty"`
}

type MenuNode struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	NodeType       string         `json:"nodeType"`
	Theme          string         `json:"theme"`
	OptionsPayload map[string]any `json:"optionsPayload,omitempty"`
	Links          []*MenuLink    `json:"links,omitempty"`
}

type MenuLink struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Featured    bool   `json:"featured"`
	ActionLisp  string `json:"actionLisp"`
}

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

type BeliefNode struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	NodeType     string   `json:"nodeType"`
	Slug         string   `json:"slug"`
	Scale        string   `json:"scale"`
	CustomValues []string `json:"customValues,omitempty"`
}

type EpinetNode struct {
	ID       string        `json:"id"`
	NodeType string        `json:"nodeType"`
	Title    string        `json:"title"`
	Promoted bool          `json:"promoted"`
	Steps    []*EpinetStep `json:"steps"`
}

type EpinetStep struct {
	GateType   string   `json:"gateType"`
	Title      string   `json:"title"`
	Values     []string `json:"values"`
	ObjectType *string  `json:"objectType,omitempty"`
	ObjectIDs  []string `json:"objectIds,omitempty"`
}

type ImageFileNode struct {
	ID             string  `json:"id"`
	Filename       string  `json:"filename"`
	NodeType       string  `json:"nodeType"`
	AltDescription string  `json:"altDescription"`
	URL            string  `json:"url"`
	SrcSet         *string `json:"srcSet,omitempty"`
}
