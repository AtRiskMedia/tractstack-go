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
	Store(tenantID string, tractStack *content.TractStackNode) error
	Update(tenantID string, tractStack *content.TractStackNode) error
	Delete(tenantID, id string) error
}

type StoryFragmentRepository interface {
	FindByID(id string) (*content.StoryFragmentNode, error)
	FindBySlug(slug string) (*content.StoryFragmentNode, error)
	FindByTractStackID(tractStackID string) ([]*content.StoryFragmentNode, error)
	FindAll() ([]*content.StoryFragmentNode, error)
	Store(storyFragment *content.StoryFragmentNode) error
	Update(storyFragment *content.StoryFragmentNode) error
	Delete(id string) error
}

type PaneRepository interface {
	FindByID(id string) (*content.PaneNode, error)
	FindBySlug(slug string) (*content.PaneNode, error)
	FindByIDs(ids []string) ([]*content.PaneNode, error)
	FindAll() ([]*content.PaneNode, error)
	Store(pane *content.PaneNode) error
	Update(pane *content.PaneNode) error
	Delete(id string) error
}

type MenuRepository interface {
	FindByID(id string) (*content.MenuNode, error)
	FindAll() ([]*content.MenuNode, error)
	Store(menu *content.MenuNode) error
	Update(menu *content.MenuNode) error
	Delete(id string) error
}

type ResourceRepository interface {
	FindByID(id string) (*content.ResourceNode, error)
	FindBySlug(slug string) (*content.ResourceNode, error)
	FindByCategory(category string) ([]*content.ResourceNode, error)
	FindAll() ([]*content.ResourceNode, error)
	Store(resource *content.ResourceNode) error
	Update(resource *content.ResourceNode) error
	Delete(id string) error
}

type BeliefRepository interface {
	FindByID(id string) (*content.BeliefNode, error)
	FindBySlug(slug string) (*content.BeliefNode, error)
	FindAll() ([]*content.BeliefNode, error)
	Store(belief *content.BeliefNode) error
	Update(belief *content.BeliefNode) error
	Delete(id string) error
}

type EpinetRepository interface {
	FindByID(id string) (*content.EpinetNode, error)
	FindAll() ([]*content.EpinetNode, error)
	Store(epinet *content.EpinetNode) error
	Update(epinet *content.EpinetNode) error
	Delete(id string) error
}

type ImageFileRepository interface {
	FindByID(id string) (*content.ImageFileNode, error)
	FindAll() ([]*content.ImageFileNode, error)
	Store(imageFile *content.ImageFileNode) error
	Update(imageFile *content.ImageFileNode) error
	Delete(id string) error
}
