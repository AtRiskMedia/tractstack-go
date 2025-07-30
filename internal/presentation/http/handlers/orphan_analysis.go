// Package handlers provides HTTP handlers for orphan analysis endpoints
package handlers

import (
	"log"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// OrphanAnalysisHandler handles orphan analysis HTTP requests
type OrphanAnalysisHandler struct {
	orphanAnalysisService *services.OrphanAnalysisService
}

// NewOrphanAnalysisHandler creates a new orphan analysis handler
func NewOrphanAnalysisHandler(orphanAnalysisService *services.OrphanAnalysisService) *OrphanAnalysisHandler {
	return &OrphanAnalysisHandler{
		orphanAnalysisService: orphanAnalysisService,
	}
}

// GetOrphanAnalysisHandler handles GET /api/v1/admin/orphan-analysis
func (h *OrphanAnalysisHandler) GetOrphanAnalysisHandler(c *gin.Context) {
	// ************** WARNING --> ROUTE NEEDS TO BE PROTECTED
	// TODO: Add admin authentication middleware to protect this route
	log.Println("************** WARNING --> ROUTE NEEDS TO BE PROTECTED")

	// Get tenant context from middleware
	tenantCtx, ok := middleware.GetTenantContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context required"})
		return
	}

	// Get client's ETag for cache validation
	clientETag := c.GetHeader("If-None-Match")

	// Get orphan analysis with async pattern
	payload, etag, err := h.orphanAnalysisService.GetOrphanAnalysis(tenantCtx.TenantID, clientETag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Set ETag header if available
	if etag != "" {
		c.Header("ETag", etag)
		c.Header("Cache-Control", "private, must-revalidate")
	}

	// Return orphan analysis data (either loading status or complete payload)
	c.JSON(http.StatusOK, payload)
}
