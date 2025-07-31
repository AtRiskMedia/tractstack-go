// Package handlers provides HTTP handlers for tractstack endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// TractStackIDsRequest represents the request body for bulk tractstack loading
type TractStackIDsRequest struct {
	TractStackIDs []string `json:"tractStackIds" binding:"required"`
}

// GetAllTractStackIDsHandler returns all tractstack IDs using cache-first pattern
func GetAllTractStackIDsHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackService := services.NewTractStackService(tractStackRepo)

	tractStackIDs, err := tractStackService.GetAllIDs(tenantCtx.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tractStackIds": tractStackIDs,
		"count":         len(tractStackIDs),
	})
}

// GetTractStacksByIDsHandler returns multiple tractstacks by IDs using cache-first pattern
func GetTractStacksByIDsHandler(c *gin.Context) {
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

	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackService := services.NewTractStackService(tractStackRepo)

	tractStacks, err := tractStackService.GetByIDs(tenantCtx.TenantID, req.TractStackIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tractStacks": tractStacks,
		"count":       len(tractStacks),
	})
}

// GetTractStackByIDHandler returns a specific tractstack by ID using cache-first pattern
func GetTractStackByIDHandler(c *gin.Context) {
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

	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackService := services.NewTractStackService(tractStackRepo)

	tractStackNode, err := tractStackService.GetByID(tenantCtx.TenantID, tractStackID)
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

// GetTractStackBySlugHandler returns a specific tractstack by slug using cache-first pattern
func GetTractStackBySlugHandler(c *gin.Context) {
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

	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackService := services.NewTractStackService(tractStackRepo)

	tractStackNode, err := tractStackService.GetBySlug(tenantCtx.TenantID, slug)
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
