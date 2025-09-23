// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"sort"
	"strings"
	"sync"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// DiscoverySuggestion represents a single, unique suggestion for the search modal.
type DiscoverySuggestion struct {
	Term string `json:"term"`
	Type string `json:"type"` // "TITLE", "TOPIC", "CONTENT", or "COLLECTION"
}

// CategorizedResults holds the final, de-duplicated search results.
type CategorizedResults struct {
	StoryFragmentResults []repositories.FTSResult `json:"storyFragmentResults"`
	ContextPaneResults   []repositories.FTSResult `json:"contextPaneResults"`
	ResourceResults      []repositories.FTSResult `json:"resourceResults"`
}

// SearchService orchestrates search operations across multiple content types.
type SearchService struct {
	paneService          *PaneService
	storyFragmentService *StoryFragmentService
	resourceService      *ResourceService
	contentMapService    *ContentMapService
}

// NewSearchService creates a new search service.
func NewSearchService(ps *PaneService, sfs *StoryFragmentService, rs *ResourceService, cms *ContentMapService) *SearchService {
	return &SearchService{
		paneService:          ps,
		storyFragmentService: sfs,
		resourceService:      rs,
		contentMapService:    cms,
	}
}

// GetDiscoverSuggestions provides "as-you-type" suggestions using FTS indexes.
func (s *SearchService) GetDiscoverSuggestions(tenantCtx *tenant.Context, query string) ([]DiscoverySuggestion, error) {
	var wg sync.WaitGroup
	var mutex sync.Mutex
	suggestions := make(map[string]string) // Use map for easy de-duplication: term -> type
	var firstError error

	// Helper function to safely add suggestions with type priority
	addSuggestion := func(term, termType string) {
		mutex.Lock()
		defer mutex.Unlock()

		// Remove FTS snippet markers and ellipses
		term = strings.ReplaceAll(term, ">>>", "")
		term = strings.ReplaceAll(term, "<<<", "")
		term = strings.ReplaceAll(term, "...", "")
		cleanedTerm := strings.ToLower(strings.TrimSpace(term))

		// Skip empty terms or single characters
		if len(cleanedTerm) < 2 {
			return
		}

		// Prioritize better match types (TITLE > COLLECTION > TOPIC > CONTENT)
		if existingType, exists := suggestions[cleanedTerm]; exists {
			if existingType == "CONTENT" && (termType == "TITLE" || termType == "COLLECTION" || termType == "TOPIC") {
				suggestions[cleanedTerm] = termType
			} else if existingType == "TOPIC" && (termType == "TITLE" || termType == "COLLECTION") {
				suggestions[cleanedTerm] = termType
			} else if existingType == "COLLECTION" && termType == "TITLE" {
				suggestions[cleanedTerm] = termType
			}
		} else {
			suggestions[cleanedTerm] = termType
		}
	}

	setError := func(err error) {
		mutex.Lock()
		defer mutex.Unlock()
		if firstError == nil {
			firstError = err
		}
	}

	// Prepare FTS prefix search term
	searchTerm := query + "*"

	// 1. Search StoryFragment metadata (FTS) for TITLE matches
	wg.Add(1)
	go func() {
		defer wg.Done()
		results, err := s.storyFragmentService.SearchMetadata(tenantCtx, searchTerm)
		if err != nil {
			setError(err)
			return
		}

		// Extract meaningful terms from story fragment titles
		for _, result := range results {
			// Get the actual story fragment to access the title
			cachedItems, err := s.contentMapService.GetCachedContentMap(tenantCtx)
			if err != nil {
				continue
			}

			for _, item := range cachedItems {
				if item.Type == "StoryFragment" && item.ID == result.ID {
					addSuggestion(item.Title, "TITLE")
					break
				}
			}
		}
	}()

	// 2. Search Pane content (FTS) for CONTENT matches
	wg.Add(1)
	go func() {
		defer wg.Done()
		results, err := s.paneService.SearchContent(tenantCtx, searchTerm)
		if err != nil {
			setError(err)
			return
		}

		// Extract terms from FTS snippets
		for _, result := range results {
			// Parse the snippet to extract relevant terms
			snippetWords := extractWordsFromSnippet(result.Term, query)
			for _, word := range snippetWords {
				addSuggestion(word, "CONTENT")
			}
		}
	}()

	// 3. Search Resource bodies (FTS) for CONTENT matches
	wg.Add(1)
	go func() {
		defer wg.Done()
		results, err := s.resourceService.SearchBodies(tenantCtx, searchTerm)
		if err != nil {
			setError(err)
			return
		}

		// Extract terms from resource FTS snippets
		for _, result := range results {
			snippetWords := extractWordsFromSnippet(result.Term, query)
			for _, word := range snippetWords {
				addSuggestion(word, "CONTENT")
			}
		}
	}()

	// 4. Search Topics from content map (cached, fast)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cachedItems, err := s.contentMapService.GetCachedContentMap(tenantCtx)
		if err != nil {
			setError(err)
			return
		}

		queryLower := strings.ToLower(query)
		for _, item := range cachedItems {
			if item.Type == "StoryFragment" && item.Topics != nil {
				for _, topic := range item.Topics {
					topicLower := strings.ToLower(topic)
					// Only suggest topics that start with the query (prefix matching)
					if strings.HasPrefix(topicLower, queryLower) {
						addSuggestion(topic, "TOPIC")
					}
				}
			}
		}
	}()

	// 5. Search Collection titles (exact matches and prefix matches)
	if len(config.CollectionRoutes) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resources, err := s.resourceService.GetAll(tenantCtx)
			if err != nil {
				setError(err)
				return
			}

			queryLower := strings.ToLower(query)
			for _, resource := range resources {
				// Only include resources from collection routes
				if resource.CategorySlug != nil {
					isInCollection := false
					for _, route := range config.CollectionRoutes {
						if *resource.CategorySlug == route {
							isInCollection = true
							break
						}
					}

					if isInCollection {
						titleLower := strings.ToLower(resource.Title)
						// Prefix match for better relevance
						if strings.HasPrefix(titleLower, queryLower) {
							addSuggestion(resource.Title, "COLLECTION")
						}
					}
				}
			}
		}()
	}

	wg.Wait()

	if firstError != nil {
		return nil, firstError
	}

	// Convert map to slice
	resultList := make([]DiscoverySuggestion, 0, len(suggestions))
	for term, termType := range suggestions {
		resultList = append(resultList, DiscoverySuggestion{Term: term, Type: termType})
	}

	// Sort with priority: TITLE > COLLECTION > TOPIC > CONTENT, then alphabetically
	sort.Slice(resultList, func(i, j int) bool {
		typeI, typeJ := resultList[i].Type, resultList[j].Type

		// Define type priorities
		priorityMap := map[string]int{
			"TITLE":      1,
			"COLLECTION": 2,
			"TOPIC":      3,
			"CONTENT":    4,
		}

		priorityI := priorityMap[typeI]
		priorityJ := priorityMap[typeJ]

		if priorityI != priorityJ {
			return priorityI < priorityJ
		}

		// Same priority, sort alphabetically
		return resultList[i].Term < resultList[j].Term
	})

	// Limit results to prevent overwhelming the UI
	if len(resultList) > 20 {
		resultList = resultList[:20]
	}

	return resultList, nil
}

// extractWordsFromSnippet extracts meaningful words from FTS snippets
func extractWordsFromSnippet(snippet, query string) []string {
	// Remove FTS markers
	cleaned := strings.ReplaceAll(snippet, ">>>", "")
	cleaned = strings.ReplaceAll(cleaned, "<<<", "")
	cleaned = strings.ReplaceAll(cleaned, "...", "")

	// Split into words and filter
	words := strings.Fields(strings.ToLower(cleaned))
	var result []string
	queryLower := strings.ToLower(query)

	for _, word := range words {
		// Only include words that start with the query (prefix matching)
		// and are reasonable length
		if len(word) >= 3 && len(word) <= 20 && strings.HasPrefix(word, queryLower) {
			result = append(result, word)
		}
	}

	return result
}

// RetrieveFullResults performs a deep search based on a selected term.
func (s *SearchService) RetrieveFullResults(tenantCtx *tenant.Context, term string, isTopic bool) (*CategorizedResults, error) {
	// Get the entire content map from cache once. This is our primary lookup table.
	cachedItems, err := s.contentMapService.GetCachedContentMap(tenantCtx)
	if err != nil {
		return nil, err
	}

	// Initialize results
	results := &CategorizedResults{
		StoryFragmentResults: []repositories.FTSResult{},
		ContextPaneResults:   []repositories.FTSResult{},
		ResourceResults:      []repositories.FTSResult{},
	}

	// --- Topic Search Path (uses content map) ---
	if isTopic {
		termLower := strings.ToLower(term)
		for _, item := range cachedItems {
			if item.Type == "StoryFragment" && item.Topics != nil {
				for _, topic := range item.Topics {
					if strings.EqualFold(topic, termLower) {
						results.StoryFragmentResults = append(results.StoryFragmentResults, repositories.FTSResult{
							ID:        item.ID,
							Relevance: 0.99, // High relevance for topic matches
							Term:      term,
						})
						break // Only add once per story fragment
					}
				}
			}
		}
		return results, nil
	}

	// --- Standard Term Search Path (uses FTS then content map) ---

	var wg sync.WaitGroup
	var mutex sync.Mutex
	var firstError error

	setError := func(err error) {
		mutex.Lock()
		defer mutex.Unlock()
		if firstError == nil {
			firstError = err
		}
	}

	// 1. Search StoryFragment metadata (FTS)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sfResults, err := s.storyFragmentService.SearchMetadata(tenantCtx, term)
		if err != nil {
			setError(err)
			return
		}
		mutex.Lock()
		results.StoryFragmentResults = append(results.StoryFragmentResults, sfResults...)
		mutex.Unlock()
	}()

	// 2. Search Resource bodies (FTS)
	wg.Add(1)
	go func() {
		defer wg.Done()
		resourceResults, err := s.resourceService.SearchBodies(tenantCtx, term)
		if err != nil {
			setError(err)
			return
		}
		mutex.Lock()
		results.ResourceResults = append(results.ResourceResults, resourceResults...)
		mutex.Unlock()
	}()

	// 3. Search Pane content (FTS) and then resolve parents using the Content Map
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Hit the FTS table to get matching pane IDs
		paneFTSResults, err := s.paneService.SearchContent(tenantCtx, term)
		if err != nil {
			setError(err)
			return
		}
		if len(paneFTSResults) == 0 {
			return
		}

		// Create a quick lookup map of paneID -> FTS result
		ftsResultMap := make(map[string]repositories.FTSResult)
		for _, res := range paneFTSResults {
			ftsResultMap[res.ID] = res
		}

		// Now, iterate through the *cached content map* to find parents and context panes
		// This avoids any further database hits.
		parentStoryFragmentIDs := make(map[string]bool)

		mutex.Lock()
		for _, item := range cachedItems {
			// Case A: The map item is a Pane that matched our FTS search
			if item.Type == "Pane" {
				if _, ok := ftsResultMap[item.ID]; ok {
					// Check if it's a context pane
					if item.IsContext != nil && *item.IsContext {
						results.ContextPaneResults = append(results.ContextPaneResults, ftsResultMap[item.ID])
					}
				}
			}

			// Case B: The map item is a StoryFragment. Check if it contains a pane that matched our FTS search.
			if item.Type == "StoryFragment" && item.Panes != nil {
				for _, paneIDInSF := range item.Panes {
					if _, ok := ftsResultMap[paneIDInSF]; ok {
						parentStoryFragmentIDs[item.ID] = true // Found a parent
						break                                  // Move to the next story fragment
					}
				}
			}
		}

		// Add the discovered parent StoryFragments to the results
		for sfID := range parentStoryFragmentIDs {
			results.StoryFragmentResults = append(results.StoryFragmentResults, repositories.FTSResult{
				ID:        sfID,
				Relevance: 0.5, // Indicate that this was a match via pane content
				Term:      term,
			})
		}
		mutex.Unlock()
	}()

	wg.Wait()

	if firstError != nil {
		return nil, firstError
	}

	// Non-topic search: Check for COLLECTION title matches first (uses cache-first GetAll)
	if len(config.CollectionRoutes) > 0 {
		resources, err := s.resourceService.GetAll(tenantCtx) // This uses cache
		if err == nil {
			for _, resource := range resources {
				if resource.CategorySlug != nil {
					isInCollection := false
					for _, route := range config.CollectionRoutes {
						if *resource.CategorySlug == route {
							isInCollection = true
							break
						}
					}
					if isInCollection && strings.EqualFold(resource.Title, term) {
						results.ResourceResults = append(results.ResourceResults, repositories.FTSResult{
							ID: resource.ID, Relevance: 0.95, Term: term,
						})
					}
				}
			}
		}
	}

	// De-duplicate results within each category by ID
	results.StoryFragmentResults = s.deduplicateFTSResults(results.StoryFragmentResults)
	results.ContextPaneResults = s.deduplicateFTSResults(results.ContextPaneResults)
	results.ResourceResults = s.deduplicateFTSResults(results.ResourceResults)

	return results, nil
}

// deduplicateFTSResults removes duplicate entries based on ID, keeping the one with the highest relevance.
func (s *SearchService) deduplicateFTSResults(results []repositories.FTSResult) []repositories.FTSResult {
	if len(results) <= 1 {
		return results
	}

	seen := make(map[string]repositories.FTSResult)
	for _, result := range results {
		if existing, exists := seen[result.ID]; exists {
			if result.Relevance > existing.Relevance {
				seen[result.ID] = result
			}
		} else {
			seen[result.ID] = result
		}
	}

	deduplicated := make([]repositories.FTSResult, 0, len(seen))
	for _, result := range seen {
		deduplicated = append(deduplicated, result)
	}

	// Sort by relevance descending
	sort.Slice(deduplicated, func(i, j int) bool {
		return deduplicated[i].Relevance > deduplicated[j].Relevance
	})

	return deduplicated
}
