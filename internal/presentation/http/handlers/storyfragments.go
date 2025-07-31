// Package handlers provides HTTP handlers for storyfragment endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// StoryFragmentIDsRequest represents the request body for bulk storyfragment loading
type StoryFragmentIDsRequest struct {
	StoryFragmentIDs []string `json:"storyFragmentIds" binding:"required"`
}

// GetAllStoryFragmentIDsHandler returns all storyfragment IDs using cache-first pattern
func GetAllStoryFragmentIDsHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	storyFragmentRepo := content.NewStoryFragmentRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuRepo := content.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	storyFragmentService := services.NewStoryFragmentService(storyFragmentRepo, tractStackRepo, menuRepo, paneRepo)

	storyFragmentIDs, err := storyFragmentService.GetAllIDs(tenantCtx.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"storyFragmentIds": storyFragmentIDs,
		"count":            len(storyFragmentIDs),
	})
}

// GetStoryFragmentsByIDsHandler returns multiple storyfragments by IDs using cache-first pattern
func GetStoryFragmentsByIDsHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	var req StoryFragmentIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.StoryFragmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyFragmentIds array cannot be empty"})
		return
	}

	storyFragmentRepo := content.NewStoryFragmentRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuRepo := content.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	storyFragmentService := services.NewStoryFragmentService(storyFragmentRepo, tractStackRepo, menuRepo, paneRepo)

	storyFragments, err := storyFragmentService.GetByIDs(tenantCtx.TenantID, req.StoryFragmentIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"storyFragments": storyFragments,
		"count":          len(storyFragments),
	})
}

// GetStoryFragmentByIDHandler returns a specific storyfragment by ID using cache-first pattern
func GetStoryFragmentByIDHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	storyFragmentID := c.Param("id")
	if storyFragmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment ID is required"})
		return
	}

	storyFragmentRepo := content.NewStoryFragmentRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuRepo := content.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	storyFragmentService := services.NewStoryFragmentService(storyFragmentRepo, tractStackRepo, menuRepo, paneRepo)

	storyFragmentNode, err := storyFragmentService.GetByID(tenantCtx.TenantID, storyFragmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	c.JSON(http.StatusOK, storyFragmentNode)
}

// GetStoryFragmentBySlugHandler returns a specific storyfragment by slug using cache-first pattern
func GetStoryFragmentBySlugHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment slug is required"})
		return
	}

	storyFragmentRepo := content.NewStoryFragmentRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuRepo := content.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	storyFragmentService := services.NewStoryFragmentService(storyFragmentRepo, tractStackRepo, menuRepo, paneRepo)

	storyFragmentNode, err := storyFragmentService.GetBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	c.JSON(http.StatusOK, storyFragmentNode)
}

// GetStoryFragmentFullPayloadBySlugHandler returns a storyfragment with full editorial payload
func GetStoryFragmentFullPayloadBySlugHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment slug is required"})
		return
	}

	storyFragmentRepo := content.NewStoryFragmentRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuRepo := content.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	storyFragmentService := services.NewStoryFragmentService(storyFragmentRepo, tractStackRepo, menuRepo, paneRepo)

	payload, err := storyFragmentService.GetFullPayloadBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if payload == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	c.JSON(http.StatusOK, payload)
}

// GetHomeStoryFragmentHandler returns the home storyfragment using cache-first pattern
func GetHomeStoryFragmentHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	storyFragmentRepo := content.NewStoryFragmentRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	tractStackRepo := content.NewTractStackRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	menuRepo := content.NewMenuRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	paneRepo := content.NewPaneRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	storyFragmentService := services.NewStoryFragmentService(storyFragmentRepo, tractStackRepo, menuRepo, paneRepo)

	homeStoryFragment, err := storyFragmentService.GetHome(tenantCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if homeStoryFragment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "home storyfragment not found"})
		return
	}

	c.JSON(http.StatusOK, homeStoryFragment)
}
