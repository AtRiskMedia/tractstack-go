// Package handlers provides HTTP handlers for orphan analysis endpoints
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

// OrphanAnalysisHandlers contains all orphan analysis-related HTTP handlers
type OrphanAnalysisHandlers struct {
	orphanAnalysisService *services.OrphanAnalysisService
	logger                *logging.ChanneledLogger
	perfTracker           *performance.Tracker
}

// NewOrphanAnalysisHandlers creates orphan analysis handlers with injected dependencies
func NewOrphanAnalysisHandlers(orphanAnalysisService *services.OrphanAnalysisService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *OrphanAnalysisHandlers {
	return &OrphanAnalysisHandlers{
		orphanAnalysisService: orphanAnalysisService,
		logger:                logger,
		perfTracker:           perfTracker,
	}
}

// GetOrphanAnalysis handles GET /api/v1/admin/orphan-analysis
func (h *OrphanAnalysisHandlers) GetOrphanAnalysis(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("orphan_analysis_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Content().Debug("Received get orphan analysis request", "method", c.Request.Method, "path", c.Request.URL.Path)

	// Get client's ETag for cache validation
	clientETag := c.GetHeader("If-None-Match")
	payload, etag, err := h.orphanAnalysisService.GetOrphanAnalysis(tenantCtx, clientETag, tenantCtx.CacheManager)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Set ETag header if available
	if etag != "" {
		c.Header("ETag", etag)
		c.Header("Cache-Control", "private, must-revalidate")
	}

	h.logger.Content().Info("Get orphan analysis request completed", "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetOrphanAnalysis request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, payload)
}

// GetOrphanAnalysis wraps the tenant-specific orphan analysis endpoint for SysOp access
func (h *SysOpHandlers) GetOrphanAnalysis(c *gin.Context) {
	tenantID := c.Query("tenant")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant query parameter is required"})
		return
	}

	// Create tenant context for the requested tenant
	tenantCtx, err := h.container.TenantManager.NewContextFromID(tenantID)
	if err != nil {
		h.container.Logger.System().Error("SysOp failed to create context for orphan analysis", "error", err, "tenantId", tenantID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found or could not be initialized"})
		return
	}
	defer tenantCtx.Close()

	// Get client's ETag for cache validation
	clientETag := c.GetHeader("If-None-Match")

	// Call the orphan analysis service directly (same as the normal endpoint)
	payload, etag, err := h.container.OrphanAnalysisService.GetOrphanAnalysis(tenantCtx, clientETag, tenantCtx.CacheManager)
	if err != nil {
		h.container.Logger.System().Error("SysOp orphan analysis failed", "error", err, "tenantId", tenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Set ETag header if available
	if etag != "" {
		c.Header("ETag", etag)
		c.Header("Cache-Control", "private, must-revalidate")
	}

	h.container.Logger.System().Info("SysOp orphan analysis request completed", "tenantId", tenantID)
	c.JSON(http.StatusOK, payload)
}
