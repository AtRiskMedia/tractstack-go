package models

import "time"

// OrphanAnalysisPayload contains dependency mapping for all content types
type OrphanAnalysisPayload struct {
	StoryFragments map[string][]string `json:"storyFragments"` // id -> [dependent_ids]
	Panes          map[string][]string `json:"panes"`          // id -> [storyfragment_ids_using_this_pane]
	Menus          map[string][]string `json:"menus"`          // id -> [storyfragment_ids_using_this_menu]
	Files          map[string][]string `json:"files"`          // id -> [pane_ids_using_this_file]
	Resources      map[string][]string `json:"resources"`      // id -> [referencing_content_ids]
	Beliefs        map[string][]string `json:"beliefs"`        // id -> [pane_ids_requiring_this_belief]
	Epinets        map[string][]string `json:"epinets"`        // id -> [referencing_content_ids]
	TractStacks    map[string][]string `json:"tractstacks"`    // id -> [storyfragment_ids]
	Status         string              `json:"status"`         // "loading" | "complete"
}

// OrphanAnalysisCache stores cached orphan analysis with ETag
type OrphanAnalysisCache struct {
	Data        *OrphanAnalysisPayload `json:"data"`
	ETag        string                 `json:"etag"`
	LastUpdated time.Time              `json:"lastUpdated"`
}
