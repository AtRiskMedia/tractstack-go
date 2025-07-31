// Package handlers provides HTTP handlers for orphan analysis endpoints
package handlers

import (
	"log"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/bulk"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// GetOrphanAnalysisHandler handles GET /api/v1/admin/orphan-analysis
func GetOrphanAnalysisHandler(c *gin.Context) {
	// ************** WARNING --> ROUTE NEEDS TO BE PROTECTED
	// TODO: Add admin authentication middleware to protect this route
	log.Println("************** WARNING --> ROUTE NEEDS TO BE PROTECTED")

	// Get tenant context from middleware
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	// Create bulk repository for this request
	db := &database.DB{DB: tenantCtx.Database.Conn}
	bulkRepo := bulk.NewRepository(db)

	// Create service per-request with tenant's cache manager
	orphanAnalysisService := services.NewOrphanAnalysisService(bulkRepo)

	// Get client's ETag for cache validation
	clientETag := c.GetHeader("If-None-Match")

	// Get orphan analysis with async pattern using tenant's cache manager
	payload, etag, err := orphanAnalysisService.GetOrphanAnalysis(tenantCtx.TenantID, clientETag, tenantCtx.CacheManager)
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
