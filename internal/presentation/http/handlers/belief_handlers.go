// Package handlers provides HTTP handlers for belief endpoints
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// BeliefIDsRequest represents the request body for bulk belief loading
type BeliefIDsRequest struct {
	BeliefIDs []string `json:"beliefIds" binding:"required"`
}

// BeliefHandlers contains all belief-related HTTP handlers
type BeliefHandlers struct {
	beliefService *services.BeliefService
	logger        *logging.ChanneledLogger
}

// NewBeliefHandlers creates belief handlers with injected dependencies
func NewBeliefHandlers(beliefService *services.BeliefService, logger *logging.ChanneledLogger) *BeliefHandlers {
	return &BeliefHandlers{
		beliefService: beliefService,
		logger:        logger,
	}
}

// GetAllBeliefIDs returns all belief IDs using cache-first pattern
func (h *BeliefHandlers) GetAllBeliefIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get all belief IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	beliefIDs, err := h.beliefService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get all belief IDs request completed", "count", len(beliefIDs), "duration", time.Since(start))

	c.JSON(http.StatusOK, gin.H{
		"beliefIds": beliefIDs,
		"count":     len(beliefIDs),
	})
}

// GetBeliefsByIDs returns multiple beliefs by IDs using cache-first pattern
func (h *BeliefHandlers) GetBeliefsByIDs(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get beliefs by IDs request", "method", c.Request.Method, "path", c.Request.URL.Path)
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req BeliefIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.BeliefIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "beliefIds array cannot be empty"})
		return
	}

	beliefs, err := h.beliefService.GetByIDs(tenantCtx, req.BeliefIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.Content().Info("Get beliefs by IDs request completed", "requestedCount", len(req.BeliefIDs), "foundCount", len(beliefs), "duration", time.Since(start))
	c.JSON(http.StatusOK, gin.H{
		"beliefs": beliefs,
		"count":   len(beliefs),
	})
}

// GetBeliefByID returns a specific belief by ID using cache-first pattern
func (h *BeliefHandlers) GetBeliefByID(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get belief by ID request", "method", c.Request.Method, "path", c.Request.URL.Path, "beliefId", c.Param("id"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	beliefID := c.Param("id")
	if beliefID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "belief ID is required"})
		return
	}

	beliefNode, err := h.beliefService.GetByID(tenantCtx, beliefID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if beliefNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "belief not found"})
		return
	}

	h.logger.Content().Info("Get belief by ID request completed", "beliefId", beliefID, "found", beliefNode != nil, "duration", time.Since(start))

	c.JSON(http.StatusOK, beliefNode)
}

// GetBeliefBySlug returns a specific belief by slug using cache-first pattern
func (h *BeliefHandlers) GetBeliefBySlug(c *gin.Context) {
	start := time.Now()
	h.logger.Content().Debug("Received get belief by slug request", "method", c.Request.Method, "path", c.Request.URL.Path, "slug", c.Param("slug"))
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "belief slug is required"})
		return
	}

	beliefNode, err := h.beliefService.GetBySlug(tenantCtx, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if beliefNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "belief not found"})
		return
	}

	h.logger.Content().Info("Get belief by slug request completed", "slug", slug, "found", beliefNode != nil, "duration", time.Since(start))

	c.JSON(http.StatusOK, beliefNode)
}

// CreateBelief creates a new belief
func (h *BeliefHandlers) CreateBelief(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var belief content.BeliefNode
	if err := c.ShouldBindJSON(&belief); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	err := h.beliefService.Create(tenantCtx, &belief)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "belief created successfully",
		"beliefId": belief.ID,
	})
}

// UpdateBelief updates an existing belief
func (h *BeliefHandlers) UpdateBelief(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	beliefID := c.Param("id")
	if beliefID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "belief ID is required"})
		return
	}

	var belief content.BeliefNode
	if err := c.ShouldBindJSON(&belief); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Ensure ID matches URL parameter
	belief.ID = beliefID

	err := h.beliefService.Update(tenantCtx, &belief)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "belief updated successfully",
		"beliefId": belief.ID,
	})
}

// DeleteBelief deletes a belief
func (h *BeliefHandlers) DeleteBelief(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	beliefID := c.Param("id")
	if beliefID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "belief ID is required"})
		return
	}

	err := h.beliefService.Delete(tenantCtx, beliefID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "belief deleted successfully",
		"beliefId": beliefID,
	})
}
