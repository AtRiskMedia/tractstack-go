// Package handlers provides HTTP handlers for search endpoints
package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// SearchHandlers contains all search-related HTTP handlers
type SearchHandlers struct {
	searchService *services.SearchService
	logger        *logging.ChanneledLogger
	perfTracker   *performance.Tracker
}

// NewSearchHandlers creates search handlers with injected dependencies
func NewSearchHandlers(searchService *services.SearchService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *SearchHandlers {
	return &SearchHandlers{
		searchService: searchService,
		logger:        logger,
		perfTracker:   perfTracker,
	}
}

const searchThrottleDuration = 1 * time.Second

// isSearchThrottled checks if a session has made a search request within the throttle duration.
func (h *SearchHandlers) isSearchThrottled(c *gin.Context, sessionID string) bool {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		return true // Fail closed if context is missing
	}
	cache := tenantCtx.GetCacheManager()
	cacheKey := fmt.Sprintf("search_throttle:%s", sessionID)

	if lastRequestTime, found := cache.GetGeneric(tenantCtx.TenantID, cacheKey); found {
		if time.Since(lastRequestTime.(time.Time)) < searchThrottleDuration {
			return true // Throttled
		}
	}

	// Not throttled, update the timestamp and allow the request.
	cache.SetGenericWithTTL(tenantCtx.TenantID, cacheKey, time.Now(), 5*time.Second)
	return false
}

// HandleDiscovery provides "as-you-type" suggestions for the search modal.
func (h *SearchHandlers) HandleDiscovery(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	marker := h.perfTracker.StartOperation("search_discover_request", tenantCtx.TenantID)
	defer marker.Complete()

	sessionID := c.GetHeader("X-TractStack-Session-ID")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "X-TractStack-Session-ID header is required"})
		return
	}

	if h.isSearchThrottled(c, sessionID) {
		h.logger.Auth().Info("Search discovery request throttled for session", "sessionId", sessionID, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "You are searching too frequently. Please wait a moment."})
		return
	}

	query := c.Query("q")
	if len(query) < 3 {
		c.JSON(http.StatusOK, gin.H{"suggestions": []services.DiscoverySuggestion{}})
		return
	}

	suggestions, err := h.searchService.GetDiscoverSuggestions(tenantCtx, query)
	if err != nil {
		h.logger.System().Error("Failed to get discovery suggestions", "error", err, "tenantId", tenantCtx.TenantID, "query", query)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve search suggestions"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}

// HandleRetrieval performs a deep search based on a selected term from the discovery phase.
func (h *SearchHandlers) HandleRetrieval(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	marker := h.perfTracker.StartOperation("search_retrieve_request", tenantCtx.TenantID)
	defer marker.Complete()

	term := c.Query("term")
	if term == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter 'term' is required"})
		return
	}

	isTopic := c.Query("topic") == "true"

	results, err := h.searchService.RetrieveFullResults(tenantCtx, term, isTopic)
	if err != nil {
		h.logger.System().Error("Failed to retrieve full search results", "error", err, "tenantId", tenantCtx.TenantID, "term", term, "isTopic", isTopic)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve search results"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, results)
}
