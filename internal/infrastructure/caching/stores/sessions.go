// Package stores provides concrete cache store implementations
package stores

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
)

// SessionsStore implements user state caching operations with tenant isolation
type SessionsStore struct {
	tenantCaches map[string]*types.TenantUserStateCache
	mu           sync.RWMutex
	logger       *logging.ChanneledLogger
}

// NewSessionsStore creates a new sessions cache store
func NewSessionsStore(logger *logging.ChanneledLogger) *SessionsStore {
	if logger != nil {
		logger.Cache().Info("Initializing sessions cache store")
	}
	return &SessionsStore{
		tenantCaches: make(map[string]*types.TenantUserStateCache),
		logger:       logger,
	}
}

// InitializeTenant creates cache structures for a tenant
func (ss *SessionsStore) InitializeTenant(tenantID string) {
	start := time.Now()
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.logger != nil {
		ss.logger.Cache().Debug("Initializing tenant user state cache", "tenantId", tenantID)
	}

	if ss.tenantCaches[tenantID] == nil {
		ss.tenantCaches[tenantID] = &types.TenantUserStateCache{
			FingerprintStates:             make(map[string]*types.FingerprintState),
			VisitStates:                   make(map[string]*types.VisitState),
			KnownFingerprints:             make(map[string]bool),
			SessionStates:                 make(map[string]*types.SessionData),
			StoryfragmentBeliefRegistries: make(map[string]*types.StoryfragmentBeliefRegistry),
			SessionBeliefContexts:         make(map[string]*types.SessionBeliefContext),
			LastLoaded:                    time.Now().UTC(),
		}

		if ss.logger != nil {
			ss.logger.Cache().Info("Tenant user state cache initialized", "tenantId", tenantID, "duration", time.Since(start))
		}
	}
}

// GetTenantCache safely retrieves a tenant's user state cache
func (ss *SessionsStore) GetTenantCache(tenantID string) (*types.TenantUserStateCache, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	cache, exists := ss.tenantCaches[tenantID]
	return cache, exists
}

// GetAllStoryfragmentBeliefRegistryIDs returns all storyfragment IDs that have cached belief registries.
func (ss *SessionsStore) GetAllStoryfragmentBeliefRegistryIDs(tenantID string) []string {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get_all_belief_registry_ids", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return []string{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.StoryfragmentBeliefRegistries == nil {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get_all_belief_registry_ids", "tenantId", tenantID, "hit", false, "reason", "nil", "duration", time.Since(start))
		}
		return []string{}
	}

	ids := make([]string, 0, len(cache.StoryfragmentBeliefRegistries))
	for id := range cache.StoryfragmentBeliefRegistries {
		ids = append(ids, id)
	}

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "get_all_belief_registry_ids", "tenantId", tenantID, "hit", true, "count", len(ids), "duration", time.Since(start))
	}

	return ids
}

// =============================================================================
// Visit State Operations
// =============================================================================

// GetVisitState retrieves a visit state by visit ID
func (ss *SessionsStore) GetVisitState(tenantID, visitID string) (*types.VisitState, bool) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "visit_state", "tenantId", tenantID, "visitId", visitID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	// Check cache expiration (24 hours TTL for visit states)
	if time.Since(cache.LastLoaded) > 24*time.Hour {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "visit_state", "tenantId", tenantID, "visitId", visitID, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	state, found := cache.VisitStates[visitID]
	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "visit_state", "tenantId", tenantID, "visitId", visitID, "hit", found, "duration", time.Since(start))
	}
	return state, found
}

// SetVisitState stores a visit state
func (ss *SessionsStore) SetVisitState(tenantID string, state *types.VisitState) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.VisitStates[state.VisitID] = state
	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "set", "type", "visit_state", "tenantId", tenantID, "visitId", state.VisitID, "duration", time.Since(start))
	}
}

// =============================================================================
// Fingerprint State Operations
// =============================================================================

// GetFingerprintState retrieves a fingerprint state by fingerprint ID
func (ss *SessionsStore) GetFingerprintState(tenantID, fingerprintID string) (*types.FingerprintState, bool) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "fingerprint_state", "tenantId", tenantID, "fingerprintId", fingerprintID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "fingerprint_state", "tenantId", tenantID, "fingerprintId", fingerprintID, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	state, found := cache.FingerprintStates[fingerprintID]
	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "fingerprint_state", "tenantId", tenantID, "fingerprintId", fingerprintID, "hit", found, "duration", time.Since(start))
	}
	return state, found
}

// SetFingerprintState stores a fingerprint state
func (ss *SessionsStore) SetFingerprintState(tenantID string, state *types.FingerprintState) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.FingerprintStates[state.FingerprintID] = state
	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "set", "type", "fingerprint_state", "tenantId", tenantID, "fingerprintId", state.FingerprintID, "duration", time.Since(start))
	}
}

// IsKnownFingerprint checks if a fingerprint is known
func (ss *SessionsStore) IsKnownFingerprint(tenantID, fingerprintID string) bool {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "is_known", "type", "fingerprint", "tenantId", tenantID, "fingerprintId", fingerprintID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "is_known", "type", "fingerprint", "tenantId", tenantID, "fingerprintId", fingerprintID, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return false
	}

	known, exists := cache.KnownFingerprints[fingerprintID]
	result := exists && known
	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "is_known", "type", "fingerprint", "tenantId", tenantID, "fingerprintId", fingerprintID, "hit", exists, "known", result, "duration", time.Since(start))
	}
	return result
}

// SetKnownFingerprint marks a fingerprint as known or unknown
func (ss *SessionsStore) SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.KnownFingerprints[fingerprintID] = isKnown
	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "set_known", "type", "fingerprint", "tenantId", tenantID, "fingerprintId", fingerprintID, "isKnown", isKnown, "duration", time.Since(start))
	}
}

// LoadKnownFingerprints bulk loads known fingerprints
func (ss *SessionsStore) LoadKnownFingerprints(tenantID string, fingerprints map[string]bool) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	// Merge with existing known fingerprints
	for fpID, isKnown := range fingerprints {
		cache.KnownFingerprints[fpID] = isKnown
	}
	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "bulk_load", "type", "fingerprints", "tenantId", tenantID, "count", len(fingerprints), "duration", time.Since(start))
	}
}

// =============================================================================
// Session Operations
// =============================================================================

// GetSession retrieves session data by session ID
func (ss *SessionsStore) GetSession(tenantID, sessionID string) (*types.SessionData, bool) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "session", "tenantId", tenantID, "sessionId", sessionID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "session", "tenantId", tenantID, "sessionId", sessionID, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	session, found := cache.SessionStates[sessionID]
	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "session", "tenantId", tenantID, "sessionId", sessionID, "hit", found, "duration", time.Since(start))
	}
	return session, found
}

// SetSession stores session data
func (ss *SessionsStore) SetSession(tenantID string, sessionData *types.SessionData) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.SessionStates[sessionData.SessionID] = sessionData
	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "set", "type", "session", "tenantId", tenantID, "sessionId", sessionData.SessionID, "duration", time.Since(start))
	}
}

// =============================================================================
// Belief Registry Operations
// =============================================================================

// GetStoryfragmentBeliefRegistry retrieves belief registry for a storyfragment
func (ss *SessionsStore) GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*types.StoryfragmentBeliefRegistry, bool) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "belief_registry", "tenantId", tenantID, "storyfragmentId", storyfragmentID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "belief_registry", "tenantId", tenantID, "storyfragmentId", storyfragmentID, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	registry, found := cache.StoryfragmentBeliefRegistries[storyfragmentID]
	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "belief_registry", "tenantId", tenantID, "storyfragmentId", storyfragmentID, "hit", found, "duration", time.Since(start))
	}
	return registry, found
}

// SetStoryfragmentBeliefRegistry stores belief registry for a storyfragment
func (ss *SessionsStore) SetStoryfragmentBeliefRegistry(tenantID string, registry *types.StoryfragmentBeliefRegistry) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.StoryfragmentBeliefRegistries[registry.StoryfragmentID] = registry
	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "set", "type", "belief_registry", "tenantId", tenantID, "storyfragmentId", registry.StoryfragmentID, "duration", time.Since(start))
	}
}

// InvalidateStoryfragmentBeliefRegistry removes belief registry for a storyfragment
func (ss *SessionsStore) InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "invalidate", "type", "belief_registry", "tenantId", tenantID, "storyfragmentId", storyfragmentID, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	delete(cache.StoryfragmentBeliefRegistries, storyfragmentID)
	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "invalidate", "type", "belief_registry", "tenantId", tenantID, "storyfragmentId", storyfragmentID, "duration", time.Since(start))
	}
}

// =============================================================================
// Session Belief Context Operations
// =============================================================================

// GetSessionBeliefContext retrieves session belief context
func (ss *SessionsStore) GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*types.SessionBeliefContext, bool) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "session_belief_context", "tenantId", tenantID, "sessionId", sessionID, "storyfragmentId", storyfragmentID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "session_belief_context", "tenantId", tenantID, "sessionId", sessionID, "storyfragmentId", storyfragmentID, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	contextKey := sessionID + ":" + storyfragmentID
	context, found := cache.SessionBeliefContexts[contextKey]
	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "get", "type", "session_belief_context", "tenantId", tenantID, "sessionId", sessionID, "storyfragmentId", storyfragmentID, "hit", found, "duration", time.Since(start))
	}
	return context, found
}

// SetSessionBeliefContext stores session belief context
func (ss *SessionsStore) SetSessionBeliefContext(tenantID string, context *types.SessionBeliefContext) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	contextKey := context.SessionID + ":" + context.StoryfragmentID
	cache.SessionBeliefContexts[contextKey] = context
	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "set", "type", "session_belief_context", "tenantId", tenantID, "sessionId", context.SessionID, "storyfragmentId", context.StoryfragmentID, "duration", time.Since(start))
	}
}

// InvalidateSessionBeliefContext removes session belief context
func (ss *SessionsStore) InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "invalidate", "type", "session_belief_context", "tenantId", tenantID, "sessionId", sessionID, "storyfragmentId", storyfragmentID, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	// 1. Create the correct key.
	contextKey := sessionID + ":" + storyfragmentID

	// 2. Delete from the correct map: SessionBeliefContexts.
	delete(cache.SessionBeliefContexts, contextKey)

	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Warn("Cache operation", "operation", "invalidate", "type", "session_belief_context", "tenantId", tenantID, "sessionId", sessionID, "storyfragmentId", storyfragmentID, "duration", time.Since(start))
	}
}

// =============================================================================
// Cache Management Operations
// =============================================================================

// InvalidateUserStateCache clears all user state cache for a tenant
func (ss *SessionsStore) InvalidateUserStateCache(tenantID string) {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "invalidate_all", "type", "user_state", "tenantId", tenantID, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return
	}

	if ss.logger != nil {
		ss.logger.Cache().Debug("Invalidating all user state cache", "tenantId", tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	// Clear all user state caches
	cache.FingerprintStates = make(map[string]*types.FingerprintState)
	cache.VisitStates = make(map[string]*types.VisitState)
	cache.KnownFingerprints = make(map[string]bool)
	cache.SessionStates = make(map[string]*types.SessionData)
	cache.SessionBeliefContexts = make(map[string]*types.SessionBeliefContext)
	cache.StoryfragmentBeliefRegistries = make(map[string]*types.StoryfragmentBeliefRegistry)

	cache.LastLoaded = time.Now().UTC()

	if ss.logger != nil {
		ss.logger.Cache().Info("All user state cache invalidated", "tenantId", tenantID, "duration", time.Since(start))
	}
}

// GetUserStateSummary returns cache status summary for debugging
func (ss *SessionsStore) GetUserStateSummary(tenantID string) map[string]any {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get_summary", "type", "user_state", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return map[string]any{
			"exists": false,
		}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	summary := map[string]any{
		"exists":                        true,
		"fingerprintStates":             len(cache.FingerprintStates),
		"visitStates":                   len(cache.VisitStates),
		"knownFingerprints":             len(cache.KnownFingerprints),
		"sessionStates":                 len(cache.SessionStates),
		"sessionBeliefContexts":         len(cache.SessionBeliefContexts),
		"storyfragmentBeliefRegistries": len(cache.StoryfragmentBeliefRegistries),
		"lastLoaded":                    cache.LastLoaded,
	}

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "get_summary", "type", "user_state", "tenantId", tenantID, "hit", true, "fingerprintStates", len(cache.FingerprintStates), "visitStates", len(cache.VisitStates), "sessionStates", len(cache.SessionStates), "duration", time.Since(start))
	}

	return summary
}

// GetSessionsByFingerprint returns all session IDs for a given fingerprint
func (ss *SessionsStore) GetSessionsByFingerprint(tenantID, fingerprintID string) []string {
	start := time.Now()
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get_sessions_by_fingerprint", "type", "session", "tenantId", tenantID, "fingerprintId", fingerprintID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return []string{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		if ss.logger != nil {
			ss.logger.Cache().Debug("Cache operation", "operation", "get_sessions_by_fingerprint", "type", "session", "tenantId", tenantID, "fingerprintId", fingerprintID, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return []string{}
	}

	var sessionIDs []string
	for sessionID, sessionData := range cache.SessionStates {
		if sessionData.FingerprintID == fingerprintID {
			sessionIDs = append(sessionIDs, sessionID)
		}
	}
	if ss.logger != nil {
		ss.logger.Cache().Warn("DIAGNOSIS: GetSessionsByFingerprint results",
			"fingerprintId", fingerprintID,
			"foundSessionCount", len(sessionIDs),
			"foundSessionIds", sessionIDs,
			"totalCachedSessions", len(cache.SessionStates))
	}

	if ss.logger != nil {
		ss.logger.Cache().Debug("Cache operation", "operation", "get_sessions_by_fingerprint", "type", "session", "tenantId", tenantID, "fingerprintId", fingerprintID, "hit", true, "sessionCount", len(sessionIDs), "duration", time.Since(start))
	}

	return sessionIDs
}
