// Package cleanup provides background worker
package cleanup

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// Worker handles background cache cleanup operations
type Worker struct {
	cache    interfaces.Cache
	detector *tenant.Detector
	config   *Config
}

// NewWorker creates a new cleanup worker with injected configuration
func NewWorker(cache interfaces.Cache, detector *tenant.Detector, config *Config) *Worker {
	return &Worker{
		cache:    cache,
		detector: detector,
		config:   config,
	}
}

// Start begins the cleanup worker routine, using the configured interval
func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.config.CleanupInterval)
	defer ticker.Stop()

	log.Printf("Cache cleanup worker started (interval: %v, verbose: %v)",
		w.config.CleanupInterval, w.config.VerboseReporting)

	for {
		select {
		case <-ctx.Done():
			log.Println("Cache cleanup worker stopping...")
			return
		case <-ticker.C:
			w.performCleanup(ctx)
		}
	}
}

// performCleanup executes cleanup for all active tenants
func (w *Worker) performCleanup(ctx context.Context) {
	start := time.Now()
	reporter := NewReporter(w.cache)

	tenants, err := w.getActiveTenants()
	if err != nil {
		reporter.LogError("Cache cleanup failed to get active tenants", err)
		return
	}

	if w.config.VerboseReporting {
		reporter.LogStage("PERIODIC CACHE CLEANUP")

		// ALWAYS show detailed cache reports for all tenants
		for _, tenantID := range tenants {
			fmt.Print(reporter.GenerateTenantReport(tenantID))
		}
	}

	var totalCleaned int
	for _, tenantID := range tenants {
		select {
		case <-ctx.Done():
			return
		default:
			cleaned := w.cleanupTenant(tenantID)
			totalCleaned += cleaned
		}
	}

	duration := time.Since(start)
	if totalCleaned > 0 {
		reporter.LogSuccess("Cache cleanup finished: %d items cleaned from %d tenants in %v",
			totalCleaned, len(tenants), duration)
	} else if w.config.VerboseReporting {
		reporter.LogInfo("Cache cleanup completed - no expired items found (%v)", duration)
	}
}

// cleanupTenant performs TTL-based cleanup for a single tenant
func (w *Worker) cleanupTenant(tenantID string) int {
	var totalCleaned int
	now := time.Now().UTC()

	// Type assert to access the Manager's methods to get underlying stores
	manager, ok := w.cache.(*manager.Manager)
	if !ok {
		// Fallback for generic interface, though less efficient
		w.cache.PurgeExpiredBins(tenantID, "expired")
		return 1 // Conservative estimate
	}

	// 1. Content Cache Cleanup (24 hour TTL)
	contentCache, err := manager.GetTenantContentCache(tenantID)
	if err == nil && contentCache != nil {
		contentCache.Mu.Lock()
		if time.Since(contentCache.LastUpdated) > w.config.ContentCacheTTL {
			// Clear all content cache maps
			contentCache.TractStacks = make(map[string]*content.TractStackNode)
			contentCache.StoryFragments = make(map[string]*content.StoryFragmentNode)
			contentCache.Panes = make(map[string]*content.PaneNode)
			contentCache.Menus = make(map[string]*content.MenuNode)
			contentCache.Resources = make(map[string]*content.ResourceNode)
			contentCache.Epinets = make(map[string]*content.EpinetNode)
			contentCache.Beliefs = make(map[string]*content.BeliefNode)
			contentCache.Files = make(map[string]*content.ImageFileNode)
			contentCache.StoryfragmentBeliefRegistries = make(map[string]*types.StoryfragmentBeliefRegistry)
			contentCache.SlugToID = make(map[string]string)
			contentCache.CategoryToIDs = make(map[string][]string)
			contentCache.AllTractStackIDs = nil
			contentCache.AllStoryFragmentIDs = nil
			contentCache.AllPaneIDs = nil
			contentCache.AllMenuIDs = nil
			contentCache.AllResourceIDs = nil
			contentCache.AllBeliefIDs = nil
			contentCache.AllEpinetIDs = nil
			contentCache.AllFileIDs = nil
			contentCache.FullContentMap = nil
			contentCache.OrphanAnalysis = nil
			contentCache.LastUpdated = now
			totalCleaned++
		}
		contentCache.Mu.Unlock()
	}

	// 2. User State Cache Cleanup (2 hour TTL)
	userCache, err := manager.GetTenantUserStateCache(tenantID)
	if err == nil && userCache != nil {
		userCache.Mu.Lock()

		// Clean expired sessions
		for sessionID, session := range userCache.SessionStates {
			if time.Since(session.LastActivity) > w.config.SessionCacheTTL {
				delete(userCache.SessionStates, sessionID)
				totalCleaned++
			}
		}

		// Clean expired fingerprint states
		for fingerprintID, state := range userCache.FingerprintStates {
			if time.Since(state.LastActivity) > w.config.SessionCacheTTL {
				delete(userCache.FingerprintStates, fingerprintID)
				totalCleaned++
			}
		}

		// Clean expired visit states
		for visitID, state := range userCache.VisitStates {
			if time.Since(state.LastActivity) > w.config.SessionCacheTTL {
				delete(userCache.VisitStates, visitID)
				totalCleaned++
			}
		}

		// Clean expired session belief contexts
		for key, context := range userCache.SessionBeliefContexts {
			if time.Since(context.LastEvaluation) > w.config.SessionCacheTTL {
				delete(userCache.SessionBeliefContexts, key)
				totalCleaned++
			}
		}

		// Clear entire user cache if LastLoaded is too old
		if time.Since(userCache.LastLoaded) > w.config.SessionCacheTTL {
			userCache.FingerprintStates = make(map[string]*types.FingerprintState)
			userCache.VisitStates = make(map[string]*types.VisitState)
			userCache.KnownFingerprints = make(map[string]bool)
			userCache.SessionStates = make(map[string]*types.SessionData)
			userCache.SessionBeliefContexts = make(map[string]*types.SessionBeliefContext)
			userCache.LastLoaded = now
			totalCleaned += 5 // Count as 5 cleared maps
		}

		userCache.Mu.Unlock()
	}

	// 3. HTML Fragment Cache Cleanup (1 hour TTL)
	htmlCache, err := manager.GetTenantHTMLChunkCache(tenantID)
	if err == nil && htmlCache != nil {
		htmlCache.Mu.Lock()
		for key, chunk := range htmlCache.Chunks {
			if time.Since(chunk.LastUpdated) > w.config.FragmentCacheTTL {
				delete(htmlCache.Chunks, key)
				totalCleaned++
			}
		}
		htmlCache.Mu.Unlock()
	}

	// 4. Analytics Cache Cleanup (Various TTLs)
	analyticsCache, err := manager.GetTenantAnalyticsCache(tenantID)
	if err == nil && analyticsCache != nil {
		analyticsCache.Mu.Lock()

		// Clean expired epinet bins
		for binKey, bin := range analyticsCache.EpinetBins {
			var ttl time.Duration
			lastColonIndex := strings.LastIndex(binKey, ":")
			if lastColonIndex != -1 {
				hourKey := binKey[lastColonIndex+1:]
				currentHourKey := utils.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))
				if hourKey == currentHourKey {
					ttl = 15 * time.Minute // CurrentHourTTL
				} else {
					ttl = w.config.AnalyticsCacheTTL // 28 days
				}
			} else {
				ttl = w.config.AnalyticsCacheTTL
			}

			if time.Since(bin.ComputedAt) > ttl {
				delete(analyticsCache.EpinetBins, binKey)
				totalCleaned++
			}
		}

		// Clean expired content bins
		for binKey, bin := range analyticsCache.ContentBins {
			if time.Since(bin.ComputedAt) > w.config.AnalyticsCacheTTL {
				delete(analyticsCache.ContentBins, binKey)
				totalCleaned++
			}
		}

		// Clean expired site bins
		for binKey, bin := range analyticsCache.SiteBins {
			if time.Since(bin.ComputedAt) > w.config.AnalyticsCacheTTL {
				delete(analyticsCache.SiteBins, binKey)
				totalCleaned++
			}
		}

		// Clean expired lead metrics (5 minute TTL)
		if analyticsCache.LeadMetrics != nil {
			if time.Since(analyticsCache.LeadMetrics.LastComputed) > 5*time.Minute {
				analyticsCache.LeadMetrics = nil
				totalCleaned++
			}
		}

		// Clean expired dashboard data (10 minute TTL)
		if analyticsCache.DashboardData != nil {
			if time.Since(analyticsCache.DashboardData.LastComputed) > 10*time.Minute {
				analyticsCache.DashboardData = nil
				totalCleaned++
			}
		}

		analyticsCache.Mu.Unlock()
	}

	return totalCleaned
}

// getActiveTenants loads the tenant registry and returns active tenant IDs
func (w *Worker) getActiveTenants() ([]string, error) {
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return nil, err
	}

	activeTenants := make([]string, 0)
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeTenants = append(activeTenants, tenantID)
		}
	}

	return activeTenants, nil
}
