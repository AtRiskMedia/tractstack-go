package services

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/models/content"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// Constants for the dynamic batching strategy.
const (
	eventCountThreshold = 200000
	weeklyBatchSize     = 168 // 7 days * 24 hours
)

// EpinetAnalysis holds pre-analyzed epinet data for optimized queries
type EpinetAnalysis struct {
	BeliefValues     map[string]bool
	IdentifyAsValues map[string]bool
	ActionVerbs      map[string]bool
	ActionTypes      map[string]bool
	ObjectIDs        map[string]bool
}

// CacheWarmingService can ONLY write to cache, never read during building
type CacheWarmingService struct {
	cache cache.WriteOnlyAnalyticsCache
	ctx   *tenant.Context
}

// NewCacheWarmingService creates a new cache warmer service with an explicit context.
func NewCacheWarmingService(cacheInterface cache.WriteOnlyAnalyticsCache, ctx *tenant.Context) *CacheWarmingService {
	return &CacheWarmingService{
		cache: cacheInterface,
		ctx:   ctx,
	}
}

// WarmHourlyEpinetData builds complete bins using a dynamic batching strategy.
func (cws *CacheWarmingService) WarmHourlyEpinetData(hoursBack int) error {
	// ALWAYS warm the full 672+1 hours regardless of request
	const fullAnalyticsRange = 674

	log.Printf("Starting cache warming process for tenant '%s' - full %d hour range (requested: %d)",
		cws.ctx.TenantID, fullAnalyticsRange, hoursBack)

	epinets, err := cws.getEpinets()
	if err != nil || len(epinets) == 0 {
		log.Printf("No epinets found for tenant '%s'. Aborting warming task.", cws.ctx.TenantID)
		return nil
	}
	contentItems, err := cws.getContentItems()
	if err != nil {
		return fmt.Errorf("could not pre-fetch content items: %w", err)
	}

	now := time.Now().UTC()
	fullRangeStartTime := now.Add(-time.Duration(fullAnalyticsRange) * time.Hour)

	estimatedEvents, err := cws.countEventsInRange(fullRangeStartTime, now)
	if err != nil {
		estimatedEvents = eventCountThreshold + 1
	}

	batchSizeInHours := fullAnalyticsRange
	if estimatedEvents > eventCountThreshold {
		batchSizeInHours = weeklyBatchSize
	}

	for startHourOffset := 0; startHourOffset < fullAnalyticsRange; startHourOffset += batchSizeInHours {
		endHourOffset := startHourOffset + batchSizeInHours
		if endHourOffset > fullAnalyticsRange {
			endHourOffset = fullAnalyticsRange
		}

		batchStartTime := now.Add(-time.Duration(endHourOffset) * time.Hour)
		batchEndTime := now.Add(-time.Duration(startHourOffset) * time.Hour)

		analysis := cws.analyzeEpinet(epinets[0])
		allActionEvents, err := cws.getActionEventsForRange(batchStartTime, batchEndTime, analysis)
		if err != nil {
			return fmt.Errorf("batch failed for tenant '%s': could not get action events: %w", cws.ctx.TenantID, err)
		}
		allBeliefEvents, err := cws.getBeliefEventsForRange(batchStartTime, batchEndTime, analysis)
		if err != nil {
			return fmt.Errorf("batch failed for tenant '%s': could not get belief events: %w", cws.ctx.TenantID, err)
		}

		eventsByHour := cws.groupEventsByHour(allActionEvents, allBeliefEvents)

		// Generate ALL hour keys for this batch, not just hours with events
		batchHourKeys := cws.getHourKeysForBatch(startHourOffset, endHourOffset)

		// Process ALL hours in the batch, creating empty bins for hours with no events
		for _, hourKey := range batchHourKeys {
			for _, epinet := range epinets {
				if !cws.needsWarming(epinet.ID, hourKey) {
					continue
				}

				// Get events for this hour (may be empty)
				events, hasEvents := eventsByHour[hourKey]

				var steps map[string]*models.HourlyEpinetStepData
				var transitions map[string]map[string]*models.HourlyEpinetTransitionData

				if hasEvents {
					// Hour has events - build normal steps and transitions
					steps = cws.buildStepsFromEvents(epinet, events.ActionEvents, events.BeliefEvents, contentItems)
					transitions = cws.buildTransitionsFromSteps(steps)
				} else {
					// Hour has no events - create empty structures
					steps = make(map[string]*models.HourlyEpinetStepData)
					transitions = make(map[string]map[string]*models.HourlyEpinetTransitionData)
				}

				bin := &models.HourlyEpinetBin{
					Data: &models.HourlyEpinetData{
						Steps:       steps,
						Transitions: transitions,
					},
					ComputedAt: time.Now().UTC(),
					TTL:        cws.getTTLForHour(hourKey),
				}
				cws.cache.SetHourlyEpinetBin(cws.ctx.TenantID, epinet.ID, hourKey, bin)
			}
		}
	}

	log.Printf("Cache warming process for tenant '%s' completed successfully - full %d hour range ready.", cws.ctx.TenantID, fullAnalyticsRange)
	return nil
}

// getHourKeysForBatch generates hour keys for a specific batch range
func (cws *CacheWarmingService) getHourKeysForBatch(startHourOffset, endHourOffset int) []string {
	now := time.Now().UTC()
	var hourKeys []string

	for i := startHourOffset; i < endHourOffset; i++ {
		hourTime := now.Add(-time.Duration(i) * time.Hour)
		hourKey := utils.FormatHourKey(hourTime)
		hourKeys = append(hourKeys, hourKey)
	}
	return hourKeys
}

// WarmEpinetRange is kept for specific, smaller range warming tasks and does not need batching.
func (cws *CacheWarmingService) WarmEpinetRange(epinetID string, startHour, endHour int) error {
	log.Printf("Starting specific range warming for epinet '%s'...", epinetID)
	epinets, err := cws.getEpinets()
	if err != nil {
		return fmt.Errorf("failed to get epinets: %w", err)
	}
	var targetEpinet *models.EpinetConfig
	for i := range epinets {
		if epinets[i].ID == epinetID {
			targetEpinet = &epinets[i]
			break
		}
	}
	if targetEpinet == nil {
		return fmt.Errorf("epinet not found: %s", epinetID)
	}
	contentItems, err := cws.getContentItems()
	if err != nil {
		return fmt.Errorf("failed to get content items: %w", err)
	}
	hourKeys := utils.GetHourKeysForCustomRange(startHour, endHour)
	for _, hourKey := range hourKeys {
		bin, err := cws.buildCompleteHourlyBin(*targetEpinet, hourKey, contentItems)
		if err != nil {
			log.Printf("ERROR: Failed to build bin for epinet %s hour %s: %v", epinetID, hourKey, err)
			continue
		}
		cws.cache.SetHourlyEpinetBin(cws.ctx.TenantID, epinetID, hourKey, bin)
	}
	return nil
}

// buildCompleteHourlyBin is now primarily used by the specific WarmEpinetRange.
func (cws *CacheWarmingService) buildCompleteHourlyBin(epinet models.EpinetConfig, hourKey string, contentItems map[string]models.ContentItem) (*models.HourlyEpinetBin, error) {
	startTime, endTime, err := cws.getTimeRangeForHour(hourKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hour key %s: %w", hourKey, err)
	}
	analysis := cws.analyzeEpinet(epinet)
	allEvents, err := cws.getActionEventsForRange(startTime, endTime, analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to get action events: %w", err)
	}
	allBeliefs, err := cws.getBeliefEventsForRange(startTime, endTime, analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to get belief events: %w", err)
	}
	steps := cws.buildStepsFromEvents(epinet, allEvents, allBeliefs, contentItems)
	transitions := cws.buildTransitionsFromSteps(steps)
	bin := &models.HourlyEpinetBin{
		Data: &models.HourlyEpinetData{
			Steps:       steps,
			Transitions: transitions,
		},
		ComputedAt: time.Now().UTC(),
		TTL:        cws.getTTLForHour(hourKey),
	}
	return bin, nil
}

// --- Dynamic Batching Helper Functions ---

func (cws *CacheWarmingService) countEventsInRange(startTime, endTime time.Time) (int, error) {
	var actionCount, beliefCount int
	actionQuery := `SELECT COUNT(*) FROM actions WHERE created_at >= ? AND created_at < ?`
	err := cws.ctx.Database.Conn.QueryRow(actionQuery, startTime, endTime).Scan(&actionCount)
	if err != nil {
		return 0, fmt.Errorf("failed to count action events: %w", err)
	}
	beliefQuery := `SELECT COUNT(*) FROM heldbeliefs WHERE updated_at >= ? AND updated_at < ?`
	err = cws.ctx.Database.Conn.QueryRow(beliefQuery, startTime, endTime).Scan(&beliefCount)
	if err != nil {
		return 0, fmt.Errorf("failed to count belief events: %w", err)
	}
	return actionCount + beliefCount, nil
}

type hourlyEvents struct {
	ActionEvents []models.ActionEvent
	BeliefEvents []models.BeliefEvent
}

func (cws *CacheWarmingService) groupEventsByHour(actions []models.ActionEvent, beliefs []models.BeliefEvent) map[string]hourlyEvents {
	grouped := make(map[string]hourlyEvents)
	for _, event := range actions {
		hourKey := utils.FormatHourKey(event.CreatedAt)
		data := grouped[hourKey]
		data.ActionEvents = append(data.ActionEvents, event)
		grouped[hourKey] = data
	}
	for _, event := range beliefs {
		hourKey := utils.FormatHourKey(event.UpdatedAt)
		data := grouped[hourKey]
		data.BeliefEvents = append(data.BeliefEvents, event)
		grouped[hourKey] = data
	}
	return grouped
}

// --- Data Fetching and Processing Helpers ---

func (cws *CacheWarmingService) getActionEventsForRange(startTime, endTime time.Time, analysis *EpinetAnalysis) ([]models.ActionEvent, error) {
	var events []models.ActionEvent
	if len(analysis.ActionVerbs) == 0 {
		return events, nil
	}

	verbPlaceholders := strings.Repeat("?,", len(analysis.ActionVerbs)-1) + "?"
	args := []interface{}{startTime, endTime}
	for verb := range analysis.ActionVerbs {
		args = append(args, verb)
	}

	query := fmt.Sprintf(`
        SELECT object_id, object_type, verb, fingerprint_id, created_at
        FROM actions
        WHERE created_at >= ? AND created_at < ? AND verb IN (%s)
    `, verbPlaceholders)

	rows, err := cws.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query action events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var event models.ActionEvent
		var createdAtStr string // Scan timestamp as string first

		if err := rows.Scan(&event.ObjectID, &event.ObjectType, &event.Verb, &event.FingerprintID, &createdAtStr); err != nil {
			log.Printf("WARN: Failed to scan action event row: %v", err)
			continue
		}

		// Parse the timestamp string into time.Time
		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			// Try alternative timestamp formats if RFC3339 fails
			if createdAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr); err != nil {
				if createdAt, err = time.Parse("2006-01-02T15:04:05.000Z", createdAtStr); err != nil {
					log.Printf("WARN: Failed to parse created_at timestamp '%s': %v", createdAtStr, err)
					continue
				}
			}
		}
		event.CreatedAt = createdAt

		events = append(events, event)
	}

	return events, nil
}

func (cws *CacheWarmingService) getBeliefEventsForRange(startTime, endTime time.Time, analysis *EpinetAnalysis) ([]models.BeliefEvent, error) {
	var events []models.BeliefEvent
	if len(analysis.BeliefValues) == 0 && len(analysis.IdentifyAsValues) == 0 {
		return events, nil
	}

	var allValues []string
	for val := range analysis.BeliefValues {
		allValues = append(allValues, val)
	}
	for val := range analysis.IdentifyAsValues {
		allValues = append(allValues, val)
	}
	if len(allValues) == 0 {
		return events, nil
	}

	valuePlaceholders := strings.Repeat("?,", len(allValues)-1) + "?"
	args := []interface{}{startTime, endTime}
	for _, val := range allValues {
		args = append(args, val)
	}

	query := fmt.Sprintf(`
        SELECT belief_id, fingerprint_id, verb, object, updated_at
        FROM heldbeliefs
        WHERE updated_at >= ? AND updated_at < ? AND object IN (%s)
    `, valuePlaceholders)

	rows, err := cws.ctx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query belief events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var event models.BeliefEvent
		var updatedAtStr string // Scan timestamp as string first

		if err := rows.Scan(&event.BeliefID, &event.FingerprintID, &event.Verb, &event.Object, &updatedAtStr); err != nil {
			log.Printf("WARN: Failed to scan belief event row: %v", err)
			continue
		}

		// Parse the timestamp string into time.Time
		updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			// Try alternative timestamp formats if RFC3339 fails
			if updatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAtStr); err != nil {
				if updatedAt, err = time.Parse("2006-01-02T15:04:05.000Z", updatedAtStr); err != nil {
					log.Printf("WARN: Failed to parse updated_at timestamp '%s': %v", updatedAtStr, err)
					continue
				}
			}
		}
		event.UpdatedAt = updatedAt

		events = append(events, event)
	}

	return events, nil
}

func (cws *CacheWarmingService) getEpinets() ([]models.EpinetConfig, error) {
	if cws.ctx == nil {
		return nil, fmt.Errorf("tenant context not set")
	}
	epinetService := content.NewEpinetService(cws.ctx, nil)
	epinetIDs, err := epinetService.GetAllIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get epinet IDs: %w", err)
	}
	epinetNodes, err := epinetService.GetByIDs(epinetIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets: %w", err)
	}
	var epinets []models.EpinetConfig
	for _, node := range epinetNodes {
		if node != nil {
			var steps []models.EpinetStep
			for _, nodeStep := range node.Steps {
				step := models.EpinetStep{
					GateType:   nodeStep.GateType,
					Title:      nodeStep.Title,
					Values:     nodeStep.Values,
					ObjectType: "",
					ObjectIds:  nodeStep.ObjectIDs,
				}
				if nodeStep.ObjectType != nil {
					step.ObjectType = *nodeStep.ObjectType
				}
				steps = append(steps, step)
			}
			epinets = append(epinets, models.EpinetConfig{
				ID:    node.ID,
				Title: node.Title,
				Steps: steps,
			})
		}
	}
	return epinets, nil
}

func (cws *CacheWarmingService) getContentItems() (map[string]models.ContentItem, error) {
	if cws.ctx == nil {
		return nil, fmt.Errorf("tenant context not set")
	}
	contentItems := make(map[string]models.ContentItem)
	storyFragmentService := content.NewStoryFragmentService(cws.ctx, nil)
	storyFragmentIDs, err := storyFragmentService.GetAllIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get story fragment IDs: %w", err)
	}
	storyFragments, err := storyFragmentService.GetByIDs(storyFragmentIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get story fragments: %w", err)
	}
	for _, sf := range storyFragments {
		if sf != nil {
			contentItems[sf.ID] = models.ContentItem{Title: sf.Title, Slug: sf.Slug}
		}
	}
	paneService := content.NewPaneService(cws.ctx, nil)
	paneIDs, err := paneService.GetAllIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get pane IDs: %w", err)
	}
	panes, err := paneService.GetByIDs(paneIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get panes: %w", err)
	}
	for _, pane := range panes {
		if pane != nil {
			contentItems[pane.ID] = models.ContentItem{Title: pane.Title, Slug: pane.Slug}
		}
	}
	return contentItems, nil
}

func (cws *CacheWarmingService) analyzeEpinet(epinet models.EpinetConfig) *EpinetAnalysis {
	analysis := &EpinetAnalysis{
		BeliefValues:     make(map[string]bool),
		IdentifyAsValues: make(map[string]bool),
		ActionVerbs:      make(map[string]bool),
		ActionTypes:      make(map[string]bool),
		ObjectIDs:        make(map[string]bool),
	}
	for _, step := range epinet.Steps {
		switch step.GateType {
		case "belief":
			for _, val := range step.Values {
				analysis.BeliefValues[val] = true
			}
		case "identifyAs":
			for _, val := range step.Values {
				analysis.IdentifyAsValues[val] = true
			}
		case "commitmentAction", "conversionAction":
			for _, val := range step.Values {
				analysis.ActionVerbs[val] = true
			}
			if step.ObjectType != "" {
				analysis.ActionTypes[step.ObjectType] = true
			}
			for _, id := range step.ObjectIds {
				analysis.ObjectIDs[id] = true
			}
		}
	}
	return analysis
}

func (cws *CacheWarmingService) buildStepsFromEvents(epinet models.EpinetConfig, actionEvents []models.ActionEvent, beliefEvents []models.BeliefEvent, contentItems map[string]models.ContentItem) map[string]*models.HourlyEpinetStepData {
	steps := make(map[string]*models.HourlyEpinetStepData)
	for _, event := range actionEvents {
		for stepIndex, step := range epinet.Steps {
			if cws.eventMatchesStep(event, step) {
				nodeID := cws.getStepNodeID(step, event.ObjectID, event.Verb)
				nodeName := cws.getNodeName(step, event.ObjectID, contentItems, event.Verb)
				if steps[nodeID] == nil {
					steps[nodeID] = &models.HourlyEpinetStepData{
						Visitors:  make(map[string]bool),
						Name:      nodeName,
						StepIndex: stepIndex + 1,
					}
				}
				steps[nodeID].Visitors[event.FingerprintID] = true
			}
		}
	}
	for _, event := range beliefEvents {
		for stepIndex, step := range epinet.Steps {
			if cws.beliefEventMatchesStep(event, step) {
				nodeID := cws.getStepNodeID(step, "", event.BeliefID)
				nodeName := cws.getNodeName(step, "", contentItems, event.BeliefID)
				if steps[nodeID] == nil {
					steps[nodeID] = &models.HourlyEpinetStepData{
						Visitors:  make(map[string]bool),
						Name:      nodeName,
						StepIndex: stepIndex + 1,
					}
				}
				steps[nodeID].Visitors[event.FingerprintID] = true
			}
		}
	}
	return steps
}

func (cws *CacheWarmingService) buildTransitionsFromSteps(steps map[string]*models.HourlyEpinetStepData) map[string]map[string]*models.HourlyEpinetTransitionData {
	transitions := make(map[string]map[string]*models.HourlyEpinetTransitionData)
	visitorSteps := make(map[string][]string)
	for nodeID, stepData := range steps {
		for visitor := range stepData.Visitors {
			if visitorSteps[visitor] == nil {
				visitorSteps[visitor] = make([]string, 0)
			}
			visitorSteps[visitor] = append(visitorSteps[visitor], nodeID)
		}
	}
	for visitor, nodeIDs := range visitorSteps {
		sort.Slice(nodeIDs, func(i, j int) bool {
			return steps[nodeIDs[i]].StepIndex < steps[nodeIDs[j]].StepIndex
		})
		visitorSteps[visitor] = nodeIDs
	}
	for visitor, nodeIDs := range visitorSteps {
		for i := 0; i < len(nodeIDs)-1; i++ {
			fromNode, toNode := nodeIDs[i], nodeIDs[i+1]
			if transitions[fromNode] == nil {
				transitions[fromNode] = make(map[string]*models.HourlyEpinetTransitionData)
			}
			if transitions[fromNode][toNode] == nil {
				transitions[fromNode][toNode] = &models.HourlyEpinetTransitionData{
					Visitors: make(map[string]bool),
				}
			}
			transitions[fromNode][toNode].Visitors[visitor] = true
		}
	}
	return transitions
}

func (cws *CacheWarmingService) eventMatchesStep(event models.ActionEvent, step models.EpinetStep) bool {
	switch step.GateType {
	case "commitmentAction", "conversionAction":
		verbMatch := false
		for _, verb := range step.Values {
			if verb == event.Verb {
				verbMatch = true
				break
			}
		}
		if !verbMatch {
			return false
		}
		if step.ObjectType != "" && step.ObjectType != event.ObjectType {
			return false
		}
		if len(step.ObjectIds) > 0 {
			objectMatch := false
			for _, objID := range step.ObjectIds {
				if objID == event.ObjectID {
					objectMatch = true
					break
				}
			}
			return objectMatch
		}
		return true
	}
	return false
}

func (cws *CacheWarmingService) beliefEventMatchesStep(event models.BeliefEvent, step models.EpinetStep) bool {
	switch step.GateType {
	case "belief", "identifyAs":
		for _, val := range step.Values {
			if val == event.BeliefID {
				return true
			}
		}
	}
	return false
}

func (cws *CacheWarmingService) getStepNodeID(step models.EpinetStep, contentID string, matchedValue string) string {
	parts := []string{step.GateType}
	switch step.GateType {
	case "belief", "identifyAs":
		if len(step.Values) > 0 {
			parts = append(parts, step.Values[0])
		}
	case "commitmentAction", "conversionAction":
		parts = append(parts, step.ObjectType)
		if matchedValue != "" && cws.containsString(step.Values, matchedValue) {
			parts = append(parts, matchedValue)
		} else if len(step.Values) > 0 {
			parts = append(parts, step.Values[0])
		}
		if contentID != "" {
			parts = append(parts, contentID)
		}
	}
	return strings.Join(parts, "_")
}

func (cws *CacheWarmingService) getNodeName(step models.EpinetStep, contentID string, contentItems map[string]models.ContentItem, matchedValue string) string {
	var parts []string
	switch step.GateType {
	case "belief", "identifyAs":
		if step.Title != "" {
			parts = append(parts, step.Title)
		}
		if len(step.Values) > 0 {
			parts = append(parts, step.Values[0])
		}
	case "commitmentAction", "conversionAction":
		if step.Title != "" {
			parts = append(parts, step.Title)
		}
		if matchedValue != "" {
			parts = append(parts, matchedValue)
		}
		if contentID != "" {
			if item, exists := contentItems[contentID]; exists {
				if item.Title != "" {
					parts = append(parts, item.Title)
				} else if item.Slug != "" {
					parts = append(parts, item.Slug)
				}
			}
		}
	}
	if len(parts) == 0 {
		return "Unnamed Step"
	}
	return strings.Join(parts, " - ")
}

func (cws *CacheWarmingService) containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (cws *CacheWarmingService) needsWarming(epinetID, hourKey string) bool {
	return true
}

func (cws *CacheWarmingService) getTimeRangeForHour(hourKey string) (time.Time, time.Time, error) {
	startTime, err := utils.ParseHourKeyToDate(hourKey)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return startTime, startTime.Add(time.Hour), nil
}

// getTTLForHour calculates the Time-To-Live for a given hourly bin.
// This corrects the typo from `CacheWimmingService` to `CacheWarmingService`.
func (cws *CacheWarmingService) getTTLForHour(hourKey string) time.Duration {
	now := time.Now().UTC()
	currentHour := utils.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))
	if hourKey == currentHour {
		return 15 * time.Minute
	}
	return 24 * time.Hour
}

func (cws *CacheWarmingService) getHourKeysForTimeRange(hoursBack int) []string {
	return utils.GetHourKeysForTimeRange(hoursBack)
}
