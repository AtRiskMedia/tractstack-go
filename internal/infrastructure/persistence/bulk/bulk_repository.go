// Package bulk provides concrete implementation of bulk query repository
package bulk

import (
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

// Repository implements BulkQueryRepository interface
type Repository struct {
	contentMapBuilder *ContentMapBuilder
	dependencyScanner *DependencyScanner
}

// NewRepository creates a new bulk query repository
func NewRepository(db *database.DB) *Repository {
	return &Repository{
		contentMapBuilder: NewContentMapBuilder(db),
		dependencyScanner: NewDependencyScanner(db),
	}
}

// BuildContentMap implements ContentMapRepository
func (r *Repository) BuildContentMap(tenantID string) ([]*content.ContentMapItem, error) {
	return r.contentMapBuilder.BuildContentMap(tenantID)
}

// ScanAllContentIDs implements DependencyRepository
func (r *Repository) ScanAllContentIDs(tenantID string) (*admin.ContentIDMap, error) {
	return r.dependencyScanner.ScanAllContentIDs(tenantID)
}

// ScanStoryFragmentDependencies implements DependencyRepository
func (r *Repository) ScanStoryFragmentDependencies(tenantID string) (map[string][]string, error) {
	return r.dependencyScanner.ScanStoryFragmentDependencies(tenantID)
}

// ScanPaneDependencies implements DependencyRepository
func (r *Repository) ScanPaneDependencies(tenantID string) (map[string][]string, error) {
	return r.dependencyScanner.ScanPaneDependencies(tenantID)
}

// ScanMenuDependencies implements DependencyRepository
func (r *Repository) ScanMenuDependencies(tenantID string) (map[string][]string, error) {
	return r.dependencyScanner.ScanMenuDependencies(tenantID)
}

// ScanFileDependencies implements DependencyRepository
func (r *Repository) ScanFileDependencies(tenantID string) (map[string][]string, error) {
	return r.dependencyScanner.ScanFileDependencies(tenantID)
}

// ScanBeliefDependencies implements DependencyRepository
func (r *Repository) ScanBeliefDependencies(tenantID string) (map[string][]string, error) {
	return r.dependencyScanner.ScanBeliefDependencies(tenantID)
}
