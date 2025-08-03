// Package services provides orphan analysis orchestration
package services

import (
	"crypto/md5"
	"fmt"
	"time"

	domainservices "github.com/AtRiskMedia/tractstack-go/internal/domain/services"
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
func (s *OrphanAnalysisService) computeOrphanAnalysisAsync(tenantCtx *tenant.Context, cacheManager interfaces.Cache) {
	start := time.Now()
	// Use bulk repository from tenant context
	bulkRepo := tenantCtx.BulkRepo()
	integrityService := domainservices.NewContentIntegrityService()

	// 1. Build content ID map using existing method
	contentIDMap, err := bulkRepo.ScanAllContentIDs(tenantCtx.TenantID)
	if err != nil {
		fmt.Printf("Failed to scan content IDs: %v", err)
		return
	}

	// 2. Build all 5 dependency maps using existing methods
	storyFragmentDeps, err := bulkRepo.ScanStoryFragmentDependencies(tenantCtx.TenantID)
	if err != nil {
		fmt.Printf("Failed to scan storyfragment dependencies: %v", err)
		return
	}

	paneDeps, err := bulkRepo.ScanPaneDependencies(tenantCtx.TenantID)
	if err != nil {
		fmt.Printf("Failed to scan pane dependencies: %v", err)
		return
	}

	menuDeps, err := bulkRepo.ScanMenuDependencies(tenantCtx.TenantID)
	if err != nil {
		fmt.Printf("Failed to scan menu dependencies: %v", err)
		return
	}

	fileDeps, err := bulkRepo.ScanFileDependencies(tenantCtx.TenantID)
	if err != nil {
		fmt.Printf("Failed to scan file dependencies: %v", err)
		return
	}

	beliefDeps, err := bulkRepo.ScanBeliefDependencies(tenantCtx.TenantID)
	if err != nil {
		fmt.Printf("Failed to scan belief dependencies: %v", err)
		return
	}

	// 3. Call CalculateOrphans with correct signature (6 parameters)
	orphans := integrityService.CalculateOrphans(
		contentIDMap,
		storyFragmentDeps,
		paneDeps,
		menuDeps,
		fileDeps,
		beliefDeps,
	)

	// Build final payload
	payload := &types.OrphanAnalysisPayload{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Beliefs:        make(map[string][]string),
		Status:         "complete",
	}

	// Populate orphan data - identify which content types each orphan belongs to
	for _, orphanID := range orphans {
		// Check which content type this orphan ID belongs to and add to appropriate map
		if _, exists := contentIDMap.StoryFragments[orphanID]; exists {
			payload.StoryFragments[orphanID] = []string{} // Empty deps = orphan
		} else if _, exists := contentIDMap.Panes[orphanID]; exists {
			payload.Panes[orphanID] = []string{}
		} else if _, exists := contentIDMap.Menus[orphanID]; exists {
			payload.Menus[orphanID] = []string{}
		} else if _, exists := contentIDMap.Files[orphanID]; exists {
			payload.Files[orphanID] = []string{}
		} else if _, exists := contentIDMap.Beliefs[orphanID]; exists {
			payload.Beliefs[orphanID] = []string{}
		}
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
