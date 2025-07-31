// Package handlers provides HTTP handlers for epinet endpoints
package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	persistence "github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/content"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// EpinetIDsRequest represents the request body for bulk epinet loading
type EpinetIDsRequest struct {
	EpinetIDs []string `json:"epinetIds" binding:"required"`
}

// GetAllEpinetIDsHandler returns all epinet IDs using cache-first pattern
func GetAllEpinetIDsHandler(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	epinetRepo := persistence.NewEpinetRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	epinetService := services.NewEpinetService(epinetRepo)

	epinetIDs, err := epinetService.GetAllIDs(tenantCtx.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"epinetIds": epinetIDs,
		"count":     len(epinetIDs),
	})
}

// GetEpinetsByIDsHandler returns multiple epinets by IDs using cache-first pattern
func GetEpinetsByIDsHandler(c *gin.Context) {
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

	epinetRepo := persistence.NewEpinetRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	epinetService := services.NewEpinetService(epinetRepo)

	epinets, err := epinetService.GetByIDs(tenantCtx.TenantID, req.EpinetIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"epinets": epinets,
		"count":   len(epinets),
	})
}

// GetEpinetByIDHandler returns a specific epinet by ID using cache-first pattern
func GetEpinetByIDHandler(c *gin.Context) {
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

	epinetRepo := persistence.NewEpinetRepository(tenantCtx.Database.Conn, tenantCtx.CacheManager)
	epinetService := services.NewEpinetService(epinetRepo)

	epinetNode, err := epinetService.GetByID(tenantCtx.TenantID, epinetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if epinetNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "epinet not found"})
		return
	}

	c.JSON(http.StatusOK, epinetNode)
}
