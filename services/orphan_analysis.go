package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
)

// OrphanAnalysisService handles orphan detection computations
type OrphanAnalysisService struct {
	ctx *tenant.Context
}

// NewOrphanAnalysisService creates a new orphan analysis service
func NewOrphanAnalysisService(ctx *tenant.Context) *OrphanAnalysisService {
	return &OrphanAnalysisService{ctx: ctx}
}

// ComputeOrphanAnalysis performs the expensive computation of dependency mapping
func (oas *OrphanAnalysisService) ComputeOrphanAnalysis() (*models.OrphanAnalysisPayload, error) {
	payload := &models.OrphanAnalysisPayload{
		StoryFragments: make(map[string][]string),
		Panes:          make(map[string][]string),
		Menus:          make(map[string][]string),
		Files:          make(map[string][]string),
		Resources:      make(map[string][]string),
		Beliefs:        make(map[string][]string),
		Epinets:        make(map[string][]string),
		TractStacks:    make(map[string][]string),
		Status:         "complete",
	}

	// Initialize all content IDs with empty arrays
	if err := oas.initializeAllContentIDs(payload); err != nil {
		return nil, err
	}

	// Compute dependencies for each content type
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
	if err := oas.computeResourceDependencies(payload); err != nil {
		return nil, err
	}
	if err := oas.computeBeliefDependencies(payload); err != nil {
		return nil, err
	}
	if err := oas.computeEpinetDependencies(payload); err != nil {
		return nil, err
	}
	if err := oas.computeTractStackDependencies(payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// initializeAllContentIDs gets all content IDs and initializes with empty arrays
func (oas *OrphanAnalysisService) initializeAllContentIDs(payload *models.OrphanAnalysisPayload) error {
	// Get all StoryFragments
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

	// Get all Panes
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

	// Get all Menus
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

	// Get all Files
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

	// Get all Resources
	if rows, err := oas.ctx.Database.Conn.Query("SELECT id FROM resources"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				payload.Resources[id] = []string{}
			}
		}
	} else {
		return err
	}

	// Get all Beliefs
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

	// Get all Epinets
	if rows, err := oas.ctx.Database.Conn.Query("SELECT id FROM epinets"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				payload.Epinets[id] = []string{}
			}
		}
	} else {
		return err
	}

	// Get all TractStacks
	if rows, err := oas.ctx.Database.Conn.Query("SELECT id FROM tractstacks"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				payload.TractStacks[id] = []string{}
			}
		}
	} else {
		return err
	}

	return nil
}

// computeStoryFragmentDependencies finds what depends on each story fragment
func (oas *OrphanAnalysisService) computeStoryFragmentDependencies(payload *models.OrphanAnalysisPayload) error {
	// Mark home page as used
	homeSlug := oas.ctx.Config.BrandConfig.HomeSlug
	if homeSlug != "" {
		// Find home page storyfragment ID
		var homeID string
		if err := oas.ctx.Database.Conn.QueryRow("SELECT id FROM storyfragments WHERE slug = ?", homeSlug).Scan(&homeID); err == nil {
			if dependents, exists := payload.StoryFragments[homeID]; exists {
				payload.StoryFragments[homeID] = append(dependents, "HOME_PAGE")
			}
		}
	}

	// Check menu ActionLisp references
	rows, err := oas.ctx.Database.Conn.Query("SELECT id, title, options_payload FROM menus")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var menuID, menuTitle, optionsPayload string
		if err := rows.Scan(&menuID, &menuTitle, &optionsPayload); err != nil {
			continue
		}

		// Parse options_payload to find ActionLisp references
		if optionsPayload != "" {
			var options []map[string]interface{}
			if err := json.Unmarshal([]byte(optionsPayload), &options); err == nil {
				for _, option := range options {
					if actionLisp, ok := option["actionLisp"].(string); ok {
						// Parse ActionLisp to find story fragment references
						if targetSlug := oas.parseActionLispTarget(actionLisp); targetSlug != "" {
							// Find storyfragment ID by slug
							var sfID string
							if err := oas.ctx.Database.Conn.QueryRow("SELECT id FROM storyfragments WHERE slug = ?", targetSlug).Scan(&sfID); err == nil {
								if dependents, exists := payload.StoryFragments[sfID]; exists {
									payload.StoryFragments[sfID] = append(dependents, fmt.Sprintf("MENU:%s", menuTitle))
								}
							}
						}
					}
				}
			}
		}
	}

	// Check topic associations
	topicRows, err := oas.ctx.Database.Conn.Query(`
        SELECT sf.id, t.title 
        FROM storyfragments sf
        JOIN storyfragment_has_topic ht ON sf.id = ht.storyfragment_id
        JOIN storyfragment_topics t ON ht.topic_id = t.id
    `)
	if err != nil {
		return err
	}
	defer topicRows.Close()

	for topicRows.Next() {
		var sfID, topicTitle string
		if err := topicRows.Scan(&sfID, &topicTitle); err == nil {
			if dependents, exists := payload.StoryFragments[sfID]; exists {
				payload.StoryFragments[sfID] = append(dependents, fmt.Sprintf("TOPIC:%s", topicTitle))
			}
		}
	}

	return nil
}

// computePaneDependencies finds what depends on each pane
func (oas *OrphanAnalysisService) computePaneDependencies(payload *models.OrphanAnalysisPayload) error {
	// Check storyfragment_panes junction table
	rows, err := oas.ctx.Database.Conn.Query(`
        SELECT sp.pane_id, sf.title 
        FROM storyfragment_panes sp
        JOIN storyfragments sf ON sp.storyfragment_id = sf.id
    `)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var paneID, sfTitle string
		if err := rows.Scan(&paneID, &sfTitle); err == nil {
			if dependents, exists := payload.Panes[paneID]; exists {
				payload.Panes[paneID] = append(dependents, fmt.Sprintf("STORYFRAGMENT:%s", sfTitle))
			}
		}
	}

	return nil
}

// computeMenuDependencies finds what depends on each menu
func (oas *OrphanAnalysisService) computeMenuDependencies(payload *models.OrphanAnalysisPayload) error {
	// Check storyfragments.menu_id references
	rows, err := oas.ctx.Database.Conn.Query(`
        SELECT m.id, sf.title 
        FROM menus m
        JOIN storyfragments sf ON m.id = sf.menu_id
    `)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var menuID, sfTitle string
		if err := rows.Scan(&menuID, &sfTitle); err == nil {
			if dependents, exists := payload.Menus[menuID]; exists {
				payload.Menus[menuID] = append(dependents, fmt.Sprintf("STORYFRAGMENT:%s", sfTitle))
			}
		}
	}

	return nil
}

// computeFileDependencies finds what depends on each file
func (oas *OrphanAnalysisService) computeFileDependencies(payload *models.OrphanAnalysisPayload) error {
	// Check file_panes junction table
	rows, err := oas.ctx.Database.Conn.Query(`
        SELECT fp.file_id, p.title 
        FROM file_panes fp
        JOIN panes p ON fp.pane_id = p.id
    `)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var fileID, paneTitle string
		if err := rows.Scan(&fileID, &paneTitle); err == nil {
			if dependents, exists := payload.Files[fileID]; exists {
				payload.Files[fileID] = append(dependents, fmt.Sprintf("PANE:%s", paneTitle))
			}
		}
	}

	// Check pane options_payload for fileId references
	paneRows, err := oas.ctx.Database.Conn.Query("SELECT id, title, options_payload FROM panes WHERE options_payload LIKE '%fileId%'")
	if err != nil {
		return err
	}
	defer paneRows.Close()

	for paneRows.Next() {
		var paneID, paneTitle, optionsPayload string
		if err := paneRows.Scan(&paneID, &paneTitle, &optionsPayload); err != nil {
			continue
		}

		// Search for fileId references in JSON
		for fileID := range payload.Files {
			if strings.Contains(optionsPayload, fmt.Sprintf(`"fileId":"%s"`, fileID)) {
				if dependents, exists := payload.Files[fileID]; exists {
					payload.Files[fileID] = append(dependents, fmt.Sprintf("PANE:%s", paneTitle))
				}
			}
		}
	}

	return nil
}

// computeResourceDependencies finds what depends on each resource
func (oas *OrphanAnalysisService) computeResourceDependencies(payload *models.OrphanAnalysisPayload) error {
	// TODO: Implement based on V1 logic when resource usage patterns are known
	// This may include category relationships, pane references, etc.
	return nil
}

// computeBeliefDependencies finds what depends on each belief
func (oas *OrphanAnalysisService) computeBeliefDependencies(payload *models.OrphanAnalysisPayload) error {
	// TODO: Implement based on pane belief requirements
	// Check pane held_beliefs and withheld_beliefs fields
	return nil
}

// computeEpinetDependencies finds what depends on each epinet
func (oas *OrphanAnalysisService) computeEpinetDependencies(payload *models.OrphanAnalysisPayload) error {
	// TODO: Implement based on V1 logic when epinet usage patterns are known
	return nil
}

// computeTractStackDependencies finds what depends on each tractstack
func (oas *OrphanAnalysisService) computeTractStackDependencies(payload *models.OrphanAnalysisPayload) error {
	// Check storyfragments that belong to tractstacks
	rows, err := oas.ctx.Database.Conn.Query(`
        SELECT ts.id, sf.title 
        FROM tractstacks ts
        JOIN storyfragments sf ON ts.id = sf.tractstack_id
    `)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tractStackID, sfTitle string
		if err := rows.Scan(&tractStackID, &sfTitle); err == nil {
			if dependents, exists := payload.TractStacks[tractStackID]; exists {
				payload.TractStacks[tractStackID] = append(dependents, fmt.Sprintf("STORYFRAGMENT:%s", sfTitle))
			}
		}
	}

	return nil
}

// parseActionLispTarget extracts target slug from ActionLisp - SIMPLIFIED VERSION
func (oas *OrphanAnalysisService) parseActionLispTarget(actionLisp string) string {
	// Simplified ActionLisp parsing - looks for common patterns
	// This covers most cases from the V1 logic without requiring full lisp parser

	// Look for (goto storyFragment "slug") pattern
	if strings.Contains(actionLisp, "storyFragment") {
		parts := strings.Split(actionLisp, `"`)
		if len(parts) >= 2 {
			return parts[1] // Return the first quoted string
		}
	}

	// Look for (goto storyFragmentPane "slug" "pane") pattern
	if strings.Contains(actionLisp, "storyFragmentPane") {
		parts := strings.Split(actionLisp, `"`)
		if len(parts) >= 2 {
			return parts[1] // Return the first quoted string (slug)
		}
	}

	return ""
}
