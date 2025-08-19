// Package services provides application-level services for tailwind operations
package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// TailwindService handles tailwind CSS class extraction and updates
type TailwindService struct {
	paneService   *PaneService
	configService *ConfigService
	logger        *logging.ChanneledLogger
	perfTracker   *performance.Tracker
}

// NewTailwindService creates a new tailwind service
func NewTailwindService(
	paneService *PaneService,
	configService *ConfigService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *TailwindService {
	return &TailwindService{
		paneService:   paneService,
		configService: configService,
		logger:        logger,
		perfTracker:   perfTracker,
	}
}

// GetTailwindClasses extracts Tailwind classes from non-excluded panes plus static whitelist
func (s *TailwindService) GetTailwindClasses(tenantCtx *tenant.Context, excludePaneIDs []string) ([]string, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_tailwind_classes", tenantCtx.TenantID)
	defer marker.Complete()

	// Get all pane IDs
	allPaneIDs, err := s.paneService.GetAllIDs(tenantCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all pane IDs: %w", err)
	}

	// Filter out excluded pane IDs
	excludeMap := make(map[string]bool)
	for _, id := range excludePaneIDs {
		excludeMap[id] = true
	}

	var includedPaneIDs []string
	for _, id := range allPaneIDs {
		if !excludeMap[id] {
			includedPaneIDs = append(includedPaneIDs, id)
		}
	}

	s.logger.System().Debug("Filtering panes for class extraction",
		"totalPanes", len(allPaneIDs),
		"excludedPanes", len(excludePaneIDs),
		"includedPanes", len(includedPaneIDs))

	// Get the included panes (this will populate cache automatically)
	panes, err := s.paneService.GetByIDs(tenantCtx, includedPaneIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get panes by IDs: %w", err)
	}

	// Extract classes from pane options payloads
	classSet := make(map[string]bool)
	for _, pane := range panes {
		if pane.OptionsPayload != nil {
			classes := s.extractClassesFromOptionsPayload(pane.OptionsPayload)
			for _, class := range classes {
				if class != "" {
					classSet[class] = true
				}
			}
		}
	}

	// Load static whitelist from filesystem
	staticClasses, err := s.loadStaticWhitelist(tenantCtx.TenantID)
	if err != nil {
		s.logger.System().Warn("Failed to load static whitelist", "error", err, "tenantId", tenantCtx.TenantID)
		// Continue without static whitelist rather than fail
	} else {
		for _, class := range staticClasses {
			if class != "" {
				classSet[class] = true
			}
		}
	}

	// Convert set to slice
	result := make([]string, 0, len(classSet))
	for class := range classSet {
		result = append(result, class)
	}

	s.logger.System().Info("Successfully extracted Tailwind classes",
		"tenantId", tenantCtx.TenantID,
		"processedPanes", len(panes),
		"extractedClasses", len(result),
		"duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for GetTailwindClasses", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return result, nil
}

// UpdateTailwindCSS writes frontend CSS file and updates styles version in brand config
func (s *TailwindService) UpdateTailwindCSS(tenantCtx *tenant.Context, frontendCSS string) (int64, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("update_tailwind_css", tenantCtx.TenantID)
	defer marker.Complete()

	// Get media path
	mediaPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantCtx.TenantID, "media")
	stylesDir := filepath.Join(mediaPath, "styles")

	// Ensure styles directory exists
	if err := os.MkdirAll(stylesDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create styles directory: %w", err)
	}

	// Write frontend CSS file
	frontendPath := filepath.Join(stylesDir, "frontend.css")

	if err := os.WriteFile(frontendPath, []byte(frontendCSS), 0644); err != nil {
		return 0, fmt.Errorf("failed to write frontend.css: %w", err)
	}

	// Update styles version in brand config
	newVersion := time.Now().Unix()

	// Get current brand config
	currentConfig := tenantCtx.Config.BrandConfig
	if currentConfig == nil {
		return 0, fmt.Errorf("brand configuration not loaded")
	}

	// Update version
	updatedConfig := *currentConfig
	updatedConfig.StylesVer = newVersion

	// Save updated config
	if err := s.configService.SaveBrandConfig(tenantCtx.TenantID, &updatedConfig); err != nil {
		return 0, fmt.Errorf("failed to update brand config: %w", err)
	}

	// Update in-memory config
	tenantCtx.Config.BrandConfig = &updatedConfig

	s.logger.System().Info("Successfully updated Tailwind CSS",
		"tenantId", tenantCtx.TenantID,
		"frontendSize", len(frontendCSS),
		"newStylesVer", newVersion,
		"duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for UpdateTailwindCSS", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return newVersion, nil
}

// extractClassesFromOptionsPayload extracts CSS classes from a pane's options payload
// This implements the business logic from legacy getTailwindWhitelist.ts
func (s *TailwindService) extractClassesFromOptionsPayload(optionsPayload map[string]any) []string {
	var classes []string

	// Look for nodes array in options payload
	if nodes, exists := optionsPayload["nodes"]; exists {
		if nodesArray, ok := nodes.([]any); ok {
			for _, nodeInterface := range nodesArray {
				if node, ok := nodeInterface.(map[string]any); ok {
					// Extract from parentCss array
					if parentCSS, exists := node["parentCss"]; exists {
						if parentCSSArray, ok := parentCSS.([]any); ok {
							for _, cssInterface := range parentCSSArray {
								if cssString, ok := cssInterface.(string); ok {
									// Split on spaces and add individual classes
									for _, class := range splitClasses(cssString) {
										if class != "" {
											classes = append(classes, class)
										}
									}
								}
							}
						}
					}

					// Extract from elementCss string
					if elementCSS, exists := node["elementCss"]; exists {
						if cssString, ok := elementCSS.(string); ok {
							// Split on spaces and add individual classes
							for _, class := range splitClasses(cssString) {
								if class != "" {
									classes = append(classes, class)
								}
							}
						}
					}
				}
			}
		}
	}

	return classes
}

// loadStaticWhitelist loads the static whitelist from filesystem
func (s *TailwindService) loadStaticWhitelist(tenantID string) ([]string, error) {
	whitelistPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantID, "tailwindWhitelist.json")

	data, err := os.ReadFile(whitelistPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read whitelist file: %w", err)
	}

	var whitelist struct {
		Safelist []string `json:"safelist"`
	}

	if err := json.Unmarshal(data, &whitelist); err != nil {
		return nil, fmt.Errorf("failed to parse whitelist JSON: %w", err)
	}

	return whitelist.Safelist, nil
}

// splitClasses splits a CSS class string on whitespace and trims each class
func splitClasses(classString string) []string {
	if classString == "" {
		return nil
	}

	var classes []string
	for _, class := range strings.Fields(classString) {
		trimmed := strings.TrimSpace(class)
		if trimmed != "" {
			classes = append(classes, trimmed)
		}
	}
	return classes
}
