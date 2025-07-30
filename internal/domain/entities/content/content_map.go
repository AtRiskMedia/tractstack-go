// Package content defines the content map
package content

type ContentMapItem struct {
	ID    string         `json:"id"`
	Title string         `json:"title"`
	Slug  string         `json:"slug"`
	Type  string         `json:"type"`
	Extra map[string]any `json:"extra,omitempty"`
}
