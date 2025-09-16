// Package services provides orphan analysis orchestration
package services

import (
	"crypto/md5"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching"
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

// GetOrphanAnalysis returns orphan analysis with ETag caching and warming lock protection
func (s *OrphanAnalysisService) GetOrphanAnalysis(
	tenantCtx *tenant.Context,
	clientETag string,
	cacheManager interfaces.Cache,
) (*types.OrphanAnalysisPayload, string, error) {
	start := time.Now()

	// Check cache first
	cachedPayload, cachedETag, exists := cacheManager.GetOrphanAnalysis(tenantCtx.TenantID)
	if exists {
		if clientETag == cachedETag {
			s.logger.Content().Debug(
				"Orphan analysis cache hit with matching ETag",
				"tenantId", tenantCtx.TenantID,
				"etag", clientETag,
				"duration", time.Since(start),
			)
			return nil, cachedETag, nil
		}
		s.logger.Content().Debug(
			"Orphan analysis cache hit with different ETag",
			"tenantId", tenantCtx.TenantID,
			"cachedETag", cachedETag,
			"clientETag", clientETag,
			"duration", time.Since(start),
		)
		return cachedPayload, cachedETag, nil
	}

	// Use warming lock to prevent multiple concurrent computations
	warmingLock := caching.GetGlobalWarmingLock()
	lockKey := fmt.Sprintf("warm:orphan:%s", tenantCtx.TenantID)

	// Attempt to acquire lock
	if !warmingLock.TryLock(lockKey) {
		s.logger.Content().Debug(
			"Orphan analysis warming lock already held, returning loading status",
			"tenantId", tenantCtx.TenantID,
			"lockKey", lockKey,
		)

		// Return loading payload while computation is in progress
		loadingPayload := &types.OrphanAnalysisPayload{
			StoryFragments: make(map[string][]string),
			Panes:          make(map[string][]string),
			Menus:          make(map[string][]string),
			Files:          make(map[string][]string),
			Beliefs:        make(map[string][]string),
			Status:         "loading",
		}

		// Stable ETag for loading
		etag := s.generateETag(tenantCtx.TenantID)
		return loadingPayload, etag, nil
	}

	// Lock acquired → start background computation
	s.logger.Content().Debug(
		"Orphan analysis warming lock acquired, starting background computation",
		"tenantId", tenantCtx.TenantID,
		"lockKey", lockKey,
	)

	// Return loading immediately, background compute updates cache
	loadingPayload := &types.OrphanAnalysisPayload{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Beliefs:        make(map[string][]string),
		Status:         "loading",
	}

	go func() {
		defer warmingLock.Unlock(lockKey)
		s.logger.Content().Debug(
			"Starting parallel orphan analysis computation",
			"tenantId", tenantCtx.TenantID,
		)
		s.computeOrphanAnalysisParallel(tenantCtx, cacheManager)
		s.logger.Content().Debug(
			"Completed parallel orphan analysis computation",
			"tenantId", tenantCtx.TenantID,
		)
	}()

	// Stable ETag for loading
	etag := s.generateETag(tenantCtx.TenantID)
	s.logger.Content().Debug(
		"Orphan analysis request completed with loading status",
		"tenantId", tenantCtx.TenantID,
		"etag", etag,
		"duration", time.Since(start),
	)

	return loadingPayload, etag, nil
}

// computeOrphanAnalysisParallel performs the analysis computation with parallel execution
func (s *OrphanAnalysisService) computeOrphanAnalysisParallel(tenantCtx *tenant.Context, cacheManager interfaces.Cache) {
	start := time.Now()
	s.logger.Content().Debug("Starting parallel dependency scans", "tenantId", tenantCtx.TenantID)

	// Use bulk repository from tenant context
	bulkRepo := tenantCtx.BulkRepo()

	type scanResult struct {
		name string
		data map[string][]string
		err  error
	}

	results := make(chan scanResult, 5)
	var wg sync.WaitGroup

	// Launch all scans in parallel
	wg.Add(5)

	go func() {
		defer wg.Done()
		s.logger.Content().Debug("Starting story fragment dependency scan", "tenantId", tenantCtx.TenantID)
		data, err := bulkRepo.ScanStoryFragmentDependencies(tenantCtx.TenantID)
		s.logger.Content().Debug("Story fragment dependency scan completed", "tenantId", tenantCtx.TenantID, "count", len(data), "hasError", err != nil)
		results <- scanResult{"storyfragments", data, err}
	}()

	go func() {
		defer wg.Done()
		s.logger.Content().Debug("Starting pane dependency scan", "tenantId", tenantCtx.TenantID)
		data, err := bulkRepo.ScanPaneDependencies(tenantCtx.TenantID)
		s.logger.Content().Debug("Pane dependency scan completed", "tenantId", tenantCtx.TenantID, "count", len(data), "hasError", err != nil)
		results <- scanResult{"panes", data, err}
	}()

	go func() {
		defer wg.Done()
		s.logger.Content().Debug("Starting menu dependency scan", "tenantId", tenantCtx.TenantID)
		data, err := bulkRepo.ScanMenuDependencies(tenantCtx.TenantID)
		s.logger.Content().Debug("Menu dependency scan completed", "tenantId", tenantCtx.TenantID, "count", len(data), "hasError", err != nil)
		results <- scanResult{"menus", data, err}
	}()

	go func() {
		defer wg.Done()
		s.logger.Content().Debug("Starting file dependency scan", "tenantId", tenantCtx.TenantID)
		data, err := s.scanFileDependenciesOptimized(tenantCtx)
		s.logger.Content().Debug("File dependency scan completed", "tenantId", tenantCtx.TenantID, "count", len(data), "hasError", err != nil)
		results <- scanResult{"files", data, err}
	}()

	go func() {
		defer wg.Done()
		s.logger.Content().Debug("Starting belief dependency scan", "tenantId", tenantCtx.TenantID)
		data, err := s.scanBeliefDependenciesFromCache(tenantCtx, cacheManager)
		s.logger.Content().Debug("Belief dependency scan completed", "tenantId", tenantCtx.TenantID, "count", len(data), "hasError", err != nil)
		results <- scanResult{"beliefs", data, err}
	}()

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var (
		storyFragmentDeps map[string][]string
		paneDeps          map[string][]string
		menuDeps          map[string][]string
		fileDeps          map[string][]string
		beliefDeps        map[string][]string
	)

	for result := range results {
		if result.err != nil {
			s.logger.Content().Error("Failed to scan dependencies", "type", result.name, "error", result.err, "tenantId", tenantCtx.TenantID)
			// Ensure lock is released on error
			return
		}

		switch result.name {
		case "storyfragments":
			storyFragmentDeps = result.data
		case "panes":
			paneDeps = result.data
		case "menus":
			menuDeps = result.data
		case "files":
			fileDeps = result.data
		case "beliefs":
			beliefDeps = result.data
		}
	}

	s.logger.Content().Debug("All parallel dependency scans completed", "tenantId", tenantCtx.TenantID,
		"storyFragments", len(storyFragmentDeps), "panes", len(paneDeps), "menus", len(menuDeps),
		"files", len(fileDeps), "beliefs", len(beliefDeps))

	payload := &types.OrphanAnalysisPayload{
		StoryFragments: storyFragmentDeps,
		Panes:          paneDeps,
		Menus:          menuDeps,
		Files:          fileDeps,
		Beliefs:        beliefDeps,
		Status:         "complete",
	}

	etag := s.generateETag(tenantCtx.TenantID)
	cacheManager.SetOrphanAnalysis(tenantCtx.TenantID, payload, etag)

	s.logger.Content().Debug("Orphan analysis cached with complete status", "tenantId", tenantCtx.TenantID, "etag", etag, "duration", time.Since(start))
}

// scanFileDependenciesOptimized uses only the file_panes table without redundant JSON parsing
func (s *OrphanAnalysisService) scanFileDependenciesOptimized(tenantCtx *tenant.Context) (map[string][]string, error) {
	start := time.Now()
	s.logger.Content().Debug("Starting optimized file dependency scan", "tenantId", tenantCtx.TenantID)

	db := tenantCtx.Database.Conn
	dependencies := make(map[string][]string)

	// Get all file IDs first
	rows, err := db.Query("SELECT id FROM files")
	if err != nil {
		s.logger.Content().Error("File IDs query failed", "error", err.Error(), "tenantId", tenantCtx.TenantID)
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Content().Debug("Failed to close rows", "error", err.Error(), "tenantId", tenantCtx.TenantID)
		}
	}()

	fileCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			fileCount++
		}
	}
	s.logger.Content().Debug("File IDs loaded", "fileCount", fileCount, "tenantId", tenantCtx.TenantID)

	// Get file dependencies ONLY from file_panes table (no JSON parsing needed)
	depRows, err := db.Query("SELECT file_id, pane_id FROM file_panes")
	if err != nil {
		s.logger.Content().Error("File dependencies query failed", "error", err.Error(), "tenantId", tenantCtx.TenantID)
		return nil, err
	}
	defer func() {
		if err := depRows.Close(); err != nil {
			s.logger.Content().Debug("Failed to close depRows", "error", err.Error(), "tenantId", tenantCtx.TenantID)
		}
	}()

	depCount := 0
	for depRows.Next() {
		var fileID, paneID string
		if err := depRows.Scan(&fileID, &paneID); err == nil {
			if _, exists := dependencies[fileID]; exists {
				dependencies[fileID] = append(dependencies[fileID], paneID)
				depCount++
			}
		}
	}

	s.logger.Content().Debug("Optimized file dependencies scan completed", "tenantId", tenantCtx.TenantID,
		"fileCount", fileCount, "depCount", depCount, "duration", time.Since(start))
	return dependencies, nil
}

// scanBeliefDependenciesFromCache uses cached belief registries instead of JSON parsing
func (s *OrphanAnalysisService) scanBeliefDependenciesFromCache(tenantCtx *tenant.Context, cacheManager interfaces.Cache) (map[string][]string, error) {
	start := time.Now()
	s.logger.Content().Debug("Starting belief dependency scan from cache", "tenantId", tenantCtx.TenantID)

	db := tenantCtx.Database.Conn
	dependencies := make(map[string][]string)

	// Get all belief IDs first
	rows, err := db.Query("SELECT id FROM beliefs")
	if err != nil {
		s.logger.Content().Error("Belief IDs query failed", "error", err.Error(), "tenantId", tenantCtx.TenantID)
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Content().Debug("Failed to close belief rows", "error", err.Error(), "tenantId", tenantCtx.TenantID)
		}
	}()

	beliefCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			beliefCount++
		}
	}
	s.logger.Content().Debug("Belief IDs loaded", "beliefCount", beliefCount, "tenantId", tenantCtx.TenantID)

	// Get all storyfragment belief registry IDs
	registryIDs := cacheManager.GetAllStoryfragmentBeliefRegistryIDs(tenantCtx.TenantID)
	s.logger.Content().Debug("Retrieved belief registry IDs", "registryCount", len(registryIDs), "tenantId", tenantCtx.TenantID)

	// Build belief slug to ID mapping
	beliefSlugToID := make(map[string]string)
	slugRows, err := db.Query("SELECT id, slug FROM beliefs")
	if err != nil {
		s.logger.Content().Error("Belief slug query failed", "error", err.Error(), "tenantId", tenantCtx.TenantID)
		return dependencies, nil // Continue with what we have
	}
	defer func() {
		if err := slugRows.Close(); err != nil {
			s.logger.Content().Debug("Failed to close slugRows", "error", err.Error(), "tenantId", tenantCtx.TenantID)
		}
	}()

	for slugRows.Next() {
		var id, slug string
		if err := slugRows.Scan(&id, &slug); err == nil {
			beliefSlugToID[slug] = id
		}
	}
	s.logger.Content().Debug("Belief slug mapping created", "mappingCount", len(beliefSlugToID), "tenantId", tenantCtx.TenantID)

	depCount := 0
	// Process each belief registry
	for _, sfID := range registryIDs {
		registry, found := cacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, sfID)
		if !found || registry == nil {
			continue
		}

		// Process pane-level belief requirements
		for paneID, paneBeliefData := range registry.PaneBeliefPayloads {
			// Process held beliefs
			for beliefSlug := range paneBeliefData.HeldBeliefs {
				if beliefID, exists := beliefSlugToID[beliefSlug]; exists {
					if _, ok := dependencies[beliefID]; ok {
						dependencies[beliefID] = append(dependencies[beliefID], paneID)
						depCount++
					}
				}
			}
			// Process withheld beliefs
			for beliefSlug := range paneBeliefData.WithheldBeliefs {
				if beliefID, exists := beliefSlugToID[beliefSlug]; exists {
					if _, ok := dependencies[beliefID]; ok {
						dependencies[beliefID] = append(dependencies[beliefID], paneID)
						depCount++
					}
				}
			}
		}

		// Process widget-level belief requirements
		for paneID, beliefSlugs := range registry.PaneWidgetBeliefs {
			for _, beliefSlug := range beliefSlugs {
				if beliefID, exists := beliefSlugToID[beliefSlug]; exists {
					if _, ok := dependencies[beliefID]; ok {
						// Check for duplicates using slices.Contains
						if !slices.Contains(dependencies[beliefID], paneID) {
							dependencies[beliefID] = append(dependencies[beliefID], paneID)
							depCount++
						}
					}
				}
			}
		}
	}

	s.logger.Content().Debug("Belief dependencies scan from cache completed", "tenantId", tenantCtx.TenantID,
		"beliefCount", beliefCount, "depCount", depCount, "registryCount", len(registryIDs), "duration", time.Since(start))
	return dependencies, nil
}

// generateETag creates a unique ETag from a given string
func (s *OrphanAnalysisService) generateETag(data string) string {
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("\"%x\"", hash)
}
