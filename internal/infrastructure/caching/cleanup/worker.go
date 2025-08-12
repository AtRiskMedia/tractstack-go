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
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/utilities"
)

// Worker handles background cache cleanup operations
type Worker struct {
	cache    interfaces.Cache
	detector *tenant.Detector
	config   *Config
	logger   *logging.ChanneledLogger
}

// NewWorker creates a new cleanup worker with injected configuration
func NewWorker(cache interfaces.Cache, detector *tenant.Detector, config *Config, logger *logging.ChanneledLogger) *Worker {
	return &Worker{
		cache:    cache,
		detector: detector,
		config:   config,
		logger:   logger,
	}
}

// Start begins the cleanup worker routine, using the configured interval
func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.config.CleanupInterval)
	defer ticker.Stop()

	log.Printf("Cache cleanup worker started (interval: %v, verbose: %v)",
		w.config.CleanupInterval, w.config.VerboseReporting)
	w.logger.Cache().Info("Cache cleanup worker started", "interval", w.config.CleanupInterval, "verbose", w.config.VerboseReporting)

	for {
		select {
		case <-ctx.Done():
			log.Println("Cache cleanup worker stopping...")
			w.logger.Cache().Info("Cache cleanup worker stopping...")
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
		w.logger.Cache().Error("Cache cleanup failed to get active tenants", "error", err)
		return
	}

	if w.config.VerboseReporting {
		reporter.LogStage("PERIODIC CACHE CLEANUP")
		w.logger.Cache().Info("Starting periodic cache cleanup cycle")

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
			reporter.LogInfo("Running cleanup for tenant: %s", tenantID)
			w.logger.Cache().Info("Running cleanup for tenant", "tenantId", tenantID)
			cleaned := w.cleanupTenant(tenantID)
			if cleaned > 0 {
				reporter.LogStepSuccess("Cleaned %d items for tenant %s", cleaned, tenantID)
				w.logger.Cache().Debug("Cleaned items for tenant", "count", cleaned, "tenantId", tenantID)
			}
			totalCleaned += cleaned
		}
	}

	duration := time.Since(start)
	if totalCleaned > 0 {
		reporter.LogSuccess("Cache cleanup finished: %d items cleaned from %d tenants in %v",
			totalCleaned, len(tenants), duration)
		w.logger.Cache().Info("Cache cleanup finished", "itemsCleaned", totalCleaned, "tenants", len(tenants), "duration", duration)
	} else if w.config.VerboseReporting {
		reporter.LogInfo("Cache cleanup completed - no expired items found (%v)", duration)
		w.logger.Cache().Info("Cache cleanup completed, no expired items found", "duration", duration)
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

	// 2. User State Cache Cleanup (2 hour TTL) - FIXED FOR INVERTED INDEX
	userCache, err := manager.GetTenantUserStateCache(tenantID)
	if err == nil && userCache != nil {
		userCache.Mu.Lock()

		// Clean expired sessions - PROPERLY maintaining inverted index
		var expiredSessionIDs []string
		for sessionID, session := range userCache.SessionStates {
			if time.Since(session.LastActivity) > w.config.SessionCacheTTL {
				expiredSessionIDs = append(expiredSessionIDs, sessionID)
			}
		}

		// Remove expired sessions properly (maintaining index consistency)
		for _, sessionID := range expiredSessionIDs {
			if sessionData, exists := userCache.SessionStates[sessionID]; exists {
				// Remove from inverted index FIRST
				w.removeSessionFromFingerprintIndex(userCache, sessionData.FingerprintID, sessionID)
				// Then remove from session states
				delete(userCache.SessionStates, sessionID)
				totalCleaned++

				if w.logger != nil {
					w.logger.Cache().Debug("Cleanup removed expired session", "tenantId", tenantID, "sessionId", sessionID, "fingerprintId", sessionData.FingerprintID)
				}
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
			userCache.StoryfragmentBeliefRegistries = make(map[string]*types.StoryfragmentBeliefRegistry)
			userCache.FingerprintToSessions = make(map[string][]string) // FIXED: Clear inverted index too
			userCache.LastLoaded = now
			totalCleaned += 6 // Updated count to include FingerprintToSessions

			if w.logger != nil {
				w.logger.Cache().Info("Cleanup cleared entire user cache", "tenantId", tenantID, "reason", "expired_lastLoaded")
			}
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
				currentHourKey := utilities.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))
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

// removeSessionFromFingerprintIndex removes a session from the fingerprint's session list
// MUST be called with userCache.Mu.Lock() held
func (w *Worker) removeSessionFromFingerprintIndex(cache *types.TenantUserStateCache, fingerprintID, sessionID string) {
	sessions := cache.FingerprintToSessions[fingerprintID]

	// Find and remove the session
	for i, existingSessionID := range sessions {
		if existingSessionID == sessionID {
			// Remove by swapping with last element and truncating
			sessions[i] = sessions[len(sessions)-1]
			cache.FingerprintToSessions[fingerprintID] = sessions[:len(sessions)-1]

			// If no sessions left for this fingerprint, remove the key
			if len(cache.FingerprintToSessions[fingerprintID]) == 0 {
				delete(cache.FingerprintToSessions, fingerprintID)
			}

			if w.logger != nil {
				w.logger.Cache().Debug("Cleanup removed session from fingerprint index", "fingerprintId", fingerprintID, "sessionId", sessionID, "remainingSessions", len(cache.FingerprintToSessions[fingerprintID]))
			}
			break
		}
	}
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
