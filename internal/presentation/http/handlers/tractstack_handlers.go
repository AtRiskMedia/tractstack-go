// Package handlers provides HTTP handlers for tractstack endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// TractStackIDsRequest represents the request body for bulk tractstack loading
type TractStackIDsRequest struct {
	TractStackIDs []string `json:"tractStackIds" binding:"required"`
}

// TractStackHandlers contains all tractstack-related HTTP handlers
type TractStackHandlers struct {
	tractStackService *services.TractStackService
}

// NewTractStackHandlers creates tractstack handlers with injected dependencies
func NewTractStackHandlers(tractStackService *services.TractStackService) *TractStackHandlers {
	return &TractStackHandlers{
		tractStackService: tractStackService,
	}
}

// GetAllTractStackIDs returns all tractstack IDs using cache-first pattern
func (h *TractStackHandlers) GetAllTractStackIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	tractStackIDs, err := h.tractStackService.GetAllIDs(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tractStackIds": tractStackIDs,
		"count":         len(tractStackIDs),
	})
}

// GetTractStacksByIDs returns multiple tractstacks by IDs using cache-first pattern
func (h *TractStackHandlers) GetTractStacksByIDs(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req TractStackIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.TractStackIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractStackIds array cannot be empty"})
		return
	}

	tractStacks, err := h.tractStackService.GetByIDs(tenantCtx, req.TractStackIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tractStacks": tractStacks,
		"count":       len(tractStacks),
	})
}

// GetTractStackByID returns a specific tractstack by ID using cache-first pattern
func (h *TractStackHandlers) GetTractStackByID(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	tractStackID := c.Param("id")
	if tractStackID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractstack ID is required"})
		return
	}

	tractStackNode, err := h.tractStackService.GetByID(tenantCtx, tractStackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if tractStackNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tractstack not found"})
		return
	}

	c.JSON(http.StatusOK, tractStackNode)
}

// GetTractStackBySlug returns a specific tractstack by slug using cache-first pattern
func (h *TractStackHandlers) GetTractStackBySlug(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tractstack slug is required"})
		return
	}

	tractStackNode, err := h.tractStackService.GetBySlug(tenantCtx, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if tractStackNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tractstack not found"})
		return
	}

	c.JSON(http.StatusOK, tractStackNode)
}
