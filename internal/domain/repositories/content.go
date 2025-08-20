// Package repositories defines the repository interfaces for content entities.
// These repositories abstract the data persistence details, ensuring the core
// application is clean and decoupled from the database.
package repositories

import (
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
)

type TractStackRepository interface {
	FindByID(tenantID, id string) (*content.TractStackNode, error)
	FindBySlug(tenantID, slug string) (*content.TractStackNode, error)
	FindAll(tenantID string) ([]*content.TractStackNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.TractStackNode, error)
	Store(tenantID string, tractStack *content.TractStackNode) error
	Update(tenantID string, tractStack *content.TractStackNode) error
	Delete(tenantID, id string) error
}

type StoryFragmentRepository interface {
	FindByID(tenantID, id string) (*content.StoryFragmentNode, error)
	FindBySlug(tenantID, slug string) (*content.StoryFragmentNode, error)
	FindByTractStackID(tenantID, tractStackID string) ([]*content.StoryFragmentNode, error)
	FindAll(tenantID string) ([]*content.StoryFragmentNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.StoryFragmentNode, error)
	Store(tenantID string, storyFragment *content.StoryFragmentNode) error
	Update(tenantID string, storyFragment *content.StoryFragmentNode) error
	Delete(tenantID, id string) error
	UpdatePaneRelationships(tenantID, storyFragmentID string, paneIDs []string) error
	UpdateTopics(tenantID, storyFragmentID string, topics []string) error
	UpdateDescription(tenantID, storyFragmentID string, description *string) error
}

type PaneRepository interface {
	FindByID(tenantID, id string) (*content.PaneNode, error)
	FindBySlug(tenantID, slug string) (*content.PaneNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.PaneNode, error)
	FindAll(tenantID string) ([]*content.PaneNode, error)
	FindContext(tenantID string) ([]*content.PaneNode, error)
	Store(tenantID string, pane *content.PaneNode) error
	Update(tenantID string, pane *content.PaneNode) error
	Delete(tenantID, id string) error
}

type MenuRepository interface {
	FindByID(tenantID, id string) (*content.MenuNode, error)
	FindAll(tenantID string) ([]*content.MenuNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.MenuNode, error)
	Store(tenantID string, menu *content.MenuNode) error
	Update(tenantID string, menu *content.MenuNode) error
	Delete(tenantID, id string) error
}

type ResourceRepository interface {
	FindByID(tenantID, id string) (*content.ResourceNode, error)
	FindBySlug(tenantID, slug string) (*content.ResourceNode, error)
	FindByCategory(tenantID, category string) ([]*content.ResourceNode, error)
	FindAll(tenantID string) ([]*content.ResourceNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.ResourceNode, error)
	FindByFilters(tenantID string, ids []string, categories []string, slugs []string) ([]*content.ResourceNode, error)
	Store(tenantID string, resource *content.ResourceNode) error
	Update(tenantID string, resource *content.ResourceNode) error
	Delete(tenantID, id string) error
}

type BeliefRepository interface {
	FindByID(tenantID, id string) (*content.BeliefNode, error)
	FindBySlug(tenantID, slug string) (*content.BeliefNode, error)
	FindIDBySlug(tenantID, slug string) (string, error)
	FindAll(tenantID string) ([]*content.BeliefNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.BeliefNode, error)
	Store(tenantID string, belief *content.BeliefNode) error
	Update(tenantID string, belief *content.BeliefNode) error
	Delete(tenantID, id string) error
}

type EpinetRepository interface {
	FindByID(tenantID, id string) (*content.EpinetNode, error)
	FindAll(tenantID string) ([]*content.EpinetNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.EpinetNode, error)
	Store(tenantID string, epinet *content.EpinetNode) error
	Update(tenantID string, epinet *content.EpinetNode) error
	Delete(tenantID, id string) error
}

type ImageFileRepository interface {
	FindByID(tenantID, id string) (*content.ImageFileNode, error)
	FindAll(tenantID string) ([]*content.ImageFileNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.ImageFileNode, error)
	Store(tenantID string, imageFile *content.ImageFileNode) error
	Update(tenantID string, imageFile *content.ImageFileNode) error
	Delete(tenantID, id string) error
}
