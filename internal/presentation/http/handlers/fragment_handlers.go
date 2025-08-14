package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// FragmentHandlers handles HTTP requests for fragment endpoints
// This is a thin wrapper around FragmentService following the established pattern
type FragmentHandlers struct {
	fragmentService *services.FragmentService
	logger          *logging.ChanneledLogger
	perfTracker     *performance.Tracker
}

// NewFragmentHandlers creates a new fragment handlers instance
func NewFragmentHandlers(fragmentService *services.FragmentService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *FragmentHandlers {
	return &FragmentHandlers{
		fragmentService: fragmentService,
		logger:          logger,
		perfTracker:     perfTracker,
	}
}

// PreviewFromPayloadRequest represents the request body for preview generation
type PreviewFromPayloadRequest struct {
	Panes []PreviewPaneData `json:"panes"`
}

type PreviewPaneData struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	OptionsPayload map[string]any `json:"optionsPayload"`
}

// GetPaneFragment handles GET /api/v1/fragments/panes/:id
func (h *FragmentHandlers) GetPaneFragment(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get fragment request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("get_pane_fragment_request", tenantCtx.TenantID)
	defer marker.Complete()

	// Extract path parameter
	paneID := c.Param("id")
	if paneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pane ID is required"})
		return
	}

	// Extract headers for personalization context
	sessionID := c.GetHeader("X-TractStack-Session-ID")
	storyfragmentID := c.GetHeader("X-StoryFragment-ID")

	// Generate fragment using service
	html, err := h.fragmentService.GenerateFragment(tenantCtx, paneID, sessionID, storyfragmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get fragment request completed", "duration", time.Since(start))

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetPaneFragment request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneId", paneID)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// BatchFragmentRequest represents the request body for batch fragment operations
type BatchFragmentRequest struct {
	PaneIDs []string `json:"paneIds" binding:"required"`
}

// GetPaneFragmentBatch handles POST /api/v1/fragments/panes
func (h *FragmentHandlers) GetPaneFragmentBatch(c *gin.Context) {
	// Extract tenant context from middleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("get_pane_fragment_batch_request", tenantCtx.TenantID)
	defer marker.Complete()

	// Parse request body
	var req BatchFragmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate pane IDs
	if len(req.PaneIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one pane ID is required"})
		return
	}

	// Extract headers for personalization context
	sessionID := c.GetHeader("X-TractStack-Session-ID")
	storyfragmentID := c.GetHeader("X-StoryFragment-ID")

	// Generate fragments using service
	results, errors, err := h.fragmentService.GenerateFragmentBatch(
		tenantCtx, req.PaneIDs, sessionID, storyfragmentID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response matching legacy format
	response := gin.H{
		"fragments": results,
	}

	// Include errors if any occurred
	if len(errors) > 0 {
		response["errors"] = errors
	}

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetPaneFragmentBatch request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneCount", len(req.PaneIDs))
	c.JSON(http.StatusOK, response)
}

// GeneratePreviewFromPayload handles POST /api/v1/fragments/preview
func (h *FragmentHandlers) GeneratePreviewFromPayload(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("generate_preview_from_payload_request", tenantCtx.TenantID)
	defer marker.Complete()

	var req PreviewFromPayloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.Panes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one pane is required"})
		return
	}

	// Generate HTML from payloads without database persistence
	results := make(map[string]string)
	errors := make(map[string]string)

	for _, paneData := range req.Panes {
		// Pass the pane ID from the request to maintain parent-child relationships
		html, err := h.fragmentService.GenerateHTMLFromPayload(tenantCtx, paneData.ID, paneData.OptionsPayload)
		if err != nil {
			errors[paneData.ID] = err.Error()
			continue
		}
		results[paneData.ID] = html
	}

	response := gin.H{"fragments": results}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GeneratePreviewFromPayload request",
		"duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true, "paneCount", len(req.Panes))

	c.JSON(http.StatusOK, response)
}
