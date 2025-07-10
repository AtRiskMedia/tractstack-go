// Package analytics provides analytics data loading and processing functionality.
package analytics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/models"
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

	// 1. Get all epinets for tenant
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
	// log.Printf("DEBUG: Current hour key from formatHourKey: %s", currentHour)

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
		// log.Printf("DEBUG: All epinet-hour pairs cached and within TTL for tenant %s", ctx.TenantID)
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

// loader.go
func processEpinetDataForTimeRange(ctx *tenant.Context, missingEpinetHours map[string][]string, epinets []EpinetConfig,
	analysis *EpinetAnalysis, startTime, endTime time.Time, contentItems map[string]ContentItem,
) error {
	cacheManager := cache.GetGlobalManager()

	// Initialize empty data structure only for missing epinet-hour pairs
	for epinetID, hourKeys := range missingEpinetHours {
		for _, hourKey := range hourKeys {
			// Check if bin already exists (in case it was partially cached)
			_, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey)
			if !exists {
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
				cacheManager.SetHourlyEpinetBin(ctx.TenantID, epinetID, hourKey, bin)
			}
		}
	}

	// Process belief-related data for each epinet
	if len(analysis.BeliefValues) > 0 || len(analysis.IdentifyAsValues) > 0 {
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
			// Process beliefs for this epinet's missing hours
			err := processBeliefData(ctx, hourKeys, []EpinetConfig{epinet}, analysis, startTime, endTime, contentItems)
			if err != nil {
				log.Printf("WARNING: Failed to process belief data for epinet %s: %v", epinetID, err)
				continue
			}
		}
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

func getEpinets(ctx *tenant.Context) ([]EpinetConfig, error) {
	query := `SELECT id, title, options_payload FROM epinets`

	rows, err := ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query epinets: %w", err)
	}
	defer rows.Close()

	var epinets []EpinetConfig
	for rows.Next() {
		var id, title string
		var optionsPayload sql.NullString

		err := rows.Scan(&id, &title, &optionsPayload)
		if err != nil {
			return nil, fmt.Errorf("failed to scan epinet row: %w", err)
		}

		// Parse steps from options_payload using exact V1 logic
		steps, err := parseEpinetSteps(optionsPayload.String)
		if err != nil {
			log.Printf("ERROR: Failed to parse options_payload for epinet %s: %v", id, err)
			continue
		}

		epinets = append(epinets, EpinetConfig{
			ID:    id,
			Title: title,
			Steps: steps,
		})
	}

	return epinets, nil
}

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

// getContentItems retrieves content items for title mapping (exact V1 pattern)
func getContentItems(ctx *tenant.Context) (map[string]ContentItem, error) {
	contentItems := make(map[string]ContentItem)

	// Get StoryFragments
	query := `SELECT id, title, slug FROM storyfragments`
	rows, err := ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query storyfragments: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, slug string
		err := rows.Scan(&id, &title, &slug)
		if err != nil {
			return nil, fmt.Errorf("failed to scan storyfragment row: %w", err)
		}
		contentItems[id] = ContentItem{Title: title, Slug: slug}
	}

	// Get Panes
	query = `SELECT id, title, slug FROM panes`
	rows, err = ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query panes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, slug string
		err := rows.Scan(&id, &title, &slug)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pane row: %w", err)
		}
		contentItems[id] = ContentItem{Title: title, Slug: slug}
	}

	// Get Resources
	query = `SELECT id, title, slug FROM resources`
	rows, err = ctx.Database.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query resources: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, slug string
		err := rows.Scan(&id, &title, &slug)
		if err != nil {
			return nil, fmt.Errorf("failed to scan resource row: %w", err)
		}
		contentItems[id] = ContentItem{Title: title, Slug: slug}
	}

	return contentItems, nil
}

func getHourKeysForTimeRange(hours int) []string {
	var hourKeys []string
	now := time.Now().UTC()
	endHour := now.Truncate(time.Hour) // Current hour, inclusive
	startHour := endHour.Add(-time.Duration(hours) * time.Hour)

	log.Printf("DEBUG: Generating hourKeys from %s to %s", startHour.Format(time.RFC3339), endHour.Format(time.RFC3339))

	for t := startHour; !t.After(endHour); t = t.Add(time.Hour) {
		hourKey := formatHourKey(t)
		hourKeys = append(hourKeys, hourKey)
	}

	return hourKeys
}

// formatHourKey formats a time as an hour key (exact V1 pattern)
func formatHourKey(t time.Time) string {
	return fmt.Sprintf("%d-%02d-%02d-%02d", t.Year(), t.Month(), t.Day(), t.Hour())
}

func LoadCurrentHourData(ctx *tenant.Context) error {
	// log.Printf("DEBUG: LoadCurrentHourData started for tenant %s", ctx.TenantID)
	// Use existing LoadHourlyEpinetData but only for current hour (1 hour back)
	return LoadHourlyEpinetData(ctx, 1)
}
