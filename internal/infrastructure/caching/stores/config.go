// Package stores provides concrete cache store implementations
package stores

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// ConfigStore implements configuration caching operations with tenant isolation
type ConfigStore struct {
	tenantCaches map[string]*types.TenantConfigCache
	mu           sync.RWMutex
}

// NewConfigStore creates a new configuration cache store
func NewConfigStore() *ConfigStore {
	return &ConfigStore{
		tenantCaches: make(map[string]*types.TenantConfigCache),
	}
}

// InitializeTenant creates cache structures for a tenant
func (cs *ConfigStore) InitializeTenant(tenantID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.tenantCaches[tenantID] == nil {
		cs.tenantCaches[tenantID] = &types.TenantConfigCache{
			BrandConfig:               nil,
			BrandConfigLastUpdated:    time.Time{},
			AdvancedConfig:            nil,
			AdvancedConfigLastUpdated: time.Time{},
			LastUpdated:               time.Now().UTC(),
		}
	}
}

// GetTenantCache safely retrieves a tenant's config cache
func (cs *ConfigStore) GetTenantCache(tenantID string) (*types.TenantConfigCache, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	cache, exists := cs.tenantCaches[tenantID]
	return cache, exists
}

// =============================================================================
// Brand Configuration Operations
// =============================================================================

// GetBrandConfig retrieves cached brand configuration
func (cs *ConfigStore) GetBrandConfig(tenantID string) (*types.BrandConfig, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.BrandConfig == nil {
		return nil, false
	}

	// Brand config has no TTL - it's loaded once and cached until invalidated
	return cache.BrandConfig, true
}

// SetBrandConfig stores brand configuration
func (cs *ConfigStore) SetBrandConfig(tenantID string, config *types.BrandConfig) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.BrandConfig = config
	cache.BrandConfigLastUpdated = time.Now().UTC()
	cache.LastUpdated = time.Now().UTC()
}

// InvalidateBrandConfig clears cached brand configuration
func (cs *ConfigStore) InvalidateBrandConfig(tenantID string) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.BrandConfig = nil
	cache.BrandConfigLastUpdated = time.Time{}
	cache.LastUpdated = time.Now().UTC()
}

// GetBrandConfigLastUpdated returns when brand config was last updated
func (cs *ConfigStore) GetBrandConfigLastUpdated(tenantID string) time.Time {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return time.Time{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	return cache.BrandConfigLastUpdated
}

// =============================================================================
// Advanced Configuration Operations
// =============================================================================

// GetAdvancedConfig retrieves cached advanced configuration
func (cs *ConfigStore) GetAdvancedConfig(tenantID string) (*types.AdvancedConfig, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.AdvancedConfig == nil {
		return nil, false
	}

	// Advanced config has no TTL - it's loaded once and cached until invalidated
	return cache.AdvancedConfig, true
}

// SetAdvancedConfig stores advanced configuration
func (cs *ConfigStore) SetAdvancedConfig(tenantID string, config *types.AdvancedConfig) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.AdvancedConfig = config
	cache.AdvancedConfigLastUpdated = time.Now().UTC()
	cache.LastUpdated = time.Now().UTC()
}

// InvalidateAdvancedConfig clears cached advanced configuration
func (cs *ConfigStore) InvalidateAdvancedConfig(tenantID string) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.AdvancedConfig = nil
	cache.AdvancedConfigLastUpdated = time.Time{}
	cache.LastUpdated = time.Now().UTC()
}

// GetAdvancedConfigLastUpdated returns when advanced config was last updated
func (cs *ConfigStore) GetAdvancedConfigLastUpdated(tenantID string) time.Time {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return time.Time{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	return cache.AdvancedConfigLastUpdated
}

// =============================================================================
// Cache Management Operations
// =============================================================================

// InvalidateConfigCache clears all configuration cache for a tenant
func (cs *ConfigStore) InvalidateConfigCache(tenantID string) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.BrandConfig = nil
	cache.BrandConfigLastUpdated = time.Time{}
	cache.AdvancedConfig = nil
	cache.AdvancedConfigLastUpdated = time.Time{}
	cache.LastUpdated = time.Now().UTC()
}

// GetConfigSummary returns cache status summary for debugging
func (cs *ConfigStore) GetConfigSummary(tenantID string) map[string]interface{} {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return map[string]interface{}{
			"exists": false,
		}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	return map[string]interface{}{
		"exists":                    true,
		"hasBrandConfig":            cache.BrandConfig != nil,
		"brandConfigLastUpdated":    cache.BrandConfigLastUpdated,
		"hasAdvancedConfig":         cache.AdvancedConfig != nil,
		"advancedConfigLastUpdated": cache.AdvancedConfigLastUpdated,
		"lastUpdated":               cache.LastUpdated,
	}
}
