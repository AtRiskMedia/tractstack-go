// Package content defines the content map
package content

type ContentMapItem struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Slug            string   `json:"slug"`
	Type            string   `json:"type"`
	Theme           *string  `json:"theme,omitempty"`
	CategorySlug    *string  `json:"categorySlug,omitempty"`
	IsContext       *bool    `json:"isContext,omitempty"`
	ParentID        *string  `json:"parentId,omitempty"`
	ParentTitle     *string  `json:"parentTitle,omitempty"`
	ParentSlug      *string  `json:"parentSlug,omitempty"`
	Panes           []string `json:"panes,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Topics          []string `json:"topics,omitempty"`
	Changed         *string  `json:"changed,omitempty"`
	SocialImagePath *string  `json:"socialImagePath,omitempty"`
	ThumbSrc        *string  `json:"thumbSrc,omitempty"`
	ThumbSrcSet     *string  `json:"thumbSrcSet,omitempty"`
	Scale           *string  `json:"scale,omitempty"`
	Promoted        *bool    `json:"promoted,omitempty"`
}
