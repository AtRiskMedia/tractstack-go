// Package analytics provides analytics data loading and processing functionality.
package analytics

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// =============================================================================
// Database Query Types (Exact V1 Translation)
// =============================================================================

type EpinetConfig struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Steps []EpinetStep `json:"steps"`
}

type EpinetStep struct {
	GateType   string   `json:"gateType"`   // "belief", "identifyAs", "commitmentAction", "conversionAction"
	Values     []string `json:"values"`     // Verbs or objects to match
	ObjectType string   `json:"objectType"` // "StoryFragment", "Pane"
	ObjectIDs  []string `json:"objectIds"`  // Specific content IDs
	Title      string   `json:"title"`
}

type ActionEvent struct {
	ObjectID      string `json:"objectId"`
	ObjectType    string `json:"objectType"`
	Verb          string `json:"verb"`
	HourKey       string
	FingerprintID string    `json:"fingerprintId"`
	CreatedAt     time.Time `json:"createdAt"`
}

type BeliefEvent struct {
	BeliefID      string  `json:"beliefId"`
	FingerprintID string  `json:"fingerprintId"`
	Verb          string  `json:"verb"`
	Object        *string `json:"object"`
	HourKey       string
	UpdatedAt     time.Time `json:"updatedAt"`
}

type ContentItem struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

type EpinetAnalysis struct {
	BeliefValues     map[string]bool
	IdentifyAsValues map[string]bool
	ActionVerbs      map[string]bool
	ActionTypes      map[string]bool
	ObjectIDs        map[string]bool
}

type StoryfragmentAnalytics struct {
	ID                    string `json:"id"`
	Slug                  string `json:"slug"`
	TotalActions          int    `json:"total_actions"`
	UniqueVisitors        int    `json:"unique_visitors"`
	Last24hActions        int    `json:"last_24h_actions"`
	Last7dActions         int    `json:"last_7d_actions"`
	Last28dActions        int    `json:"last_28d_actions"`
	Last24hUniqueVisitors int    `json:"last_24h_unique_visitors"`
	Last7dUniqueVisitors  int    `json:"last_7d_unique_visitors"`
	Last28dUniqueVisitors int    `json:"last_28d_unique_visitors"`
	TotalLeads            int    `json:"total_leads"`
}

// =============================================================================
// Core Analytics Engine (Exact V1 Pattern Translation)
// =============================================================================

func LoadHourlyEpinetData(ctx *tenant.Context, hoursBack int) error {
	log.Printf("DEBUG: LoadHourlyEpinetData started for tenant %s, hoursBack %d", ctx.TenantID, hoursBack)

	// 1. Get all epinets for tenant (now cache-first)
	epinets, err := getEpinets(ctx)
	if err != nil {
		return fmt.Errorf("failed to get epinets: %w", err)
	}

	if len(epinets) == 0 {
		log.Printf("DEBUG: No epinets found for tenant %s", ctx.TenantID)
		return nil
	}

	// 2. Get content items for node naming
	contentItems, err := getContentItems(ctx)
	if err != nil {
		return fmt.Errorf("failed to get content items: %w", err)
	}

	// 3. Generate hour keys for the time range
	hourKeys := getHourKeysForTimeRange(hoursBack)
	if len(hourKeys) == 0 {
		return nil
	}

	cacheManager := cache.GetGlobalManager()
	// Track missing or expired epinet-hour pairs
	missingEpinetHours := make(map[string][]string) // epinetID -> []hourKey
	now := time.Now().UTC()
	currentHour := formatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))

	// Check cache for each epinet and hour
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			bin, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			// Process if current hour, cache missing, or TTL expired
			ttl := func() time.Duration {
				if hourKey == currentHour {
					return config.CurrentHourTTL
				}
				return config.AnalyticsBinTTL
			}()
			if hourKey == currentHour || !exists || (exists && bin.ComputedAt.Add(ttl).Before(time.Now().UTC())) {
				if missingEpinetHours[epinet.ID] == nil {
					missingEpinetHours[epinet.ID] = []string{}
				}
				missingEpinetHours[epinet.ID] = append(missingEpinetHours[epinet.ID], hourKey)
			}
		}
	}

	if len(missingEpinetHours) == 0 {
		return nil
	}

	// 4. Set up time period for queries
	// Find the overall time range for missing hours
	var firstHourKey, lastHourKey string
	for _, hours := range missingEpinetHours {
		for _, hourKey := range hours {
			if firstHourKey == "" || hourKey < firstHourKey {
				firstHourKey = hourKey
			}
			if lastHourKey == "" || hourKey > lastHourKey {
				lastHourKey = hourKey
			}
		}
	}

	if firstHourKey == "" || lastHourKey == "" {
		return nil
	}

	startTime, err := utils.ParseHourKeyToDate(firstHourKey)
	if err != nil {
		return fmt.Errorf("failed to parse start hour key: %w", err)
	}

	endTime, err := utils.ParseHourKeyToDate(lastHourKey)
	if err != nil {
		return fmt.Errorf("failed to parse end hour key: %w", err)
	}
	endTime = endTime.Add(time.Hour) // Make end time exclusive

	log.Printf("DEBUG: Processing time range %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))

	// 5. Pre-analyze all epinets to determine what data we need
	epinetAnalysis := analyzeEpinets(epinets)

	// 6. Process epinet data for missing epinet-hour pairs
	err = processEpinetDataForTimeRange(ctx, missingEpinetHours, epinets, epinetAnalysis, startTime, endTime, contentItems)
	if err != nil {
		return fmt.Errorf("failed to process epinet data: %w", err)
	}

	return nil
}

// analyzeEpinets pre-analyzes epinets to optimize database queries (exact V1 pattern)
func analyzeEpinets(epinets []EpinetConfig) *EpinetAnalysis {
	analysis := &EpinetAnalysis{
		BeliefValues:     make(map[string]bool),
		IdentifyAsValues: make(map[string]bool),
		ActionVerbs:      make(map[string]bool),
		ActionTypes:      make(map[string]bool),
		ObjectIDs:        make(map[string]bool),
	}

	for _, epinet := range epinets {
		for _, step := range epinet.Steps {
			switch step.GateType {
			case "belief":
				if step.Values != nil {
					for _, val := range step.Values {
						analysis.BeliefValues[val] = true
					}
				}
			case "identifyAs":
				if step.Values != nil {
					for _, val := range step.Values {
						analysis.IdentifyAsValues[val] = true
					}
				}
			case "commitmentAction", "conversionAction":
				if step.Values != nil {
					for _, val := range step.Values {
						analysis.ActionVerbs[val] = true
					}
				}
				if step.ObjectType != "" {
					analysis.ActionTypes[step.ObjectType] = true
				}
				if step.ObjectIDs != nil {
					for _, id := range step.ObjectIDs {
						analysis.ObjectIDs[id] = true
					}
				}
			}
		}
	}

	return analysis
}

func processEpinetDataForTimeRange(ctx *tenant.Context, missingEpinetHours map[string][]string, epinets []EpinetConfig,
	analysis *EpinetAnalysis, startTime, endTime time.Time, contentItems map[string]ContentItem,
) error {
	// Initialize empty data structure for all missing epinet-hour pairs
	err := initializeEpinetDataStructure(ctx, getAllMissingHourKeys(missingEpinetHours), epinets)
	if err != nil {
		return fmt.Errorf("failed to initialize epinet data structure: %w", err)
	}

	// Process action-related data for each epinet
	if len(analysis.ActionVerbs) > 0 {
		for epinetID, hourKeys := range missingEpinetHours {
			// Find the epinet config
			var epinet EpinetConfig
			for _, e := range epinets {
				if e.ID == epinetID {
					epinet = e
					break
				}
			}
			if epinet.ID == "" {
				log.Printf("WARNING: Epinet %s not found for processing", epinetID)
				continue
			}
			// Process actions for this epinet's missing hours
			err := processActionData(ctx, hourKeys, []EpinetConfig{epinet}, analysis, startTime, endTime, contentItems)
			if err != nil {
				log.Printf("WARNING: Failed to process action data for epinet %s: %v", epinetID, err)
				continue
			}
		}
	}

	// Calculate transitions for each missing epinet-hour pair
	for epinetID, hourKeys := range missingEpinetHours {
		for _, hourKey := range hourKeys {
			err := calculateChronologicalTransitions(ctx, epinetID, hourKey)
			if err != nil {
				log.Printf("WARNING: Failed to calculate transitions for epinet %s hour %s: %v", epinetID, hourKey, err)
			}
		}
	}

	return nil
}

// initializeEpinetDataStructure initializes empty data for all epinets and hours (exact V1 pattern)
func initializeEpinetDataStructure(ctx *tenant.Context, hourKeys []string, epinets []EpinetConfig) error {
	cacheManager := cache.GetGlobalManager()

	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			// Check if bin already exists
			_, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				// Create empty bin
				emptyData := &models.HourlyEpinetData{
					Steps:       make(map[string]*models.HourlyEpinetStepData),
					Transitions: make(map[string]map[string]*models.HourlyEpinetTransitionData),
				}

				bin := &models.HourlyEpinetBin{
					Data:       emptyData,
					ComputedAt: time.Now().UTC(),
					TTL: func() time.Duration {
						now := time.Now().UTC()
						currentHourKey := formatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))
						if hourKey == currentHourKey {
							return config.CurrentHourTTL
						}
						return config.AnalyticsBinTTL
					}(),
				}

				cacheManager.SetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey, bin)
			}
		}
	}

	return nil
}

// Helper function to extract all unique hour keys from missing epinet hours
func getAllMissingHourKeys(missingEpinetHours map[string][]string) []string {
	hourKeySet := make(map[string]bool)
	for _, hourKeys := range missingEpinetHours {
		for _, hourKey := range hourKeys {
			hourKeySet[hourKey] = true
		}
	}

	var uniqueHourKeys []string
	for hourKey := range hourKeySet {
		uniqueHourKeys = append(uniqueHourKeys, hourKey)
	}

	return uniqueHourKeys
}

// getContentItems loads content items for node naming
func getContentItems(ctx *tenant.Context) (map[string]ContentItem, error) {
	contentItems := make(map[string]ContentItem)

	// Get story fragments
	query := `SELECT id, title, slug FROM storyfragments`
	rows, err := ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query storyfragments: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, slug string
		if err := rows.Scan(&id, &title, &slug); err != nil {
			continue
		}
		contentItems[id] = ContentItem{Title: title, Slug: slug}
	}

	// Get panes
	query = `SELECT id, title, slug FROM panes`
	rows, err = ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query panes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, slug string
		if err := rows.Scan(&id, &title, &slug); err != nil {
			continue
		}
		contentItems[id] = ContentItem{Title: title, Slug: slug}
	}

	return contentItems, nil
}

// getHourKeysForTimeRange generates hour keys for the specified time range
func getHourKeysForTimeRange(hoursBack int) []string {
	var hourKeys []string
	now := time.Now().UTC()

	for i := 0; i < hoursBack; i++ {
		hourTime := now.Add(-time.Duration(i) * time.Hour)
		hourKey := formatHourKey(time.Date(hourTime.Year(), hourTime.Month(), hourTime.Day(), hourTime.Hour(), 0, 0, 0, time.UTC))
		hourKeys = append(hourKeys, hourKey)
	}

	return hourKeys
}

// formatHourKey formats a time into an hour key string
func formatHourKey(t time.Time) string {
	return fmt.Sprintf("%04d-%02d-%02d-%02d", t.Year(), t.Month(), t.Day(), t.Hour())
}

// LoadCurrentHourData loads analytics data for just the current hour
func LoadCurrentHourData(ctx *tenant.Context) error {
	// Use existing LoadHourlyEpinetData but only for current hour (1 hour back)
	return LoadHourlyEpinetData(ctx, 1)
}

// parseEpinetSteps parses steps from JSON (kept for backward compatibility)
func parseEpinetSteps(optionsPayload string) ([]EpinetStep, error) {
	if optionsPayload == "" {
		return []EpinetStep{}, nil
	}

	var steps []EpinetStep

	var options interface{}
	err := json.Unmarshal([]byte(optionsPayload), &options)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Handle array format
	if optionsArray, ok := options.([]interface{}); ok {
		steps, err = parseStepsArray(optionsArray)
		if err != nil {
			return nil, err
		}
	} else if optionsObject, ok := options.(map[string]interface{}); ok {
		// Handle object format
		if stepsArray, exists := optionsObject["steps"]; exists {
			if stepsSlice, ok := stepsArray.([]interface{}); ok {
				steps, err = parseStepsArray(stepsSlice)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return steps, nil
}

// getEpinets - cache-first wrapper function (maintains existing API)
func getEpinets(ctx *tenant.Context) ([]EpinetConfig, error) {
	// Use cache-first epinet service
	epinetService := content.NewEpinetService(ctx, nil)
	epinetIDs, err := epinetService.GetAllIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get epinet IDs: %w", err)
	}

	epinetNodes, err := epinetService.GetByIDs(epinetIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets: %w", err)
	}

	// Convert EpinetNode to EpinetConfig for compatibility
	var epinets []EpinetConfig
	for _, node := range epinetNodes {
		if node != nil {
			// Convert EpinetNodeStep to EpinetStep
			var steps []EpinetStep
			for _, nodeStep := range node.Steps {
				step := EpinetStep{
					GateType:   nodeStep.GateType,
					Title:      nodeStep.Title,
					Values:     nodeStep.Values,
					ObjectType: "",
					ObjectIDs:  nodeStep.ObjectIDs,
				}
				if nodeStep.ObjectType != nil {
					step.ObjectType = *nodeStep.ObjectType
				}
				steps = append(steps, step)
			}

			epinets = append(epinets, EpinetConfig{
				ID:    node.ID,
				Title: node.Title,
				Steps: steps,
			})
		}
	}

	return epinets, nil
}

// parseStepsArray parses steps from JSON array
func parseStepsArray(stepsArray []interface{}) ([]EpinetStep, error) {
	var steps []EpinetStep

	for _, stepInterface := range stepsArray {
		stepMap, ok := stepInterface.(map[string]interface{})
		if !ok {
			continue
		}

		step := EpinetStep{}

		// Parse gateType
		if gateType, exists := stepMap["gateType"]; exists {
			if gateTypeStr, ok := gateType.(string); ok {
				step.GateType = gateTypeStr
			}
		}

		// Parse title
		if title, exists := stepMap["title"]; exists {
			if titleStr, ok := title.(string); ok {
				step.Title = titleStr
			}
		}

		// Parse values
		if values, exists := stepMap["values"]; exists {
			if valuesArray, ok := values.([]interface{}); ok {
				for _, val := range valuesArray {
					if valStr, ok := val.(string); ok {
						step.Values = append(step.Values, valStr)
					}
				}
			}
		}

		// Parse objectType
		if objectType, exists := stepMap["objectType"]; exists {
			if objectTypeStr, ok := objectType.(string); ok {
				step.ObjectType = objectTypeStr
			}
		}

		// Parse objectIds
		if objectIDs, exists := stepMap["objectIds"]; exists {
			if objectIDsArray, ok := objectIDs.([]interface{}); ok {
				for _, id := range objectIDsArray {
					if idStr, ok := id.(string); ok {
						step.ObjectIDs = append(step.ObjectIDs, idStr)
					}
				}
			}
		}

		steps = append(steps, step)
	}

	return steps, nil
}
