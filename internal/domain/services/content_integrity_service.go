// Package services provides orphan detection
package services

import (
	"encoding/json"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/admin"
)

type ContentIntegrityService struct{}

func NewContentIntegrityService() *ContentIntegrityService {
	return &ContentIntegrityService{}
}

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

func (s *ContentIntegrityService) BuildOrphanAnalysisPayload(
	contentIDMap *admin.ContentIDMap,
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
			if copy, ok := v["copy"].(string); ok {
				if params, ok := v["codeHookParams"].([]any); ok && len(params) > 0 {
					if strings.HasPrefix(copy, "belief(") ||
						strings.HasPrefix(copy, "toggle(") ||
						strings.HasPrefix(copy, "identifyAs(") {
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
