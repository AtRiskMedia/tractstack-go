// Package api provides content map handlers
package api

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
)

// GetFullContentMapHandler returns the unified content map with timestamp-based caching
func GetFullContentMapHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cacheManager := cache.GetGlobalManager()

	// Get client's lastUpdated parameter
	clientLastUpdated := c.Query("lastUpdated")

	// Check cache first
	if cachedContentMap, found := cacheManager.GetFullContentMap(ctx.TenantID); found {
		// Get server's last updated timestamp
		cacheManager.Mu.RLock()
		tenantCache := cacheManager.ContentCache[ctx.TenantID]
		cacheManager.Mu.RUnlock()

		tenantCache.Mu.RLock()
		serverLastUpdated := tenantCache.ContentMapLastUpdated.Unix()
		tenantCache.Mu.RUnlock()

		// Compare timestamps if client provided one
		if clientLastUpdated != "" {
			if clientTimestamp, err := strconv.ParseInt(clientLastUpdated, 10, 64); err == nil {
				if clientTimestamp == serverLastUpdated {
					c.Status(http.StatusNotModified)
					return
				}
			}
		}

		// Return data with wrapper that works with TractStackAPI
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"data":        cachedContentMap,
				"lastUpdated": serverLastUpdated,
			},
		})
		return
	}

	// Cache miss - build content map from database
	contentMap, err := buildFullContentMapFromDB(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Store in cache
	cacheManager.SetFullContentMap(ctx.TenantID, contentMap)

	// Get timestamp after caching
	cacheManager.Mu.RLock()
	tenantCache := cacheManager.ContentCache[ctx.TenantID]
	cacheManager.Mu.RUnlock()

	tenantCache.Mu.RLock()
	serverLastUpdated := tenantCache.ContentMapLastUpdated.Unix()
	tenantCache.Mu.RUnlock()

	// Return data with wrapper that works with TractStackAPI
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"data":        contentMap,
			"lastUpdated": serverLastUpdated,
		},
	})
}

// buildFullContentMapFromDB builds the content map using a single efficient UNION query
func buildFullContentMapFromDB(ctx *tenant.Context) ([]models.FullContentMapItem, error) {
	// Single UNION ALL query to get all content types efficiently
	query := `
		SELECT 
			id, 
			id as slug, 
			title, 
			'Menu' as type, 
			theme as extra, 
			NULL as parent_id, 
			NULL as parent_title, 
			NULL as parent_slug, 
			NULL as changed,
			NULL as pane_ids, 
			NULL as description,
			NULL as topics,
			NULL as is_context,
			NULL as category_slug,
			NULL as scale,
			NULL as promoted,
			NULL as social_image_path
		FROM menus
		
		UNION ALL
		
		SELECT 
			id, 
			slug, 
			title, 
			'Pane' as type, 
			NULL as extra,
			NULL as parent_id, 
			NULL as parent_title, 
			NULL as parent_slug,
			NULL as changed,
			NULL as pane_ids, 
			NULL as description,
			NULL as topics,
			is_context_pane as is_context,
			NULL as category_slug,
			NULL as scale,
			NULL as promoted,
			NULL as social_image_path
		FROM panes
		
		UNION ALL
		
		SELECT 
			id, 
			slug, 
			title, 
			'Resource' as type, 
			NULL as extra,
			NULL as parent_id, 
			NULL as parent_title, 
			NULL as parent_slug,
			NULL as changed,
			NULL as pane_ids, 
			NULL as description,
			NULL as topics,
			NULL as is_context,
			category_slug,
			NULL as scale,
			NULL as promoted,
			NULL as social_image_path
		FROM resources
		
		UNION ALL
		
		SELECT 
			id, 
			id as slug, 
			title, 
			'Epinet' as type, 
			options_payload as extra,
			NULL as parent_id, 
			NULL as parent_title, 
			NULL as parent_slug,
			NULL as changed,
			NULL as pane_ids, 
			NULL as description,
			NULL as topics,
			NULL as is_context,
			NULL as category_slug,
			NULL as scale,
			NULL as promoted,
			NULL as social_image_path
		FROM epinets
		
		UNION ALL
		
		SELECT 
			sf.id, 
			sf.slug, 
			sf.title, 
			'StoryFragment' as type, 
			NULL as extra,
			ts.id as parent_id,
			ts.title as parent_title,
			ts.slug as parent_slug,
			sf.changed,
			(
				SELECT GROUP_CONCAT(pane_id)
				FROM storyfragment_panes sp
				WHERE sp.storyfragment_id = sf.id
			) as pane_ids,
			sfd.description,
			(
				SELECT GROUP_CONCAT(st.title)
				FROM storyfragment_has_topic sht
				JOIN storyfragment_topics st ON sht.topic_id = st.id
				WHERE sht.storyfragment_id = sf.id
			) as topics,
			NULL as is_context,
			NULL as category_slug,
			NULL as scale,
			NULL as promoted,
			sf.social_image_path
		FROM storyfragments sf
		JOIN tractstacks ts ON sf.tractstack_id = ts.id
		LEFT JOIN storyfragment_details sfd ON sfd.storyfragment_id = sf.id
		
		UNION ALL
		
		SELECT 
			id, 
			slug, 
			title, 
			'TractStack' as type, 
			NULL as extra,
			NULL as parent_id, 
			NULL as parent_title, 
			NULL as parent_slug,
			NULL as changed,
			NULL as pane_ids, 
			NULL as description,
			NULL as topics,
			NULL as is_context,
			NULL as category_slug,
			NULL as scale,
			NULL as promoted,
			social_image_path
		FROM tractstacks
		
		UNION ALL
		
		SELECT 
			id, 
			slug, 
			title, 
			'Belief' as type, 
			NULL as extra,
			NULL as parent_id, 
			NULL as parent_title, 
			NULL as parent_slug,
			NULL as changed,
			NULL as pane_ids, 
			NULL as description,
			NULL as topics,
			NULL as is_context,
			NULL as category_slug,
			scale,
			NULL as promoted,
			NULL as social_image_path
		FROM beliefs
		
		ORDER BY title`

	rows, err := ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query content map: %w", err)
	}
	defer rows.Close()

	var contentMap []models.FullContentMapItem

	for rows.Next() {
		var (
			id              string
			slug            string
			title           string
			contentType     string
			extra           *string
			parentID        *string
			parentTitle     *string
			parentSlug      *string
			changed         *string
			paneIDs         *string
			description     *string
			topics          *string
			isContext       *bool
			categorySlug    *string
			scale           *string
			promoted        *bool
			socialImagePath *string
		)

		err := rows.Scan(
			&id, &slug, &title, &contentType, &extra,
			&parentID, &parentTitle, &parentSlug, &changed,
			&paneIDs, &description, &topics, &isContext,
			&categorySlug, &scale, &promoted, &socialImagePath,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan content map row: %w", err)
		}

		// Build the appropriate content map item based on type
		item := buildContentMapItem(
			id, slug, title, contentType, extra,
			parentID, parentTitle, parentSlug, changed,
			paneIDs, description, topics, isContext,
			categorySlug, scale, socialImagePath,
		)

		contentMap = append(contentMap, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	// Add special "all-topics" entry
	allTopics, err := getAllTopics(ctx)
	if err == nil && len(allTopics) > 0 {
		topicStrings := make([]string, len(allTopics))
		for i, topic := range allTopics {
			topicStrings[i] = topic.Title
		}

		topicItem := models.FullContentMapItem{
			ID:     "all-topics",
			Title:  "All Topics",
			Slug:   "all-topics",
			Type:   "Topic",
			Topics: topicStrings,
		}
		contentMap = append(contentMap, topicItem)
	}

	return contentMap, nil
}

// buildContentMapItem constructs the appropriate FullContentMapItem based on content type
func buildContentMapItem(
	id, slug, title, contentType string, extra *string,
	parentID, parentTitle, parentSlug, changed *string,
	paneIDs, description, topics *string, isContext *bool,
	categorySlug, scale *string, socialImagePath *string,
) models.FullContentMapItem {
	switch contentType {
	case "Menu":
		item := models.FullContentMapItem{
			ID:    id,
			Title: title,
			Slug:  slug,
			Type:  "Menu",
		}
		if extra != nil {
			item.Theme = extra
		}
		return item

	case "Pane":
		item := models.FullContentMapItem{
			ID:    id,
			Title: title,
			Slug:  slug,
			Type:  "Pane",
		}
		if isContext != nil {
			item.IsContext = isContext
		}
		return item

	case "Resource":
		item := models.FullContentMapItem{
			ID:           id,
			Title:        title,
			Slug:         slug,
			Type:         "Resource",
			CategorySlug: categorySlug,
		}
		return item

	case "Epinet":
		item := models.FullContentMapItem{
			ID:    id,
			Title: title,
			Slug:  slug,
			Type:  "Epinet",
		}

		// Parse options_payload to extract promoted flag
		if extra != nil && *extra != "" {
			// Simple JSON parsing for promoted field
			// This could be enhanced with proper JSON parsing if needed
			if strings.Contains(*extra, `"promoted":true`) {
				promoted := true
				item.Promoted = &promoted
			}
		}
		return item

	case "StoryFragment":
		item := models.FullContentMapItem{
			ID:              id,
			Title:           title,
			Slug:            slug,
			Type:            "StoryFragment",
			ParentID:        parentID,
			ParentTitle:     parentTitle,
			ParentSlug:      parentSlug,
			SocialImagePath: socialImagePath,
			Description:     description,
		}

		// Add pane IDs
		if paneIDs != nil && *paneIDs != "" {
			item.Panes = strings.Split(*paneIDs, ",")
		}

		// Add topics
		if topics != nil && *topics != "" {
			item.Topics = strings.Split(*topics, ",")
			// Trim whitespace from topics
			for i, topic := range item.Topics {
				item.Topics[i] = strings.TrimSpace(topic)
			}
		}

		// Add changed timestamp
		if changed != nil {
			item.Changed = changed
		}

		// Generate thumbnail paths if social image exists
		if socialImagePath != nil && *socialImagePath != "" {
			cacheBuster := time.Now().Unix()
			if changed != nil {
				if parsedTime, err := time.Parse(time.RFC3339, *changed); err == nil {
					cacheBuster = parsedTime.Unix()
				}
			}

			// Extract basename from social image path
			basename := path.Base(*socialImagePath)
			if extPos := strings.LastIndex(basename, "."); extPos > 0 {
				basename = basename[:extPos]
			}
			if basename == "" {
				basename = id // fallback to ID
			}

			thumbSrc := fmt.Sprintf("/images/thumbs/%s_1200px.webp?v=%d", basename, cacheBuster)
			thumbSrcSet := fmt.Sprintf(
				"/images/thumbs/%s_1200px.webp?v=%d 1200w, /images/thumbs/%s_600px.webp?v=%d 600w, /images/thumbs/%s_300px.webp?v=%d 300w",
				basename, cacheBuster, basename, cacheBuster, basename, cacheBuster,
			)

			item.ThumbSrc = &thumbSrc
			item.ThumbSrcSet = &thumbSrcSet
		}

		return item

	case "TractStack":
		item := models.FullContentMapItem{
			ID:              id,
			Title:           title,
			Slug:            slug,
			Type:            "TractStack",
			SocialImagePath: socialImagePath,
		}
		return item

	case "Belief":
		item := models.FullContentMapItem{
			ID:    id,
			Title: title,
			Slug:  slug,
			Type:  "Belief",
			Scale: scale,
		}
		return item

	default:
		// Fallback for unknown types
		return models.FullContentMapItem{
			ID:    id,
			Title: title,
			Slug:  slug,
			Type:  contentType,
		}
	}
}

// getAllTopics fetches all topics for the special "all-topics" entry
func getAllTopics(ctx *tenant.Context) ([]models.Topic, error) {
	query := `SELECT id, title FROM storyfragment_topics ORDER BY title ASC`

	rows, err := ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []models.Topic
	for rows.Next() {
		var topic models.Topic
		if err := rows.Scan(&topic.ID, &topic.Title); err != nil {
			return nil, err
		}
		topics = append(topics, topic)
	}

	return topics, rows.Err()
}
