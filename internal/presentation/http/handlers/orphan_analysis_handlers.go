// Package handlers provides HTTP handlers for orphan analysis endpoints
package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// OrphanAnalysisHandlers contains all orphan analysis-related HTTP handlers
type OrphanAnalysisHandlers struct {
	orphanAnalysisService *services.OrphanAnalysisService
	logger                *logging.ChanneledLogger
}

// NewOrphanAnalysisHandlers creates orphan analysis handlers with injected dependencies
func NewOrphanAnalysisHandlers(orphanAnalysisService *services.OrphanAnalysisService, logger *logging.ChanneledLogger) *OrphanAnalysisHandlers {
	return &OrphanAnalysisHandlers{
		orphanAnalysisService: orphanAnalysisService,
		logger:                logger,
	}
}

// GetOrphanAnalysis handles GET /api/v1/admin/orphan-analysis
func (h *OrphanAnalysisHandlers) GetOrphanAnalysis(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get orphan analysis request", "method", c.Request.Method, "path", c.Request.URL.Path)
	// TODO: Add admin authentication middleware to protect this route
	log.Println("************** WARNING --> ROUTE NEEDS TO BE PROTECTED")

	// Get tenant context from middleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

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

	c.JSON(http.StatusOK, payload)
}
