// Package models provides content map types for unified content API responses
package models

import (
	"fmt"
	"time"
)

// ContentMapItem represents the base structure for all content map items
type ContentMapItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
	Type  string `json:"type"`
}

// MenuContentMap represents menu content in the content map
type MenuContentMap struct {
	ContentMapItem
	Theme string `json:"theme"`
}

// ResourceContentMap represents resource content in the content map
type ResourceContentMap struct {
	ContentMapItem
	CategorySlug *string `json:"categorySlug"`
}

// PaneContentMap represents pane content in the content map
type PaneContentMap struct {
	ContentMapItem
	IsContext bool `json:"isContext"`
}

// StoryFragmentContentMap represents storyfragment content in the content map
type StoryFragmentContentMap struct {
	ContentMapItem
	ParentID        *string  `json:"parentId,omitempty"`
	ParentTitle     *string  `json:"parentTitle,omitempty"`
	ParentSlug      *string  `json:"parentSlug,omitempty"`
	Panes           []string `json:"panes,omitempty"`
	SocialImagePath *string  `json:"socialImagePath,omitempty"`
	ThumbSrc        *string  `json:"thumbSrc,omitempty"`
	ThumbSrcSet     *string  `json:"thumbSrcSet,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Topics          []string `json:"topics,omitempty"`
	Changed         *string  `json:"changed,omitempty"`
}

// TractStackContentMap represents tractstack content in the content map
type TractStackContentMap struct {
	ContentMapItem
	SocialImagePath *string `json:"socialImagePath,omitempty"`
}

// BeliefContentMap represents belief content in the content map
type BeliefContentMap struct {
	ContentMapItem
	Scale string `json:"scale"`
}

// Topic represents a topic for topic content map
type Topic struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// TopicContentMap represents topic aggregation in the content map
type TopicContentMap struct {
	ContentMapItem
	Topics []Topic `json:"topics"`
}

// FullContentMapResponse represents the complete content map API response
type FullContentMapResponse []interface{}

// ContentMapBuilder provides methods to build content map items
type ContentMapBuilder struct{}

// NewContentMapBuilder creates a new content map builder
func NewContentMapBuilder() *ContentMapBuilder {
	return &ContentMapBuilder{}
}

// BuildMenuContentMap creates a menu content map item
func (cmb *ContentMapBuilder) BuildMenuContentMap(menu *MenuNode) MenuContentMap {
	return MenuContentMap{
		ContentMapItem: ContentMapItem{
			ID:    menu.ID,
			Title: menu.Title,
			Slug:  menu.ID, // Menus use ID as slug in V1
			Type:  "Menu",
		},
		Theme: menu.Theme,
	}
}

// BuildResourceContentMap creates a resource content map item
func (cmb *ContentMapBuilder) BuildResourceContentMap(resource *ResourceNode) ResourceContentMap {
	return ResourceContentMap{
		ContentMapItem: ContentMapItem{
			ID:    resource.ID,
			Title: resource.Title,
			Slug:  resource.Slug,
			Type:  "Resource",
		},
		CategorySlug: resource.CategorySlug,
	}
}

// BuildPaneContentMap creates a pane content map item
func (cmb *ContentMapBuilder) BuildPaneContentMap(pane *PaneNode) PaneContentMap {
	return PaneContentMap{
		ContentMapItem: ContentMapItem{
			ID:    pane.ID,
			Title: pane.Title,
			Slug:  pane.Slug,
			Type:  "Pane",
		},
		IsContext: pane.IsContextPane,
	}
}

// BuildStoryFragmentContentMap creates a storyfragment content map item
func (cmb *ContentMapBuilder) BuildStoryFragmentContentMap(
	sf *StoryFragmentNode,
	tractstack *TractStackNode,
	paneIds []string,
	description *string,
	topics []string,
) StoryFragmentContentMap {
	item := StoryFragmentContentMap{
		ContentMapItem: ContentMapItem{
			ID:    sf.ID,
			Title: sf.Title,
			Slug:  sf.Slug,
			Type:  "StoryFragment",
		},
		Panes: paneIds,
	}

	// Add parent tractstack info
	if tractstack != nil {
		item.ParentID = &tractstack.ID
		item.ParentTitle = &tractstack.Title
		item.ParentSlug = &tractstack.Slug
	}

	// Add social image path
	if sf.SocialImagePath != nil {
		item.SocialImagePath = sf.SocialImagePath
	}

	// Add description
	if description != nil {
		item.Description = description
	}

	// Add topics
	if len(topics) > 0 {
		item.Topics = topics
	}

	// Add changed timestamp
	if sf.Changed != nil {
		changedStr := sf.Changed.Format(time.RFC3339)
		item.Changed = &changedStr
	}

	// Generate thumbnail paths (matching V1 logic)
	if sf.SocialImagePath != nil && *sf.SocialImagePath != "" {
		cacheBuster := time.Now().Unix()
		if sf.Changed != nil {
			cacheBuster = sf.Changed.Unix()
		}

		// Extract basename from social image path
		socialPath := *sf.SocialImagePath
		// Simple basename extraction (could be enhanced)
		basename := sf.ID // fallback to ID
		if lastSlash := len(socialPath) - 1; lastSlash >= 0 {
			for i := lastSlash; i >= 0; i-- {
				if socialPath[i] == '/' {
					basename = socialPath[i+1:]
					break
				}
			}
			// Remove extension
			if lastDot := len(basename) - 1; lastDot >= 0 {
				for i := lastDot; i >= 0; i-- {
					if basename[i] == '.' {
						basename = basename[:i]
						break
					}
				}
			}
		}

		thumbSrc := fmt.Sprintf("/images/thumbs/%s_1200px.webp?v=%d", basename, cacheBuster)
		thumbSrcSet := fmt.Sprintf(
			"/images/thumbs/%s_1200px.webp?v=%d 1200w, /images/thumbs/%s_600px.webp?v=%d 600w, /images/thumbs/%s_300px.webp?v=%d 300w",
			basename, cacheBuster, basename, cacheBuster, basename, cacheBuster,
		)

		item.ThumbSrc = &thumbSrc
		item.ThumbSrcSet = &thumbSrcSet
	}

	return item
}

// BuildTractStackContentMap creates a tractstack content map item
func (cmb *ContentMapBuilder) BuildTractStackContentMap(tractstack *TractStackNode) TractStackContentMap {
	return TractStackContentMap{
		ContentMapItem: ContentMapItem{
			ID:    tractstack.ID,
			Title: tractstack.Title,
			Slug:  tractstack.Slug,
			Type:  "TractStack",
		},
		SocialImagePath: tractstack.SocialImagePath,
	}
}

// BuildBeliefContentMap creates a belief content map item
func (cmb *ContentMapBuilder) BuildBeliefContentMap(belief *BeliefNode) BeliefContentMap {
	return BeliefContentMap{
		ContentMapItem: ContentMapItem{
			ID:    belief.ID,
			Title: belief.Title,
			Slug:  belief.Slug,
			Type:  "Belief",
		},
		Scale: belief.Scale,
	}
}

// BuildTopicContentMap creates a topic content map item
func (cmb *ContentMapBuilder) BuildTopicContentMap(topics []Topic) TopicContentMap {
	return TopicContentMap{
		ContentMapItem: ContentMapItem{
			ID:    "all-topics",
			Title: "All Topics",
			Slug:  "all-topics",
			Type:  "Topic",
		},
		Topics: topics,
	}
}

// Add these structs to models/content_map.go (alongside existing content map types)

// EpinetContentMap represents an epinet in the content map
type EpinetContentMap struct {
	ContentMapItem
	Promoted bool                   `json:"promoted"`
	Steps    []EpinetContentMapStep `json:"steps"`
}

// EpinetContentMapStep represents an epinet step in the content map
type EpinetContentMapStep struct {
	GateType   string   `json:"gateType"`
	Title      string   `json:"title"`
	Values     []string `json:"values"`
	ObjectType *string  `json:"objectType,omitempty"`
	ObjectIds  []string `json:"objectIds,omitempty"`
}

// Add this method to ContentMapBuilder (if it exists)

// BuildEpinetContentMap creates an epinet content map item
func (cmb *ContentMapBuilder) BuildEpinetContentMap(epinet *EpinetNode) EpinetContentMap {
	// Convert EpinetNodeStep to EpinetContentMapStep
	var steps []EpinetContentMapStep
	for _, step := range epinet.Steps {
		steps = append(steps, EpinetContentMapStep{
			GateType:   step.GateType,
			Title:      step.Title,
			Values:     step.Values,
			ObjectType: step.ObjectType,
			ObjectIds:  step.ObjectIDs,
		})
	}

	return EpinetContentMap{
		ContentMapItem: ContentMapItem{
			ID:    epinet.ID,
			Title: epinet.Title,
			Slug:  epinet.ID, // Epinets use ID as slug
			Type:  "Epinet",
		},
		Promoted: epinet.Promoted,
		Steps:    steps,
	}
}
