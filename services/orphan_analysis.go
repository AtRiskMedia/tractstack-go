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

// computePaneDependencies finds what depends on each pane
func (oas *OrphanAnalysisService) computePaneDependencies(payload *models.OrphanAnalysisPayload) error {
	rows, err := oas.ctx.Database.Conn.Query(`
		SELECT sp.pane_id, sp.storyfragment_id 
		FROM storyfragment_panes sp
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var paneID, sfID string
		if err := rows.Scan(&paneID, &sfID); err == nil {
			if dependents, exists := payload.Panes[paneID]; exists {
				payload.Panes[paneID] = append(dependents, sfID)
			}
		}
	}

	return nil
}

// computeMenuDependencies finds what depends on each menu
func (oas *OrphanAnalysisService) computeMenuDependencies(payload *models.OrphanAnalysisPayload) error {
	rows, err := oas.ctx.Database.Conn.Query(`
		SELECT sf.menu_id, sf.id 
		FROM storyfragments sf
		WHERE sf.menu_id IS NOT NULL
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

// computeBeliefDependencies finds what panes depend on each belief
func (oas *OrphanAnalysisService) computeBeliefDependencies(payload *models.OrphanAnalysisPayload) error {
	// Find belief usage in pane options_payload
	paneRows, err := oas.ctx.Database.Conn.Query(`
		SELECT id, options_payload 
		FROM panes 
		WHERE options_payload LIKE '%heldBeliefs%' 
		   OR options_payload LIKE '%withheldBeliefs%'
	`)
	if err != nil {
		return err
	}
	defer paneRows.Close()

	for paneRows.Next() {
		var paneID, optionsPayload string
		if err := paneRows.Scan(&paneID, &optionsPayload); err != nil {
			continue
		}

		// Parse options payload to extract belief references
		var options map[string]interface{}
		if err := json.Unmarshal([]byte(optionsPayload), &options); err != nil {
			continue
		}

		// Check held beliefs
		if heldBeliefs, ok := options["heldBeliefs"].(map[string]interface{}); ok {
			for beliefSlug := range heldBeliefs {
				oas.addBeliefDependency(payload, beliefSlug, paneID)
			}
		}

		// Check withheld beliefs
		if withheldBeliefs, ok := options["withheldBeliefs"].(map[string]interface{}); ok {
			for beliefSlug := range withheldBeliefs {
				oas.addBeliefDependency(payload, beliefSlug, paneID)
			}
		}
	}

	return nil
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
