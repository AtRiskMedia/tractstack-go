// Package stores provides concrete cache store implementations
package stores

import (
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
)

// ContentStore implements content caching operations with tenant isolation
type ContentStore struct {
	tenantCaches map[string]*types.TenantContentCache
	mu           sync.RWMutex
}

// NewContentStore creates a new content cache store
func NewContentStore() *ContentStore {
	return &ContentStore{
		tenantCaches: make(map[string]*types.TenantContentCache),
	}
}

// InitializeTenant creates cache structures for a tenant
func (cs *ContentStore) InitializeTenant(tenantID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

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
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if len(cache.FullContentMap) == 0 {
		return nil, false
	}

	return cache.FullContentMap, true
}

// SetFullContentMap stores the full content map for a tenant
func (cs *ContentStore) SetFullContentMap(tenantID string, contentMap []types.FullContentMapItem) {
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
}

// =============================================================================
// Orphan Analysis Operations
// =============================================================================

// GetOrphanAnalysis retrieves orphan analysis data with ETag
func (cs *ContentStore) GetOrphanAnalysis(tenantID string) (*types.OrphanAnalysisPayload, string, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, "", false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if cache.OrphanAnalysis == nil {
		return nil, "", false
	}

	// Check if data is expired (24 hours TTL)
	if time.Since(cache.OrphanAnalysis.LastUpdated) > 24*time.Hour {
		return nil, "", false
	}

	return cache.OrphanAnalysis.Data, cache.OrphanAnalysis.ETag, true
}

// SetOrphanAnalysis stores orphan analysis data with ETag
func (cs *ContentStore) SetOrphanAnalysis(tenantID string, payload *types.OrphanAnalysisPayload, etag string) {
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
}

// =============================================================================
// Individual Content Operations
// =============================================================================

// GetTractStack retrieves a tractstack by ID
func (cs *ContentStore) GetTractStack(tenantID, id string) (*content.TractStackNode, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	// Check cache expiration (24 hours TTL)
	if time.Since(cache.LastUpdated) > 24*time.Hour {
		return nil, false
	}

	node, exists := cache.TractStacks[id]
	return node, exists
}

// SetTractStack stores a tractstack
func (cs *ContentStore) SetTractStack(tenantID string, node *content.TractStackNode) {
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
}

// GetStoryFragment retrieves a storyfragment by ID
func (cs *ContentStore) GetStoryFragment(tenantID, id string) (*content.StoryFragmentNode, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		return nil, false
	}

	node, exists := cache.StoryFragments[id]
	return node, exists
}

// SetStoryFragment stores a storyfragment
func (cs *ContentStore) SetStoryFragment(tenantID string, node *content.StoryFragmentNode) {
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
}

// GetPane retrieves a pane by ID
func (cs *ContentStore) GetPane(tenantID, id string) (*content.PaneNode, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		return nil, false
	}

	node, exists := cache.Panes[id]
	return node, exists
}

// SetPane stores a pane
func (cs *ContentStore) SetPane(tenantID string, node *content.PaneNode) {
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
}

// GetMenu retrieves a menu by ID
func (cs *ContentStore) GetMenu(tenantID, id string) (*content.MenuNode, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		return nil, false
	}

	node, exists := cache.Menus[id]
	return node, exists
}

// SetMenu stores a menu
func (cs *ContentStore) SetMenu(tenantID string, node *content.MenuNode) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Menus[node.ID] = node
	cache.LastUpdated = time.Now().UTC()
}

// GetResource retrieves a resource by ID
func (cs *ContentStore) GetResource(tenantID, id string) (*content.ResourceNode, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		return nil, false
	}

	node, exists := cache.Resources[id]
	return node, exists
}

// SetResource stores a resource
func (cs *ContentStore) SetResource(tenantID string, node *content.ResourceNode) {
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
}

// GetEpinet retrieves an epinet by ID
func (cs *ContentStore) GetEpinet(tenantID, id string) (*content.EpinetNode, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		return nil, false
	}

	node, exists := cache.Epinets[id]
	return node, exists
}

// SetEpinet stores an epinet
func (cs *ContentStore) SetEpinet(tenantID string, node *content.EpinetNode) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Epinets[node.ID] = node
	cache.LastUpdated = time.Now().UTC()
}

// GetBelief retrieves a belief by ID
func (cs *ContentStore) GetBelief(tenantID, id string) (*content.BeliefNode, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		return nil, false
	}

	node, exists := cache.Beliefs[id]
	return node, exists
}

// SetBelief stores a belief
func (cs *ContentStore) SetBelief(tenantID string, node *content.BeliefNode) {
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
}

// GetImageFile retrieves an imagefile by ID
func (cs *ContentStore) GetImageFile(tenantID, id string) (*content.ImageFileNode, bool) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return nil, false
	}

	cache.Mu.RLock()
	defer cache.Mu.RUnlock()

	if time.Since(cache.LastUpdated) > 24*time.Hour {
		return nil, false
	}

	node, exists := cache.Files[id]
	return node, exists
}

// SetImageFile stores an imagefile
func (cs *ContentStore) SetImageFile(tenantID string, node *content.ImageFileNode) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		cs.InitializeTenant(tenantID)
		cache, _ = cs.GetTenantCache(tenantID)
	}

	cache.Mu.Lock()
	defer cache.Mu.Unlock()

	cache.Files[node.ID] = node
	cache.LastUpdated = time.Now().UTC()
}

// =============================================================================
// Cache Management Operations
// =============================================================================

// InvalidateContentCache clears all content cache for a tenant
func (cs *ContentStore) InvalidateContentCache(tenantID string) {
	cache, exists := cs.GetTenantCache(tenantID)
	if !exists {
		return
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
}
