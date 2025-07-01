// Package api provides HTTP handlers for fragment rendering endpoints
package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/html"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
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
	nodesData, parentChildMap, err := extractNodesFromPane(paneNode)
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

	// Extract user ID for general context (not belief-related)
	userID := extractUserIDFromRequest(c)

	// Create render context with real data
	renderCtx := &models.RenderContext{
		AllNodes:    nodesData,
		ParentNodes: parentChildMap,
		TenantID:    ctx.TenantID,
		UserID:      userID,
	}

	// Create HTML generator
	generator := html.NewGenerator(renderCtx)

	// ===== GENERATE BASE HTML (CACHE-FIRST) =====
	// Use our PaneRenderer to render the complete pane structure
	// This will call our PaneRenderer.Render() which implements the complete Pane.astro logic
	var htmlContent string
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

// extractUserIDFromRequest extracts user ID from request context, cookies, or headers
func extractUserIDFromRequest(c *gin.Context) string {
	// Try to get user ID from context first
	if userID, exists := c.Get("userID"); exists {
		if uid, ok := userID.(string); ok {
			return uid
		}
	}

	// Try to extract from cookies
	if cookie, err := c.Cookie("user_id"); err == nil && cookie != "" {
		return cookie
	}

	// Try to extract from headers
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		// Simple bearer token extraction - adjust based on your auth system
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	// Return empty string if no user ID found
	return ""
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

// extractNodesFromPane parses the optionsPayload.nodes array and builds data structures
func extractNodesFromPane(paneNode *models.PaneNode) (map[string]*models.NodeRenderData, map[string][]string, error) {
	nodesData := make(map[string]*models.NodeRenderData)
	parentChildMap := make(map[string][]string)

	// Check if optionsPayload exists and has nodes
	if paneNode.OptionsPayload == nil {
		return nodesData, parentChildMap, nil
	}

	// Extract nodes array from optionsPayload
	nodesInterface, exists := paneNode.OptionsPayload["nodes"]
	if !exists {
		return nodesData, parentChildMap, nil
	}

	// Convert to array of maps
	nodesArray, ok := nodesInterface.([]any)
	if !ok {
		return nodesData, parentChildMap, fmt.Errorf("nodes is not an array")
	}

	// Parse each node
	for _, nodeInterface := range nodesArray {
		nodeMap, ok := nodeInterface.(map[string]any)
		if !ok {
			continue
		}

		nodeData, err := parseNodeFromMap(nodeMap)
		if err != nil {
			continue // Skip invalid nodes rather than failing entirely
		}

		if nodeData.ID != "" {
			nodesData[nodeData.ID] = nodeData

			// Build parent-child relationships
			if nodeData.ParentID != "" {
				if parentChildMap[nodeData.ParentID] == nil {
					parentChildMap[nodeData.ParentID] = make([]string, 0)
				}
				parentChildMap[nodeData.ParentID] = append(parentChildMap[nodeData.ParentID], nodeData.ID)
			}
		}
	}

	return nodesData, parentChildMap, nil
}

// parseNodeFromMap converts a map[string]any to NodeRenderData
func parseNodeFromMap(nodeMap map[string]any) (*models.NodeRenderData, error) {
	nodeData := &models.NodeRenderData{}

	// Extract required fields
	if id, ok := nodeMap["id"].(string); ok {
		nodeData.ID = id
	} else {
		return nil, fmt.Errorf("missing or invalid node id")
	}

	if nodeType, ok := nodeMap["nodeType"].(string); ok {
		nodeData.NodeType = nodeType
	} else {
		return nil, fmt.Errorf("missing or invalid nodeType")
	}

	// Extract optional fields
	if tagName, ok := nodeMap["tagName"].(string); ok {
		nodeData.TagName = &tagName
	}

	if copy, ok := nodeMap["copy"].(string); ok {
		nodeData.Copy = &copy
	}

	if elementCSS, ok := nodeMap["elementCss"].(string); ok {
		nodeData.ElementCSS = &elementCSS
	}

	if parentID, ok := nodeMap["parentId"].(string); ok {
		nodeData.ParentID = parentID
	}

	// Handle ParentCSS array
	if parentCSS, ok := nodeMap["parentCss"].([]any); ok {
		cssStrings := make([]string, 0, len(parentCSS))
		for _, css := range parentCSS {
			if cssStr, ok := css.(string); ok {
				cssStrings = append(cssStrings, cssStr)
			}
		}
		nodeData.ParentCSS = cssStrings
	}

	// Handle image-related fields
	if src, ok := nodeMap["src"].(string); ok {
		nodeData.ImageURL = &src
	}

	if srcSet, ok := nodeMap["srcSet"].(string); ok {
		nodeData.SrcSet = &srcSet
	}

	if alt, ok := nodeMap["alt"].(string); ok {
		nodeData.AltText = &alt
	}

	// Handle link fields
	if href, ok := nodeMap["href"].(string); ok {
		nodeData.Href = &href
	}

	if target, ok := nodeMap["target"].(string); ok {
		nodeData.Target = &target
	}

	// Handle BgPane specific fields
	if nodeData.NodeType == "BgPane" {
		bgImageData := &models.BackgroundImageData{}

		if nodeType, ok := nodeMap["type"].(string); ok {
			bgImageData.Type = nodeType
		}

		if position, ok := nodeMap["position"].(string); ok {
			bgImageData.Position = position
		}

		if size, ok := nodeMap["size"].(string); ok {
			bgImageData.Size = size
		}

		nodeData.BgImageData = bgImageData
	}

	return nodeData, nil
}

type PaneFragmentsBatchRequest struct {
	PaneIds []string `json:"paneIds" binding:"required"`
}

type PaneFragmentsBatchResponse struct {
	Fragments map[string]string `json:"fragments"`
	Errors    map[string]string `json:"errors,omitempty"`
}

// GetPaneFragmentsBatchHandler handles batch requests for multiple pane fragments
// OPTIMIZED VERSION: Uses bulk database loading to eliminate N+1 query problem
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

	response := PaneFragmentsBatchResponse{
		Fragments: make(map[string]string),
		Errors:    make(map[string]string),
	}

	// Use cache-first pane service (same as single handler)
	paneService := content.NewPaneService(ctx, cache.GetGlobalManager())

	// Extract user ID for general context (same as single handler)
	userID := extractUserIDFromRequest(c)

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
		nodesData, parentChildMap, err := extractNodesFromPane(paneNode)
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
			AllNodes:    nodesData,
			ParentNodes: parentChildMap,
			TenantID:    ctx.TenantID,
			UserID:      userID,
		}

		// Create HTML generator (same as single handler)
		generator := html.NewGenerator(renderCtx)

		// Generate base HTML (same as single handler)
		var htmlContent string
		// Check cache first for non-personalized content
		variant := models.PaneVariantDefault
		if cachedHTML, exists := cache.GetGlobalManager().GetHTMLChunk(ctx.TenantID, paneID, variant); exists {
			htmlContent = cachedHTML
		} else {
			htmlContent = generator.Render(paneID)
			// Cache the generated HTML
			cache.GetGlobalManager().SetHTMLChunk(ctx.TenantID, paneID, variant, htmlContent, []string{paneID})
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
