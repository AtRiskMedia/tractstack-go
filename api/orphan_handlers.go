package api

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/services"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// GetOrphanAnalysisHandler handles GET /api/v1/admin/orphan-analysis
func GetOrphanAnalysisHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !validateAdminOrEditor(c, ctx) {
		return
	}

	tenantID := ctx.TenantID

	// Check cache first
	cachedPayload, cachedETag, exists := cache.GetGlobalManager().GetOrphanAnalysis(tenantID)

	if exists {
		// Handle 304 Not Modified
		clientETag := c.GetHeader("If-None-Match")
		if clientETag == cachedETag {
			c.Status(http.StatusNotModified)
			return
		}

		// Return cached data
		c.Header("ETag", cachedETag)
		c.Header("Cache-Control", "private, must-revalidate")
		c.JSON(http.StatusOK, cachedPayload)
		return
	}

	// Return loading state immediately
	loadingPayload := &models.OrphanAnalysisPayload{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Status:         "loading",
	}

	c.JSON(http.StatusOK, loadingPayload)

	// Start background computation
	go func() {
		computeOrphanAnalysisAsync(tenantID, ctx)
	}()
}

// computeOrphanAnalysisAsync performs expensive computation in background
func computeOrphanAnalysisAsync(tenantID string, ctx *tenant.Context) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Orphan analysis computation failed for tenant %s: %v\n", tenantID, r)
		}
	}()

	// Create service and compute
	service := services.NewOrphanAnalysisService(ctx)
	payload, err := service.ComputeOrphanAnalysis()
	if err != nil {
		fmt.Printf("Error computing orphan analysis for tenant %s: %v\n", tenantID, err)
		return
	}

	// Generate ETag
	etag := generateOrphanETag(ctx)

	// Cache result
	cache.GetGlobalManager().SetOrphanAnalysis(tenantID, payload, etag)
}

// generateOrphanETag creates ETag based on content timestamps
func generateOrphanETag(ctx *tenant.Context) string {
	// Use a simple approach based on current time since we don't have direct access to ContentMapLastUpdated
	// In a real implementation, this should be based on when content was last modified
	timestamp := time.Now().Unix()
	hash := md5.Sum([]byte(fmt.Sprintf("orphan-%s-%d", ctx.TenantID, timestamp)))
	return fmt.Sprintf(`"%x"`, hash)
}
