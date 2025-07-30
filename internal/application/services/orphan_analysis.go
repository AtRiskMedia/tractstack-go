// Package services provides orphan analysis async orchestration
package services

import (
	"crypto/md5"
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/bulk"
)

// OrphanAnalysisService orchestrates async orphan analysis computation
type OrphanAnalysisService struct {
	bulkRepo bulk.BulkQueryRepository
	cache    interfaces.ContentCache
}

// NewOrphanAnalysisService creates a new orphan analysis service
func NewOrphanAnalysisService(bulkRepo bulk.BulkQueryRepository, cache interfaces.ContentCache) *OrphanAnalysisService {
	return &OrphanAnalysisService{
		bulkRepo: bulkRepo,
		cache:    cache,
	}
}

// GetOrphanAnalysis returns cached results or starts background computation
func (oas *OrphanAnalysisService) GetOrphanAnalysis(tenantID, clientETag string) (*admin.OrphanAnalysisPayload, string, error) {
	// Check cache first
	if cachedPayload, etag, exists := oas.cache.GetOrphanAnalysis(tenantID); exists {
		// Convert from types.OrphanAnalysisPayload to admin.OrphanAnalysisPayload
		convertedPayload := &admin.OrphanAnalysisPayload{
			StoryFragments: cachedPayload.StoryFragments,
			Panes:          cachedPayload.Panes,
			Menus:          cachedPayload.Menus,
			Files:          cachedPayload.Files,
			Beliefs:        cachedPayload.Beliefs,
			Status:         cachedPayload.Status,
		}

		// Handle 304 Not Modified
		if clientETag == etag {
			return nil, etag, nil
		}
		// Return cached data
		return convertedPayload, etag, nil
	}

	// Cache miss - return loading state and start background computation
	loadingPayload := &admin.OrphanAnalysisPayload{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Beliefs:        make(map[string][]string),
		Status:         "loading",
	}

	// Start background computation
	go func() {
		oas.computeOrphanAnalysisAsync(tenantID)
	}()

	return loadingPayload, "", nil
}

// computeOrphanAnalysisAsync performs expensive computation in background
func (oas *OrphanAnalysisService) computeOrphanAnalysisAsync(tenantID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Orphan analysis computation failed for tenant %s: %v", tenantID, r)
		}
	}()

	// Compute orphan analysis
	payload, err := oas.computeOrphanAnalysis(tenantID)
	if err != nil {
		log.Printf("Error computing orphan analysis for tenant %s: %v", tenantID, err)
		return
	}

	// Generate ETag
	etag := oas.generateOrphanETag(tenantID)

	// Cache result - convert admin.OrphanAnalysisPayload to types.OrphanAnalysisPayload
	convertedPayload := &types.OrphanAnalysisPayload{
		StoryFragments: payload.StoryFragments,
		Panes:          payload.Panes,
		Menus:          payload.Menus,
		Files:          payload.Files,
		Beliefs:        payload.Beliefs,
		Status:         payload.Status,
	}
	oas.cache.SetOrphanAnalysis(tenantID, convertedPayload, etag)

	log.Printf("Orphan analysis completed for tenant %s", tenantID)
}

// computeOrphanAnalysis performs the complete dependency mapping computation
func (oas *OrphanAnalysisService) computeOrphanAnalysis(tenantID string) (*admin.OrphanAnalysisPayload, error) {
	// Initialize content IDs
	contentIDs, err := oas.bulkRepo.ScanAllContentIDs(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan content IDs: %w", err)
	}

	payload := &admin.OrphanAnalysisPayload{
		StoryFragments: contentIDs.StoryFragments,
		Panes:          contentIDs.Panes,
		Menus:          contentIDs.Menus,
		Files:          contentIDs.Files,
		Beliefs:        contentIDs.Beliefs,
		Status:         "complete",
	}

	// Compute story fragment dependencies
	sfDeps, err := oas.bulkRepo.ScanStoryFragmentDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan story fragment dependencies: %w", err)
	}
	for id, deps := range sfDeps {
		payload.StoryFragments[id] = deps
	}

	// Compute pane dependencies
	paneDeps, err := oas.bulkRepo.ScanPaneDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan pane dependencies: %w", err)
	}
	for id, deps := range paneDeps {
		payload.Panes[id] = deps
	}

	// Compute menu dependencies
	menuDeps, err := oas.bulkRepo.ScanMenuDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan menu dependencies: %w", err)
	}
	for id, deps := range menuDeps {
		payload.Menus[id] = deps
	}

	// Compute file dependencies
	fileDeps, err := oas.bulkRepo.ScanFileDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan file dependencies: %w", err)
	}
	for id, deps := range fileDeps {
		payload.Files[id] = deps
	}

	// Compute belief dependencies
	beliefDeps, err := oas.bulkRepo.ScanBeliefDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan belief dependencies: %w", err)
	}
	for id, deps := range beliefDeps {
		payload.Beliefs[id] = deps
	}

	return payload, nil
}

// generateOrphanETag creates ETag based on tenant and timestamp
func (oas *OrphanAnalysisService) generateOrphanETag(tenantID string) string {
	timestamp := time.Now().Unix()
	hash := md5.Sum([]byte(fmt.Sprintf("orphan-%s-%d", tenantID, timestamp)))
	return fmt.Sprintf(`"%x"`, hash)
}
