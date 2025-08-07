// Package services provides orphan analysis orchestration
package services

import (
	"crypto/md5"
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// OrphanAnalysisService orchestrates orphan detection with cache-first repository pattern
type OrphanAnalysisService struct {
	logger *logging.ChanneledLogger
}

// NewOrphanAnalysisService creates a new orphan analysis service singleton
func NewOrphanAnalysisService(logger *logging.ChanneledLogger) *OrphanAnalysisService {
	return &OrphanAnalysisService{
		logger: logger,
	}
}

// GetOrphanAnalysis returns orphan analysis with ETag caching
func (s *OrphanAnalysisService) GetOrphanAnalysis(tenantCtx *tenant.Context, clientETag string, cacheManager interfaces.Cache) (*types.OrphanAnalysisPayload, string, error) {
	start := time.Now()
	cachedPayload, cachedETag, exists := cacheManager.GetOrphanAnalysis(tenantCtx.TenantID)

	if exists {
		if clientETag == cachedETag {
			return nil, cachedETag, nil
		}
		return cachedPayload, cachedETag, nil
	}

	loadingPayload := &types.OrphanAnalysisPayload{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Beliefs:        make(map[string][]string),
		Status:         "loading",
	}

	go s.computeOrphanAnalysisAsync(tenantCtx, cacheManager)

	etag := s.generateETag(tenantCtx.TenantID)

	s.logger.Content().Info("Successfully retrieved orphan analysis", "tenantId", tenantCtx.TenantID, "fromCache", exists, "etag", etag, "duration", time.Since(start))

	return loadingPayload, etag, nil
}

// computeOrphanAnalysisAsync performs the analysis computation in background
// computeOrphanAnalysisAsync performs the analysis computation in background
func (s *OrphanAnalysisService) computeOrphanAnalysisAsync(tenantCtx *tenant.Context, cacheManager interfaces.Cache) {
	start := time.Now()
	// Use bulk repository from tenant context
	bulkRepo := tenantCtx.BulkRepo()

	// 1. Build all 5 dependency maps using existing methods
	storyFragmentDeps, err := bulkRepo.ScanStoryFragmentDependencies(tenantCtx.TenantID)
	if err != nil {
		s.logger.Content().Error("Failed to scan storyfragment dependencies", "error", err, "tenantId", tenantCtx.TenantID)
		return
	}

	paneDeps, err := bulkRepo.ScanPaneDependencies(tenantCtx.TenantID)
	if err != nil {
		s.logger.Content().Error("Failed to scan pane dependencies", "error", err, "tenantId", tenantCtx.TenantID)
		return
	}

	menuDeps, err := bulkRepo.ScanMenuDependencies(tenantCtx.TenantID)
	if err != nil {
		s.logger.Content().Error("Failed to scan menu dependencies", "error", err, "tenantId", tenantCtx.TenantID)
		return
	}

	fileDeps, err := bulkRepo.ScanFileDependencies(tenantCtx.TenantID)
	if err != nil {
		s.logger.Content().Error("Failed to scan file dependencies", "error", err, "tenantId", tenantCtx.TenantID)
		return
	}

	beliefDeps, err := bulkRepo.ScanBeliefDependencies(tenantCtx.TenantID)
	if err != nil {
		s.logger.Content().Error("Failed to scan belief dependencies", "error", err, "tenantId", tenantCtx.TenantID)
		return
	}

	// 2. Build final payload using the dependency maps directly
	payload := &types.OrphanAnalysisPayload{
		StoryFragments: storyFragmentDeps, // Return ALL story fragments with their dependencies
		Panes:          paneDeps,          // Return ALL panes with their dependencies
		Menus:          menuDeps,          // Return ALL menus with their dependencies
		Files:          fileDeps,          // Return ALL files with their dependencies
		Beliefs:        beliefDeps,        // Return ALL beliefs with their dependencies
		Status:         "complete",
	}

	// Cache the result with ETag
	etag := s.generateETag(tenantCtx.TenantID)
	cacheManager.SetOrphanAnalysis(tenantCtx.TenantID, payload, etag)

	s.logger.Content().Info("Successfully computed orphan analysis async", "tenantId", tenantCtx.TenantID, "duration", time.Since(start))
}

// generateETag creates a unique ETag for the orphan analysis
func (s *OrphanAnalysisService) generateETag(tenantID string) string {
	timestamp := time.Now().Format(time.RFC3339)
	hash := md5.Sum([]byte(tenantID + timestamp))
	return fmt.Sprintf("\"%x\"", hash)
}
