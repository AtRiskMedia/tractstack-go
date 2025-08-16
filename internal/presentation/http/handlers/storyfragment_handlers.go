// Package handlers provides HTTP handlers for storyfragment endpoints
package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
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
	fragmentService      *services.FragmentService
	logger               *logging.ChanneledLogger
	perfTracker          *performance.Tracker
}

// NewStoryFragmentHandlers creates storyfragment handlers with injected dependencies
func NewStoryFragmentHandlers(storyFragmentService *services.StoryFragmentService, fragmentService *services.FragmentService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *StoryFragmentHandlers {
	return &StoryFragmentHandlers{
		storyFragmentService: storyFragmentService,
		fragmentService:      fragmentService,
		logger:               logger,
		perfTracker:          perfTracker,
	}
}

// GetAllStoryFragmentIDs returns all storyfragment IDs using cache-first pattern
func (h *StoryFragmentHandlers) GetAllStoryFragmentIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_all_storyfragment_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get all story fragment IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
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
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetAllStoryFragmentIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"storyFragmentIds": storyFragmentIDs,
		"count":            len(storyFragmentIDs),
	})
}

// GetStoryFragmentsByIDs returns multiple storyfragments by IDs using cache-first pattern
func (h *StoryFragmentHandlers) GetStoryFragmentsByIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_storyfragments_by_ids_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get story fragments by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
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
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetStoryFragmentsByIDs request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(req.StoryFragmentIDs))

	c.JSON(http.StatusOK, gin.H{
		"storyFragments": storyFragments,
		"count":          len(storyFragments),
	})
}

// GetStoryFragmentByID returns a specific storyfragment by ID using cache-first pattern
func (h *StoryFragmentHandlers) GetStoryFragmentByID(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_storyfragment_by_id_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get story fragment by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "storyFragmentId", c.Param("id"))
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
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetStoryFragmentByID request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "storyFragmentId", storyFragmentID)

	c.JSON(http.StatusOK, storyFragmentNode)
}

// GetStoryFragmentBySlug returns a specific storyfragment by slug using cache-first pattern
func (h *StoryFragmentHandlers) GetStoryFragmentBySlug(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_storyfragment_by_slug_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get story fragment by slug request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
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

	sessionID := c.GetHeader("X-TractStack-Session-ID")
	err = h.storyFragmentService.EnrichWithMetadata(tenantCtx, storyFragmentNode, sessionID)
	if err != nil {
		h.logger.Content().Warn("Failed to enrich storyfragment metadata", "error", err)
	}

	h.logger.Content().Info("Get story fragment by slug request completed", "slug", slug, "found", storyFragmentNode != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetStoryFragmentBySlug request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	c.JSON(http.StatusOK, storyFragmentNode)
}

// GetStoryFragmentFullPayloadBySlug returns a storyfragment with full editorial payload
func (h *StoryFragmentHandlers) GetStoryFragmentFullPayloadBySlug(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_storyfragment_full_payload_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get story fragment full payload request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
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

	// === V1 COMPATIBILITY PROCESSING ===
	var allChildNodes []any
	cleanedPanes := make([]*content.PaneNode, len(fullPayload.Panes))

	for i, pane := range fullPayload.Panes {
		if pane == nil {
			continue
		}

		// Extract child nodes from this pane's OptionsPayload
		if pane.OptionsPayload != nil {
			if nodes, exists := pane.OptionsPayload["nodes"]; exists {
				if nodesArray, ok := nodes.([]any); ok {
					allChildNodes = append(allChildNodes, nodesArray...)
				}
			}
		}

		// Create cleaned pane (without embedded nodes)
		cleanedPane := *pane
		cleanedPane.OptionsPayload = make(map[string]any)

		// Copy all fields except "nodes"
		if pane.OptionsPayload != nil {
			for k, v := range pane.OptionsPayload {
				if k != "nodes" {
					cleanedPane.OptionsPayload[k] = v
				}
			}
		}

		cleanedPanes[i] = &cleanedPane
	}

	// === BUILD V1-COMPATIBLE RESPONSE ===
	var tractStackNodes []*content.TractStackNode
	if fullPayload.TractStack != nil {
		tractStackNodes = []*content.TractStackNode{fullPayload.TractStack}
	}

	response := gin.H{
		"storyfragmentNodes": []*content.StoryFragmentNode{fullPayload.StoryFragment},
		"paneNodes":          cleanedPanes,
		"childNodes":         allChildNodes,
		"tractstackNodes":    tractStackNodes,
	}

	// Add menu nodes if they exist
	if fullPayload.Menu != nil {
		response["menuNodes"] = []*content.MenuNode{fullPayload.Menu}
	}

	h.logger.Content().Info("Get story fragment full payload request completed", "slug", slug, "found", fullPayload != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetStoryFragmentFullPayloadBySlug request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	c.JSON(http.StatusOK, response)
}

// GetHomeStoryFragment returns the home storyfragment using cache-first pattern
func (h *StoryFragmentHandlers) GetHomeStoryFragment(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_home_storyfragment_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get home story fragment request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	sessionID := c.GetHeader("X-TractStack-Session-ID")
	storyFragmentNode, err := h.storyFragmentService.GetHome(tenantCtx, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "home storyfragment not found"})
		return
	}

	h.logger.Content().Info("Get home story fragment request completed", "found", storyFragmentNode != nil, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetHomeStoryFragment request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, storyFragmentNode)
}

// CreateStoryFragment creates a new storyfragment
func (h *StoryFragmentHandlers) CreateStoryFragment(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("create_storyfragment_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received create story fragment request", "method", c.Request.Method, "path", c.Request.URL.Path)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var sf content.StoryFragmentNode
	if err := c.ShouldBindJSON(&sf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := h.storyFragmentService.Create(tenantCtx, &sf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Create story fragment request completed", "storyFragmentId", sf.ID, "title", sf.Title, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for CreateStoryFragment request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "storyFragmentId", sf.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message":         "storyfragment created successfully",
		"storyFragmentId": sf.ID,
	})
}

// UpdateStoryFragment updates an existing storyfragment
func (h *StoryFragmentHandlers) UpdateStoryFragment(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("update_storyfragment_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received update story fragment request", "method", c.Request.Method, "path", c.Request.URL.Path, "storyFragmentId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	storyFragmentID := c.Param("id")
	if storyFragmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment ID is required"})
		return
	}

	var sf content.StoryFragmentNode
	if err := c.ShouldBindJSON(&sf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	sf.ID = storyFragmentID

	if err := h.storyFragmentService.Update(tenantCtx, &sf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Update story fragment request completed", "storyFragmentId", storyFragmentID, "title", sf.Title, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for UpdateStoryFragment request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "storyFragmentId", storyFragmentID)

	c.JSON(http.StatusOK, gin.H{
		"message":         "storyfragment updated successfully",
		"storyFragmentId": storyFragmentID,
	})
}

// DeleteStoryFragment deletes a storyfragment
func (h *StoryFragmentHandlers) DeleteStoryFragment(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("delete_storyfragment_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received delete story fragment request", "method", c.Request.Method, "path", c.Request.URL.Path, "storyFragmentId", c.Param("id"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	storyFragmentID := c.Param("id")
	if storyFragmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment ID is required"})
		return
	}

	if err := h.storyFragmentService.Delete(tenantCtx, storyFragmentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Delete story fragment request completed", "storyFragmentId", storyFragmentID, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for DeleteStoryFragment request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "storyFragmentId", storyFragmentID)

	c.JSON(http.StatusOK, gin.H{
		"message":         "storyfragment deleted successfully",
		"storyFragmentId": storyFragmentID,
	})
}

func (h *StoryFragmentHandlers) GetStoryFragmentPersonalizedPayloadBySlug(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	start := time.Now()
	marker := h.perfTracker.StartOperation("get_storyfragment_personalized_payload_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get story fragment personalized payload request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	sessionID := c.GetHeader("X-TractStack-Session-ID")

	var storyFragment *content.StoryFragmentNode
	var err error

	if slug == "" {
		storyFragment, err = h.storyFragmentService.GetHome(tenantCtx, sessionID)
	} else {
		storyFragment, err = h.storyFragmentService.GetBySlug(tenantCtx, slug)
		if err == nil && storyFragment != nil {
			err = h.storyFragmentService.EnrichWithMetadata(tenantCtx, storyFragment, sessionID)
		}
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	h.logger.Content().Debug("=== HANDLER IMPRESSIONS DEBUG ===", "storyfragmentID", storyFragment.ID, "paneIDsCount", len(storyFragment.PaneIDs))

	var fragmentsData map[string]string
	if len(storyFragment.PaneIDs) > 0 {
		results, errors, err := h.fragmentService.GenerateFragmentBatch(
			tenantCtx, storyFragment.PaneIDs, sessionID, storyFragment.ID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		fragmentsData = results

		if len(errors) > 0 {
			for paneID, errMsg := range errors {
				fragmentsData[paneID] = fmt.Sprintf(`<div class="error">Failed to load pane %s: %s</div>`, paneID, errMsg)
			}
		}
	} else {
		fragmentsData = make(map[string]string)
	}

	// Get impressions for this storyfragment
	h.logger.Content().Debug("About to call GetImpressionsByPaneIDs", "paneIDsCount", len(storyFragment.PaneIDs))

	var impressions []map[string]any
	if len(storyFragment.PaneIDs) > 0 {
		impressions, err = h.storyFragmentService.GetImpressionsByPaneIDs(tenantCtx, storyFragment.PaneIDs)
		if err != nil {
			h.logger.Content().Debug("Failed to fetch impressions", "error", err.Error())
			impressions = []map[string]any{}
		} else {
			h.logger.Content().Debug("Successfully fetched impressions", "count", len(impressions))
		}
	} else {
		h.logger.Content().Debug("No pane IDs to fetch impressions for")
		impressions = []map[string]any{}
	}

	h.logger.Content().Debug("Final impressions before response", "count", len(impressions), "impressions", impressions)

	response := gin.H{
		"id":              storyFragment.ID,
		"title":           storyFragment.Title,
		"slug":            storyFragment.Slug,
		"paneIds":         storyFragment.PaneIDs,
		"codeHookTargets": storyFragment.CodeHookTargets,
		"menu":            storyFragment.Menu,
		"isHome":          storyFragment.IsHome,
		"created":         storyFragment.Created,
		"fragments":       fragmentsData,
		"impressions":     impressions,
	}

	h.logger.Content().Info("Get story fragment personalized payload request completed", "slug", slug, "sessionId", sessionID, "paneCount", len(storyFragment.PaneIDs), "impressionCount", len(impressions), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetStoryFragmentPersonalizedPayloadBySlug request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	c.JSON(http.StatusOK, response)
}
