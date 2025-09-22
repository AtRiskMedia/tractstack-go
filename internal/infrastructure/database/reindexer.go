// Package database provides database utilities, including re-indexing.
package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/fts"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// ReindexFTSTables clears and re-populates all FTS indexes from the source tables.
func ReindexFTSTables(db *sql.DB, ftsService *fts.FTSService, logger *logging.ChanneledLogger) error {
	logger.Database().Info("Starting FTS re-population...")

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin FTS repopulation transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			logger.Database().Error("failed to rollback FTS re-population transaction", "error", err)
		}
	}()

	// Helper function for string truncation
	truncateString := func(s string, maxLen int) string {
		if len(s) <= maxLen {
			return s
		}
		return s[:maxLen]
	}

	// Re-index Panes
	logger.Database().Info("Re-indexing pane content...")
	paneRows, err := tx.Query(`SELECT p.id, m.body FROM panes p JOIN markdowns m ON p.markdown_id = m.id`)
	if err != nil {
		return fmt.Errorf("failed to query panes for re-indexing: %w", err)
	}
	defer func() {
		if err := paneRows.Close(); err != nil {
			logger.Database().Error("failed to close paneRows in FTS re-indexer", "error", err)
		}
	}()

	paneCount := 0
	for paneRows.Next() {
		var paneID, body string
		if err := paneRows.Scan(&paneID, &body); err != nil {
			return fmt.Errorf("failed to scan pane for re-indexing: %w", err)
		}
		if err := ftsService.IndexPaneContent(tx, paneID, body); err != nil {
			logger.System().Warn("FTS re-indexing failed for pane", "paneId", paneID, "error", err)
		} else {
			paneCount++
		}
	}
	logger.Database().Info("Pane content re-indexing complete", "indexedCount", paneCount)

	// Re-index StoryFragments
	logger.Database().Info("Re-indexing story fragment metadata...")
	sfRows, err := tx.Query(`SELECT sf.id, sf.title, COALESCE(sfd.description, '') FROM storyfragments sf LEFT JOIN storyfragment_details sfd ON sf.id = sfd.storyfragment_id`)
	if err != nil {
		return fmt.Errorf("failed to query storyfragments for re-indexing: %w", err)
	}
	defer func() {
		if err := sfRows.Close(); err != nil {
			logger.Database().Error("failed to close sfRows in FTS re-indexer", "error", err)
		}
	}()

	sfCount := 0
	for sfRows.Next() {
		var sfID, title, description string
		if err := sfRows.Scan(&sfID, &title, &description); err != nil {
			return fmt.Errorf("failed to scan storyfragment for re-indexing: %w", err)
		}
		if err := ftsService.IndexStoryFragmentMetadata(tx, sfID, title, description); err != nil {
			logger.System().Warn("FTS re-indexing failed for storyfragment", "sfId", sfID, "error", err)
		} else {
			sfCount++
		}
	}
	logger.Database().Info("Story fragment metadata re-indexing complete", "indexedCount", sfCount)

	// Re-index Resources
	logger.Database().Info("Re-indexing resource bodies...", "collectionRoutes", config.CollectionRoutes)

	resourceRows, err := tx.Query(`SELECT id, title, category_slug, options_payload FROM resources`)
	if err != nil {
		return fmt.Errorf("failed to query resources for re-indexing: %w", err)
	}
	defer func() {
		if err := resourceRows.Close(); err != nil {
			logger.Database().Error("failed to close resourceRows in FTS re-indexer", "error", err)
		}
	}()

	resourceCount := 0
	indexedCount := 0
	skippedCount := 0
	for resourceRows.Next() {
		resourceCount++
		var resourceID, title, optionsPayloadStr string
		var categorySlug sql.NullString

		if err := resourceRows.Scan(&resourceID, &title, &categorySlug, &optionsPayloadStr); err != nil {
			logger.System().Warn("Failed to scan resource for re-indexing", "resourceId", resourceID, "error", err)
			continue
		}

		logger.Database().Debug("Processing resource for FTS indexing", "resourceId", resourceID, "title", title, "category", categorySlug.String)

		// Only index resources in COLLECTION_ROUTES (if configured)
		shouldIndex := false
		if len(config.CollectionRoutes) == 0 {
			// If no collection routes configured, index all resources
			shouldIndex = true
			logger.Database().Debug("No COLLECTION_ROUTES configured, indexing all resources")
		} else if categorySlug.Valid {
			// Check if this resource's category is in the collection routes
			for _, route := range config.CollectionRoutes {
				if categorySlug.String == route {
					shouldIndex = true
					logger.Database().Debug("Category matches collection route, will index", "category", categorySlug.String, "route", route)
					break
				}
			}
			if !shouldIndex {
				logger.Database().Debug("Category not in COLLECTION_ROUTES, skipping", "category", categorySlug.String)
				skippedCount++
			}
		} else {
			logger.Database().Debug("No category_slug, skipping resource", "resourceId", resourceID)
			skippedCount++
		}

		if shouldIndex {
			// Parse options payload to extract body
			var optionsPayload map[string]interface{}
			if err := json.Unmarshal([]byte(optionsPayloadStr), &optionsPayload); err != nil {
				logger.System().Warn("Failed to parse resource options payload for re-indexing", "resourceId", resourceID, "error", err)
				continue
			}

			// Extract body content and index it - handle both string and array formats
			var bodyText string
			if body, ok := optionsPayload["body"].(string); ok && body != "" {
				bodyText = body
				logger.Database().Debug("Found string body for resource", "resourceId", resourceID, "bodyPreview", truncateString(bodyText, 50))
			} else if bodyArray, ok := optionsPayload["body"].([]interface{}); ok && len(bodyArray) > 0 {
				// Convert array of strings to single string
				var bodyParts []string
				for _, part := range bodyArray {
					if partStr, ok := part.(string); ok {
						bodyParts = append(bodyParts, partStr)
					}
				}
				bodyText = strings.Join(bodyParts, " ")
				logger.Database().Debug("Found array body for resource", "resourceId", resourceID, "arrayLength", len(bodyArray), "bodyPreview", truncateString(bodyText, 50))
			} else {
				logger.Database().Debug("No body field found for resource", "resourceId", resourceID)
			}

			if bodyText != "" {
				if err := ftsService.IndexResourceBody(tx, resourceID, bodyText); err != nil {
					logger.System().Warn("FTS re-indexing failed for resource", "resourceId", resourceID, "error", err)
				} else {
					indexedCount++
					logger.Database().Debug("Successfully indexed resource", "resourceId", resourceID)
				}
			}
		}
	}

	logger.Database().Info("Resource body re-indexing complete", "totalResources", resourceCount, "indexedCount", indexedCount, "skippedCount", skippedCount)

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit FTS re-population transaction: %w", err)
	}

	logger.Database().Info("FTS re-population complete", "panesIndexed", paneCount, "storyFragmentsIndexed", sfCount, "resourcesIndexed", indexedCount)
	return nil
}
