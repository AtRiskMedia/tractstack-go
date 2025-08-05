// Package services provides content map orchestration
package services

import (
	"fmt"
	"strconv"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// ContentMapService orchestrates content map building and caching
type ContentMapService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewContentMapService creates a new content map service singleton
func NewContentMapService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ContentMapService {
	return &ContentMapService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// ContentMapResponse represents the API response structure
type ContentMapResponse struct {
	Data        []*content.ContentMapItem `json:"data"`
	LastUpdated int64                     `json:"lastUpdated"`
}

// GetContentMap returns content map with timestamp-based caching
func (cms *ContentMapService) GetContentMap(tenantCtx *tenant.Context, clientLastUpdated string, cache interfaces.ContentCache) (*ContentMapResponse, bool, error) {
	marker := cms.perfTracker.StartOperation("get_content_map", tenantCtx.TenantID)
	defer marker.Complete()
	start := time.Now()

	// Check cache first
	if cachedItems, exists := cache.GetFullContentMap(tenantCtx.TenantID); exists {
		convertedItems := make([]*content.ContentMapItem, len(cachedItems))

		// Convert cached items with type-specific fields (FIXED)
		for i, item := range cachedItems {
			switch item.Type {
			case "Resource":
				convertedItems[i] = &content.ContentMapItem{
					ID:           item.ID,
					Title:        item.Title,
					Slug:         item.Slug,
					Type:         item.Type,
					CategorySlug: item.CategorySlug,
				}
			case "Menu":
				convertedItems[i] = &content.ContentMapItem{
					ID:    item.ID,
					Title: item.Title,
					Slug:  item.Slug,
					Type:  item.Type,
					Theme: item.Theme,
				}
			case "Pane":
				convertedItems[i] = &content.ContentMapItem{
					ID:        item.ID,
					Title:     item.Title,
					Slug:      item.Slug,
					Type:      item.Type,
					IsContext: item.IsContext,
				}
			case "StoryFragment":
				convertedItems[i] = &content.ContentMapItem{
					ID:              item.ID,
					Title:           item.Title,
					Slug:            item.Slug,
					Type:            item.Type,
					ParentID:        item.ParentID,
					ParentTitle:     item.ParentTitle,
					ParentSlug:      item.ParentSlug,
					Panes:           item.Panes,
					Description:     item.Description,
					Topics:          item.Topics,
					Changed:         item.Changed,
					SocialImagePath: item.SocialImagePath,
					ThumbSrc:        item.ThumbSrc,
					ThumbSrcSet:     item.ThumbSrcSet,
				}
			case "TractStack":
				convertedItems[i] = &content.ContentMapItem{
					ID:              item.ID,
					Title:           item.Title,
					Slug:            item.Slug,
					Type:            item.Type,
					SocialImagePath: item.SocialImagePath,
				}
			case "Belief":
				convertedItems[i] = &content.ContentMapItem{
					ID:    item.ID,
					Title: item.Title,
					Slug:  item.Slug,
					Type:  item.Type,
					Scale: item.Scale,
				}
			case "Epinet":
				convertedItems[i] = &content.ContentMapItem{
					ID:       item.ID,
					Title:    item.Title,
					Slug:     item.Slug,
					Type:     item.Type,
					Promoted: item.Promoted,
				}
			case "Topic":
				// Special case for Topic items (all-topics)
				convertedItems[i] = &content.ContentMapItem{
					ID:     item.ID,
					Title:  item.Title,
					Slug:   item.Slug,
					Type:   item.Type,
					Topics: item.Topics,
				}
			default:
				// Fallback for unknown types
				convertedItems[i] = &content.ContentMapItem{
					ID:    item.ID,
					Title: item.Title,
					Slug:  item.Slug,
					Type:  item.Type,
				}
			}
		}

		// Use current time as timestamp since we don't have cache metadata timestamp yet
		timestamp := time.Now().Unix()

		// Compare timestamps if client provided one
		if clientLastUpdated != "" {
			if clientTimestamp, err := strconv.ParseInt(clientLastUpdated, 10, 64); err == nil {
				if clientTimestamp == timestamp {
					// Client has current version - return not modified
					marker.SetSuccess(true)
					cms.logger.Perf().Info("Performance for GetContentMap", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)
					return nil, true, nil
				}
			}
		}
		// Return cached data
		marker.SetSuccess(true)
		cms.logger.Perf().Info("Performance for GetContentMap", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)
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

	// Convert domain entities to cache types before storing
	cacheItems := cms.convertToFullContentMapItems(contentMap)
	cache.SetFullContentMap(tenantCtx.TenantID, cacheItems)

	// Convert to response format with type-specific fields (FIXED)
	convertedItems := make([]*content.ContentMapItem, len(contentMap))
	for i, item := range contentMap {
		switch item.Type {
		case "Resource":
			convertedItems[i] = &content.ContentMapItem{
				ID:           item.ID,
				Title:        item.Title,
				Slug:         item.Slug,
				Type:         item.Type,
				CategorySlug: item.CategorySlug,
			}
		case "Menu":
			convertedItems[i] = &content.ContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
				Theme: item.Theme,
			}
		case "Pane":
			convertedItems[i] = &content.ContentMapItem{
				ID:        item.ID,
				Title:     item.Title,
				Slug:      item.Slug,
				Type:      item.Type,
				IsContext: item.IsContext,
			}
		case "StoryFragment":
			convertedItems[i] = &content.ContentMapItem{
				ID:              item.ID,
				Title:           item.Title,
				Slug:            item.Slug,
				Type:            item.Type,
				ParentID:        item.ParentID,
				ParentTitle:     item.ParentTitle,
				ParentSlug:      item.ParentSlug,
				Panes:           item.Panes,
				Description:     item.Description,
				Topics:          item.Topics,
				Changed:         item.Changed,
				SocialImagePath: item.SocialImagePath,
				ThumbSrc:        item.ThumbSrc,
				ThumbSrcSet:     item.ThumbSrcSet,
			}
		case "TractStack":
			convertedItems[i] = &content.ContentMapItem{
				ID:              item.ID,
				Title:           item.Title,
				Slug:            item.Slug,
				Type:            item.Type,
				SocialImagePath: item.SocialImagePath,
			}
		case "Belief":
			convertedItems[i] = &content.ContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
				Scale: item.Scale,
			}
		case "Epinet":
			convertedItems[i] = &content.ContentMapItem{
				ID:       item.ID,
				Title:    item.Title,
				Slug:     item.Slug,
				Type:     item.Type,
				Promoted: item.Promoted,
			}
		default:
			// Fallback for unknown types
			convertedItems[i] = &content.ContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
			}
		}
	}

	cms.logger.Content().Info("Successfully retrieved content map", "tenantId", tenantCtx.TenantID, "itemCount", len(convertedItems), "fromCache", false, "notModified", false, "duration", time.Since(start))

	marker.SetSuccess(true)
	cms.logger.Perf().Info("Performance for GetContentMap", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)
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
			ID:              item.ID,
			Title:           item.Title,
			Slug:            item.Slug,
			Type:            item.Type,
			Theme:           item.Theme,
			CategorySlug:    item.CategorySlug,
			IsContext:       item.IsContext,
			ParentID:        item.ParentID,
			ParentTitle:     item.ParentTitle,
			ParentSlug:      item.ParentSlug,
			Panes:           item.Panes,
			Description:     item.Description,
			Topics:          item.Topics,
			Changed:         item.Changed,
			SocialImagePath: item.SocialImagePath,
			ThumbSrc:        item.ThumbSrc,
			ThumbSrcSet:     item.ThumbSrcSet,
			Scale:           item.Scale,
			Promoted:        item.Promoted,
		}

		cacheItems[i] = cacheItem
	}

	return cacheItems
}

// RefreshContentMap forces a refresh of the content map cache with thundering herd protection
func (cms *ContentMapService) RefreshContentMap(tenantCtx *tenant.Context, cache interfaces.ContentCache) error {
	lockKey := fmt.Sprintf("contentmap:%s", tenantCtx.TenantID)

	// Try to acquire warming lock to prevent thundering herd
	warmingLock := caching.GetGlobalWarmingLock()
	if !warmingLock.TryLock(lockKey) {
		// Another thread is already rebuilding, just return success
		if cms.logger != nil {
			cms.logger.Content().Debug("Content map rebuild already in progress",
				"tenantId", tenantCtx.TenantID, "lockKey", lockKey)
		}
		return nil
	}
	defer warmingLock.Unlock(lockKey)

	marker := cms.perfTracker.StartOperation("refresh_content_map", tenantCtx.TenantID)
	defer marker.Complete()
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

	cms.logger.Content().Info("Successfully refreshed content map",
		"tenantId", tenantCtx.TenantID, "itemCount", len(contentMap),
		"lockKey", lockKey, "duration", time.Since(start))

	marker.SetSuccess(true)
	return nil
}
