// Package handlers provides HTTP handlers for tailwind endpoints
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

// TailwindClassesRequest represents the request body for getting tailwind classes
type TailwindClassesRequest struct {
	ExcludePaneIDs []string `json:"excludePaneIds" binding:"required"`
}

// TailwindUpdateRequest represents the request body for updating CSS files
type TailwindUpdateRequest struct {
	FrontendCSS string `json:"frontendCss" binding:"required"`
}

// TailwindHandlers contains all tailwind-related HTTP handlers
type TailwindHandlers struct {
	tailwindService *services.TailwindService
	logger          *logging.ChanneledLogger
	perfTracker     *performance.Tracker
}

// NewTailwindHandlers creates tailwind handlers with injected dependencies
func NewTailwindHandlers(tailwindService *services.TailwindService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *TailwindHandlers {
	return &TailwindHandlers{
		tailwindService: tailwindService,
		logger:          logger,
		perfTracker:     perfTracker,
	}
}

// GetTailwindClasses handles POST /api/v1/tailwind/classes
func (h *TailwindHandlers) GetTailwindClasses(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_tailwind_classes_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received get tailwind classes request", "method", c.Request.Method, "path", c.Request.URL.Path)

	var req TailwindClassesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	classes, err := h.tailwindService.GetTailwindClasses(tenantCtx, req.ExcludePaneIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.System().Info("Get tailwind classes request completed", "excludedPanes", len(req.ExcludePaneIDs), "classCount", len(classes), "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetTailwindClasses request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"classes": classes,
		"count":   len(classes),
	})
}

// UpdateTailwindCSS handles POST /api/v1/tailwind/update
func (h *TailwindHandlers) UpdateTailwindCSS(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("update_tailwind_css_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received update tailwind CSS request", "method", c.Request.Method, "path", c.Request.URL.Path)

	var req TailwindUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	stylesVer, err := h.tailwindService.UpdateTailwindCSS(tenantCtx, req.FrontendCSS)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.System().Info("Update tailwind CSS request completed", "frontendSize", len(req.FrontendCSS), "stylesVer", stylesVer, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for UpdateTailwindCSS request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"stylesVer": stylesVer,
	})
}
