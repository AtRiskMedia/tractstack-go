// Package handlers provides HTTP handlers for epinet endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// EpinetIDsRequest represents the request body for bulk epinet loading
type EpinetIDsRequest struct {
	EpinetIDs []string `json:"epinetIds" binding:"required"`
}

// EpinetHandlers contains all epinet-related HTTP handlers
type EpinetHandlers struct {
	epinetService *services.EpinetService
	logger        *logging.ChanneledLogger
}

// NewEpinetHandlers creates epinet handlers with injected dependencies
func NewEpinetHandlers(epinetService *services.EpinetService, logger *logging.ChanneledLogger) *EpinetHandlers {
	return &EpinetHandlers{
		epinetService: epinetService,
		logger:        logger,
	}
}

// GetAllEpinetIDs returns all epinet IDs using cache-first pattern
func (h *EpinetHandlers) GetAllEpinetIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get all epinet IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	epinetIDs, err := h.epinetService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get all epinet IDs request completed", "count", len(epinetIDs), "duration", time.Since(start))

	c.JSON(http.StatusOK, gin.H{
		"epinetIds": epinetIDs,
		"count":     len(epinetIDs),
	})
}

// GetEpinetsByIDs returns multiple epinets by IDs using cache-first pattern
func (h *EpinetHandlers) GetEpinetsByIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get epinets by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req EpinetIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.EpinetIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "epinetIds array cannot be empty"})
		return
	}

	epinets, err := h.epinetService.GetByIDs(tenantCtx, req.EpinetIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get epinets by IDs request completed", "requestedCount", len(req.EpinetIDs), "foundCount", len(epinets), "duration", time.Since(start))

	c.JSON(http.StatusOK, gin.H{
		"epinets": epinets,
		"count":   len(epinets),
	})
}

// GetEpinetByID returns a specific epinet by ID using cache-first pattern
func (h *EpinetHandlers) GetEpinetByID(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get epinet by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "epinetId", c.Param("id"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	epinetID := c.Param("id")
	if epinetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "epinet ID is required"})
		return
	}

	epinetNode, err := h.epinetService.GetByID(tenantCtx, epinetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if epinetNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "epinet not found"})
		return
	}

	h.logger.Content().Info("Get epinet by ID request completed", "epinetId", epinetID, "found", epinetNode != nil, "duration", time.Since(start))

	c.JSON(http.StatusOK, epinetNode)
}
