// Package stores provides concrete cache store implementations
package stores

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
)

// ConfigStore implements configuration caching operations with tenant isolation
type ConfigStore struct {
	tenantCaches map[string]*types.TenantConfigCache
	mu           sync.RWMutex
	logger       *logging.ChanneledLogger
}

// NewConfigStore creates a new configuration cache store
func NewConfigStore(logger *logging.ChanneledLogger) *ConfigStore {
	if logger != nil {
		logger.Cache().Info("Initializing configuration cache store")
	}
	return &ConfigStore{
		tenantCaches: make(map[string]*types.TenantConfigCache),
		logger:       logger,
	}
}

// InitializeTenant creates cache structures for a tenant
func (cs *ConfigStore) InitializeTenant(tenantID string) {
	start := time.Now()
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Initializing tenant configuration cache", "tenantId", tenantID)
	}

	if cs.tenantCaches[tenantID] == nil {
		cs.tenantCaches[tenantID] = &types.TenantConfigCache{
			BrandConfig:               nil,
			BrandConfigLastUpdated:    time.Time{},
			AdvancedConfig:            nil,
			AdvancedConfigLastUpdated: time.Time{},
			LastUpdated:               time.Now().UTC(),
		}

		if cs.logger != nil {
			cs.logger.Cache().Info("Tenant configuration cache initialized", "tenantId", tenantID, "duration", time.Since(start))
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
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "brand_config", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.BrandConfig == nil {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "brand_config", "tenantId", tenantID, "hit", false, "reason", "nil", "duration", time.Since(start))
		}
		return nil, false
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "brand_config", "tenantId", tenantID, "hit", true, "duration", time.Since(start))
	}

	// Brand config has no TTL - it's loaded once and cached until invalidated
	return cache.BrandConfig, true
}

// SetBrandConfig stores brand configuration
func (cs *ConfigStore) SetBrandConfig(tenantID string, config *types.BrandConfig) {
	start := time.Now()
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

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "brand_config", "tenantId", tenantID, "duration", time.Since(start))
	}
}

// InvalidateBrandConfig clears cached brand configuration
func (cs *ConfigStore) InvalidateBrandConfig(tenantID string) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "invalidate", "type", "brand_config", "tenantId", tenantID, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Invalidating brand configuration cache", "tenantId", tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.BrandConfig = nil
	cache.BrandConfigLastUpdated = time.Time{}
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Info("Brand configuration cache invalidated", "tenantId", tenantID, "duration", time.Since(start))
	}
}

// GetBrandConfigLastUpdated returns when brand config was last updated
func (cs *ConfigStore) GetBrandConfigLastUpdated(tenantID string) time.Time {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get_last_updated", "type", "brand_config", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return time.Time{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get_last_updated", "type", "brand_config", "tenantId", tenantID, "hit", true, "lastUpdated", cache.BrandConfigLastUpdated, "duration", time.Since(start))
	}

	return cache.BrandConfigLastUpdated
}

// =============================================================================
// Advanced Configuration Operations
// =============================================================================

// GetAdvancedConfig retrieves cached advanced configuration
func (cs *ConfigStore) GetAdvancedConfig(tenantID string) (*types.AdvancedConfig, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "advanced_config", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.AdvancedConfig == nil {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "advanced_config", "tenantId", tenantID, "hit", false, "reason", "nil", "duration", time.Since(start))
		}
		return nil, false
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "advanced_config", "tenantId", tenantID, "hit", true, "duration", time.Since(start))
	}

	// Advanced config has no TTL - it's loaded once and cached until invalidated
	return cache.AdvancedConfig, true
}

// SetAdvancedConfig stores advanced configuration
func (cs *ConfigStore) SetAdvancedConfig(tenantID string, config *types.AdvancedConfig) {
	start := time.Now()
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

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "advanced_config", "tenantId", tenantID, "duration", time.Since(start))
	}
}

// InvalidateAdvancedConfig clears cached advanced configuration
func (cs *ConfigStore) InvalidateAdvancedConfig(tenantID string) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "invalidate", "type", "advanced_config", "tenantId", tenantID, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Invalidating advanced configuration cache", "tenantId", tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.AdvancedConfig = nil
	cache.AdvancedConfigLastUpdated = time.Time{}
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Info("Advanced configuration cache invalidated", "tenantId", tenantID, "duration", time.Since(start))
	}
}

// GetAdvancedConfigLastUpdated returns when advanced config was last updated
func (cs *ConfigStore) GetAdvancedConfigLastUpdated(tenantID string) time.Time {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get_last_updated", "type", "advanced_config", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return time.Time{}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get_last_updated", "type", "advanced_config", "tenantId", tenantID, "hit", true, "lastUpdated", cache.AdvancedConfigLastUpdated, "duration", time.Since(start))
	}

	return cache.AdvancedConfigLastUpdated
}

// =============================================================================
// Cache Management Operations
// =============================================================================

// InvalidateConfigCache clears all configuration cache for a tenant
func (cs *ConfigStore) InvalidateConfigCache(tenantID string) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "invalidate_all", "type", "config", "tenantId", tenantID, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Invalidating all configuration cache", "tenantId", tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.BrandConfig = nil
	cache.BrandConfigLastUpdated = time.Time{}
	cache.AdvancedConfig = nil
	cache.AdvancedConfigLastUpdated = time.Time{}
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Info("All configuration cache invalidated", "tenantId", tenantID, "duration", time.Since(start))
	}
}

// GetConfigSummary returns cache status summary for debugging
func (cs *ConfigStore) GetConfigSummary(tenantID string) map[string]interface{} {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get_summary", "type", "config", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return map[string]interface{}{
			"exists": false,
		}
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	summary := map[string]interface{}{
		"exists":                    true,
		"hasBrandConfig":            cache.BrandConfig != nil,
		"brandConfigLastUpdated":    cache.BrandConfigLastUpdated,
		"hasAdvancedConfig":         cache.AdvancedConfig != nil,
		"advancedConfigLastUpdated": cache.AdvancedConfigLastUpdated,
		"lastUpdated":               cache.LastUpdated,
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get_summary", "type", "config", "tenantId", tenantID, "hit", true, "hasBrandConfig", cache.BrandConfig != nil, "hasAdvancedConfig", cache.AdvancedConfig != nil, "duration", time.Since(start))
	}

	return summary
}
