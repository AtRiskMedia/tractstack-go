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
// NOW CACHE-FIRST: Only hits database if data not available in cache
func extractAndWarmStoryfragmentMetadata(ctx *tenant.Context, storyFragmentNode *models.StoryFragmentNode, c *gin.Context) (gin.H, error) {
	storyFragmentID := storyFragmentNode.ID

	homeSlug := ctx.Config.HomeSlug
	if homeSlug == "" {
		homeSlug = "hello" // Same fallback as GetHomeStoryFragmentHandler
	}
	storyFragmentNode.IsHome = (storyFragmentNode.Slug == homeSlug)

	// === CACHE-FIRST MENU LOADING ===
	if storyFragmentNode.MenuID != nil && storyFragmentNode.Menu == nil {
		menuService := content.NewMenuService(ctx, cache.GetGlobalManager())
		menuData, err := menuService.GetByID(*storyFragmentNode.MenuID)
		if err != nil {
			log.Printf("Failed to load menu %s for storyfragment %s: %v",
				*storyFragmentNode.MenuID, storyFragmentNode.ID, err)
		} else {
			storyFragmentNode.Menu = menuData
		}
	}

	// === CACHE-FIRST CODEHOOK TARGETS ===
	if storyFragmentNode.CodeHookTargets == nil {
		// Need to load panes to extract codeHook targets
		paneService := content.NewPaneService(ctx, cache.GetGlobalManager())
		loadedPanes, err := paneService.GetByIDs(storyFragmentNode.PaneIDs)
		if err != nil {
			log.Printf("CACHE-FIRST: Failed to load panes for codeHook extraction: %v", err)
		} else {
			codeHookTargets := make(map[string]string)
			extractedCount := 0

			for _, paneNode := range loadedPanes {
				if paneNode != nil && paneNode.CodeHookTarget != nil && *paneNode.CodeHookTarget != "" {
					codeHookTargets[paneNode.ID] = *paneNode.CodeHookTarget
					extractedCount++
				}
			}

			storyFragmentNode.CodeHookTargets = codeHookTargets
		}
	}

	// === CACHE-FIRST BELIEF REGISTRY ===
	beliefRegistryService := services.NewBeliefRegistryService(ctx)

	// Check if registry is already cached
	registry, err := beliefRegistryService.BuildRegistryFromLoadedPanes(storyFragmentID, nil)
	if err != nil {
		log.Printf("Failed to get belief registry for storyfragment %s: %v", storyFragmentID, err)
	}

	// === SESSION BELIEF CONTEXT WARMING ===
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
	}

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

// GetStoryFragmentFullPayloadBySlugHandler returns a complete V1-compatible payload for a storyfragment
// This includes the storyfragment, all its panes, extracted child nodes, tractstack, and menu data
func GetStoryFragmentFullPayloadBySlugHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Authentication - Admin OR Editor required
	if !validateAdminOrEditor(c, ctx) {
		return
	}

	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storyfragment slug is required"})
		return
	}

	// === STEP 1: Get StoryFragment ===
	storyFragmentService := content.NewStoryFragmentService(ctx, cache.GetGlobalManager())
	storyFragment, err := storyFragmentService.GetBySlug(slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if storyFragment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storyfragment not found"})
		return
	}

	// === STEP 2: Get TractStack ===
	tractStackService := content.NewTractStackService(ctx, cache.GetGlobalManager())
	tractStack, err := tractStackService.GetByID(storyFragment.TractStackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if tractStack == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tractstack not found"})
		return
	}

	// === STEP 3: Get All Panes ===
	var panes []*models.PaneNode
	if len(storyFragment.PaneIDs) > 0 {
		paneService := content.NewPaneService(ctx, cache.GetGlobalManager())
		panes, err = paneService.GetByIDs(storyFragment.PaneIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// === STEP 4: Extract Child Nodes and Clean Panes ===
	var allChildNodes []interface{}
	cleanedPanes := make([]*models.PaneNode, len(panes))

	for i, pane := range panes {
		if pane == nil {
			continue
		}

		// Extract child nodes from this pane's OptionsPayload
		if pane.OptionsPayload != nil {
			if nodes, exists := pane.OptionsPayload["nodes"]; exists {
				if nodesArray, ok := nodes.([]interface{}); ok {
					allChildNodes = append(allChildNodes, nodesArray...)
				}
			}
		}

		// Create cleaned pane (without embedded nodes)
		cleanedPane := *pane
		cleanedPane.OptionsPayload = make(map[string]interface{})

		// Copy all fields except "nodes"
		if pane.OptionsPayload != nil {
			for k, v := range pane.OptionsPayload {
				if k != "nodes" {
					cleanedPane.OptionsPayload[k] = v
				}
			}
		}

		cleanedPanes[i] = &cleanedPane
	}

	// === STEP 5: Get Menu (if exists) ===
	var menuNodes []*models.MenuNode
	if storyFragment.MenuID != nil {
		menuService := content.NewMenuService(ctx, cache.GetGlobalManager())
		menu, err := menuService.GetByID(*storyFragment.MenuID)
		if err != nil {
			log.Printf("Failed to load menu %s: %v", *storyFragment.MenuID, err)
		} else if menu != nil {
			menuNodes = []*models.MenuNode{menu}
		}
	}

	// === STEP 6: Build V1-Compatible Response ===
	response := gin.H{
		"storyfragmentNodes": []*models.StoryFragmentNode{storyFragment},
		"paneNodes":          cleanedPanes,
		"childNodes":         allChildNodes,
		"tractstackNodes":    []*models.TractStackNode{tractStack},
	}

	// Add menu nodes if they exist
	if len(menuNodes) > 0 {
		response["menuNodes"] = menuNodes
	}

	c.JSON(http.StatusOK, response)
}
