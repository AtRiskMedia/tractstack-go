// Package handlers provides HTTP handlers for belief endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	persistence "github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// BeliefIDsRequest represents the request body for bulk belief loading
type BeliefIDsRequest struct {
	BeliefIDs []string `json:"beliefIds" binding:"required"`
}

// GetAllBeliefIDsHandler returns all belief IDs using cache-first pattern
func GetAllBeliefIDsHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	beliefRepo := persistence.NewBeliefRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	beliefService := services.NewBeliefService(beliefRepo)

	beliefIDs, err := beliefService.GetAllIDs(tenantCtx.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"beliefIds": beliefIDs,
		"count":     len(beliefIDs),
	})
}

// GetBeliefsByIDsHandler returns multiple beliefs by IDs using cache-first pattern
func GetBeliefsByIDsHandler(c *gin.Context) {
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

	beliefRepo := persistence.NewBeliefRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	beliefService := services.NewBeliefService(beliefRepo)

	beliefs, err := beliefService.GetByIDs(tenantCtx.TenantID, req.BeliefIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"beliefs": beliefs,
		"count":   len(beliefs),
	})
}

// GetBeliefByIDHandler returns a specific belief by ID using cache-first pattern
func GetBeliefByIDHandler(c *gin.Context) {
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

	beliefRepo := persistence.NewBeliefRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	beliefService := services.NewBeliefService(beliefRepo)

	beliefNode, err := beliefService.GetByID(tenantCtx.TenantID, beliefID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if beliefNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "belief not found"})
		return
	}

	c.JSON(http.StatusOK, beliefNode)
}

// GetBeliefBySlugHandler returns a specific belief by slug using cache-first pattern
func GetBeliefBySlugHandler(c *gin.Context) {
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

	beliefRepo := persistence.NewBeliefRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	beliefService := services.NewBeliefService(beliefRepo)

	beliefNode, err := beliefService.GetBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if beliefNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "belief not found"})
		return
	}

	c.JSON(http.StatusOK, beliefNode)
}

// CreateBeliefHandler creates a new belief
func CreateBeliefHandler(c *gin.Context) {
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

	beliefRepo := persistence.NewBeliefRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	beliefService := services.NewBeliefService(beliefRepo)

	err := beliefService.Create(tenantCtx.TenantID, &belief)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "belief created successfully",
		"beliefId": belief.ID,
	})
}

// UpdateBeliefHandler updates an existing belief
func UpdateBeliefHandler(c *gin.Context) {
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

	beliefRepo := persistence.NewBeliefRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	beliefService := services.NewBeliefService(beliefRepo)

	err := beliefService.Update(tenantCtx.TenantID, &belief)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "belief updated successfully",
		"beliefId": belief.ID,
	})
}

// DeleteBeliefHandler deletes a belief
func DeleteBeliefHandler(c *gin.Context) {
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

	beliefRepo := persistence.NewBeliefRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	beliefService := services.NewBeliefService(beliefRepo)

	err := beliefService.Delete(tenantCtx.TenantID, beliefID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "belief deleted successfully",
		"beliefId": beliefID,
	})
}
