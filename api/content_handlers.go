// Package api provides content map handlers
package api

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/gin-gonic/gin"
)

// FullContentMapItem matches V1's FullContentMap structure exactly
type FullContentMapItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
	Type  string `json:"type"`
	// Menu specific
	Theme *string `json:"theme,omitempty"`
	// Resource specific
	CategorySlug *string `json:"categorySlug,omitempty"`
	// Pane specific
	IsContext *bool `json:"isContext,omitempty"`
	// StoryFragment specific
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
	// Belief specific
	Scale *string `json:"scale,omitempty"`
}

// GetFullContentMapHandler returns the unified content map (equivalent to V1's getFullContentMap)
func GetFullContentMapHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cacheManager := cache.GetGlobalManager()
	var contentMap []FullContentMapItem

	// Get all TractStacks
	tractStackService := content.NewTractStackService(ctx, cacheManager)
	tractStackIDs, err := tractStackService.GetAllIDs()
	if err == nil {
		tractStacks, err := tractStackService.GetByIDs(tractStackIDs)
		if err == nil {
			for _, tractStack := range tractStacks {
				if tractStack != nil {
					item := FullContentMapItem{
						ID:              tractStack.ID,
						Title:           tractStack.Title,
						Slug:            tractStack.Slug,
						Type:            "TractStack",
						SocialImagePath: tractStack.SocialImagePath,
					}
					contentMap = append(contentMap, item)
				}
			}
		}
	}

	// Get all StoryFragments
	storyFragmentService := content.NewStoryFragmentService(ctx, cacheManager)
	storyFragmentIDs, err := storyFragmentService.GetAllIDs()
	if err == nil {
		storyFragments, err := storyFragmentService.GetByIDs(storyFragmentIDs)
		if err == nil {
			for _, sf := range storyFragments {
				if sf != nil {
					item := FullContentMapItem{
						ID:    sf.ID,
						Title: sf.Title,
						Slug:  sf.Slug,
						Type:  "StoryFragment",
						Panes: sf.PaneIDs,
					}

					// Get tractstack for parent info
					if sf.TractStackID != "" {
						if tractStack, err := tractStackService.GetByID(sf.TractStackID); err == nil && tractStack != nil {
							item.ParentID = &tractStack.ID
							item.ParentTitle = &tractStack.Title
							item.ParentSlug = &tractStack.Slug
						}
					}

					// Add social image path
					if sf.SocialImagePath != nil {
						item.SocialImagePath = sf.SocialImagePath
					}

					// Add changed timestamp
					if sf.Changed != nil {
						changedStr := sf.Changed.Format(time.RFC3339)
						item.Changed = &changedStr
					}

					contentMap = append(contentMap, item)
				}
			}
		}
	}

	// Get all Panes
	paneService := content.NewPaneService(ctx, cacheManager)
	paneIDs, err := paneService.GetAllIDs()
	if err == nil {
		panes, err := paneService.GetByIDs(paneIDs)
		if err == nil {
			for _, pane := range panes {
				if pane != nil {
					isContext := pane.IsContextPane
					item := FullContentMapItem{
						ID:        pane.ID,
						Title:     pane.Title,
						Slug:      pane.Slug,
						Type:      "Pane",
						IsContext: &isContext,
					}
					contentMap = append(contentMap, item)
				}
			}
		}
	}

	// Get all Resources
	resourceService := content.NewResourceService(ctx, cacheManager)
	resourceIDs, err := resourceService.GetAllIDs()
	if err == nil {
		resources, err := resourceService.GetByIDs(resourceIDs)
		if err == nil {
			for _, resource := range resources {
				if resource != nil {
					item := FullContentMapItem{
						ID:           resource.ID,
						Title:        resource.Title,
						Slug:         resource.Slug,
						Type:         "Resource",
						CategorySlug: resource.CategorySlug,
					}
					contentMap = append(contentMap, item)
				}
			}
		}
	}

	// Get all Menus
	menuService := content.NewMenuService(ctx, cacheManager)
	menuIDs, err := menuService.GetAllIDs()
	if err == nil {
		menus, err := menuService.GetByIDs(menuIDs)
		if err == nil {
			for _, menu := range menus {
				if menu != nil {
					item := FullContentMapItem{
						ID:    menu.ID,
						Title: menu.Title,
						Slug:  menu.ID, // Menus use ID as slug in V1
						Type:  "Menu",
						Theme: &menu.Theme,
					}
					contentMap = append(contentMap, item)
				}
			}
		}
	}

	// Get all Beliefs
	beliefService := content.NewBeliefService(ctx, cacheManager)
	beliefIDs, err := beliefService.GetAllIDs()
	if err == nil {
		beliefs, err := beliefService.GetByIDs(beliefIDs)
		if err == nil {
			for _, belief := range beliefs {
				if belief != nil {
					item := FullContentMapItem{
						ID:    belief.ID,
						Title: belief.Title,
						Slug:  belief.Slug,
						Type:  "Belief",
						Scale: &belief.Scale,
					}
					contentMap = append(contentMap, item)
				}
			}
		}
	}

	c.JSON(http.StatusOK, contentMap)
}
