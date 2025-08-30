// Package bulk provides concrete implementation of bulk query repository
package bulk

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

// Repository implements BulkQueryRepository interface
type Repository struct {
	contentMapBuilder *ContentMapBuilder
	dependencyScanner *DependencyScanner
	logger            *logging.ChanneledLogger
}

// NewRepository creates a new bulk query repository
func NewRepository(db *database.DB, logger *logging.ChanneledLogger) *Repository {
	return &Repository{
		contentMapBuilder: NewContentMapBuilder(db, logger),
		dependencyScanner: NewDependencyScanner(db, logger),
		logger:            logger,
	}
}

// BuildContentMap implements ContentMapRepository
func (r *Repository) BuildContentMap(tenantID string) ([]*content.ContentMapItem, error) {
	start := time.Now()
	r.logger.Database().Debug("Starting bulk content map build", "tenantID", tenantID)
	result, err := r.contentMapBuilder.BuildContentMap(tenantID)
	if err != nil {
		r.logger.Database().Error("Bulk content map build failed", "error", err.Error(), "tenantID", tenantID)
		return nil, err
	}
	r.logger.Database().Info("Bulk content map build completed", "tenantID", tenantID, "itemCount", len(result))
	duration := time.Since(start)
	database.CheckAndLogSlowQuery(r.logger,
		"BULK_CONTENT_MAP_BUILD", duration, tenantID)
	return result, nil
}

// ScanAllContentIDs implements DependencyRepository
func (r *Repository) ScanAllContentIDs(tenantID string) (*admin.ContentIDMap, error) {
	start := time.Now()
	r.logger.Database().Debug("Starting bulk content ID scan", "tenantID", tenantID)
	result, err := r.dependencyScanner.ScanAllContentIDs(tenantID)
	if err != nil {
		r.logger.Database().Error("Bulk content ID scan failed", "error", err.Error(), "tenantID", tenantID)
		return nil, err
	}
	r.logger.Database().Info("Bulk content ID scan completed", "tenantID", tenantID)
	duration := time.Since(start)
	database.CheckAndLogSlowQuery(r.logger,
		"BULK_CONTENT_ID_SCAN", duration, tenantID)
	return result, nil
}

// ScanStoryFragmentDependencies implements DependencyRepository
func (r *Repository) ScanStoryFragmentDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	r.logger.Database().Debug("Starting story fragment dependency scan", "tenantID", tenantID)
	result, err := r.dependencyScanner.ScanStoryFragmentDependencies(tenantID)
	if err != nil {
		r.logger.Database().Error("Story fragment dependency scan failed", "error", err.Error(), "tenantID", tenantID)
		return nil, err
	}
	r.logger.Database().Info("Story fragment dependency scan completed", "tenantID", tenantID, "sfCount", len(result))
	duration := time.Since(start)
	database.CheckAndLogSlowQuery(r.logger,
		"BULK_SF_DEPENDENCY_SCAN", duration, tenantID)
	return result, nil
}

// ScanPaneDependencies implements DependencyRepository
func (r *Repository) ScanPaneDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	r.logger.Database().Debug("Starting pane dependency scan", "tenantID", tenantID)
	result, err := r.dependencyScanner.ScanPaneDependencies(tenantID)
	if err != nil {
		r.logger.Database().Error("Pane dependency scan failed", "error", err.Error(), "tenantID", tenantID)
		return nil, err
	}
	r.logger.Database().Info("Pane dependency scan completed", "tenantID", tenantID, "paneCount", len(result))
	duration := time.Since(start)
	database.CheckAndLogSlowQuery(r.logger,
		"BULK_PANE_DEPENDENCY_SCAN", duration, tenantID)
	return result, nil
}

// ScanMenuDependencies implements DependencyRepository
func (r *Repository) ScanMenuDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	r.logger.Database().Debug("Starting menu dependency scan", "tenantID", tenantID)
	result, err := r.dependencyScanner.ScanMenuDependencies(tenantID)
	if err != nil {
		r.logger.Database().Error("Menu dependency scan failed", "error", err.Error(), "tenantID", tenantID)
		return nil, err
	}
	r.logger.Database().Info("Menu dependency scan completed", "tenantID", tenantID, "menuCount", len(result))
	duration := time.Since(start)
	database.CheckAndLogSlowQuery(r.logger,
		"BULK_MENU_DEPENDENCY_SCAN", duration, tenantID)
	return result, nil
}

// ScanFileDependencies implements DependencyRepository
func (r *Repository) ScanFileDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	r.logger.Database().Debug("Starting file dependency scan", "tenantID", tenantID)
	result, err := r.dependencyScanner.ScanFileDependencies(tenantID)
	if err != nil {
		r.logger.Database().Error("File dependency scan failed", "error", err.Error(), "tenantID", tenantID)
		return nil, err
	}
	r.logger.Database().Info("File dependency scan completed", "tenantID", tenantID, "fileCount", len(result))
	duration := time.Since(start)
	database.CheckAndLogSlowQuery(r.logger,
		"BULK_FILE_DEPENDENCY_SCAN", duration, tenantID)
	return result, nil
}

// ScanBeliefDependencies implements DependencyRepository
func (r *Repository) ScanBeliefDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	r.logger.Database().Debug("Starting belief dependency scan", "tenantID", tenantID)
	result, err := r.dependencyScanner.ScanBeliefDependencies(tenantID)
	if err != nil {
		r.logger.Database().Error("Belief dependency scan failed", "error", err.Error(), "tenantID", tenantID)
		return nil, err
	}
	r.logger.Database().Info("Belief dependency scan completed", "tenantID", tenantID, "beliefCount", len(result))
	duration := time.Since(start)
	database.CheckAndLogSlowQuery(r.logger,
		"BULK_BELIEF_DEPENDENCY_SCAN", duration, tenantID)
	return result, nil
}
