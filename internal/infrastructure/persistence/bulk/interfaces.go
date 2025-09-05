// Package bulk provides interfaces for efficient bulk database operations
// that scan entire database tables for content map and dependency analysis.
package bulk

import (
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
)

// ContentMapRepository provides efficient bulk queries for building content maps
type ContentMapRepository interface {
	// BuildContentMap executes a single UNION query across all content tables
	// to efficiently retrieve all content items for the content map
	BuildContentMap(tenantID string) ([]*content.ContentMapItem, error)
}

// DependencyRepository provides bulk queries for dependency analysis
type DependencyRepository interface {
	// ScanAllContentIDs returns all content IDs by type for dependency initialization
	ScanAllContentIDs(tenantID string) (*admin.ContentIDMap, error)

	// ScanStoryFragmentDependencies finds what depends on each story fragment
	ScanStoryFragmentDependencies(tenantID string) (map[string][]string, error)

	// ScanPaneDependencies finds what story fragments depend on each pane
	ScanPaneDependencies(tenantID string) (map[string][]string, error)

	// ScanMenuDependencies finds what story fragments depend on each menu
	ScanMenuDependencies(tenantID string) (map[string][]string, error)

	// ScanFileDependencies finds what panes depend on each file
	ScanFileDependencies(tenantID string) (map[string][]string, error)

	// ScanBeliefDependencies finds what panes depend on each belief
	ScanBeliefDependencies(tenantID string) (map[string][]string, error)
}

// BulkQueryRepository combines both content map and dependency operations
type BulkQueryRepository interface {
	ContentMapRepository
	DependencyRepository
}
