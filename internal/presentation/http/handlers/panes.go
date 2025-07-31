// Package handlers provides HTTP handlers for pane endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// PaneIDsRequest represents the request body for bulk pane loading
type PaneIDsRequest struct {
	PaneIDs []string `json:"paneIds" binding:"required"`
}

// GetAllPaneIDsHandler returns all pane IDs using cache-first pattern
func GetAllPaneIDsHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneService := services.NewPaneService(paneRepo)

	paneIDs, err := paneService.GetAllIDs(tenantCtx.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"paneIds": paneIDs,
		"count":   len(paneIDs),
	})
}

// GetPanesByIDsHandler returns multiple panes by IDs using cache-first pattern
func GetPanesByIDsHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req PaneIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.PaneIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paneIds array cannot be empty"})
		return
	}

	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneService := services.NewPaneService(paneRepo)

	panes, err := paneService.GetByIDs(tenantCtx.TenantID, req.PaneIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"panes": panes,
		"count": len(panes),
	})
}

// GetPaneByIDHandler returns a specific pane by ID using cache-first pattern
func GetPaneByIDHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	paneID := c.Param("id")
	if paneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pane ID is required"})
		return
	}

	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneService := services.NewPaneService(paneRepo)

	paneNode, err := paneService.GetByID(tenantCtx.TenantID, paneID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if paneNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pane not found"})
		return
	}

	c.JSON(http.StatusOK, paneNode)
}

// GetPaneBySlugHandler returns a specific pane by slug using cache-first pattern
func GetPaneBySlugHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pane slug is required"})
		return
	}

	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneService := services.NewPaneService(paneRepo)

	paneNode, err := paneService.GetBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if paneNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pane not found"})
		return
	}

	c.JSON(http.StatusOK, paneNode)
}

// GetContextPanesHandler returns all context panes using cache-first pattern
func GetContextPanesHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneService := services.NewPaneService(paneRepo)

	contextPanes, err := paneService.GetContextPanes(tenantCtx.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"contextPanes": contextPanes,
		"count":        len(contextPanes),
	})
}
