package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// FragmentHandlers handles HTTP requests for fragment endpoints
// This is a thin wrapper around FragmentService following the established pattern
type FragmentHandlers struct {
	fragmentService *services.FragmentService
	logger          *logging.ChanneledLogger
}

// NewFragmentHandlers creates a new fragment handlers instance
func NewFragmentHandlers(fragmentService *services.FragmentService, logger *logging.ChanneledLogger) *FragmentHandlers {
	return &FragmentHandlers{
		fragmentService: fragmentService,
		logger:          logger,
	}
}

// GetPaneFragment handles GET /api/v1/fragments/panes/:id
// This implements the exact API contract from legacy api/pane_fragment_handler.go
func (h *FragmentHandlers) GetPaneFragment(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get fragment request", "method", c.Request.Method, "path", c.Request.URL.Path)
	// Extract tenant context from middleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Tenant context not found"})
		return
	}

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

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// BatchFragmentRequest represents the request body for batch fragment operations
type BatchFragmentRequest struct {
	PaneIDs []string `json:"paneIds" binding:"required"`
}

// GetPaneFragmentBatch handles POST /api/v1/fragments/panes
// This implements batch fragment generation from legacy
func (h *FragmentHandlers) GetPaneFragmentBatch(c *gin.Context) {
	// Extract tenant context from middleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Tenant context not found"})
		return
	}

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

	c.JSON(http.StatusOK, response)
}
