package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils/lisp"
)

// OrphanAnalysisService handles orphan detection computations
type OrphanAnalysisService struct {
	ctx *tenant.Context
}

// NewOrphanAnalysisService creates a new orphan analysis service
func NewOrphanAnalysisService(ctx *tenant.Context) *OrphanAnalysisService {
	return &OrphanAnalysisService{ctx: ctx}
}

// BrandConfigAdapter adapts tenant config to lisp parser interface
type BrandConfigAdapter struct {
	homeSlug string
}

func (b *BrandConfigAdapter) GetHomeSlug() string {
	return b.homeSlug
}

// ComputeOrphanAnalysis performs the complete dependency mapping computation
func (oas *OrphanAnalysisService) ComputeOrphanAnalysis() (*models.OrphanAnalysisPayload, error) {
	payload := &models.OrphanAnalysisPayload{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Beliefs:        make(map[string][]string),
		Status:         "complete",
	}

	// Initialize content IDs
	if err := oas.initializeContentIDs(payload); err != nil {
		return nil, err
	}

	// Compute dependencies using V1 logic
	if err := oas.computeStoryFragmentDependencies(payload); err != nil {
		return nil, err
	}
	if err := oas.computePaneDependencies(payload); err != nil {
		return nil, err
	}
	if err := oas.computeMenuDependencies(payload); err != nil {
		return nil, err
	}
	if err := oas.computeFileDependencies(payload); err != nil {
		return nil, err
	}
	if err := oas.computeBeliefDependencies(payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// initializeContentIDs populates content type maps
func (oas *OrphanAnalysisService) initializeContentIDs(payload *models.OrphanAnalysisPayload) error {
	// StoryFragments
	if rows, err := oas.ctx.Database.Conn.Query("SELECT id FROM storyfragments"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				payload.StoryFragments[id] = []string{}
			}
		}
	} else {
		return err
	}

	// Panes
	if rows, err := oas.ctx.Database.Conn.Query("SELECT id FROM panes"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				payload.Panes[id] = []string{}
			}
		}
	} else {
		return err
	}

	// Menus
	if rows, err := oas.ctx.Database.Conn.Query("SELECT id FROM menus"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				payload.Menus[id] = []string{}
			}
		}
	} else {
		return err
	}

	// Files
	if rows, err := oas.ctx.Database.Conn.Query("SELECT id FROM files"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				payload.Files[id] = []string{}
			}
		}
	} else {
		return err
	}

	// Beliefs
	if rows, err := oas.ctx.Database.Conn.Query("SELECT id FROM beliefs"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				payload.Beliefs[id] = []string{}
			}
		}
	} else {
		return err
	}

	return nil
}

// computeStoryFragmentDependencies finds what depends on each story fragment
func (oas *OrphanAnalysisService) computeStoryFragmentDependencies(payload *models.OrphanAnalysisPayload) error {
	brandConfig := &BrandConfigAdapter{homeSlug: oas.ctx.Config.BrandConfig.HomeSlug}

	// Mark home page as used
	if brandConfig.homeSlug != "" {
		var homeID string
		if err := oas.ctx.Database.Conn.QueryRow("SELECT id FROM storyfragments WHERE slug = ?", brandConfig.homeSlug).Scan(&homeID); err == nil {
			if dependents, exists := payload.StoryFragments[homeID]; exists {
				payload.StoryFragments[homeID] = append(dependents, "Home Page")
			}
		}
	}

	// Menu ActionLisp references
	menuRows, err := oas.ctx.Database.Conn.Query("SELECT id, options_payload FROM menus")
	if err != nil {
		return err
	}
	defer menuRows.Close()

	for menuRows.Next() {
		var menuID, optionsPayload string
		if err := menuRows.Scan(&menuID, &optionsPayload); err != nil {
			continue
		}

		if optionsPayload != "" {
			var options []map[string]interface{}
			if err := json.Unmarshal([]byte(optionsPayload), &options); err == nil {
				for _, option := range options {
					if actionLisp, ok := option["actionLisp"].(string); ok && actionLisp != "" {
						// Parse ActionLisp
						tokens, _, err := lisp.LispLexer(actionLisp, false)
						if err != nil {
							continue
						}

						targetURL := lisp.PreParseAction(tokens, "", false, brandConfig)
						if targetURL == "" {
							continue
						}

						// Extract slug from URL
						slug := oas.extractSlugFromURL(targetURL, brandConfig.homeSlug)
						if slug == "" {
							continue
						}

						// Find story fragment ID by slug
						var sfID string
						if err := oas.ctx.Database.Conn.QueryRow("SELECT id FROM storyfragments WHERE slug = ?", slug).Scan(&sfID); err == nil {
							if dependents, exists := payload.StoryFragments[sfID]; exists {
								payload.StoryFragments[sfID] = append(dependents, menuID)
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// computePaneDependencies finds what story fragments depend on each pane
func (oas *OrphanAnalysisService) computePaneDependencies(payload *models.OrphanAnalysisPayload) error {
	// storyfragment_panes junction table
	rows, err := oas.ctx.Database.Conn.Query(`
		SELECT sfp.storyfragment_id, sfp.pane_id 
		FROM storyfragment_panes sfp
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var sfID, paneID string
		if err := rows.Scan(&sfID, &paneID); err == nil {
			if dependents, exists := payload.Panes[paneID]; exists {
				payload.Panes[paneID] = append(dependents, sfID)
			}
		}
	}

	return nil
}

// computeMenuDependencies finds what story fragments depend on each menu
func (oas *OrphanAnalysisService) computeMenuDependencies(payload *models.OrphanAnalysisPayload) error {
	// Direct menu_id references in storyfragments
	rows, err := oas.ctx.Database.Conn.Query(`
		SELECT menu_id, id 
		FROM storyfragments 
		WHERE menu_id IS NOT NULL
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var menuID, sfID string
		if err := rows.Scan(&menuID, &sfID); err == nil {
			if dependents, exists := payload.Menus[menuID]; exists {
				payload.Menus[menuID] = append(dependents, sfID)
			}
		}
	}

	return nil
}

// computeFileDependencies finds what depends on each file
func (oas *OrphanAnalysisService) computeFileDependencies(payload *models.OrphanAnalysisPayload) error {
	// file_panes junction table
	rows, err := oas.ctx.Database.Conn.Query(`
		SELECT fp.file_id, fp.pane_id 
		FROM file_panes fp
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var fileID, paneID string
		if err := rows.Scan(&fileID, &paneID); err == nil {
			if dependents, exists := payload.Files[fileID]; exists {
				payload.Files[fileID] = append(dependents, paneID)
			}
		}
	}

	// pane options_payload fileId references
	paneRows, err := oas.ctx.Database.Conn.Query("SELECT id, options_payload FROM panes WHERE options_payload LIKE '%fileId%'")
	if err != nil {
		return err
	}
	defer paneRows.Close()

	for paneRows.Next() {
		var paneID, optionsPayload string
		if err := paneRows.Scan(&paneID, &optionsPayload); err != nil {
			continue
		}

		for fileID := range payload.Files {
			if strings.Contains(optionsPayload, fmt.Sprintf(`"fileId":"%s"`, fileID)) {
				if dependents, exists := payload.Files[fileID]; exists {
					found := false
					for _, dep := range dependents {
						if dep == paneID {
							found = true
							break
						}
					}
					if !found {
						payload.Files[fileID] = append(dependents, paneID)
					}
				}
			}
		}
	}

	return nil
}

// computeBeliefDependencies finds what panes depend on each belief - COMPREHENSIVE FIX
func (oas *OrphanAnalysisService) computeBeliefDependencies(payload *models.OrphanAnalysisPayload) error {
	// Bulk load all panes with belief detection
	rows, err := oas.ctx.Database.Conn.Query("SELECT id, options_payload FROM panes")
	if err != nil {
		return err
	}
	defer rows.Close()

	// Collect all belief slugs first, then bulk lookup belief IDs
	beliefSlugs := make(map[string][]string) // beliefSlug -> []paneIDs

	for rows.Next() {
		var paneID, optionsPayload string
		if err := rows.Scan(&paneID, &optionsPayload); err != nil {
			continue
		}

		if optionsPayload == "" {
			continue
		}

		// Parse options payload once
		var options map[string]interface{}
		if err := json.Unmarshal([]byte(optionsPayload), &options); err != nil {
			continue
		}

		// 1. Traditional pane-level beliefs
		if heldBeliefs, ok := options["heldBeliefs"].(map[string]interface{}); ok {
			for beliefSlug := range heldBeliefs {
				if beliefSlug != "MATCH-ACROSS" && beliefSlug != "LINKED-BELIEFS" {
					beliefSlugs[beliefSlug] = append(beliefSlugs[beliefSlug], paneID)
				}
			}
		}

		if withheldBeliefs, ok := options["withheldBeliefs"].(map[string]interface{}); ok {
			for beliefSlug := range withheldBeliefs {
				beliefSlugs[beliefSlug] = append(beliefSlugs[beliefSlug], paneID)
			}
		}

		// 2. Widget beliefs using efficient string scanning (no full node extraction)
		widgetBeliefs := oas.scanForWidgetBeliefs(optionsPayload)
		for _, beliefSlug := range widgetBeliefs {
			beliefSlugs[beliefSlug] = append(beliefSlugs[beliefSlug], paneID)
		}
	}

	// Bulk lookup belief IDs and populate payload
	for beliefSlug, paneIDs := range beliefSlugs {
		var beliefID string
		if err := oas.ctx.Database.Conn.QueryRow("SELECT id FROM beliefs WHERE slug = ?", beliefSlug).Scan(&beliefID); err == nil {
			if dependents, exists := payload.Beliefs[beliefID]; exists {
				// Add all pane IDs for this belief, avoiding duplicates
				for _, paneID := range paneIDs {
					found := false
					for _, existing := range dependents {
						if existing == paneID {
							found = true
							break
						}
					}
					if !found {
						payload.Beliefs[beliefID] = append(dependents, paneID)
					}
				}
			}
		}
	}

	return nil
}

// scanForWidgetBeliefs efficiently scans JSON for belief widgets without full node extraction
func (oas *OrphanAnalysisService) scanForWidgetBeliefs(optionsPayload string) []string {
	var beliefs []string

	// Look for code elements with codeHookParams - efficient string scanning
	if !strings.Contains(optionsPayload, `"tagName":"code"`) {
		return beliefs
	}

	if !strings.Contains(optionsPayload, `"codeHookParams"`) {
		return beliefs
	}

	// Parse just enough to find code nodes with belief widgets
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(optionsPayload), &payload); err != nil {
		return beliefs
	}

	// Recursively scan for code nodes
	oas.scanMapForBeliefWidgets(payload, &beliefs)
	return beliefs
}

// scanMapForBeliefWidgets recursively scans a map for belief widgets
func (oas *OrphanAnalysisService) scanMapForBeliefWidgets(data interface{}, beliefs *[]string) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this is a code TagElement with belief widget
		if tagName, ok := v["tagName"].(string); ok && tagName == "code" {
			if copy, ok := v["copy"].(string); ok {
				if params, ok := v["codeHookParams"].([]interface{}); ok && len(params) > 0 {
					// Extract widget type from copy (belief(...), toggle(...), identifyAs(...))
					if strings.HasPrefix(copy, "belief(") || strings.HasPrefix(copy, "toggle(") || strings.HasPrefix(copy, "identifyAs(") {
						if beliefSlug, ok := params[0].(string); ok && beliefSlug != "" {
							*beliefs = append(*beliefs, beliefSlug)
						}
					}
				}
			}
		}

		// Recursively scan all values
		for _, value := range v {
			oas.scanMapForBeliefWidgets(value, beliefs)
		}
	case []interface{}:
		// Recursively scan array elements
		for _, item := range v {
			oas.scanMapForBeliefWidgets(item, beliefs)
		}
	}
}

// addBeliefDependency adds a pane dependency to a belief by slug
func (oas *OrphanAnalysisService) addBeliefDependency(payload *models.OrphanAnalysisPayload, beliefSlug, paneID string) {
	// Find belief ID by slug
	var beliefID string
	if err := oas.ctx.Database.Conn.QueryRow("SELECT id FROM beliefs WHERE slug = ?", beliefSlug).Scan(&beliefID); err == nil {
		if dependents, exists := payload.Beliefs[beliefID]; exists {
			// Check if paneID is already in the list
			found := false
			for _, dep := range dependents {
				if dep == paneID {
					found = true
					break
				}
			}
			if !found {
				payload.Beliefs[beliefID] = append(dependents, paneID)
			}
		}
	}
}

// extractSlugFromURL extracts slug from URL path
func (oas *OrphanAnalysisService) extractSlugFromURL(url, homeSlug string) string {
	if url == "/" || url == "" {
		return homeSlug
	}

	url = strings.TrimPrefix(url, "/")

	if idx := strings.Index(url, "#"); idx != -1 {
		url = url[:idx]
	}
	if idx := strings.Index(url, "?"); idx != -1 {
		url = url[:idx]
	}

	if strings.HasPrefix(url, "storykeep") ||
		strings.HasPrefix(url, "concierge") ||
		strings.HasPrefix(url, "context") ||
		strings.HasPrefix(url, "sandbox") {
		return ""
	}

	return url
}
