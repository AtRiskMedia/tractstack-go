package bulk

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

type DependencyScanner struct {
	db     *database.DB
	logger *logging.ChanneledLogger
}

func NewDependencyScanner(db *database.DB, logger *logging.ChanneledLogger) *DependencyScanner {
	return &DependencyScanner{
		db:     db,
		logger: logger,
	}
}

func (ds *DependencyScanner) ScanAllContentIDs(tenantID string) (*admin.ContentIDMap, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting content IDs scan", "tenantID", tenantID)

	contentMap := &admin.ContentIDMap{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Beliefs:        make(map[string][]string),
	}

	queries := map[string]*map[string][]string{
		"SELECT id FROM storyfragments": &contentMap.StoryFragments,
		"SELECT id FROM panes":          &contentMap.Panes,
		"SELECT id FROM menus":          &contentMap.Menus,
		"SELECT id FROM files":          &contentMap.Files,
		"SELECT id FROM beliefs":        &contentMap.Beliefs,
	}

	totalItems := 0
	for query, targetMap := range queries {
		queryStart := time.Now()
		ds.logger.Database().Debug("Executing content ID query", "query", query)

		rows, err := ds.db.Query(query)
		if err != nil {
			ds.logger.Database().Error("Content ID query failed", "error", err.Error(), "query", query)
			return nil, err
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				(*targetMap)[id] = []string{}
				count++
				totalItems++
			}
		}

		ds.logger.Database().Info("Content ID query completed", "query", query, "count", count, "duration", time.Since(queryStart))
	}

	ds.logger.Database().Info("Content IDs scan completed", "tenantID", tenantID, "totalItems", totalItems, "duration", time.Since(start))
	return contentMap, nil
}

func (ds *DependencyScanner) ScanStoryFragmentDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting story fragment dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	// Step 1: Initialize all story fragments with empty arrays
	query := "SELECT id FROM storyfragments"
	ds.logger.Database().Debug("Executing story fragment dependency query", "query", query)

	rows, err := ds.db.Query(query)
	if err != nil {
		ds.logger.Database().Error("Story fragment dependency query failed", "error", err.Error(), "query", query)
		return nil, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			count++
		}
	}

	ds.logger.Database().Info("Story fragment IDs loaded", "sfCount", count)

	// Step 2: Get home slug from brand config (simplified - in real implementation this would come from tenant context)
	// For now, we'll mark home page if it exists
	var homeSlug string
	if err := ds.db.QueryRow("SELECT value FROM config WHERE key = 'homeSlug' LIMIT 1").Scan(&homeSlug); err == nil && homeSlug != "" {
		var homeID string
		if err := ds.db.QueryRow("SELECT id FROM storyfragments WHERE slug = ?", homeSlug).Scan(&homeID); err == nil {
			if _, exists := dependencies[homeID]; exists {
				dependencies[homeID] = append(dependencies[homeID], "Home Page")
			}
		}
	}

	// Step 3: Menu ActionLisp references - PORT THE ACTUAL WORKING LOGIC
	menuRows, err := ds.db.Query("SELECT id, options_payload FROM menus")
	if err != nil {
		ds.logger.Database().Error("Menu query failed", "error", err.Error())
		return dependencies, nil // Continue with what we have
	}
	defer menuRows.Close()

	menuDepCount := 0
	for menuRows.Next() {
		var menuID, optionsPayload string
		if err := menuRows.Scan(&menuID, &optionsPayload); err != nil {
			continue
		}

		if optionsPayload != "" {
			var options []map[string]any
			if err := json.Unmarshal([]byte(optionsPayload), &options); err == nil {
				for _, option := range options {
					if actionLisp, ok := option["actionLisp"].(string); ok && actionLisp != "" {
						// Simple URL extraction from ActionLisp - this is a simplified version
						// In the real implementation, this would use lisp.PreParseAction
						slug := ds.extractSlugFromActionLisp(actionLisp, homeSlug)
						if slug != "" {
							// Find story fragment ID by slug
							var sfID string
							if err := ds.db.QueryRow("SELECT id FROM storyfragments WHERE slug = ?", slug).Scan(&sfID); err == nil {
								if _, exists := dependencies[sfID]; exists {
									dependencies[sfID] = append(dependencies[sfID], menuID)
									menuDepCount++
								}
							}
						}
					}
				}
			}
		}
	}

	ds.logger.Database().Info("Story fragment dependencies scan completed", "tenantID", tenantID, "sfCount", count, "menuDeps", menuDepCount, "duration", time.Since(start))
	return dependencies, nil
}

func (ds *DependencyScanner) ScanPaneDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting pane dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	// First query: Get all pane IDs
	query1 := "SELECT id FROM panes"
	ds.logger.Database().Debug("Executing pane IDs query", "query", query1)

	rows, err := ds.db.Query(query1)
	if err != nil {
		ds.logger.Database().Error("Pane IDs query failed", "error", err.Error(), "query", query1)
		return nil, err
	}
	defer rows.Close()

	paneCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			paneCount++
		}
	}

	ds.logger.Database().Info("Pane IDs loaded", "paneCount", paneCount)

	// Second query: Get pane dependencies
	query2 := "SELECT pane_id, storyfragment_id FROM storyfragment_panes"
	ds.logger.Database().Debug("Executing pane dependencies query", "query", query2)

	sfPaneRows, err := ds.db.Query(query2)
	if err != nil {
		ds.logger.Database().Error("Pane dependencies query failed", "error", err.Error(), "query", query2)
		return nil, err
	}
	defer sfPaneRows.Close()

	depCount := 0
	for sfPaneRows.Next() {
		var paneID, sfID string
		if err := sfPaneRows.Scan(&paneID, &sfID); err == nil {
			if _, exists := dependencies[paneID]; exists {
				dependencies[paneID] = append(dependencies[paneID], sfID)
				depCount++
			}
		}
	}

	ds.logger.Database().Info("Pane dependencies scan completed", "tenantID", tenantID, "paneCount", paneCount, "depCount", depCount, "duration", time.Since(start))
	return dependencies, nil
}

func (ds *DependencyScanner) ScanMenuDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting menu dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	// First query: Get all menu IDs
	query1 := "SELECT id FROM menus"
	ds.logger.Database().Debug("Executing menu IDs query", "query", query1)

	rows, err := ds.db.Query(query1)
	if err != nil {
		ds.logger.Database().Error("Menu IDs query failed", "error", err.Error(), "query", query1)
		return nil, err
	}
	defer rows.Close()

	menuCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			menuCount++
		}
	}

	ds.logger.Database().Info("Menu IDs loaded", "menuCount", menuCount)

	// Second query: Get menu dependencies
	query2 := "SELECT menu_id, id FROM storyfragments WHERE menu_id IS NOT NULL"
	ds.logger.Database().Debug("Executing menu dependencies query", "query", query2)

	sfMenuRows, err := ds.db.Query(query2)
	if err != nil {
		ds.logger.Database().Error("Menu dependencies query failed", "error", err.Error(), "query", query2)
		return nil, err
	}
	defer sfMenuRows.Close()

	depCount := 0
	for sfMenuRows.Next() {
		var menuID, sfID string
		if err := sfMenuRows.Scan(&menuID, &sfID); err == nil {
			if _, exists := dependencies[menuID]; exists {
				dependencies[menuID] = append(dependencies[menuID], sfID)
				depCount++
			}
		}
	}

	ds.logger.Database().Info("Menu dependencies scan completed", "tenantID", tenantID, "menuCount", menuCount, "depCount", depCount, "duration", time.Since(start))
	return dependencies, nil
}

func (ds *DependencyScanner) ScanFileDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting file dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	// First query: Get all file IDs
	query1 := "SELECT id FROM files"
	ds.logger.Database().Debug("Executing file IDs query", "query", query1)

	rows, err := ds.db.Query(query1)
	if err != nil {
		ds.logger.Database().Error("File IDs query failed", "error", err.Error(), "query", query1)
		return nil, err
	}
	defer rows.Close()

	fileCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			fileCount++
		}
	}

	ds.logger.Database().Info("File IDs loaded", "fileCount", fileCount)

	// Second query: Get file dependencies from junction table
	query2 := "SELECT file_id, pane_id FROM file_panes"
	ds.logger.Database().Debug("Executing file dependencies query", "query", query2)

	filePaneRows, err := ds.db.Query(query2)
	if err != nil {
		ds.logger.Database().Error("File dependencies query failed", "error", err.Error(), "query", query2)
		return nil, err
	}
	defer filePaneRows.Close()

	depCount := 0
	for filePaneRows.Next() {
		var fileID, paneID string
		if err := filePaneRows.Scan(&fileID, &paneID); err == nil {
			if _, exists := dependencies[fileID]; exists {
				dependencies[fileID] = append(dependencies[fileID], paneID)
				depCount++
			}
		}
	}

	// Third query: Get file dependencies from pane options_payload - PORT THE MISSING LOGIC
	paneRows, err := ds.db.Query("SELECT id, options_payload FROM panes WHERE options_payload LIKE '%fileId%'")
	if err != nil {
		ds.logger.Database().Error("Pane options query failed", "error", err.Error())
	} else {
		defer paneRows.Close()

		for paneRows.Next() {
			var paneID, optionsPayload string
			if err := paneRows.Scan(&paneID, &optionsPayload); err != nil {
				continue
			}

			// Check each file ID to see if it's referenced in this pane's options
			for fileID := range dependencies {
				if strings.Contains(optionsPayload, fmt.Sprintf(`"fileId":"%s"`, fileID)) {
					// Check if paneID is already in the list
					found := false
					for _, dep := range dependencies[fileID] {
						if dep == paneID {
							found = true
							break
						}
					}
					if !found {
						dependencies[fileID] = append(dependencies[fileID], paneID)
						depCount++
					}
				}
			}
		}
	}

	ds.logger.Database().Info("File dependencies scan completed", "tenantID", tenantID, "fileCount", fileCount, "depCount", depCount, "duration", time.Since(start))
	return dependencies, nil
}

func (ds *DependencyScanner) ScanBeliefDependencies(tenantID string) (map[string][]string, error) {
	start := time.Now()
	ds.logger.Database().Debug("Starting belief dependencies scan", "tenantID", tenantID)

	dependencies := make(map[string][]string)

	// First query: Get all belief IDs
	query := "SELECT id FROM beliefs"
	ds.logger.Database().Debug("Executing belief IDs query", "query", query)

	rows, err := ds.db.Query(query)
	if err != nil {
		ds.logger.Database().Error("Belief IDs query failed", "error", err.Error(), "query", query)
		return nil, err
	}
	defer rows.Close()

	beliefCount := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			dependencies[id] = []string{}
			beliefCount++
		}
	}

	ds.logger.Database().Info("Belief IDs loaded", "beliefCount", beliefCount)

	// Second query: PORT THE COMPREHENSIVE BELIEF DETECTION LOGIC
	paneRows, err := ds.db.Query("SELECT id, options_payload FROM panes")
	if err != nil {
		ds.logger.Database().Error("Pane belief query failed", "error", err.Error())
		return dependencies, nil
	}
	defer paneRows.Close()

	// Collect all belief slugs first, then bulk lookup belief IDs
	beliefSlugs := make(map[string][]string) // beliefSlug -> []paneIDs
	depCount := 0

	for paneRows.Next() {
		var paneID, optionsPayload string
		if err := paneRows.Scan(&paneID, &optionsPayload); err != nil {
			continue
		}

		if optionsPayload == "" {
			continue
		}

		// Parse options payload once
		var options map[string]any
		if err := json.Unmarshal([]byte(optionsPayload), &options); err != nil {
			continue
		}

		// 1. Traditional pane-level beliefs
		if heldBeliefs, ok := options["heldBeliefs"].(map[string]any); ok {
			for beliefSlug := range heldBeliefs {
				if beliefSlug != "MATCH-ACROSS" && beliefSlug != "LINKED-BELIEFS" {
					beliefSlugs[beliefSlug] = append(beliefSlugs[beliefSlug], paneID)
				}
			}
		}

		if withheldBeliefs, ok := options["withheldBeliefs"].(map[string]any); ok {
			for beliefSlug := range withheldBeliefs {
				if beliefSlug != "MATCH-ACROSS" && beliefSlug != "LINKED-BELIEFS" {
					beliefSlugs[beliefSlug] = append(beliefSlugs[beliefSlug], paneID)
				}
			}
		}

		// 2. Belief widgets in optionsPayload - scan for code elements with belief widgets
		var beliefs []string
		ds.scanMapForBeliefWidgets(options, &beliefs)
		for _, beliefSlug := range beliefs {
			if beliefSlug != "" {
				beliefSlugs[beliefSlug] = append(beliefSlugs[beliefSlug], paneID)
			}
		}
	}

	// Convert belief slugs to IDs and add dependencies
	for beliefSlug, paneIDs := range beliefSlugs {
		var beliefID string
		if err := ds.db.QueryRow("SELECT id FROM beliefs WHERE slug = ?", beliefSlug).Scan(&beliefID); err == nil {
			if _, exists := dependencies[beliefID]; exists {
				// Add all pane dependencies for this belief
				for _, paneID := range paneIDs {
					// Check for duplicates
					found := false
					for _, dep := range dependencies[beliefID] {
						if dep == paneID {
							found = true
							break
						}
					}
					if !found {
						dependencies[beliefID] = append(dependencies[beliefID], paneID)
						depCount++
					}
				}
			}
		}
	}

	ds.logger.Database().Info("Belief dependencies scan completed", "tenantID", tenantID, "beliefCount", beliefCount, "depCount", depCount, "duration", time.Since(start))
	return dependencies, nil
}

// Helper functions

// extractSlugFromActionLisp extracts slug from ActionLisp - simplified version
func (ds *DependencyScanner) extractSlugFromActionLisp(actionLisp, homeSlug string) string {
	// This is a simplified version. The real implementation would use lisp.PreParseAction
	// For now, we'll do basic string parsing to extract common patterns

	// Look for common patterns like (goto "/slug") or similar
	if strings.Contains(actionLisp, "goto") && strings.Contains(actionLisp, "/") {
		// Extract the path - this is very basic parsing
		start := strings.Index(actionLisp, "\"/")
		if start != -1 {
			start += 2 // skip "/
			end := strings.Index(actionLisp[start:], "\"")
			if end != -1 {
				path := actionLisp[start : start+end]
				return ds.extractSlugFromURL("/"+path, homeSlug)
			}
		}
	}

	return ""
}

// extractSlugFromURL extracts slug from URL path - ported from working logic
func (ds *DependencyScanner) extractSlugFromURL(url, homeSlug string) string {
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

// scanMapForBeliefWidgets recursively scans for belief widget references - ported from working logic
func (ds *DependencyScanner) scanMapForBeliefWidgets(data any, beliefs *[]string) {
	switch v := data.(type) {
	case map[string]any:
		// Check if this is a code TagElement with belief widget
		if tagName, ok := v["tagName"].(string); ok && tagName == "code" {
			if copy, ok := v["copy"].(string); ok {
				if params, ok := v["codeHookParams"].([]any); ok && len(params) > 0 {
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
			ds.scanMapForBeliefWidgets(value, beliefs)
		}
	case []any:
		// Recursively scan array elements
		for _, item := range v {
			ds.scanMapForBeliefWidgets(item, beliefs)
		}
	}
}
