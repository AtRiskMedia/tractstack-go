// Package handlers provides HTTP handlers for storyfragment endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// StoryFragmentIDsRequest represents the request body for bulk storyfragment loading
type StoryFragmentIDsRequest struct {
	StoryFragmentIDs []string `json:"storyFragmentIds" binding:"required"`
}

// StoryFragmentHandlers contains all storyfragment-related HTTP handlers
type StoryFragmentHandlers struct {
	storyFragmentService *services.StoryFragmentService
	logger               *logging.ChanneledLogger
}

// NewStoryFragmentHandlers creates storyfragment handlers with injected dependencies
func NewStoryFragmentHandlers(storyFragmentService *services.StoryFragmentService, logger *logging.ChanneledLogger) *StoryFragmentHandlers {
	return &StoryFragmentHandlers{
		storyFragmentService: storyFragmentService,
		logger:               logger,
	}
}

// GetAllStoryFragmentIDs returns all storyfragment IDs using cache-first pattern
func (h *StoryFragmentHandlers) GetAllStoryFragmentIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get all story fragment IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	storyFragmentIDs, err := h.storyFragmentService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get all story fragment IDs request completed", "count", len(storyFragmentIDs), "duration", time.Since(start))

	c.JSON(http.StatusOK, gin.H{
		"storyFragmentIds": storyFragmentIDs,
		"count":            len(storyFragmentIDs),
	})
}

// GetStoryFragmentsByIDs returns multiple storyfragments by IDs using cache-first pattern
func (h *StoryFragmentHandlers) GetStoryFragmentsByIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get story fragments by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req StoryFragmentIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.StoryFragmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyFragmentIds array cannot be empty"})
		return
	}

	storyFragments, err := h.storyFragmentService.GetByIDs(tenantCtx, req.StoryFragmentIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get story fragments by IDs request completed", "requestedCount", len(req.StoryFragmentIDs), "foundCount", len(storyFragments), "duration", time.Since(start))

	c.JSON(http.StatusOK, gin.H{
		"storyFragments": storyFragments,
		"count":          len(storyFragments),
	})
}

// GetStoryFragmentByID returns a specific storyfragment by ID using cache-first pattern
func (h *StoryFragmentHandlers) GetStoryFragmentByID(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get story fragment by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "storyFragmentId", c.Param("id"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	storyFragmentID := c.Param("id")
	if storyFragmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment ID is required"})
		return
	}

	storyFragmentNode, err := h.storyFragmentService.GetByID(tenantCtx, storyFragmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	h.logger.Content().Info("Get story fragment by ID request completed", "storyFragmentId", storyFragmentID, "found", storyFragmentNode != nil, "duration", time.Since(start))

	c.JSON(http.StatusOK, storyFragmentNode)
}

// GetStoryFragmentBySlug returns a specific storyfragment by slug using cache-first pattern
func (h *StoryFragmentHandlers) GetStoryFragmentBySlug(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get story fragment by slug request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment slug is required"})
		return
	}

	storyFragmentNode, err := h.storyFragmentService.GetBySlug(tenantCtx, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	h.logger.Content().Info("Get story fragment by slug request completed", "slug", slug, "found", storyFragmentNode != nil, "duration", time.Since(start))

	c.JSON(http.StatusOK, storyFragmentNode)
}

// GetStoryFragmentFullPayloadBySlug returns a storyfragment with full editorial payload
func (h *StoryFragmentHandlers) GetStoryFragmentFullPayloadBySlug(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get story fragment full payload request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment slug is required"})
		return
	}

	fullPayload, err := h.storyFragmentService.GetFullPayloadBySlug(tenantCtx, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if fullPayload == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	h.logger.Content().Info("Get story fragment full payload request completed", "slug", slug, "found", fullPayload != nil, "duration", time.Since(start))

	c.JSON(http.StatusOK, fullPayload)
}

// GetHomeStoryFragment returns the home storyfragment using cache-first pattern
func (h *StoryFragmentHandlers) GetHomeStoryFragment(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get home story fragment request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// FIXED: Call GetHome method (not GetHomeStoryFragment)
	homeStoryFragment, err := h.storyFragmentService.GetHome(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if homeStoryFragment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "home storyfragment not found"})
		return
	}

	h.logger.Content().Info("Get home story fragment request completed", "found", homeStoryFragment != nil, "duration", time.Since(start))

	c.JSON(http.StatusOK, homeStoryFragment)
}
