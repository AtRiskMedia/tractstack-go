// Package fts provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package fts

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
)

// FTSService provides methods to interact with the Full-Text Search (FTS5) virtual tables.
// It is designed to be called from within existing database transactions in the repository layer.
type FTSService struct {
	logger *logging.ChanneledLogger
}

// NewFTSService creates a new FTS service.
func NewFTSService(logger *logging.ChanneledLogger) *FTSService {
	return &FTSService{
		logger: logger,
	}
}

// markdownRegex is used to strip markdown formatting for FTS indexing.
// It replaces block-level elements with spaces to preserve word boundaries.
var markdownRegex = regexp.MustCompile("(?m)`{3,}.*?`{3,}|`[^`]+`|" + // Code blocks and inline code
	"#{1,6} |" + // Headers
	"\\*\\*|__|\\*|_|~~|" + // Bold, italic, strikethrough
	"\\[[^\\]]+\\]\\([^\\)]+\\)|" + // Links
	"!\\[[^\\]]+\\]\\([^\\)]+\\)|" + // Images
	"^\\s*[-*+] |^\\s*[0-9]+\\. ") // List items

// sanitizeMarkdown strips markdown and prepares text for FTS indexing.
func (s *FTSService) sanitizeMarkdown(input string) string {
	// Replace markdown with spaces to avoid merging words
	sanitized := markdownRegex.ReplaceAllString(input, " ")
	// Normalize whitespace
	return strings.Join(strings.Fields(sanitized), " ")
}

// IndexPaneContent sanitizes and indexes the body of a markdown pane.
func (s *FTSService) IndexPaneContent(tx *sql.Tx, paneID, content string) error {
	sanitizedContent := s.sanitizeMarkdown(content)
	if sanitizedContent == "" {
		// If there's no content, ensure any old index entry is gone.
		return s.DeletePaneContent(tx, paneID)
	}

	query := `INSERT INTO pane_content_fts (rowid, pane_id, content) VALUES ((SELECT rowid FROM pane_content_fts WHERE pane_id = ?), ?, ?)`
	_, err := tx.Exec(query, paneID, paneID, sanitizedContent)
	if err != nil {
		// This is an upsert pattern for FTS5. The first failure means the row doesn't exist, so we try an insert.
		query = `INSERT INTO pane_content_fts (pane_id, content) VALUES (?, ?)`
		if _, err := tx.Exec(query, paneID, sanitizedContent); err != nil {
			s.logger.Database().Error("Failed to insert into pane_content_fts", "error", err, "paneId", paneID)
			return fmt.Errorf("failed to insert pane FTS content for pane %s: %w", paneID, err)
		}
	}
	return nil
}

// IndexStoryFragmentMetadata sanitizes and indexes the title and description of a story fragment.
func (s *FTSService) IndexStoryFragmentMetadata(tx *sql.Tx, sfID, title, description string) error {
	combinedContent := s.sanitizeMarkdown(title + " " + description)
	if combinedContent == "" {
		return s.DeleteStoryFragmentMetadata(tx, sfID)
	}

	query := `INSERT INTO storyfragment_metadata_fts (rowid, storyfragment_id, content) VALUES ((SELECT rowid FROM storyfragment_metadata_fts WHERE storyfragment_id = ?), ?, ?)`
	_, err := tx.Exec(query, sfID, sfID, combinedContent)
	if err != nil {
		query = `INSERT INTO storyfragment_metadata_fts (storyfragment_id, content) VALUES (?, ?)`
		if _, err := tx.Exec(query, sfID, combinedContent); err != nil {
			s.logger.Database().Error("Failed to insert into storyfragment_metadata_fts", "error", err, "storyFragmentId", sfID)
			return fmt.Errorf("failed to insert storyfragment FTS metadata for storyfragment %s: %w", sfID, err)
		}
	}
	return nil
}

// IndexResourceBody sanitizes and indexes the body content of a resource from its options payload.
func (s *FTSService) IndexResourceBody(tx *sql.Tx, resourceID, bodyContent string) error {
	sanitizedContent := s.sanitizeMarkdown(bodyContent)
	if sanitizedContent == "" {
		return s.deleteResourceBodyIndex(tx, resourceID)
	}

	query := `INSERT INTO resource_body_fts (rowid, resource_id, content) VALUES ((SELECT rowid FROM resource_body_fts WHERE resource_id = ?), ?, ?)`
	_, err := tx.Exec(query, resourceID, resourceID, sanitizedContent)
	if err != nil {
		query = `INSERT INTO resource_body_fts (resource_id, content) VALUES (?, ?)`
		if _, err := tx.Exec(query, resourceID, sanitizedContent); err != nil {
			s.logger.Database().Error("Failed to insert into resource_body_fts", "error", err, "resourceId", resourceID)
			return fmt.Errorf("failed to insert resource FTS body for resource %s: %w", resourceID, err)
		}
	}
	return nil
}

// DeletePaneContent removes a pane's content from the FTS index.
func (s *FTSService) DeletePaneContent(tx *sql.Tx, paneID string) error {
	query := `DELETE FROM pane_content_fts WHERE pane_id = ?`
	if _, err := tx.Exec(query, paneID); err != nil {
		s.logger.Database().Error("Failed to delete from pane_content_fts", "error", err, "paneId", paneID)
		return fmt.Errorf("failed to delete pane FTS content for pane %s: %w", paneID, err)
	}
	return nil
}

// DeleteStoryFragmentMetadata removes a story fragment's metadata from the FTS index.
func (s *FTSService) DeleteStoryFragmentMetadata(tx *sql.Tx, sfID string) error {
	query := `DELETE FROM storyfragment_metadata_fts WHERE storyfragment_id = ?`
	if _, err := tx.Exec(query, sfID); err != nil {
		s.logger.Database().Error("Failed to delete from storyfragment_metadata_fts", "error", err, "storyFragmentId", sfID)
		return fmt.Errorf("failed to delete storyfragment FTS metadata for storyfragment %s: %w", sfID, err)
	}
	return nil
}

// deleteResourceBodyIndex removes a resource's body from the FTS index.
func (s *FTSService) deleteResourceBodyIndex(tx *sql.Tx, resourceID string) error {
	query := `DELETE FROM resource_body_fts WHERE resource_id = ?`
	if _, err := tx.Exec(query, resourceID); err != nil {
		s.logger.Database().Error("Failed to delete from resource_body_fts", "error", err, "resourceId", resourceID)
		return fmt.Errorf("failed to delete resource FTS body for resource %s: %w", resourceID, err)
	}
	return nil
}
