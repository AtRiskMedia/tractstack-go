// Package bulk provides efficient content map building via single UNION query
package bulk

import (
	"database/sql"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

// ContentMapBuilder implements efficient content map construction
type ContentMapBuilder struct {
	db     *database.DB
	logger *logging.ChanneledLogger
}

// NewContentMapBuilder creates a new content map builder
func NewContentMapBuilder(db *database.DB, logger *logging.ChanneledLogger) *ContentMapBuilder {
	return &ContentMapBuilder{
		db:     db,
		logger: logger,
	}
}

// BuildContentMap executes single UNION query to build complete content map
func (cmb *ContentMapBuilder) BuildContentMap(tenantID string) ([]*content.ContentMapItem, error) {
	start := time.Now()
	cmb.logger.Database().Debug("Starting content map build", "tenantID", tenantID)

	// CORRECTED: This query is now aligned with the working implementation from api/content_handlers.go
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

	cmb.logger.Database().Debug("Executing content map UNION query")

	rows, err := cmb.db.Query(query)
	if err != nil {
		cmb.logger.Database().Error("Content map UNION query failed", "error", err.Error(), "tenantID", tenantID)
		return nil, fmt.Errorf("failed to execute content map query: %w", err)
	}
	defer rows.Close()

	var items []*content.ContentMapItem
	rowCount := 0
	for rows.Next() {
		item, err := cmb.scanContentMapRow(rows)
		if err != nil {
			cmb.logger.Database().Error("Failed to scan content map row", "error", err.Error(), "rowNumber", rowCount+1)
			return nil, fmt.Errorf("failed to scan content map row: %w", err)
		}
		items = append(items, item)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		cmb.logger.Database().Error("Content map row iteration error", "error", err.Error(), "rowsProcessed", rowCount)
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	cmb.logger.Database().Info("Content map build completed", "tenantID", tenantID, "itemCount", len(items), "duration", time.Since(start))
	return items, nil
}

// scanContentMapRow scans a single row into ContentMapItem
func (cmb *ContentMapBuilder) scanContentMapRow(rows *sql.Rows) (*content.ContentMapItem, error) {
	var item content.ContentMapItem
	var extra, parentID, parentTitle, parentSlug sql.NullString
	var changed, paneIDs, description, topics sql.NullString
	var isContext sql.NullBool
	var categorySlug, scale sql.NullString
	var promoted sql.NullBool
	var socialImagePath sql.NullString

	err := rows.Scan(
		&item.ID,
		&item.Slug,
		&item.Title,
		&item.Type,
		&extra,
		&parentID,
		&parentTitle,
		&parentSlug,
		&changed,
		&paneIDs,
		&description,
		&topics,
		&isContext,
		&categorySlug,
		&scale,
		&promoted,
		&socialImagePath,
	)
	if err != nil {
		return nil, err
	}

	// Set type-specific fields directly on domain entity (like legacy does)
	switch item.Type {
	case "Menu":
		if extra.Valid {
			item.Theme = &extra.String
		}
	case "Pane":
		if isContext.Valid {
			item.IsContext = &isContext.Bool
		}
	case "Resource":
		if categorySlug.Valid {
			item.CategorySlug = &categorySlug.String
		}
	case "Belief":
		if scale.Valid {
			item.Scale = &scale.String
		}
	case "Epinet":
		if promoted.Valid {
			item.Promoted = &promoted.Bool
		}
	case "StoryFragment":
		if parentID.Valid {
			item.ParentID = &parentID.String
		}
		if parentTitle.Valid {
			item.ParentTitle = &parentTitle.String
		}
		if parentSlug.Valid {
			item.ParentSlug = &parentSlug.String
		}
		if changed.Valid {
			item.Changed = &changed.String
		}
		if paneIDs.Valid && paneIDs.String != "" {
			item.Panes = strings.Split(paneIDs.String, ",")
		}
		if topics.Valid && topics.String != "" {
			topicsSlice := strings.Split(topics.String, ",")
			for i, topic := range topicsSlice {
				topicsSlice[i] = strings.TrimSpace(topic)
			}
			item.Topics = topicsSlice
		}
		if description.Valid {
			item.Description = &description.String
		}
		if socialImagePath.Valid {
			item.SocialImagePath = &socialImagePath.String
			// Generate thumbnail paths
			cmb.addThumbnailPaths(&item, socialImagePath.String)
		}
	case "TractStack":
		if socialImagePath.Valid {
			item.SocialImagePath = &socialImagePath.String
		}
	}

	return &item, nil
}

func (cmb *ContentMapBuilder) addThumbnailPaths(item *content.ContentMapItem, socialImagePath string) {
	if socialImagePath == "" {
		return
	}

	basename := path.Base(socialImagePath)
	if dotIndex := strings.LastIndex(basename, "."); dotIndex != -1 {
		basename = basename[:dotIndex]
	}

	cacheBuster := time.Now().Unix()

	thumbSrc := fmt.Sprintf("/images/thumbs/%s_1200px.webp?v=%d", basename, cacheBuster)
	thumbSrcSet := fmt.Sprintf(
		"/images/thumbs/%s_1200px.webp?v=%d 1200w, /images/thumbs/%s_600px.webp?v=%d 600w, /images/thumbs/%s_300px.webp?v=%d 300w",
		basename, cacheBuster, basename, cacheBuster, basename, cacheBuster,
	)

	item.ThumbSrc = &thumbSrc
	item.ThumbSrcSet = &thumbSrcSet
}

func convertToFullContentMapItems(contentMap []*content.ContentMapItem) []types.FullContentMapItem {
	cacheItems := make([]types.FullContentMapItem, len(contentMap))

	for i, item := range contentMap {
		switch item.Type {
		case "Menu":
			cacheItem := types.FullContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
			}
			if item.Theme != nil {
				cacheItem.Theme = item.Theme
			}
			cacheItems[i] = cacheItem

		case "Resource":
			cacheItem := types.FullContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
			}
			if item.CategorySlug != nil {
				cacheItem.CategorySlug = item.CategorySlug
			}
			cacheItems[i] = cacheItem

		case "Pane":
			cacheItem := types.FullContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
			}
			if item.IsContext != nil {
				cacheItem.IsContext = item.IsContext
			}
			cacheItems[i] = cacheItem

		case "StoryFragment":
			cacheItem := types.FullContentMapItem{
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
			cacheItems[i] = cacheItem

		case "TractStack":
			cacheItem := types.FullContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
			}
			if item.SocialImagePath != nil {
				cacheItem.SocialImagePath = item.SocialImagePath
			}
			cacheItems[i] = cacheItem

		case "Belief":
			cacheItem := types.FullContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
			}
			if item.Scale != nil {
				cacheItem.Scale = item.Scale
			}
			cacheItems[i] = cacheItem

		case "Epinet":
			cacheItem := types.FullContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
			}
			if item.Promoted != nil {
				cacheItem.Promoted = item.Promoted
			}
			cacheItems[i] = cacheItem

		default:
			// Fallback for unknown types
			cacheItem := types.FullContentMapItem{
				ID:    item.ID,
				Title: item.Title,
				Slug:  item.Slug,
				Type:  item.Type,
			}
			cacheItems[i] = cacheItem
		}
	}

	return cacheItems
}

// addThumbnailPaths generates thumbnail URLs for social images
func addThumbnailPaths(extra map[string]any, socialImagePath string) {
	if socialImagePath == "" {
		return
	}

	// Extract basename and generate cache buster
	basename := path.Base(socialImagePath)
	if dotIndex := strings.LastIndex(basename, "."); dotIndex != -1 {
		basename = basename[:dotIndex]
	}

	// Simple cache buster based on current time
	cacheBuster := time.Now().Unix()

	thumbSrc := fmt.Sprintf("/images/thumbs/%s_1200px.webp?v=%d", basename, cacheBuster)
	thumbSrcSet := fmt.Sprintf(
		"/images/thumbs/%s_1200px.webp?v=%d 1200w, /images/thumbs/%s_600px.webp?v=%d 600w, /images/thumbs/%s_300px.webp?v=%d 300w",
		basename, cacheBuster, basename, cacheBuster, basename, cacheBuster,
	)

	extra["thumbSrc"] = thumbSrc
	extra["thumbSrcSet"] = thumbSrcSet
}
