// Package api provide storyfragment handlers
package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/services"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
)

// StoryFragmentIDsRequest represents the request body for bulk storyfragment loading
type StoryFragmentIDsRequest struct {
	StoryFragmentIDs []string `json:"storyFragmentIds" binding:"required"`
}

// extractAndWarmStoryfragmentMetadata extracts belief registry and codeHook targets, plus session warming for storyfragments
func extractAndWarmStoryfragmentMetadata(ctx *tenant.Context, storyFragmentNode *models.StoryFragmentNode, c *gin.Context) (gin.H, error) {
	storyFragmentID := storyFragmentNode.ID

	// Load all panes once for this storyfragment
	paneService := content.NewPaneService(ctx, nil)
	loadedPanes, err := paneService.GetByIDs(storyFragmentNode.PaneIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load panes: %w", err)
	}

	homeSlug := ctx.Config.HomeSlug
	if homeSlug == "" {
		homeSlug = "hello" // Same fallback as GetHomeStoryFragmentHandler
	}
	storyFragmentNode.IsHome = (storyFragmentNode.Slug == homeSlug)

	// Load menu if MenuID exists
	if storyFragmentNode.MenuID != nil {
		menuService := content.NewMenuService(ctx, cache.GetGlobalManager())
		menuData, err := menuService.GetByID(*storyFragmentNode.MenuID)
		if err != nil {
			log.Printf("Failed to load menu %s for storyfragment %s: %v",
				*storyFragmentNode.MenuID, storyFragmentNode.ID, err)
		} else {
			storyFragmentNode.Menu = menuData
		}
	}

	// Extract and cache belief registry using loaded panes
	beliefRegistryService := services.NewBeliefRegistryService(ctx)
	registry, err := beliefRegistryService.BuildRegistryFromLoadedPanes(storyFragmentID, loadedPanes)
	if err != nil {
		// Log the error but don't fail the request - belief registry is optional
		log.Printf("Failed to extract belief registry for storyfragment %s: %v", storyFragmentID, err)
	}

	// Extract codeHook targets from the same loaded panes
	codeHookTargets := make(map[string]string)
	for _, paneNode := range loadedPanes {
		if paneNode != nil && paneNode.CodeHookTarget != nil && *paneNode.CodeHookTarget != "" {
			codeHookTargets[paneNode.ID] = *paneNode.CodeHookTarget
		}
	}
	// Populate codeHook targets in the storyfragment node
	storyFragmentNode.CodeHookTargets = codeHookTargets

	// ===== SESSION BELIEF CONTEXT WARMING =====
	// Extract session ID from header
	sessionID := c.GetHeader("X-TractStack-Session-ID")
	if sessionID != "" {
		// Retrieve user beliefs from session -> fingerprint -> fingerprint state
		userBeliefs := make(map[string][]string)

		// Get session data to find fingerprint ID
		if sessionData, exists := cache.GetGlobalManager().GetSession(ctx.TenantID, sessionID); exists {
			// Get fingerprint state to extract user beliefs
			if fingerprintState, exists := cache.GetGlobalManager().GetFingerprintState(ctx.TenantID, sessionData.FingerprintID); exists {
				userBeliefs = fingerprintState.HeldBeliefs
			}
		}

		// Create and cache session belief context for this session + storyfragment combination
		sessionBeliefContext := &models.SessionBeliefContext{
			TenantID:        ctx.TenantID,
			SessionID:       sessionID,
			StoryfragmentID: storyFragmentID,
			UserBeliefs:     userBeliefs,
			LastEvaluation:  time.Now().UTC(),
		}

		// Cache the session belief context for subsequent pane requests
		cache.GetGlobalManager().SetSessionBeliefContext(ctx.TenantID, sessionBeliefContext)

		log.Printf("DEBUG: Warmed session belief context for session %s on storyfragment %s with %d beliefs",
			sessionID, storyFragmentID, len(userBeliefs))
	}
	// ===== END SESSION BELIEF CONTEXT WARMING =====

	// Build belief registry info for response
	beliefInfo := gin.H{
		"hasBeliefs": false,
		"hasBadges":  false,
	}
	if registry != nil {
		beliefInfo["hasBeliefs"] = len(registry.RequiredBeliefs) > 0
		beliefInfo["hasBadges"] = len(registry.RequiredBadges) > 0
		beliefInfo["lastUpdated"] = registry.LastUpdated
	}

	return beliefInfo, nil
}

// GetAllStoryFragmentIDsHandler returns all storyfragment IDs using cache-first pattern
func GetAllStoryFragmentIDsHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Use cache-first storyfragment service with global cache manager
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragmentIDs, err := storyFragmentService.GetAllIDs()
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
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Parse request body
	var req StoryFragmentIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if len(req.StoryFragmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyFragmentIds array cannot be empty"})
		return
	}

	// Use cache-first storyfragment service with global cache manager
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragments, err := storyFragmentService.GetByIDs(req.StoryFragmentIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Extract metadata for each storyfragment
	for _, storyFragmentNode := range storyFragments {
		_, err := extractAndWarmStoryfragmentMetadata(ctx, storyFragmentNode, c)
		if err != nil {
			log.Printf("Failed to extract metadata for storyfragment %s: %v", storyFragmentNode.ID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"storyFragments": storyFragments,
		"count":          len(storyFragments),
	})
}

// GetStoryFragmentByIDHandler returns a specific storyfragment by ID using cache-first pattern
// UPGRADED WITH SESSION BELIEF CONTEXT WARMING
func GetStoryFragmentByIDHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	storyFragmentID := c.Param("id")
	if storyFragmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment ID is required"})
		return
	}

	// Use cache-first storyfragment service with global cache manager
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragmentNode, err := storyFragmentService.GetByID(storyFragmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	// Extract metadata and warm session context
	beliefInfo, err := extractAndWarmStoryfragmentMetadata(ctx, storyFragmentNode, c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Modify the final response to include belief info
	c.JSON(http.StatusOK, gin.H{
		"storyFragment":  storyFragmentNode,
		"beliefRegistry": beliefInfo,
	})
}

// GetStoryFragmentBySlugHandler returns a specific storyfragment by slug using cache-first pattern
func GetStoryFragmentBySlugHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment slug is required"})
		return
	}

	// Use cache-first storyfragment service with global cache manager
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragmentNode, err := storyFragmentService.GetBySlug(slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	// Extract metadata and warm session context
	_, err = extractAndWarmStoryfragmentMetadata(ctx, storyFragmentNode, c)
	if err != nil {
		log.Printf("Failed to extract metadata for storyfragment %s: %v", storyFragmentNode.ID, err)
	}

	c.JSON(http.StatusOK, storyFragmentNode)
}

// GetHomeStoryFragmentHandler returns the home storyfragment metadata based on tenant's HOME_SLUG configuration
// Returns same JSON structure as GetStoryFragmentBySlugHandler
func GetHomeStoryFragmentHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get HOME_SLUG from tenant configuration
	homeSlug := ctx.Config.HomeSlug
	if homeSlug == "" {
		// Fallback to default if not configured
		homeSlug = "hello"
	}

	// Use cache-first storyfragment service to get home storyfragment
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragmentNode, err := storyFragmentService.GetBySlug(homeSlug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load home storyfragment: %v", err)})
		return
	}

	if storyFragmentNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("home storyfragment not found: %s", homeSlug)})
		return
	}

	// Extract metadata and warm session context
	_, err = extractAndWarmStoryfragmentMetadata(ctx, storyFragmentNode, c)
	if err != nil {
		log.Printf("Failed to extract metadata for storyfragment %s: %v", storyFragmentNode.ID, err)
	}

	// Return same JSON structure as GetStoryFragmentBySlugHandler
	c.JSON(http.StatusOK, storyFragmentNode)
}
