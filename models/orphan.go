package models

import "time"

// OrphanAnalysisPayload contains dependency mapping for tracked content types only
type OrphanAnalysisPayload struct {
	StoryFragments map[string][]string `json:"storyFragments"` // id -> [dependent_ids]
	Panes          map[string][]string `json:"panes"`          // id -> [storyfragment_ids_using_this_pane]
	Menus          map[string][]string `json:"menus"`          // id -> [storyfragment_ids_using_this_menu]
	Files          map[string][]string `json:"files"`          // id -> [pane_ids_using_this_file]
	Beliefs        map[string][]string `json:"beliefs"`
	Status         string              `json:"status"` // "loading" | "complete"
}

// OrphanAnalysisCache stores cached orphan analysis with ETag
type OrphanAnalysisCache struct {
	Data        *OrphanAnalysisPayload `json:"data"`
	ETag        string                 `json:"etag"`
	LastUpdated time.Time              `json:"lastUpdated"`
}
