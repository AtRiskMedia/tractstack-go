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

// GetDiscoverSuggestions provides "as-you-type" suggestions.
func (s *SearchService) GetDiscoverSuggestions(tenantCtx *tenant.Context, query string) ([]DiscoverySuggestion, error) {
	var wg sync.WaitGroup
	var mutex sync.Mutex
	suggestions := make(map[string]string) // Use map for easy de-duplication: term -> type
	var firstError error

	addSuggestion := func(term, termType string) {
		mutex.Lock()
		defer mutex.Unlock()

		// Remove the snippet markers and ellipses
		term = strings.ReplaceAll(term, ">>>", "")
		term = strings.ReplaceAll(term, "<<<", "")
		term = strings.ReplaceAll(term, "...", "")
		cleanedTerm := strings.ToLower(strings.TrimSpace(term))

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

	// Wrapper for FTS searches
	runFTSSearch := func(searcher func(*tenant.Context, string) ([]repositories.FTSResult, error)) {
		defer wg.Done()
		results, err := searcher(tenantCtx, query)
		if err != nil {
			setError(err)
			return
		}
		for _, res := range results {
			addSuggestion(res.Term, "CONTENT")
		}
	}

	// Goroutine 1, 2, 3: FTS Searches
	wg.Add(3)
	go runFTSSearch(s.paneService.SearchContent)
	go runFTSSearch(s.storyFragmentService.SearchMetadata)
	go runFTSSearch(s.resourceService.SearchBodies)

	// Goroutine 4: Topic Search using ContentMap
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Get cached content map
		cachedItems, err := s.contentMapService.GetCachedContentMap(tenantCtx)
		if err != nil {
			setError(err)
			return
		}

		// Find the special "all-topics" entry
		var allTopics []string
		for _, item := range cachedItems {
			if item.Type == "Topic" && item.ID == "all-topics" {
				allTopics = item.Topics
				break
			}
		}

		// Filter topics by query (case-insensitive substring match)
		queryLower := strings.ToLower(query)
		for _, topic := range allTopics {
			if strings.Contains(strings.ToLower(topic), queryLower) {
				addSuggestion(topic, "TOPIC")
			}
		}
	}()

	// Goroutine 5: Resource Title N-gram Search (COLLECTION)
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Only search collection resources if COLLECTION_ROUTES is configured
		if len(config.CollectionRoutes) == 0 {
			return
		}

		// Get all resources from cache-first repository
		resources, err := s.resourceService.GetAll(tenantCtx)
		if err != nil {
			setError(err)
			return
		}

		// Filter and search resource titles for n-gram matches
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
					// Simple n-gram match: check if query is contained in title
					if strings.Contains(titleLower, queryLower) {
						addSuggestion(resource.Title, "COLLECTION")
					}
				}
			}
		}
	}()

	wg.Wait()

	if firstError != nil {
		return nil, firstError
	}

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

	return resultList, nil
}

// RetrieveFullResults performs a deep search based on a selected term.
func (s *SearchService) RetrieveFullResults(tenantCtx *tenant.Context, term string, isTopic bool) (*CategorizedResults, error) {
	var wg sync.WaitGroup
	var mutex sync.Mutex
	var firstError error

	// Initialize results
	results := &CategorizedResults{
		StoryFragmentResults: []repositories.FTSResult{},
		ContextPaneResults:   []repositories.FTSResult{},
		ResourceResults:      []repositories.FTSResult{},
	}

	setError := func(err error) {
		mutex.Lock()
		defer mutex.Unlock()
		if firstError == nil {
			firstError = err
		}
	}

	// If this is a topic search, ONLY search by topic
	if isTopic {
		// Get cached content map
		cachedItems, err := s.contentMapService.GetCachedContentMap(tenantCtx)
		if err != nil {
			return nil, err
		}

		// Find story fragments with the matching topic (case-insensitive)
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

	// Non-topic search: Check for COLLECTION title matches first
	if len(config.CollectionRoutes) > 0 {
		resources, err := s.resourceService.GetAll(tenantCtx)
		if err == nil {
			for _, resource := range resources {
				if resource.CategorySlug != nil {
					// Check if resource is in COLLECTION_ROUTES
					isInCollection := false
					for _, route := range config.CollectionRoutes {
						if *resource.CategorySlug == route {
							isInCollection = true
							break
						}
					}

					if isInCollection {
						// Check for exact title match (case-insensitive)
						if strings.EqualFold(resource.Title, term) {
							results.ResourceResults = append(results.ResourceResults, repositories.FTSResult{
								ID:        resource.ID,
								Relevance: 0.95, // High relevance for exact title match
								Term:      term,
							})
						}
					}
				}
			}
		}
	}

	// Search StoryFragment metadata
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

	// Search ContextPane content
	wg.Add(1)
	go func() {
		defer wg.Done()
		paneResults, err := s.paneService.SearchContent(tenantCtx, term)
		if err != nil {
			setError(err)
			return
		}

		// Filter for context panes only
		if len(paneResults) > 0 {
			paneRepo := tenantCtx.PaneRepo()
			resultsByPaneID := make(map[string]repositories.FTSResult)
			for _, result := range paneResults {
				resultsByPaneID[result.ID] = result
			}

			paneIDs := make([]string, 0, len(resultsByPaneID))
			for paneID := range resultsByPaneID {
				paneIDs = append(paneIDs, paneID)
			}

			panes, err := paneRepo.FindByIDs(tenantCtx.TenantID, paneIDs)
			if err != nil {
				setError(err)
				return
			}

			mutex.Lock()
			for _, pane := range panes {
				if pane.IsContextPane {
					paneID := pane.ID
					results.ContextPaneResults = append(results.ContextPaneResults, repositories.FTSResult{
						ID:        paneID,
						Relevance: resultsByPaneID[paneID].Relevance,
						Term:      resultsByPaneID[paneID].Term,
					})
				}
			}
			mutex.Unlock()
		}
	}()

	// Search Resource bodies (FTS search in addition to title matches above)
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

	wg.Wait()

	if firstError != nil {
		return nil, firstError
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
