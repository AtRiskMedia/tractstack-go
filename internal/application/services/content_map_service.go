// Package services provides content map orchestration
package services

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// ContentMapService orchestrates content map building and caching
type ContentMapService struct {
	logger *logging.ChanneledLogger
}

// NewContentMapService creates a new content map service singleton
func NewContentMapService(logger *logging.ChanneledLogger) *ContentMapService {
	return &ContentMapService{
		logger: logger,
	}
}

// ContentMapResponse represents the API response structure
type ContentMapResponse struct {
	Data        []*content.ContentMapItem `json:"data"`
	LastUpdated int64                     `json:"lastUpdated"`
}

// GetContentMap returns content map with timestamp-based caching
func (cms *ContentMapService) GetContentMap(tenantCtx *tenant.Context, clientLastUpdated string, cache interfaces.ContentCache) (*ContentMapResponse, bool, error) {
	start := time.Now()
	// Check cache first
	if cachedItems, exists := cache.GetFullContentMap(tenantCtx.TenantID); exists {
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

	// Cache miss - build content map from database using bulk repository
	bulkRepo := tenantCtx.BulkRepo()
	contentMap, err := bulkRepo.BuildContentMap(tenantCtx.TenantID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to build content map: %w", err)
	}

	// Current timestamp for the response
	timestamp := time.Now().Unix()

	// FIXED: Convert domain entities to cache types before storing
	cacheItems := cms.convertToFullContentMapItems(contentMap)
	cache.SetFullContentMap(tenantCtx.TenantID, cacheItems)

	// Convert to response format
	convertedItems := make([]*content.ContentMapItem, len(contentMap))
	for i, item := range contentMap {
		convertedItems[i] = &content.ContentMapItem{
			ID:    item.ID,
			Title: item.Title,
			Slug:  item.Slug,
			Type:  item.Type,
		}
	}

	cms.logger.Content().Info("Successfully retrieved content map", "tenantId", tenantCtx.TenantID, "itemCount", len(convertedItems), "fromCache", true, "notModified", false, "duration", time.Since(start))

	return &ContentMapResponse{
		Data:        convertedItems,
		LastUpdated: timestamp,
	}, false, nil
}

// convertToFullContentMapItems converts domain entities to cache types
func (cms *ContentMapService) convertToFullContentMapItems(contentMap []*content.ContentMapItem) []types.FullContentMapItem {
	cacheItems := make([]types.FullContentMapItem, len(contentMap))

	for i, item := range contentMap {
		cacheItem := types.FullContentMapItem{
			ID:    item.ID,
			Title: item.Title,
			Slug:  item.Slug,
			Type:  item.Type,
		}

		// Copy type-specific fields based on the content type
		switch item.Type {
		case "Menu":
			// Menu-specific fields would be copied here if they exist
			// cacheItem.Theme = item.Theme (if Theme field exists)

		case "Resource":
			// Resource-specific fields would be copied here if they exist
			// cacheItem.CategorySlug = item.CategorySlug (if CategorySlug field exists)

		case "Pane":
			// Pane-specific fields would be copied here if they exist
			// cacheItem.IsContext = item.IsContext (if IsContext field exists)

		case "StoryFragment":
			// StoryFragment-specific fields would be copied here if they exist
			// cacheItem.ParentID = item.ParentID (if ParentID field exists)

		case "TractStack":
			// TractStack-specific fields would be copied here if they exist
			// cacheItem.SocialImagePath = item.SocialImagePath (if SocialImagePath field exists)

		case "Belief":
			// Belief-specific fields would be copied here if they exist
			// cacheItem.Scale = item.Scale (if Scale field exists)

		case "Epinet":
			// Epinet-specific fields would be copied here if they exist
			// cacheItem.Promoted = item.Promoted (if Promoted field exists)
		}

		cacheItems[i] = cacheItem
	}

	return cacheItems
}

// RefreshContentMap forces a refresh of the content map cache
func (cms *ContentMapService) RefreshContentMap(tenantCtx *tenant.Context, cache interfaces.ContentCache) error {
	start := time.Now()
	// Invalidate existing cache
	cache.InvalidateContentCache(tenantCtx.TenantID)

	// Rebuild from database
	bulkRepo := tenantCtx.BulkRepo()
	contentMap, err := bulkRepo.BuildContentMap(tenantCtx.TenantID)
	if err != nil {
		return fmt.Errorf("failed to rebuild content map: %w", err)
	}

	// Store in cache with type conversion
	cacheItems := cms.convertToFullContentMapItems(contentMap)
	cache.SetFullContentMap(tenantCtx.TenantID, cacheItems)

	log.Printf("Refreshed content map cache for tenant %s with %d items", tenantCtx.TenantID, len(contentMap))

	cms.logger.Content().Info("Successfully refreshed content map", "tenantId", tenantCtx.TenantID, "itemCount", len(contentMap), "duration", time.Since(start))

	return nil
}
