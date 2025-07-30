// Package bulk provides efficient dependency scanning for orphan analysis
package bulk

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

// DependencyScanner implements efficient multi-table dependency analysis
type DependencyScanner struct {
	db *database.DB
}

// NewDependencyScanner creates a new dependency scanner
func NewDependencyScanner(db *database.DB) *DependencyScanner {
	return &DependencyScanner{db: db}
}

// ScanAllContentIDs returns all content IDs by type for dependency initialization
func (ds *DependencyScanner) ScanAllContentIDs(tenantID string) (*admin.ContentIDMap, error) {
	idMap := &admin.ContentIDMap{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Beliefs:        make(map[string][]string),
	}

	// Scan story fragments
	rows, err := ds.db.Query("SELECT id FROM storyfragments")
	if err != nil {
		return nil, fmt.Errorf("failed to scan storyfragments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			idMap.StoryFragments[id] = []string{}
		}
	}

	// Scan panes
	rows, err = ds.db.Query("SELECT id FROM panes")
	if err != nil {
		return nil, fmt.Errorf("failed to scan panes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			idMap.Panes[id] = []string{}
		}
	}

	// Scan menus
	rows, err = ds.db.Query("SELECT id FROM menus")
	if err != nil {
		return nil, fmt.Errorf("failed to scan menus: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			idMap.Menus[id] = []string{}
		}
	}

	// Scan files
	rows, err = ds.db.Query("SELECT id FROM files")
	if err != nil {
		return nil, fmt.Errorf("failed to scan files: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			idMap.Files[id] = []string{}
		}
	}

	// Scan beliefs
	rows, err = ds.db.Query("SELECT id FROM beliefs")
	if err != nil {
		return nil, fmt.Errorf("failed to scan beliefs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			idMap.Beliefs[id] = []string{}
		}
	}

	return idMap, nil
}

// ScanStoryFragmentDependencies finds what depends on each story fragment
func (ds *DependencyScanner) ScanStoryFragmentDependencies(tenantID string) (map[string][]string, error) {
	// Story fragments are top-level, nothing depends on them
	return make(map[string][]string), nil
}

// ScanPaneDependencies finds what story fragments depend on each pane
func (ds *DependencyScanner) ScanPaneDependencies(tenantID string) (map[string][]string, error) {
	dependencies := make(map[string][]string)

	// storyfragment_panes junction table
	rows, err := ds.db.Query(`
		SELECT sfp.storyfragment_id, sfp.pane_id 
		FROM storyfragment_panes sfp
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to scan pane dependencies: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sfID, paneID string
		if err := rows.Scan(&sfID, &paneID); err == nil {
			dependencies[paneID] = append(dependencies[paneID], sfID)
		}
	}

	return dependencies, nil
}

// ScanMenuDependencies finds what story fragments depend on each menu
func (ds *DependencyScanner) ScanMenuDependencies(tenantID string) (map[string][]string, error) {
	dependencies := make(map[string][]string)

	// Direct menu_id references in storyfragments
	rows, err := ds.db.Query(`
		SELECT menu_id, id 
		FROM storyfragments 
		WHERE menu_id IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to scan menu dependencies: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var menuID, sfID string
		if err := rows.Scan(&menuID, &sfID); err == nil {
			dependencies[menuID] = append(dependencies[menuID], sfID)
		}
	}

	return dependencies, nil
}

// ScanFileDependencies finds what panes depend on each file
func (ds *DependencyScanner) ScanFileDependencies(tenantID string) (map[string][]string, error) {
	dependencies := make(map[string][]string)

	// file_panes junction table
	rows, err := ds.db.Query(`
		SELECT fp.file_id, fp.pane_id 
		FROM file_panes fp
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to scan file dependencies: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var fileID, paneID string
		if err := rows.Scan(&fileID, &paneID); err == nil {
			dependencies[fileID] = append(dependencies[fileID], paneID)
		}
	}

	// pane options_payload fileId references
	paneRows, err := ds.db.Query("SELECT id, options_payload FROM panes WHERE options_payload LIKE '%fileId%'")
	if err != nil {
		return nil, fmt.Errorf("failed to scan pane file references: %w", err)
	}
	defer paneRows.Close()

	for paneRows.Next() {
		var paneID, optionsPayload string
		if err := paneRows.Scan(&paneID, &optionsPayload); err != nil {
			continue
		}

		// Extract file IDs from options payload
		fileIDs := ds.extractFileIDsFromOptionsPayload(optionsPayload)
		for _, fileID := range fileIDs {
			// Check if this dependency already exists
			found := false
			for _, existingPaneID := range dependencies[fileID] {
				if existingPaneID == paneID {
					found = true
					break
				}
			}
			if !found {
				dependencies[fileID] = append(dependencies[fileID], paneID)
			}
		}
	}

	return dependencies, nil
}

// ScanBeliefDependencies finds what panes depend on each belief
func (ds *DependencyScanner) ScanBeliefDependencies(tenantID string) (map[string][]string, error) {
	dependencies := make(map[string][]string)

	// Bulk load all panes with belief detection
	rows, err := ds.db.Query("SELECT id, options_payload FROM panes")
	if err != nil {
		return nil, fmt.Errorf("failed to scan belief dependencies: %w", err)
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

		// Traditional pane-level beliefs
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

		// Widget beliefs using efficient string scanning
		widgetBeliefs := ds.scanForWidgetBeliefs(optionsPayload)
		for _, beliefSlug := range widgetBeliefs {
			beliefSlugs[beliefSlug] = append(beliefSlugs[beliefSlug], paneID)
		}
	}

	// Bulk lookup belief IDs and populate dependencies
	for beliefSlug, paneIDs := range beliefSlugs {
		var beliefID string
		if err := ds.db.QueryRow("SELECT id FROM beliefs WHERE slug = ?", beliefSlug).Scan(&beliefID); err == nil {
			// Add all pane IDs for this belief, avoiding duplicates
			for _, paneID := range paneIDs {
				found := false
				for _, existing := range dependencies[beliefID] {
					if existing == paneID {
						found = true
						break
					}
				}
				if !found {
					dependencies[beliefID] = append(dependencies[beliefID], paneID)
				}
			}
		}
	}

	return dependencies, nil
}

// extractFileIDsFromOptionsPayload extracts file IDs from pane options payload
func (ds *DependencyScanner) extractFileIDsFromOptionsPayload(optionsPayload string) []string {
	var fileIDs []string

	// Simple string search for fileId patterns
	if !strings.Contains(optionsPayload, "fileId") {
		return fileIDs
	}

	// Look for "fileId":"some-id" patterns
	parts := strings.Split(optionsPayload, `"fileId":"`)
	for i := 1; i < len(parts); i++ {
		endQuote := strings.Index(parts[i], `"`)
		if endQuote > 0 {
			fileID := parts[i][:endQuote]
			if fileID != "" {
				fileIDs = append(fileIDs, fileID)
			}
		}
	}

	return fileIDs
}

// scanForWidgetBeliefs efficiently scans JSON for belief widgets without full node extraction
func (ds *DependencyScanner) scanForWidgetBeliefs(optionsPayload string) []string {
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
	ds.scanMapForBeliefWidgets(payload, &beliefs)
	return beliefs
}

// scanMapForBeliefWidgets recursively scans a map for belief widgets
func (ds *DependencyScanner) scanMapForBeliefWidgets(data interface{}, beliefs *[]string) {
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
			ds.scanMapForBeliefWidgets(value, beliefs)
		}
	case []interface{}:
		// Recursively scan array elements
		for _, item := range v {
			ds.scanMapForBeliefWidgets(item, beliefs)
		}
	}
}
