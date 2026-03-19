// Package repositories defines the repository interfaces for content entities.
// These repositories abstract the data persistence details, ensuring the core
// application is clean and decoupled from the database.
package repositories

import (
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
)

// TractStackRepository defines the persistence interface for TractStack nodes.
type TractStackRepository interface {
	FindByID(tenantID, id string) (*content.TractStackNode, error)
	FindBySlug(tenantID, slug string) (*content.TractStackNode, error)
	FindAll(tenantID string) ([]*content.TractStackNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.TractStackNode, error)
	Store(tenantID string, tractStack *content.TractStackNode) error
	Update(tenantID string, tractStack *content.TractStackNode) error
	Delete(tenantID, id string) error
}

// StoryFragmentRepository defines the persistence interface for StoryFragment nodes and their relationships.
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
	FindIDsByPaneID(paneID string) ([]string, error)
	SearchMetadata(tenantID, term string) ([]FTSResult, error)
}

// PaneRepository defines the persistence interface for Pane nodes, including content search and markdown handling.
type PaneRepository interface {
	FindByID(tenantID, id string) (*content.PaneNode, error)
	FindBySlug(tenantID, slug string) (*content.PaneNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.PaneNode, error)
	FindAll(tenantID string) ([]*content.PaneNode, error)
	FindContext(tenantID string) ([]*content.PaneNode, error)
	Store(tenantID string, pane *content.PaneNode, markdownBody string) error
	Update(tenantID string, pane *content.PaneNode, markdownBody string) error
	Delete(tenantID, id string) error
	DeleteMany(tenantID string, ids []string) error
	UpdateFilePaneRelationships(tenantID string, relationships map[string][]string) error
	SearchMarkdownContent(tenantID, searchTerm string) ([]string, error)
	FindPaneIDsByMarkdownIDs(tenantID string, markdownIDs []string) ([]string, error)
	FindPaneContextStatus(tenantID string, paneIDs []string) (map[string]bool, error)
	SearchContent(tenantID, term string) ([]FTSResult, error)
}

// MenuRepository defines the persistence interface for Menu nodes.
type MenuRepository interface {
	FindByID(tenantID, id string) (*content.MenuNode, error)
	FindAll(tenantID string) ([]*content.MenuNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.MenuNode, error)
	Store(tenantID string, menu *content.MenuNode) error
	Update(tenantID string, menu *content.MenuNode) error
	Delete(tenantID, id string) error
}

// ResourceRepository defines the persistence interface for Resource nodes.
type ResourceRepository interface {
	FindByID(tenantID, id string) (*content.ResourceNode, error)
	FindBySlug(tenantID, slug string) (*content.ResourceNode, error)
	FindByCategory(tenantID, category string) ([]*content.ResourceNode, error)
	FindAll(tenantID string) ([]*content.ResourceNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.ResourceNode, error)
	FindByFilters(tenantID string, ids []string, categories []string, slugs []string) ([]*content.ResourceNode, error)
	Store(tenantID string, resource *content.ResourceNode, fileIDs []string) error
	Update(tenantID string, resource *content.ResourceNode, fileIDs []string) error
	Delete(tenantID, id string) error
	SearchBodies(tenantID, term string) ([]FTSResult, error)
	FindFileIDsByResourceID(tenantID string, resourceID string) ([]string, error)
	BatchUpsert(tenantID string, creates []*content.ResourceNode, updates []*content.ResourceNode) error
	ExistsByShopifyGID(tenantID, gid string) (bool, error)
}

// BeliefRepository defines the persistence interface for Belief nodes.
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

// EpinetRepository defines the persistence interface for Epinet nodes.
type EpinetRepository interface {
	FindByID(tenantID, id string) (*content.EpinetNode, error)
	FindAll(tenantID string) ([]*content.EpinetNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.EpinetNode, error)
	Store(tenantID string, epinet *content.EpinetNode) error
	Update(tenantID string, epinet *content.EpinetNode) error
	Delete(tenantID, id string) error
}

// ImageFileRepository defines the persistence interface for ImageFile nodes.
type ImageFileRepository interface {
	FindByID(tenantID, id string) (*content.ImageFileNode, error)
	FindAll(tenantID string) ([]*content.ImageFileNode, error)
	FindByIDs(tenantID string, ids []string) ([]*content.ImageFileNode, error)
	Store(tenantID string, imageFile *content.ImageFileNode) error
	Update(tenantID string, imageFile *content.ImageFileNode) error
	Delete(tenantID, id string) error
}

// FTSResult represents a generic result from a full-text search query.
type FTSResult struct {
	ID        string
	Relevance float64
	Term      string
}
