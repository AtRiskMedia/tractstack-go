// Package services provides content map orchestration
package services

import (
	"fmt"
	"strconv"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/bulk"
)

// ContentMapService orchestrates content map building and caching
type ContentMapService struct {
	bulkRepo bulk.BulkQueryRepository
}

// NewContentMapService creates a new content map service
func NewContentMapService(bulkRepo bulk.BulkQueryRepository) *ContentMapService {
	return &ContentMapService{
		bulkRepo: bulkRepo,
	}
}

// ContentMapResponse represents the API response structure
type ContentMapResponse struct {
	Data        []*content.ContentMapItem `json:"data"`
	LastUpdated int64                     `json:"lastUpdated"`
}

// GetContentMap returns content map with timestamp-based caching
func (cms *ContentMapService) GetContentMap(tenantID, clientLastUpdated string, cache interfaces.ContentCache) (*ContentMapResponse, bool, error) {
	// Check cache first
	if cachedItems, exists := cache.GetFullContentMap(tenantID); exists {
		convertedItems := make([]*content.ContentMapItem, len(cachedItems))
		for i, item := range cachedItems {
			convertedItems[i] = &content.ContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
			}
		}

		// Use current time as timestamp since we don't have cache metadata timestamp yet
		timestamp := time.Now().Unix()

		// Compare timestamps if client provided one
		if clientLastUpdated != "" {
			if clientTimestamp, err := strconv.ParseInt(clientLastUpdated, 10, 64); err == nil {
				if clientTimestamp == timestamp {
					// Client has current version - return not modified
					return nil, true, nil
				}
			}
		}
		// Return cached data
		return &ContentMapResponse{
			Data:        convertedItems,
			LastUpdated: timestamp,
		}, false, nil
	}

	// Cache miss - build content map from database
	contentMap, err := cms.bulkRepo.BuildContentMap(tenantID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to build content map: %w", err)
	}

	// Current timestamp for the response
	timestamp := time.Now().Unix()

	// Store in cache - convert content.ContentMapItem to types.FullContentMapItem
	convertedItems := make([]types.FullContentMapItem, len(contentMap))
	for i, item := range contentMap {
		convertedItems[i] = types.FullContentMapItem{
			ID:    item.ID,
			Title: item.Title,
			Slug:  item.Slug,
			Type:  item.Type,
		}
	}
	cache.SetFullContentMap(tenantID, convertedItems)

	return &ContentMapResponse{
		Data:        contentMap,
		LastUpdated: timestamp,
	}, false, nil
}

// WarmContentMap builds and caches content map during tenant activation
func (cms *ContentMapService) WarmContentMap(tenantID string, cache interfaces.ContentCache) error {
	contentMap, err := cms.bulkRepo.BuildContentMap(tenantID)
	if err != nil {
		return fmt.Errorf("failed to warm content map for tenant %s: %w", tenantID, err)
	}

	// Convert content.ContentMapItem to types.FullContentMapItem for cache storage
	convertedItems := make([]types.FullContentMapItem, len(contentMap))
	for i, item := range contentMap {
		convertedItems[i] = types.FullContentMapItem{
			ID:    item.ID,
			Title: item.Title,
			Slug:  item.Slug,
			Type:  item.Type,
		}
	}
	cache.SetFullContentMap(tenantID, convertedItems)

	return nil
}
