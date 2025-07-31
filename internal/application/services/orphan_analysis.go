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
}

// NewOrphanAnalysisService creates a new orphan analysis service
func NewOrphanAnalysisService(bulkRepo bulk.BulkQueryRepository) *OrphanAnalysisService {
	return &OrphanAnalysisService{
		bulkRepo: bulkRepo,
	}
}

// GetOrphanAnalysis returns cached results or starts background computation
func (oas *OrphanAnalysisService) GetOrphanAnalysis(tenantID, clientETag string, cache interfaces.ContentCache) (*admin.OrphanAnalysisPayload, string, error) {
	// Check cache first
	if cachedPayload, etag, exists := cache.GetOrphanAnalysis(tenantID); exists {
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
		oas.computeOrphanAnalysisAsync(tenantID, cache)
	}()

	return loadingPayload, "", nil
}

// computeOrphanAnalysisAsync performs expensive computation in background
func (oas *OrphanAnalysisService) computeOrphanAnalysisAsync(tenantID string, cache interfaces.ContentCache) {
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
	cache.SetOrphanAnalysis(tenantID, convertedPayload, etag)

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
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Beliefs:        make(map[string][]string),
		Status:         "complete",
	}

	// Initialize all content IDs with empty dependencies
	for _, id := range contentIDs.StoryFragments["all"] {
		payload.StoryFragments[id] = []string{}
	}
	for _, id := range contentIDs.Panes["all"] {
		payload.Panes[id] = []string{}
	}
	for _, id := range contentIDs.Menus["all"] {
		payload.Menus[id] = []string{}
	}
	for _, id := range contentIDs.Files["all"] {
		payload.Files[id] = []string{}
	}
	for _, id := range contentIDs.Beliefs["all"] {
		payload.Beliefs[id] = []string{}
	}

	// Compute dependencies
	if err := oas.computeStoryFragmentDependencies(tenantID, payload); err != nil {
		return nil, err
	}
	if err := oas.computePaneDependencies(tenantID, payload); err != nil {
		return nil, err
	}
	if err := oas.computeMenuDependencies(tenantID, payload); err != nil {
		return nil, err
	}
	if err := oas.computeFileDependencies(tenantID, payload); err != nil {
		return nil, err
	}
	if err := oas.computeBeliefDependencies(tenantID, payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// computeStoryFragmentDependencies maps storyfragment dependencies
func (oas *OrphanAnalysisService) computeStoryFragmentDependencies(tenantID string, payload *admin.OrphanAnalysisPayload) error {
	dependencies, err := oas.bulkRepo.ScanStoryFragmentDependencies(tenantID)
	if err != nil {
		return err
	}
	for id, deps := range dependencies {
		payload.StoryFragments[id] = deps
	}
	return nil
}

// computePaneDependencies maps pane dependencies
func (oas *OrphanAnalysisService) computePaneDependencies(tenantID string, payload *admin.OrphanAnalysisPayload) error {
	dependencies, err := oas.bulkRepo.ScanPaneDependencies(tenantID)
	if err != nil {
		return err
	}
	for id, deps := range dependencies {
		payload.Panes[id] = deps
	}
	return nil
}

// computeMenuDependencies maps menu dependencies
func (oas *OrphanAnalysisService) computeMenuDependencies(tenantID string, payload *admin.OrphanAnalysisPayload) error {
	dependencies, err := oas.bulkRepo.ScanMenuDependencies(tenantID)
	if err != nil {
		return err
	}
	for id, deps := range dependencies {
		payload.Menus[id] = deps
	}
	return nil
}

// computeFileDependencies maps file dependencies
func (oas *OrphanAnalysisService) computeFileDependencies(tenantID string, payload *admin.OrphanAnalysisPayload) error {
	dependencies, err := oas.bulkRepo.ScanFileDependencies(tenantID)
	if err != nil {
		return err
	}
	for id, deps := range dependencies {
		payload.Files[id] = deps
	}
	return nil
}

// computeBeliefDependencies maps belief dependencies
func (oas *OrphanAnalysisService) computeBeliefDependencies(tenantID string, payload *admin.OrphanAnalysisPayload) error {
	dependencies, err := oas.bulkRepo.ScanBeliefDependencies(tenantID)
	if err != nil {
		return err
	}
	for id, deps := range dependencies {
		payload.Beliefs[id] = deps
	}
	return nil
}

// generateOrphanETag creates ETag based on tenant and timestamp
func (oas *OrphanAnalysisService) generateOrphanETag(tenantID string) string {
	timestamp := time.Now().Unix()
	hash := md5.Sum([]byte(fmt.Sprintf("orphan-%s-%d", tenantID, timestamp)))
	return fmt.Sprintf(`"%x"`, hash)
}
