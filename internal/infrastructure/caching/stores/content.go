// Package stores provides concrete cache store implementations
package stores

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
)

// ContentStore implements content caching operations with tenant isolation
type ContentStore struct {
	tenantCaches map[string]*types.TenantContentCache
	mu           sync.RWMutex
	logger       *logging.ChanneledLogger
}

// NewContentStore creates a new content cache store
func NewContentStore(logger *logging.ChanneledLogger) *ContentStore {
	if logger != nil {
		logger.Cache().Info("Initializing content cache store")
	}
	return &ContentStore{
		tenantCaches: make(map[string]*types.TenantContentCache),
		logger:       logger,
	}
}

// InitializeTenant creates cache structures for a tenant
func (cs *ContentStore) InitializeTenant(tenantID string) {
	start := time.Now()
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Initializing tenant content cache", "tenantId", tenantID)
	}

	if cs.tenantCaches[tenantID] == nil {
		cs.tenantCaches[tenantID] = &types.TenantContentCache{
			TractStacks:                   make(map[string]*content.TractStackNode),
			StoryFragments:                make(map[string]*content.StoryFragmentNode),
			Panes:                         make(map[string]*content.PaneNode),
			Menus:                         make(map[string]*content.MenuNode),
			Resources:                     make(map[string]*content.ResourceNode),
			Epinets:                       make(map[string]*content.EpinetNode),
			Beliefs:                       make(map[string]*content.BeliefNode),
			Files:                         make(map[string]*content.ImageFileNode),
			StoryfragmentBeliefRegistries: make(map[string]*types.StoryfragmentBeliefRegistry),
			SlugToID:                      make(map[string]string),
			CategoryToIDs:                 make(map[string][]string),
			AllPaneIDs:                    make([]string, 0),
			FullContentMap:                make([]types.FullContentMapItem, 0),
			ContentMapLastUpdated:         time.Now().UTC(),
			LastUpdated:                   time.Now().UTC(),
			OrphanAnalysis:                nil,
		}

		if cs.logger != nil {
			cs.logger.Cache().Info("Tenant content cache initialized", "tenantId", tenantID, "duration", time.Since(start))
		}
	}
}

// GetTenantCache safely retrieves a tenant's content cache
func (cs *ContentStore) GetTenantCache(tenantID string) (*types.TenantContentCache, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	cache, exists := cs.tenantCaches[tenantID]
	return cache, exists
}

// GetAllTenantIDs returns all tenant IDs present in the store
func (cs *ContentStore) GetAllTenantIDs() []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	ids := make([]string, 0, len(cs.tenantCaches))
	for id := range cs.tenantCaches {
		ids = append(ids, id)
	}
	return ids
}

// =============================================================================
// Content Map Operations
// =============================================================================

// GetFullContentMap retrieves the full content map for a tenant
func (cs *ContentStore) GetFullContentMap(tenantID string) ([]types.FullContentMapItem, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "contentmap", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if len(cache.FullContentMap) == 0 {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "contentmap", "tenantId", tenantID, "hit", false, "reason", "empty", "duration", time.Since(start))
		}
		return nil, false
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "contentmap", "tenantId", tenantID, "hit", true, "items", len(cache.FullContentMap), "duration", time.Since(start))
	}

	return cache.FullContentMap, true
}

// SetFullContentMap stores the full content map for a tenant
func (cs *ContentStore) SetFullContentMap(tenantID string, contentMap []types.FullContentMapItem) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.FullContentMap = contentMap
	cache.ContentMapLastUpdated = time.Now().UTC()
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "contentmap", "tenantId", tenantID, "items", len(contentMap), "duration", time.Since(start))
	}
}

// =============================================================================
// Orphan Analysis Operations
// =============================================================================

// GetOrphanAnalysis retrieves orphan analysis data with ETag
func (cs *ContentStore) GetOrphanAnalysis(tenantID string) (*types.OrphanAnalysisPayload, string, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "orphan_analysis", "tenantId", tenantID, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, "", false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.OrphanAnalysis == nil {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "orphan_analysis", "tenantId", tenantID, "hit", false, "reason", "nil", "duration", time.Since(start))
		}
		return nil, "", false
	}

	// Check if data is expired (24 hours TTL)
	if time.Since(cache.OrphanAnalysis.LastUpdated) > 24*time.Hour {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "orphan_analysis", "tenantId", tenantID, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, "", false
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "orphan_analysis", "tenantId", tenantID, "hit", true, "duration", time.Since(start))
	}

	return cache.OrphanAnalysis.Data, cache.OrphanAnalysis.ETag, true
}

// SetOrphanAnalysis stores orphan analysis data with ETag
func (cs *ContentStore) SetOrphanAnalysis(tenantID string, payload *types.OrphanAnalysisPayload, etag string) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.OrphanAnalysis = &types.OrphanAnalysisCache{
		Data:        payload,
		ETag:        etag,
		LastUpdated: time.Now().UTC(),
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "orphan_analysis", "tenantId", tenantID, "etag", etag, "duration", time.Since(start))
	}
}

// =============================================================================
// Individual Content Operations
// =============================================================================

// GetTractStack retrieves a tractstack by ID
func (cs *ContentStore) GetTractStack(tenantID, id string) (*content.TractStackNode, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "tractstack", "tenantId", tenantID, "key", id, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	// Check cache expiration (24 hours TTL)
	if time.Since(cache.LastUpdated) > 24*time.Hour {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "tractstack", "tenantId", tenantID, "key", id, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	node, found := cache.TractStacks[id]
	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "tractstack", "tenantId", tenantID, "key", id, "hit", found, "duration", time.Since(start))
	}
	return node, found
}

// SetTractStack stores a tractstack
func (cs *ContentStore) SetTractStack(tenantID string, node *content.TractStackNode) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.TractStacks[node.ID] = node
	cache.SlugToID[node.Slug] = node.ID
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "tractstack", "tenantId", tenantID, "key", node.ID, "duration", time.Since(start))
	}
}

// GetStoryFragment retrieves a storyfragment by ID
func (cs *ContentStore) GetStoryFragment(tenantID, id string) (*content.StoryFragmentNode, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "storyfragment", "tenantId", tenantID, "key", id, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "storyfragment", "tenantId", tenantID, "key", id, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	node, found := cache.StoryFragments[id]
	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "storyfragment", "tenantId", tenantID, "key", id, "hit", found, "duration", time.Since(start))
	}
	return node, found
}

// SetStoryFragment stores a storyfragment
func (cs *ContentStore) SetStoryFragment(tenantID string, node *content.StoryFragmentNode) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.StoryFragments[node.ID] = node
	cache.SlugToID[node.Slug] = node.ID
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "storyfragment", "tenantId", tenantID, "key", node.ID, "duration", time.Since(start))
	}
}

// GetPane retrieves a pane by ID
func (cs *ContentStore) GetPane(tenantID, id string) (*content.PaneNode, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "pane", "tenantId", tenantID, "key", id, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "pane", "tenantId", tenantID, "key", id, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	node, found := cache.Panes[id]
	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "pane", "tenantId", tenantID, "key", id, "hit", found, "duration", time.Since(start))
	}
	return node, found
}

// SetPane stores a pane
func (cs *ContentStore) SetPane(tenantID string, node *content.PaneNode) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Panes[node.ID] = node
	cache.SlugToID[node.Slug] = node.ID
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "pane", "tenantId", tenantID, "key", node.ID, "duration", time.Since(start))
	}
}

// GetMenu retrieves a menu by ID
func (cs *ContentStore) GetMenu(tenantID, id string) (*content.MenuNode, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "menu", "tenantId", tenantID, "key", id, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "menu", "tenantId", tenantID, "key", id, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	node, found := cache.Menus[id]
	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "menu", "tenantId", tenantID, "key", id, "hit", found, "duration", time.Since(start))
	}
	return node, found
}

// SetMenu stores a menu
func (cs *ContentStore) SetMenu(tenantID string, node *content.MenuNode) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Menus[node.ID] = node
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "menu", "tenantId", tenantID, "key", node.ID, "duration", time.Since(start))
	}
}

// GetResource retrieves a resource by ID
func (cs *ContentStore) GetResource(tenantID, id string) (*content.ResourceNode, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "resource", "tenantId", tenantID, "key", id, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "resource", "tenantId", tenantID, "key", id, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	node, found := cache.Resources[id]
	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "resource", "tenantId", tenantID, "key", id, "hit", found, "duration", time.Since(start))
	}
	return node, found
}

// SetResource stores a resource
func (cs *ContentStore) SetResource(tenantID string, node *content.ResourceNode) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Resources[node.ID] = node
	cache.SlugToID[node.Slug] = node.ID
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "resource", "tenantId", tenantID, "key", node.ID, "duration", time.Since(start))
	}
}

// GetEpinet retrieves an epinet by ID
func (cs *ContentStore) GetEpinet(tenantID, id string) (*content.EpinetNode, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "epinet", "tenantId", tenantID, "key", id, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "epinet", "tenantId", tenantID, "key", id, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	node, found := cache.Epinets[id]
	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "epinet", "tenantId", tenantID, "key", id, "hit", found, "duration", time.Since(start))
	}
	return node, found
}

// SetEpinet stores an epinet
func (cs *ContentStore) SetEpinet(tenantID string, node *content.EpinetNode) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Epinets[node.ID] = node
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "epinet", "tenantId", tenantID, "key", node.ID, "duration", time.Since(start))
	}
}

// GetBelief retrieves a belief by ID
func (cs *ContentStore) GetBelief(tenantID, id string) (*content.BeliefNode, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "belief", "tenantId", tenantID, "key", id, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "belief", "tenantId", tenantID, "key", id, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	node, found := cache.Beliefs[id]
	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "belief", "tenantId", tenantID, "key", id, "hit", found, "duration", time.Since(start))
	}
	return node, found
}

// SetBelief stores a belief
func (cs *ContentStore) SetBelief(tenantID string, node *content.BeliefNode) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Beliefs[node.ID] = node
	cache.SlugToID[node.Slug] = node.ID
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "belief", "tenantId", tenantID, "key", node.ID, "duration", time.Since(start))
	}
}

// GetImageFile retrieves an imagefile by ID
func (cs *ContentStore) GetImageFile(tenantID, id string) (*content.ImageFileNode, bool) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "imagefile", "tenantId", tenantID, "key", id, "hit", false, "reason", "tenant_not_initialized", "duration", time.Since(start))
		}
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		if cs.logger != nil {
			cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "imagefile", "tenantId", tenantID, "key", id, "hit", false, "reason", "expired", "duration", time.Since(start))
		}
		return nil, false
	}

	node, found := cache.Files[id]
	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "get", "type", "imagefile", "tenantId", tenantID, "key", id, "hit", found, "duration", time.Since(start))
	}
	return node, found
}

// SetImageFile stores an imagefile
func (cs *ContentStore) SetImageFile(tenantID string, node *content.ImageFileNode) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Files[node.ID] = node
	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Debug("Cache operation", "operation", "set", "type", "imagefile", "tenantId", tenantID, "key", node.ID, "duration", time.Since(start))
	}
}

// =============================================================================
// Cache Management Operations
// =============================================================================

// InvalidateContentCache clears all content cache for a tenant
func (cs *ContentStore) InvalidateContentCache(tenantID string) {
	start := time.Now()
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return
	}

	if cs.logger != nil {
		cs.logger.Cache().Debug("Invalidating content cache", "tenantId", tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	// Clear all content caches
	cache.TractStacks = make(map[string]*content.TractStackNode)
	cache.StoryFragments = make(map[string]*content.StoryFragmentNode)
	cache.Panes = make(map[string]*content.PaneNode)
	cache.Menus = make(map[string]*content.MenuNode)
	cache.Resources = make(map[string]*content.ResourceNode)
	cache.Epinets = make(map[string]*content.EpinetNode)
	cache.Beliefs = make(map[string]*content.BeliefNode)
	cache.Files = make(map[string]*content.ImageFileNode)
	cache.SlugToID = make(map[string]string)
	cache.CategoryToIDs = make(map[string][]string)
	cache.AllPaneIDs = make([]string, 0)

	// Clear content map and orphan analysis
	cache.FullContentMap = make([]types.FullContentMapItem, 0)
	cache.OrphanAnalysis = nil

	cache.LastUpdated = time.Now().UTC()

	if cs.logger != nil {
		cs.logger.Cache().Info("Content cache invalidated", "tenantId", tenantID, "duration", time.Since(start))
	}
}
