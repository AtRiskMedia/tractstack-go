// Package bulk provides efficient content map building via single UNION query
package bulk

import (
	"database/sql"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
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

	// Set type-specific fields
	switch item.Type {
	case "Menu":
		if extra.Valid {
			item.Extra = map[string]any{"theme": extra.String}
		}
	case "Pane":
		if isContext.Valid {
			item.Extra = map[string]any{"isContext": isContext.Bool}
		}
	case "Resource":
		if categorySlug.Valid {
			item.Extra = map[string]any{"categorySlug": categorySlug.String}
		}
	case "Belief":
		if scale.Valid {
			item.Extra = map[string]any{"scale": scale.String}
		}
	case "Epinet":
		if promoted.Valid {
			item.Extra = map[string]any{"promoted": promoted.Bool}
		}
	case "StoryFragment":
		extra := make(map[string]any)
		if parentID.Valid {
			extra["parentId"] = parentID.String
		}
		if parentTitle.Valid {
			extra["parentTitle"] = parentTitle.String
		}
		if parentSlug.Valid {
			extra["parentSlug"] = parentSlug.String
		}
		if changed.Valid {
			extra["changed"] = changed.String
		}
		if paneIDs.Valid {
			// Parse pane_ids from GROUP_CONCAT (comma-separated string)
			if paneIDs.String != "" {
				extra["panes"] = strings.Split(paneIDs.String, ",")
			} else {
				extra["panes"] = []string{}
			}
		}
		if socialImagePath.Valid {
			extra["socialImagePath"] = socialImagePath.String
			// Add thumbnail paths if social image exists
			cmb.addThumbnailPaths(extra, socialImagePath.String)
		}
		item.Extra = extra
	case "TractStack":
		if socialImagePath.Valid {
			item.Extra = map[string]any{"socialImagePath": socialImagePath.String}
		}
	}

	return &item, nil
}

// addThumbnailPaths generates thumbnail URLs for social images
func (cmb *ContentMapBuilder) addThumbnailPaths(extra map[string]any, socialImagePath string) {
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
