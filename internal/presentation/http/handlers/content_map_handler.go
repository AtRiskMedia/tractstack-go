// Package handlers provides HTTP handlers for content map endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// ContentMapHandlers contains all content map-related HTTP handlers
type ContentMapHandlers struct {
	contentMapService *services.ContentMapService
	logger            *logging.ChanneledLogger
}

// NewContentMapHandlers creates content map handlers with injected dependencies
func NewContentMapHandlers(contentMapService *services.ContentMapService, logger *logging.ChanneledLogger) *ContentMapHandlers {
	return &ContentMapHandlers{
		contentMapService: contentMapService,
		logger:            logger,
	}
}

func (h *ContentMapHandlers) GetContentMap(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get content map request", "method", c.Request.Method, "path", c.Request.URL.Path)
	// Get tenant context from middleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Get client's lastUpdated parameter for timestamp comparison
	clientLastUpdated := c.Query("lastUpdated")
	response, notModified, err := h.contentMapService.GetContentMap(tenantCtx, clientLastUpdated, tenantCtx.CacheManager)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Handle 304 Not Modified
	if notModified {
		c.Status(http.StatusNotModified)
		return
	}

	h.logger.Content().Info("Get content map request completed", "itemCount", len(response.Data), "duration", time.Since(start))

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"data":        response.Data,
			"lastUpdated": response.LastUpdated,
		},
	})
}
