// Package stores provides concrete cache store implementations
package stores

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// SessionsStore implements user state caching operations with tenant isolation
type SessionsStore struct {
	tenantCaches map[string]*types.TenantUserStateCache
	mu           sync.RWMutex
}

// NewSessionsStore creates a new sessions cache store
func NewSessionsStore() *SessionsStore {
	return &SessionsStore{
		tenantCaches: make(map[string]*types.TenantUserStateCache),
	}
}

// InitializeTenant creates cache structures for a tenant
func (ss *SessionsStore) InitializeTenant(tenantID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.tenantCaches[tenantID] == nil {
		ss.tenantCaches[tenantID] = &types.TenantUserStateCache{
			FingerprintStates:     make(map[string]*types.FingerprintState),
			VisitStates:           make(map[string]*types.VisitState),
			KnownFingerprints:     make(map[string]bool),
			SessionStates:         make(map[string]*types.SessionData),
			SessionBeliefContexts: make(map[string]*types.SessionBeliefContext),
			LastLoaded:            time.Now().UTC(),
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

// =============================================================================
// Visit State Operations
// =============================================================================

// GetVisitState retrieves a visit state by visit ID
func (ss *SessionsStore) GetVisitState(tenantID, visitID string) (*types.VisitState, bool) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	// Check cache expiration (24 hours TTL for visit states)
	if time.Since(cache.LastLoaded) > 24*time.Hour {
		return nil, false
	}

	state, exists := cache.VisitStates[visitID]
	return state, exists
}

// SetVisitState stores a visit state
func (ss *SessionsStore) SetVisitState(tenantID string, state *types.VisitState) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.VisitStates[state.VisitID] = state
	cache.LastLoaded = time.Now().UTC()
}

// =============================================================================
// Fingerprint State Operations
// =============================================================================

// GetFingerprintState retrieves a fingerprint state by fingerprint ID
func (ss *SessionsStore) GetFingerprintState(tenantID, fingerprintID string) (*types.FingerprintState, bool) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		return nil, false
	}

	state, exists := cache.FingerprintStates[fingerprintID]
	return state, exists
}

// SetFingerprintState stores a fingerprint state
func (ss *SessionsStore) SetFingerprintState(tenantID string, state *types.FingerprintState) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.FingerprintStates[state.FingerprintID] = state
	cache.LastLoaded = time.Now().UTC()
}

// IsKnownFingerprint checks if a fingerprint is known
func (ss *SessionsStore) IsKnownFingerprint(tenantID, fingerprintID string) bool {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		return false
	}

	known, exists := cache.KnownFingerprints[fingerprintID]
	return exists && known
}

// SetKnownFingerprint marks a fingerprint as known or unknown
func (ss *SessionsStore) SetKnownFingerprint(tenantID, fingerprintID string, isKnown bool) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.KnownFingerprints[fingerprintID] = isKnown
	cache.LastLoaded = time.Now().UTC()
}

// LoadKnownFingerprints bulk loads known fingerprints
func (ss *SessionsStore) LoadKnownFingerprints(tenantID string, fingerprints map[string]bool) {
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
}

// =============================================================================
// Session Operations
// =============================================================================

// GetSession retrieves session data by session ID
func (ss *SessionsStore) GetSession(tenantID, sessionID string) (*types.SessionData, bool) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		return nil, false
	}

	session, exists := cache.SessionStates[sessionID]
	return session, exists
}

// SetSession stores session data
func (ss *SessionsStore) SetSession(tenantID string, sessionData *types.SessionData) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.SessionStates[sessionData.SessionID] = sessionData
	cache.LastLoaded = time.Now().UTC()
}

// =============================================================================
// Belief Registry Operations
// =============================================================================

// GetStoryfragmentBeliefRegistry retrieves belief registry for a storyfragment
func (ss *SessionsStore) GetStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) (*types.StoryfragmentBeliefRegistry, bool) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		return nil, false
	}

	registry, exists := cache.StoryfragmentBeliefRegistries[storyfragmentID]
	return registry, exists
}

// SetStoryfragmentBeliefRegistry stores belief registry for a storyfragment
func (ss *SessionsStore) SetStoryfragmentBeliefRegistry(tenantID string, registry *types.StoryfragmentBeliefRegistry) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		ss.InitializeTenant(tenantID)
		cache, _ = ss.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.StoryfragmentBeliefRegistries[registry.StoryfragmentID] = registry
	cache.LastLoaded = time.Now().UTC()
}

// InvalidateStoryfragmentBeliefRegistry removes belief registry for a storyfragment
func (ss *SessionsStore) InvalidateStoryfragmentBeliefRegistry(tenantID, storyfragmentID string) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	delete(cache.StoryfragmentBeliefRegistries, storyfragmentID)
	cache.LastLoaded = time.Now().UTC()
}

// =============================================================================
// Session Belief Context Operations
// =============================================================================

// GetSessionBeliefContext retrieves session belief context
func (ss *SessionsStore) GetSessionBeliefContext(tenantID, sessionID, storyfragmentID string) (*types.SessionBeliefContext, bool) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastLoaded) > 24*time.Hour {
		return nil, false
	}

	contextKey := sessionID + ":" + storyfragmentID
	context, exists := cache.SessionBeliefContexts[contextKey]
	return context, exists
}

// SetSessionBeliefContext stores session belief context
func (ss *SessionsStore) SetSessionBeliefContext(tenantID string, context *types.SessionBeliefContext) {
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
}

// InvalidateSessionBeliefContext removes session belief context
func (ss *SessionsStore) InvalidateSessionBeliefContext(tenantID, sessionID, storyfragmentID string) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	contextKey := sessionID + ":" + storyfragmentID
	delete(cache.SessionBeliefContexts, contextKey)
	cache.LastLoaded = time.Now().UTC()
}

// =============================================================================
// Cache Management Operations
// =============================================================================

// InvalidateUserStateCache clears all user state cache for a tenant
func (ss *SessionsStore) InvalidateUserStateCache(tenantID string) {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return
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
}

// GetUserStateSummary returns cache status summary for debugging
func (ss *SessionsStore) GetUserStateSummary(tenantID string) map[string]interface{} {
	cache, exists := ss.GetTenantCache(tenantID)
	if !exists {
		return map[string]interface{}{
			"exists": false,
		}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	return map[string]interface{}{
		"exists":                        true,
		"fingerprintStates":             len(cache.FingerprintStates),
		"visitStates":                   len(cache.VisitStates),
		"knownFingerprints":             len(cache.KnownFingerprints),
		"sessionStates":                 len(cache.SessionStates),
		"sessionBeliefContexts":         len(cache.SessionBeliefContexts),
		"storyfragmentBeliefRegistries": len(cache.StoryfragmentBeliefRegistries),
		"lastLoaded":                    cache.LastLoaded,
	}
}
