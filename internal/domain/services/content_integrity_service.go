// Package services provides orphan detection
package services

import (
	"encoding/json"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
)

// ContentIntegrityService provides logic for analyzing content relationships and detecting orphaned references.
type ContentIntegrityService struct{}

// NewContentIntegrityService creates and returns a new instance of ContentIntegrityService.
func NewContentIntegrityService() *ContentIntegrityService {
	return &ContentIntegrityService{}
}

// CalculateOrphans identifies content items (fragments, panes, menus, files, beliefs) that have no incoming references.
func (s *ContentIntegrityService) CalculateOrphans(
	contentIDMap *admin.ContentIDMap,
	storyFragmentDeps map[string][]string,
	paneDeps map[string][]string,
	menuDeps map[string][]string,
	fileDeps map[string][]string,
	beliefDeps map[string][]string,
) []string {
	var orphans []string

	for sfID := range contentIDMap.StoryFragments {
		if len(storyFragmentDeps[sfID]) == 0 {
			orphans = append(orphans, sfID)
		}
	}

	for paneID := range contentIDMap.Panes {
		if len(paneDeps[paneID]) == 0 {
			orphans = append(orphans, paneID)
		}
	}

	for menuID := range contentIDMap.Menus {
		if len(menuDeps[menuID]) == 0 {
			orphans = append(orphans, menuID)
		}
	}

	for fileID := range contentIDMap.Files {
		if len(fileDeps[fileID]) == 0 {
			orphans = append(orphans, fileID)
		}
	}

	for beliefID := range contentIDMap.Beliefs {
		if len(beliefDeps[beliefID]) == 0 {
			orphans = append(orphans, beliefID)
		}
	}

	return orphans
}

// AnalyzeFileReferences extracts a list of file IDs referenced within a given JSON options payload.
func (s *ContentIntegrityService) AnalyzeFileReferences(optionsPayload string) []string {
	var fileIDs []string
	if optionsPayload == "" {
		return fileIDs
	}

	var options map[string]any
	if err := json.Unmarshal([]byte(optionsPayload), &options); err != nil {
		return fileIDs
	}

	s.scanForFileIDs(options, &fileIDs)
	return fileIDs
}

// AnalyzeBeliefReferences identifies belief slugs referenced in a given JSON options payload, including widget parameters.
func (s *ContentIntegrityService) AnalyzeBeliefReferences(optionsPayload string) []string {
	var beliefSlugs []string
	if optionsPayload == "" {
		return beliefSlugs
	}

	var options map[string]any
	if err := json.Unmarshal([]byte(optionsPayload), &options); err != nil {
		return beliefSlugs
	}

	if heldBeliefs, ok := options["heldBeliefs"].(map[string]any); ok {
		for beliefSlug := range heldBeliefs {
			if beliefSlug != "MATCH-ACROSS" && beliefSlug != "LINKED-BELIEFS" {
				beliefSlugs = append(beliefSlugs, beliefSlug)
			}
		}
	}

	if withheldBeliefs, ok := options["withheldBeliefs"].(map[string]any); ok {
		for beliefSlug := range withheldBeliefs {
			if beliefSlug != "MATCH-ACROSS" && beliefSlug != "LINKED-BELIEFS" {
				beliefSlugs = append(beliefSlugs, beliefSlug)
			}
		}
	}

	s.scanForBeliefWidgets(options, &beliefSlugs)
	return beliefSlugs
}

// AnalyzeActionLispReferences parses an Action Lisp string to extract referenced content slugs (e.g., from "goto" or "navigate" commands).
func (s *ContentIntegrityService) AnalyzeActionLispReferences(actionLisp, homeSlug string) []string {
	var slugs []string
	if actionLisp == "" {
		return slugs
	}

	lines := strings.Split(actionLisp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "goto") || strings.Contains(line, "navigate") {
			parts := strings.Split(line, "\"")
			for i := 1; i < len(parts); i += 2 {
				slug := strings.TrimSpace(parts[i])
				if slug != "" && !s.isSystemSlug(slug) {
					cleanSlug := s.cleanSlug(slug, homeSlug)
					if cleanSlug != "" {
						slugs = append(slugs, cleanSlug)
					}
				}
			}
		}
	}
	return slugs
}

// BuildOrphanAnalysisPayload constructs the final response object containing all identified orphans grouped by type.
func (s *ContentIntegrityService) BuildOrphanAnalysisPayload(
	storyFragmentDeps map[string][]string,
	paneDeps map[string][]string,
	menuDeps map[string][]string,
	fileDeps map[string][]string,
	beliefDeps map[string][]string,
) *admin.OrphanAnalysisPayload {
	return &admin.OrphanAnalysisPayload{
		StoryFragments: storyFragmentDeps,
		Panes:          paneDeps,
		Menus:          menuDeps,
		Files:          fileDeps,
		Beliefs:        beliefDeps,
		Status:         "complete",
	}
}

func (s *ContentIntegrityService) scanForFileIDs(data any, fileIDs *[]string) {
	switch v := data.(type) {
	case map[string]any:
		if fileID, ok := v["fileId"].(string); ok && fileID != "" {
			*fileIDs = append(*fileIDs, fileID)
		}
		for _, value := range v {
			s.scanForFileIDs(value, fileIDs)
		}
	case []any:
		for _, item := range v {
			s.scanForFileIDs(item, fileIDs)
		}
	}
}

func (s *ContentIntegrityService) scanForBeliefWidgets(data any, beliefSlugs *[]string) {
	switch v := data.(type) {
	case map[string]any:
		if tagName, ok := v["tagName"].(string); ok && tagName == "code" {
			if copyVal, ok := v["copy"].(string); ok {
				if params, ok := v["codeHookParams"].([]any); ok && len(params) > 0 {
					if strings.HasPrefix(copyVal, "belief(") ||
						strings.HasPrefix(copyVal, "toggle(") ||
						strings.HasPrefix(copyVal, "identifyAs(") {
						if beliefSlug, ok := params[0].(string); ok && beliefSlug != "" {
							*beliefSlugs = append(*beliefSlugs, beliefSlug)
						}
					}
				}
			}
		}
		for _, value := range v {
			s.scanForBeliefWidgets(value, beliefSlugs)
		}
	case []any:
		for _, item := range v {
			s.scanForBeliefWidgets(item, beliefSlugs)
		}
	}
}

func (s *ContentIntegrityService) isSystemSlug(slug string) bool {
	systemPrefixes := []string{"storykeep", "concierge", "context", "sandbox"}
	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(slug, prefix) {
			return true
		}
	}
	return false
}

func (s *ContentIntegrityService) cleanSlug(url, homeSlug string) string {
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

	if s.isSystemSlug(url) {
		return ""
	}

	return url
}
