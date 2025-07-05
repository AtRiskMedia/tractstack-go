// Package api provides HTTP handlers for fragment rendering endpoints
package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/html"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
)

// GetPaneFragmentHandler returns HTML fragments for individual panes with personalization
func GetPaneFragmentHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	paneID := c.Param("id")
	if paneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pane ID is required"})
		return
	}

	// Extract session ID from header (sent by frontend)
	sessionID := c.GetHeader("X-TractStack-Session-ID")

	// Extract storyfragment ID from header (sent by HTMX)
	storyfragmentID := c.GetHeader("X-StoryFragment-ID")

	// ===== END SESSION CONTEXT EXTRACTION =====

	// Use cache-first pane service
	paneService := content.NewPaneService(ctx, cache.GetGlobalManager())
	paneNode, err := paneService.GetByID(paneID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if paneNode == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pane not found"})
		return
	}

	// Extract and parse nodes from optionsPayload
	nodesData, parentChildMap, err := html.ExtractNodesFromPane(paneNode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse pane nodes: %v", err)})
		return
	}

	// Add the pane itself to the nodes data structure
	paneNodeData := &models.NodeRenderData{
		ID:       paneID,
		NodeType: "Pane",
		PaneData: &models.PaneRenderData{
			Title:           paneNode.Title,
			Slug:            paneNode.Slug,
			IsDecorative:    paneNode.IsDecorative,
			BgColour:        extractBgColour(paneNode),
			HeldBeliefs:     paneNode.HeldBeliefs,     // Now matches []string type
			WithheldBeliefs: paneNode.WithheldBeliefs, // Now matches []string type
			CodeHookTarget:  paneNode.CodeHookTarget,
			CodeHookPayload: paneNode.CodeHookPayload,
		},
	}
	nodesData[paneID] = paneNodeData

	// Create render context with real data
	renderCtx := &models.RenderContext{
		AllNodes:        nodesData,
		ParentNodes:     parentChildMap,
		TenantID:        ctx.TenantID,
		SessionID:       sessionID,
		StoryfragmentID: storyfragmentID,
	}

	// Create HTML generator
	generator := html.NewGenerator(renderCtx)

	// ===== GENERATE BASE HTML (CACHE-FIRST WITH WIDGET BYPASS) =====
	// Check if we should bypass cache due to widgets
	var htmlContent string
	if shouldBypassCacheForWidgets(ctx, paneID, sessionID, storyfragmentID) {
		// Generate fresh HTML with sessionID for widget state
		htmlContent = generator.Render(paneID)
	} else {
		// Check cache first for non-personalized content
		variant := models.PaneVariantDefault
		if cachedHTML, exists := cache.GetGlobalManager().GetHTMLChunk(ctx.TenantID, paneID, variant); exists {
			htmlContent = cachedHTML
			// fmt.Printf("DEBUG: Cache HIT for pane %s\n", paneID)
		} else {
			htmlContent = generator.Render(paneID)
			// Cache the generated HTML
			cache.GetGlobalManager().SetHTMLChunk(ctx.TenantID, paneID, variant, htmlContent, []string{paneID})
			// fmt.Printf("DEBUG: Cache MISS for pane %s - generated and cached\n", paneID)
		}
	}
	// fmt.Printf("DEBUG: Generated base HTML for pane %s (%d chars)\n", paneID, len(htmlContent))
	// ===== END BASE HTML GENERATION =====

	// Check for session belief context and apply personalization if available
	if sessionID != "" && storyfragmentID != "" {
		fmt.Printf("DEBUG: Attempting personalization for session %s, storyfragment %s, pane %s\n",
			sessionID, storyfragmentID, paneID)

		// Get cached session belief context
		if sessionContext, exists := cache.GetGlobalManager().GetSessionBeliefContext(ctx.TenantID, sessionID, storyfragmentID); exists {
			fmt.Printf("DEBUG: Found session belief context with %d beliefs\n", len(sessionContext.UserBeliefs))

			// Get pane belief requirements from registry
			if beliefRegistry, exists := cache.GetGlobalManager().GetStoryfragmentBeliefRegistry(ctx.TenantID, storyfragmentID); exists {
				fmt.Printf("DEBUG: Found belief registry with %d panes\n", len(beliefRegistry.PaneBeliefPayloads))

				if paneBeliefs, exists := beliefRegistry.PaneBeliefPayloads[paneID]; exists {
					fmt.Printf("DEBUG: Found pane beliefs - held: %d, withheld: %d, matchAcross: %d\n",
						len(paneBeliefs.HeldBeliefs), len(paneBeliefs.WithheldBeliefs), len(paneBeliefs.MatchAcross))

					// Create belief evaluation engine
					beliefEngine := content.NewBeliefEvaluationEngine()

					// Evaluate visibility for this pane based on user beliefs
					visibility := beliefEngine.EvaluatePaneVisibility(paneBeliefs, sessionContext.UserBeliefs)

					// Apply visibility wrapper to HTML content
					htmlContent = beliefEngine.ApplyVisibilityWrapper(htmlContent, visibility)

					fmt.Printf("DEBUG: Applied personalization - pane %s visibility: %s\n", paneID, visibility)
				} else {
					fmt.Printf("DEBUG: No belief requirements found for pane %s\n", paneID)
				}
			} else {
				fmt.Printf("DEBUG: No belief registry found for storyfragment %s\n", storyfragmentID)
			}
		} else {
			fmt.Printf("DEBUG: No session belief context found for session %s, storyfragment %s\n", sessionID, storyfragmentID)

			// NEW: Instead of giving up, try to create session context if user has relevant beliefs
			if shouldCreateSessionContext(ctx, sessionID, storyfragmentID) {
				fmt.Printf("DEBUG: User has relevant beliefs, creating session context\n")
				newContext := createSessionBeliefContext(ctx, sessionID, storyfragmentID)
				cache.GetGlobalManager().SetSessionBeliefContext(ctx.TenantID, newContext)

				// Now try personalization with the new context
				if beliefRegistry, exists := cache.GetGlobalManager().GetStoryfragmentBeliefRegistry(ctx.TenantID, storyfragmentID); exists {
					if paneBeliefs, exists := beliefRegistry.PaneBeliefPayloads[paneID]; exists {
						fmt.Printf("DEBUG: Found pane beliefs for new context - held: %d, withheld: %d\n",
							len(paneBeliefs.HeldBeliefs), len(paneBeliefs.WithheldBeliefs))

						// Create belief evaluation engine
						beliefEngine := content.NewBeliefEvaluationEngine()

						// Evaluate visibility for this pane based on user beliefs
						visibility := beliefEngine.EvaluatePaneVisibility(paneBeliefs, newContext.UserBeliefs)

						// Apply visibility wrapper to HTML content
						htmlContent = beliefEngine.ApplyVisibilityWrapper(htmlContent, visibility)

						fmt.Printf("DEBUG: Applied personalization with new context - pane %s visibility: %s\n", paneID, visibility)
					}
				}
			} else {
				fmt.Printf("DEBUG: User has no relevant beliefs, using cached rendering\n")
			}
		}
	} else {
		if sessionID == "" {
			fmt.Printf("DEBUG: No session ID provided - skipping personalization\n")
		}
		if storyfragmentID == "" {
			fmt.Printf("DEBUG: No storyfragment ID provided - skipping personalization\n")
		}
	}
	// ===== END PERSONALIZATION INTEGRATION =====

	// Return HTML response
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, htmlContent)
}

// Helper function to extract background color from pane options
func extractBgColour(paneNode *models.PaneNode) *string {
	if paneNode.OptionsPayload == nil {
		return nil
	}

	if bgColour, exists := paneNode.OptionsPayload["bgColour"]; exists {
		if bgColourStr, ok := bgColour.(string); ok {
			return &bgColourStr
		}
	}

	return nil
}

// findRootNodes finds nodes that are direct children of the pane
//func findRootNodes(paneID string, nodesData map[string]*models.NodeRenderData) []string {
//	var rootNodes []string
//
//	for nodeID, nodeData := range nodesData {
//		if nodeData.ParentID == paneID {
//			rootNodes = append(rootNodes, nodeID)
//		}
//	}
//
//	return rootNodes
//}

type PaneFragmentsBatchRequest struct {
	PaneIds []string `json:"paneIds" binding:"required"`
}

type PaneFragmentsBatchResponse struct {
	Fragments map[string]string `json:"fragments"`
	Errors    map[string]string `json:"errors,omitempty"`
}

// GetPaneFragmentsBatchHandler handles batch requests for multiple pane fragments
func GetPaneFragmentsBatchHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req PaneFragmentsBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if len(req.PaneIds) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No pane IDs provided"})
		return
	}

	// Extract session context once for all panes (same as single handler)
	sessionID := c.GetHeader("X-TractStack-Session-ID")
	storyfragmentID := c.GetHeader("X-StoryFragment-ID")

	log.Printf("storyfragmentID:%s, sessionID:%s", storyfragmentID, sessionID)

	response := PaneFragmentsBatchResponse{
		Fragments: make(map[string]string),
		Errors:    make(map[string]string),
	}

	// Use cache-first pane service (same as single handler)
	paneService := content.NewPaneService(ctx, cache.GetGlobalManager())

	// ===== OPTIMIZATION: BULK LOAD ALL PANES AT ONCE =====
	// Replace individual GetByID calls with single bulk call
	paneNodes, err := paneService.GetByIDs(req.PaneIds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to load panes: %v", err)})
		return
	}

	// Convert slice to map for easy lookup during processing
	paneNodesMap := make(map[string]*models.PaneNode)
	for _, paneNode := range paneNodes {
		if paneNode != nil {
			paneNodesMap[paneNode.ID] = paneNode
		}
	}

	// ===== BUILD BELIEF REGISTRY FROM LOADED PANES =====
	// Now that we have all panes loaded, build the belief registry efficiently
	if storyfragmentID != "" {
		beliefRegistryService := content.NewBeliefRegistryService(ctx)
		_, err := beliefRegistryService.BuildRegistryFromLoadedPanes(storyfragmentID, paneNodes)
		if err != nil {
			// Log the error but don't fail the request - belief registry is optional
			log.Printf("Failed to build belief registry for storyfragment %s: %v", storyfragmentID, err)
		}
	}

	// Process each pane ID using the same logic as the single handler
	for _, paneID := range req.PaneIds {
		if paneID == "" {
			response.Errors[paneID] = "Empty pane ID"
			continue
		}

		// Get pane from our pre-loaded map instead of individual service call
		paneNode, exists := paneNodesMap[paneID]
		if !exists {
			response.Errors[paneID] = "Pane not found"
			continue
		}

		if paneNode == nil {
			response.Errors[paneID] = "Pane not found"
			continue
		}

		// Extract and parse nodes from optionsPayload (same as single handler)
		nodesData, parentChildMap, err := html.ExtractNodesFromPane(paneNode)
		if err != nil {
			response.Errors[paneID] = fmt.Sprintf("Failed to parse pane nodes: %v", err)
			continue
		}

		// Add the pane itself to the nodes data structure (same as single handler)
		paneNodeData := &models.NodeRenderData{
			ID:       paneID,
			NodeType: "Pane",
			PaneData: &models.PaneRenderData{
				Title:           paneNode.Title,
				Slug:            paneNode.Slug,
				IsDecorative:    paneNode.IsDecorative,
				BgColour:        extractBgColour(paneNode),
				HeldBeliefs:     paneNode.HeldBeliefs,
				WithheldBeliefs: paneNode.WithheldBeliefs,
				CodeHookTarget:  paneNode.CodeHookTarget,
				CodeHookPayload: paneNode.CodeHookPayload,
			},
		}
		nodesData[paneID] = paneNodeData

		// Create render context with real data (same as single handler)
		renderCtx := &models.RenderContext{
			AllNodes:        nodesData,
			ParentNodes:     parentChildMap,
			TenantID:        ctx.TenantID,
			SessionID:       sessionID,
			StoryfragmentID: storyfragmentID,
		}

		// Create HTML generator (same as single handler)
		generator := html.NewGenerator(renderCtx)

		// Generate base HTML with widget-aware cache bypass
		var htmlContent string
		if shouldBypassCacheForWidgets(ctx, paneID, sessionID, storyfragmentID) {
			// Generate fresh HTML with sessionID for widget state
			htmlContent = generator.Render(paneID)
		} else {
			// Check cache first for non-personalized content
			variant := models.PaneVariantDefault
			if cachedHTML, exists := cache.GetGlobalManager().GetHTMLChunk(ctx.TenantID, paneID, variant); exists {
				htmlContent = cachedHTML
			} else {
				htmlContent = generator.Render(paneID)
				// Cache the generated HTML
				cache.GetGlobalManager().SetHTMLChunk(ctx.TenantID, paneID, variant, htmlContent, []string{paneID})
			}
		}

		// Apply personalization if available (same logic as single handler)
		if sessionID != "" && storyfragmentID != "" {
			fmt.Printf("DEBUG: Attempting personalization for session %s, storyfragment %s, pane %s\n",
				sessionID, storyfragmentID, paneID)

			// Get cached session belief context
			if sessionContext, exists := cache.GetGlobalManager().GetSessionBeliefContext(ctx.TenantID, sessionID, storyfragmentID); exists {
				fmt.Printf("DEBUG: Found session belief context with %d beliefs\n", len(sessionContext.UserBeliefs))

				// Get pane belief requirements from registry
				if beliefRegistry, exists := cache.GetGlobalManager().GetStoryfragmentBeliefRegistry(ctx.TenantID, storyfragmentID); exists {
					fmt.Printf("DEBUG: Found belief registry with %d panes\n", len(beliefRegistry.PaneBeliefPayloads))

					if paneBeliefs, exists := beliefRegistry.PaneBeliefPayloads[paneID]; exists {
						fmt.Printf("DEBUG: Found pane beliefs - held: %d, withheld: %d, matchAcross: %d\n",
							len(paneBeliefs.HeldBeliefs), len(paneBeliefs.WithheldBeliefs), len(paneBeliefs.MatchAcross))

						// Create belief evaluation engine
						beliefEngine := content.NewBeliefEvaluationEngine()

						// Evaluate visibility for this pane based on user beliefs
						visibility := beliefEngine.EvaluatePaneVisibility(paneBeliefs, sessionContext.UserBeliefs)

						// Apply visibility wrapper to HTML content
						htmlContent = beliefEngine.ApplyVisibilityWrapper(htmlContent, visibility)

						fmt.Printf("DEBUG: Applied personalization - pane %s visibility: %s\n", paneID, visibility)
					} else {
						fmt.Printf("DEBUG: No belief requirements found for pane %s\n", paneID)
					}
				} else {
					fmt.Printf("DEBUG: No belief registry found for storyfragment %s\n", storyfragmentID)
				}
			} else {
				fmt.Printf("DEBUG: No session belief context found for session %s, storyfragment %s\n", sessionID, storyfragmentID)
			}
		}

		// Store the final HTML content
		response.Fragments[paneID] = htmlContent
	}

	c.JSON(http.StatusOK, response)
}

// shouldBypassCacheForWidgets checks if cache should be bypassed due to widgets
// Returns true if pane has widgets AND user has matching beliefs
func shouldBypassCacheForWidgets(ctx *tenant.Context, paneID, sessionID, storyfragmentID string) bool {
	// Must have sessionID to check user beliefs
	if sessionID == "" || storyfragmentID == "" {
		return false
	}

	// Get belief registry to check for widget beliefs in this pane
	cacheManager := cache.GetGlobalManager()
	registry, found := cacheManager.GetStoryfragmentBeliefRegistry(ctx.TenantID, storyfragmentID)
	if !found {
		return false // No registry = no widgets
	}

	// Check if this pane has any widget beliefs
	widgetBeliefs, hasWidgets := registry.PaneWidgetBeliefs[paneID]
	if !hasWidgets || len(widgetBeliefs) == 0 {
		return false // No widgets in this pane
	}

	// Get user's current beliefs
	userBeliefs := getUserBeliefsFromContext(ctx, sessionID)
	if userBeliefs == nil {
		return false // No user beliefs = no need to bypass
	}

	// Check if user has any beliefs that match this pane's widgets
	for _, widgetBelief := range widgetBeliefs {
		if _, hasUserBelief := userBeliefs[widgetBelief]; hasUserBelief {
			return true // User has belief matching widget = bypass cache
		}
	}

	return false // No matching beliefs = use cache
}

// getUserBeliefsFromContext extracts user beliefs from session data
// Returns nil if session or beliefs not found
func getUserBeliefsFromContext(ctx *tenant.Context, sessionID string) map[string][]string {
	if sessionID == "" {
		return nil
	}

	// Get session data to find fingerprint ID
	cacheManager := cache.GetGlobalManager()
	sessionData, sessionExists := cacheManager.GetSession(ctx.TenantID, sessionID)
	if !sessionExists {
		return nil
	}

	// Get fingerprint state to access user beliefs
	fingerprintState, fpExists := cacheManager.GetFingerprintState(ctx.TenantID, sessionData.FingerprintID)
	if !fpExists || fingerprintState.HeldBeliefs == nil {
		return nil
	}

	return fingerprintState.HeldBeliefs
}

// shouldCreateSessionContext checks if user has beliefs that would require personalization
func shouldCreateSessionContext(ctx *tenant.Context, sessionID, storyfragmentID string) bool {
	if sessionID == "" || storyfragmentID == "" {
		return false
	}

	// Get user's current beliefs
	userBeliefs := getUserBeliefsFromContext(ctx, sessionID)
	if len(userBeliefs) == 0 {
		return false // No beliefs = no need for context
	}

	// Get belief registry to check for widget beliefs or pane requirements
	cacheManager := cache.GetGlobalManager()
	registry, found := cacheManager.GetStoryfragmentBeliefRegistry(ctx.TenantID, storyfragmentID)
	if !found {
		return false // No registry = no requirements
	}

	// Check if user has any beliefs that match widget beliefs OR pane requirements
	for beliefSlug := range userBeliefs {
		// Check widget beliefs
		if registry.AllWidgetBeliefs[beliefSlug] {
			return true
		}
		// Check traditional pane belief requirements
		if registry.RequiredBeliefs[beliefSlug] {
			return true
		}
	}

	return false // No matching beliefs
}

// createSessionBeliefContext creates a new session belief context
func createSessionBeliefContext(ctx *tenant.Context, sessionID, storyfragmentID string) *models.SessionBeliefContext {
	userBeliefs := getUserBeliefsFromContext(ctx, sessionID)
	if userBeliefs == nil {
		userBeliefs = make(map[string][]string)
	}

	return &models.SessionBeliefContext{
		TenantID:        ctx.TenantID,
		SessionID:       sessionID,
		StoryfragmentID: storyfragmentID,
		UserBeliefs:     userBeliefs,
		LastEvaluation:  time.Now(),
	}
}
