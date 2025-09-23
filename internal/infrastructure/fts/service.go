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

// Stop words to filter out during indexing - common words that don't add search value
var stopWords = map[string]bool{
	// Articles, prepositions, conjunctions
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "has": true, "he": true,
	"in": true, "is": true, "it": true, "its": true, "of": true, "on": true,
	"that": true, "the": true, "to": true, "was": true, "were": true, "will": true,
	"with": true, "but": true, "if": true, "or": true, "because": true, "until": true,
	"while": true, "about": true, "against": true, "between": true, "into": true,
	"through": true, "during": true, "before": true, "after": true, "above": true,
	"below": true, "up": true, "down": true, "out": true, "off": true, "over": true,
	"under": true, "again": true, "further": true, "then": true, "once": true,

	// Pronouns
	"i": true, "me": true, "my": true, "myself": true, "we": true, "our": true,
	"ours": true, "ourselves": true, "you": true, "your": true, "yours": true,
	"yourself": true, "yourselves": true, "him": true, "his": true, "himself": true,
	"she": true, "her": true, "hers": true, "herself": true, "itself": true,
	"they": true, "them": true, "their": true, "theirs": true, "themselves": true,

	// Question words
	"what": true, "which": true, "who": true, "whom": true, "where": true,
	"when": true, "why": true, "how": true,

	// Demonstratives
	"this": true, "these": true, "those": true,

	// Common verbs
	"am": true, "been": true, "being": true, "have": true, "had": true,
	"having": true, "do": true, "does": true, "did": true, "doing": true,

	// Common modal and auxiliary verbs
	"not": true, "no": true, "nor": true, "very": true, "can": true,
	"could": true, "should": true, "would": true, "may": true, "might": true,
	"must": true, "shall": true,

	// Common short words that add noise
	"all": true, "any": true, "both": true, "each": true, "few": true, "more": true,
	"most": true, "other": true, "some": true, "such": true, "only": true, "own": true,
	"same": true, "so": true, "than": true, "too": true, "way": true, "well": true,
}

// Regex patterns for content sanitization
var (
	// markdownRegex strips markdown formatting while preserving word boundaries
	markdownRegex = regexp.MustCompile("(?m)`{3,}.*?`{3,}|`[^`]+`|" + // Code blocks and inline code
		"#{1,6} |" + // Headers
		"\\*\\*|__|\\*|_|~~|" + // Bold, italic, strikethrough
		"\\[[^\\]]+\\]\\([^\\)]+\\)|" + // Links
		"!\\[[^\\]]+\\]\\([^\\)]+\\)|" + // Images
		"^\\s*[-*+] |^\\s*[0-9]+\\. ") // List items

	// emojiRegex removes emojis and Unicode symbols that don't add search value
	emojiRegex = regexp.MustCompile(`[\x{1F600}-\x{1F64F}]|[\x{1F300}-\x{1F5FF}]|[\x{1F680}-\x{1F6FF}]|[\x{1F1E0}-\x{1F1FF}]|[\x{2600}-\x{26FF}]|[\x{2700}-\x{27BF}]|[\x{1F900}-\x{1F9FF}]|[\x{1FA70}-\x{1FAFF}]`)

	// punctuationRegex removes most punctuation but preserves apostrophes
	punctuationRegex = regexp.MustCompile(`[^\w\s'-]`)

	// standaloneNumberRegex removes standalone numbers but keeps numbers within words
	standaloneNumberRegex = regexp.MustCompile(`\b\d+\b`)

	// htmlTagRegex removes any HTML tags that might be present
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

	// urlRegex removes URLs
	urlRegex = regexp.MustCompile(`https?://[^\s]+`)

	// specialCharsRegex removes remaining special characters that add noise
	specialCharsRegex = regexp.MustCompile(`[^\w\s]`)

	// whitespaceRegex normalizes whitespace
	whitespaceRegex = regexp.MustCompile(`\s+`)

	// numericOnlyRegex checks if a word is purely numeric
	numericOnlyRegex = regexp.MustCompile(`^\d+$`)
)

// Configuration constants for sanitization behavior
const (
	minWordLength     = 3    // Minimum word length to include in index
	maxWordLength     = 50   // Maximum word length to prevent indexing of malformed content
	removeStopWords   = true // Whether to filter out stop words
	removeEmojis      = true // Whether to remove emojis and symbols
	removePunctuation = true // Whether to remove punctuation
	removeNumbers     = true // Whether to remove standalone numbers
	removeHTML        = true // Whether to remove HTML tags
	removeURLs        = true // Whether to remove URLs
)

// sanitizeMarkdown strips markdown and prepares text for FTS indexing with comprehensive cleaning.
func (s *FTSService) sanitizeMarkdown(input string) string {
	if input == "" {
		return ""
	}

	text := input

	// Step 1: Remove URLs (do this early to avoid breaking other patterns)
	if removeURLs {
		text = urlRegex.ReplaceAllString(text, " ")
	}

	// Step 2: Remove HTML tags (in case any slipped through)
	if removeHTML {
		text = htmlTagRegex.ReplaceAllString(text, " ")
	}

	// Step 3: Remove markdown formatting
	text = markdownRegex.ReplaceAllString(text, " ")

	// Step 4: Remove emojis and Unicode symbols
	if removeEmojis {
		text = emojiRegex.ReplaceAllString(text, " ")
	}

	// Step 5: Remove punctuation (keeping apostrophes and hyphens)
	if removePunctuation {
		text = punctuationRegex.ReplaceAllString(text, " ")
	}

	// Step 6: Remove standalone numbers
	if removeNumbers {
		text = standaloneNumberRegex.ReplaceAllString(text, " ")
	}

	// Step 7: Remove any remaining special characters
	text = specialCharsRegex.ReplaceAllString(text, " ")

	// Step 8: Normalize whitespace and convert to lowercase
	text = strings.ToLower(text)
	text = whitespaceRegex.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	// Step 9: Split into words and filter
	words := strings.Fields(text)
	var filteredWords []string

	for _, word := range words {
		// Skip empty words
		if word == "" {
			continue
		}

		// Skip words that are too short or too long
		if len(word) < minWordLength || len(word) > maxWordLength {
			continue
		}

		// Skip stop words
		if removeStopWords && stopWords[word] {
			continue
		}

		// Skip words that are purely numeric after cleaning
		if numericOnlyRegex.MatchString(word) {
			continue
		}

		// Skip words with only repeated characters (e.g., "aaa", "---")
		if isRepeatedChars(word) {
			continue
		}

		filteredWords = append(filteredWords, word)
	}

	// Step 10: Join back with single spaces
	result := strings.Join(filteredWords, " ")

	// Log the sanitization for debugging if the result is significantly different
	if s.logger != nil && len(result) < len(input)/2 {
		s.logger.Database().Debug("Significant content reduction during FTS sanitization",
			"originalLength", len(input),
			"sanitizedLength", len(result),
			"wordsFiltered", len(words)-len(filteredWords),
		)
	}

	return result
}

// isRepeatedChars checks if a word consists of only repeated characters
func isRepeatedChars(word string) bool {
	if len(word) < 3 {
		return false
	}

	firstChar := word[0]
	for i := 1; i < len(word); i++ {
		if word[i] != firstChar {
			return false
		}
	}
	return true
}

// IndexPaneContent sanitizes and indexes the body of a markdown pane.
func (s *FTSService) IndexPaneContent(tx *sql.Tx, paneID, content string) error {
	sanitizedContent := s.sanitizeMarkdown(content)
	if sanitizedContent == "" {
		// If there's no content after sanitization, ensure any old index entry is gone.
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

// deleteResourceBodyIndex removes a resource's body content from the FTS index.
func (s *FTSService) deleteResourceBodyIndex(tx *sql.Tx, resourceID string) error {
	query := `DELETE FROM resource_body_fts WHERE resource_id = ?`
	if _, err := tx.Exec(query, resourceID); err != nil {
		s.logger.Database().Error("Failed to delete from resource_body_fts", "error", err, "resourceId", resourceID)
		return fmt.Errorf("failed to delete resource FTS body for resource %s: %w", resourceID, err)
	}
	return nil
}
