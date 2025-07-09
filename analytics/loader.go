// Package analytics provides analytics data loading and processing functionality.
package analytics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
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
	ObjectID      string    `json:"objectId"`
	ObjectType    string    `json:"objectType"`
	Verb          string    `json:"verb"`
	FingerprintID string    `json:"fingerprintId"`
	CreatedAt     time.Time `json:"createdAt"`
}

type BeliefEvent struct {
	BeliefID      string    `json:"beliefId"`
	FingerprintID string    `json:"fingerprintId"`
	Verb          string    `json:"verb"`
	Object        *string   `json:"object"` // For identifyAs events
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

// =============================================================================
// Core Analytics Engine (Exact V1 Pattern Translation)
// =============================================================================

// LoadHourlyEpinetData is the primary entry point for analytics data loading
// Translates V1 loadHourlyEpinetData function exactly
func LoadHourlyEpinetData(ctx *tenant.Context, hoursBack int) error {
	log.Printf("DEBUG: LoadHourlyEpinetData started for tenant %s, hoursBack %d", ctx.TenantID, hoursBack)

	// 1. Get all epinets for tenant (exact V1 pattern)
	epinets, err := getEpinets(ctx)
	if err != nil {
		return fmt.Errorf("failed to get epinets: %w", err)
	}

	if len(epinets) == 0 {
		log.Printf("DEBUG: No epinets found for tenant %s", ctx.TenantID)
		return nil
	}

	log.Printf("DEBUG: Found %d epinets for tenant %s", len(epinets), ctx.TenantID)

	// 2. Get content items for node naming (exact V1 pattern)
	contentItems, err := getContentItems(ctx)
	if err != nil {
		return fmt.Errorf("failed to get content items: %w", err)
	}

	// 3. Generate hour keys for the time range (exact V1 pattern)
	hourKeys := getHourKeysForTimeRange(hoursBack)
	if len(hourKeys) == 0 {
		return nil
	}

	// NEW: Check cache first to determine what needs processing
	cacheManager := cache.GetGlobalManager()
	allCached := true
	missingHours := make(map[string]bool)

	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			_, exists := cacheManager.GetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey)
			if !exists {
				allCached = false
				missingHours[hourKey] = true
			}
		}
	}

	if allCached {
		log.Printf("DEBUG: All hours cached, skipping database processing for tenant %s", ctx.TenantID)
		return nil
	}

	// Convert missing hours back to slice for existing logic
	hourKeysToProcess := make([]string, 0, len(missingHours))
	for hourKey := range missingHours {
		hourKeysToProcess = append(hourKeysToProcess, hourKey)
	}

	log.Printf("DEBUG: Processing %d missing hours out of %d total for tenant %s", len(hourKeysToProcess), len(hourKeys), ctx.TenantID)

	// 4. Set up time period for queries (exact V1 pattern) - but only for missing hours
	if len(hourKeysToProcess) == 0 {
		return nil
	}

	firstHourKey := hourKeysToProcess[len(hourKeysToProcess)-1]
	lastHourKey := hourKeysToProcess[0]
	for _, key := range hourKeysToProcess {
		if key < firstHourKey {
			firstHourKey = key
		}
		if key > lastHourKey {
			lastHourKey = key
		}
	}

	startTime, err := parseHourKeyToDate(firstHourKey)
	if err != nil {
		return fmt.Errorf("failed to parse start hour key: %w", err)
	}

	endTime, err := parseHourKeyToDate(lastHourKey)
	if err != nil {
		return fmt.Errorf("failed to parse end hour key: %w", err)
	}
	endTime = endTime.Add(time.Hour) // Add one hour to make it exclusive

	log.Printf("DEBUG: Processing time range %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))

	// 5. Pre-analyze all epinets to determine what data we need (exact V1 pattern)
	epinetAnalysis := analyzeEpinets(epinets)

	// 6. Process epinet data for time range (exact V1 pattern) - but only for missing hours
	err = processEpinetDataForTimeRange(ctx, hourKeysToProcess, epinets, epinetAnalysis, startTime, endTime, contentItems)
	if err != nil {
		return fmt.Errorf("failed to process epinet data: %w", err)
	}

	log.Printf("DEBUG: LoadHourlyEpinetData completed successfully for tenant %s", ctx.TenantID)
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

// processEpinetDataForTimeRange processes data for the specified time range (exact V1 pattern)
func processEpinetDataForTimeRange(ctx *tenant.Context, hourKeys []string, epinets []EpinetConfig,
	analysis *EpinetAnalysis, startTime, endTime time.Time, contentItems map[string]ContentItem,
) error {
	// Initialize empty data structure for all hours and epinets (exact V1 pattern)
	err := initializeEpinetDataStructure(ctx, hourKeys, epinets)
	if err != nil {
		return fmt.Errorf("failed to initialize data structure: %w", err)
	}

	// Process all belief-related data in one consolidated query (exact V1 pattern)
	if len(analysis.BeliefValues) > 0 || len(analysis.IdentifyAsValues) > 0 {
		err = processBeliefData(ctx, hourKeys, epinets, analysis, startTime, endTime, contentItems)
		if err != nil {
			return fmt.Errorf("failed to process belief data: %w", err)
		}
	}

	// Process all action-related data in one consolidated query (exact V1 pattern)
	if len(analysis.ActionVerbs) > 0 {
		err = processActionData(ctx, hourKeys, epinets, analysis, startTime, endTime, contentItems)
		if err != nil {
			return fmt.Errorf("failed to process action data: %w", err)
		}
	}

	// Calculate transitions for each epinet and hour (exact V1 pattern)
	for _, epinet := range epinets {
		for _, hourKey := range hourKeys {
			err = calculateChronologicalTransitions(ctx, epinet.ID, hourKey)
			if err != nil {
				log.Printf("WARNING: Failed to calculate transitions for epinet %s hour %s: %v", epinet.ID, hourKey, err)
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
					ComputedAt: time.Now(),
					TTL:        cache.GetTTLForHour(hourKey),
				}

				cacheManager.SetHourlyEpinetBin(ctx.TenantID, epinet.ID, hourKey, bin)
			}
		}
	}

	return nil
}

// =============================================================================
// Database Query Functions (Exact V1 Translation)
// =============================================================================

// getEpinets retrieves all epinets for the tenant (exact V1 pattern)
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

	log.Printf("DEBUG: Retrieved %d epinets", len(epinets))
	return epinets, nil
}

// parseEpinetSteps parses epinet steps from JSON options payload (exact V1 pattern)
func parseEpinetSteps(optionsPayload string) ([]EpinetStep, error) {
	if optionsPayload == "" {
		return []EpinetStep{}, nil
	}

	// Exact V1 parsing logic from epinetLoader.ts
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

// parseStepsArray parses an array of step objects (exact V1 pattern)
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

// =============================================================================
// Time Helper Functions (Exact V1 Translation)
// =============================================================================

// getHourKeysForTimeRange generates hour keys for the specified range (exact V1 pattern)
func getHourKeysForTimeRange(hours int) []string {
	var hourKeys []string

	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)

	for i := 0; i < hours; i++ {
		hourTime := currentHour.Add(-time.Duration(i) * time.Hour)
		hourKey := formatHourKey(hourTime)
		hourKeys = append(hourKeys, hourKey)
	}

	return hourKeys
}

// formatHourKey formats a time as an hour key (exact V1 pattern)
func formatHourKey(t time.Time) string {
	return fmt.Sprintf("%d-%02d-%02d-%02d", t.Year(), t.Month(), t.Day(), t.Hour())
}

// parseHourKeyToDate parses an hour key back to a time (exact V1 pattern)
func parseHourKeyToDate(hourKey string) (time.Time, error) {
	parts := strings.Split(hourKey, "-")
	if len(parts) != 4 {
		return time.Time{}, fmt.Errorf("invalid hour key format: %s", hourKey)
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid year in hour key: %s", hourKey)
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid month in hour key: %s", hourKey)
	}

	day, err := strconv.Atoi(parts[2])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day in hour key: %s", hourKey)
	}

	hour, err := strconv.Atoi(parts[3])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid hour in hour key: %s", hourKey)
	}

	return time.Date(year, time.Month(month), day, hour, 0, 0, 0, time.UTC), nil
}
