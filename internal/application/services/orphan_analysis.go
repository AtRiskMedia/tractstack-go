package services

import (
	"crypto/md5"
	"fmt"
	"time"

	domainservices "github.com/AtRiskMedia/tractstack-go/internal/domain/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/bulk"
)

type OrphanAnalysisService struct {
	bulkRepo         *bulk.Repository
	integrityService *domainservices.ContentIntegrityService
}

func NewOrphanAnalysisService(bulkRepo *bulk.Repository) *OrphanAnalysisService {
	return &OrphanAnalysisService{
		bulkRepo:         bulkRepo,
		integrityService: domainservices.NewContentIntegrityService(),
	}
}

func (s *OrphanAnalysisService) GetOrphanAnalysis(tenantID, clientETag string, cacheManager interfaces.Cache) (*types.OrphanAnalysisPayload, string, error) {
	cachedPayload, cachedETag, exists := cacheManager.GetOrphanAnalysis(tenantID)

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

	go s.computeOrphanAnalysisAsync(tenantID, cacheManager)

	etag := s.generateETag(tenantID)
	return loadingPayload, etag, nil
}

func (s *OrphanAnalysisService) computeOrphanAnalysisAsync(tenantID string, cacheManager interfaces.Cache) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Orphan analysis computation failed for tenant %s: %v\n", tenantID, r)
		}
	}()

	contentIDMap, err := s.bulkRepo.ScanAllContentIDs(tenantID)
	if err != nil {
		fmt.Printf("Error scanning content IDs for tenant %s: %v\n", tenantID, err)
		return
	}

	storyFragmentDeps, err := s.bulkRepo.ScanStoryFragmentDependencies(tenantID)
	if err != nil {
		fmt.Printf("Error scanning story fragment dependencies for tenant %s: %v\n", tenantID, err)
		return
	}

	paneDeps, err := s.bulkRepo.ScanPaneDependencies(tenantID)
	if err != nil {
		fmt.Printf("Error scanning pane dependencies for tenant %s: %v\n", tenantID, err)
		return
	}

	menuDeps, err := s.bulkRepo.ScanMenuDependencies(tenantID)
	if err != nil {
		fmt.Printf("Error scanning menu dependencies for tenant %s: %v\n", tenantID, err)
		return
	}

	fileDeps, err := s.bulkRepo.ScanFileDependencies(tenantID)
	if err != nil {
		fmt.Printf("Error scanning file dependencies for tenant %s: %v\n", tenantID, err)
		return
	}

	beliefDeps, err := s.bulkRepo.ScanBeliefDependencies(tenantID)
	if err != nil {
		fmt.Printf("Error scanning belief dependencies for tenant %s: %v\n", tenantID, err)
		return
	}

	adminPayload := s.integrityService.BuildOrphanAnalysisPayload(
		contentIDMap,
		storyFragmentDeps,
		paneDeps,
		menuDeps,
		fileDeps,
		beliefDeps,
	)

	payload := &types.OrphanAnalysisPayload{
		StoryFragments: adminPayload.StoryFragments,
		Panes:          adminPayload.Panes,
		Menus:          adminPayload.Menus,
		Files:          adminPayload.Files,
		Beliefs:        adminPayload.Beliefs,
		Status:         adminPayload.Status,
	}

	etag := s.generateETag(tenantID)
	cacheManager.SetOrphanAnalysis(tenantID, payload, etag)
}

func (s *OrphanAnalysisService) FindOrphans(tenantID string) ([]string, error) {
	contentIDMap, err := s.bulkRepo.ScanAllContentIDs(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan content IDs: %w", err)
	}

	storyFragmentDeps, err := s.bulkRepo.ScanStoryFragmentDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan story fragment dependencies: %w", err)
	}

	paneDeps, err := s.bulkRepo.ScanPaneDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan pane dependencies: %w", err)
	}

	menuDeps, err := s.bulkRepo.ScanMenuDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan menu dependencies: %w", err)
	}

	fileDeps, err := s.bulkRepo.ScanFileDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan file dependencies: %w", err)
	}

	beliefDeps, err := s.bulkRepo.ScanBeliefDependencies(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to scan belief dependencies: %w", err)
	}

	orphans := s.integrityService.CalculateOrphans(
		contentIDMap,
		storyFragmentDeps,
		paneDeps,
		menuDeps,
		fileDeps,
		beliefDeps,
	)

	return orphans, nil
}

func (s *OrphanAnalysisService) generateETag(tenantID string) string {
	timestamp := time.Now().Unix()
	hash := md5.Sum([]byte(fmt.Sprintf("orphan-%s-%d", tenantID, timestamp)))
	return fmt.Sprintf(`"%x"`, hash)
}
