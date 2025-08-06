// Package services provides startup warming orchestration
package services

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/analytics"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/cleanup"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/interfaces"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/utilities"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/templates"
)

const (
	eventCountThreshold = 200000
	weeklyBatchSize     = 168 // 7 days * 24 hours
)

type EpinetAnalysis struct {
	BeliefValues     map[string]bool
	IdentifyAsValues map[string]bool
	ActionVerbs      map[string]bool
	ActionTypes      map[string]bool
	ObjectIDs        map[string]bool
}

type WarmingService struct {
	logger                  *logging.ChanneledLogger
	perfTracker             *performance.Tracker
	beliefEvaluationService *BeliefEvaluationService
}

func NewWarmingService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker, beliefEvaluationService *BeliefEvaluationService) *WarmingService {
	return &WarmingService{
		logger:                  logger,
		perfTracker:             perfTracker,
		beliefEvaluationService: beliefEvaluationService,
	}
}

func (ws *WarmingService) WarmAllTenants(tenantManager *tenant.Manager, cache interfaces.Cache, contentMapSvc *ContentMapService, beliefRegistrySvc *BeliefRegistryService, reporter *cleanup.Reporter) error {
	start := time.Now()

	tenants, err := ws.getActiveTenants()
	if err != nil {
		return fmt.Errorf("failed to get active tenants: %w", err)
	}

	reporter.LogHeader(fmt.Sprintf("Cache Warming %d Tenants", len(tenants)))

	var successCount int
	for _, tenantID := range tenants {
		tenantCtx, err := tenantManager.NewContextFromID(tenantID)
		if err != nil {
			reporter.LogError(fmt.Sprintf("Failed to create context for tenant %s", tenantID), err)
			ws.logger.Cache().Error("Failed to create context for tenant during warming", "tenantId", tenantID, "error", err)
			continue
		}

		if err := ws.WarmTenant(tenantCtx, tenantID, cache, contentMapSvc, beliefRegistrySvc, reporter); err != nil {
			reporter.LogError(fmt.Sprintf("Failed to warm tenant %s", tenantID), err)
			ws.logger.Cache().Error("Failed to warm tenant", "tenantId", tenantID, "error", err)
		} else {
			successCount++
		}
		tenantCtx.Close()
	}

	duration := time.Since(start)
	durationMs := float64(duration) / float64(time.Millisecond)
	reporter.LogSubHeader(fmt.Sprintf("Strategic Warming Completed in %.2fms", durationMs))
	reporter.LogSuccess("%d/%d tenants warmed successfully", successCount, len(tenants))
	ws.logger.Cache().Info("Strategic warming completed for all tenants", "successCount", successCount, "totalTenants", len(tenants), "duration", duration)

	if successCount < len(tenants) {
		return fmt.Errorf("warming failed for %d tenants", len(tenants)-successCount)
	}

	return nil
}

func (ws *WarmingService) WarmTenant(tenantCtx *tenant.Context, tenantID string, cache interfaces.Cache, contentMapSvc *ContentMapService, beliefRegistrySvc *BeliefRegistryService, reporter *cleanup.Reporter) error {
	start := time.Now()
	reporter.LogSubHeader(fmt.Sprintf("Warming Tenant: %s", tenantID))
	ws.logger.Cache().Info("Starting strategic warming for tenant", "tenantId", tenantID)

	// Warm the unified content map.
	if err := ws.warmContentMap(tenantCtx, contentMapSvc, cache); err != nil {
		return fmt.Errorf("content map warming failed: %w", err)
	}
	reporter.LogStepSuccess("Content Map warmed")
	ws.logger.Cache().Debug("Content map warmed", "tenantId", tenantID)

	// Bulk-load all content types into the cache.
	if _, err := NewTractStackService(ws.logger, ws.perfTracker, contentMapSvc).GetAllIDs(tenantCtx); err != nil {
		reporter.LogWarning("Failed to warm TractStacks: %v", err)
		ws.logger.Cache().Warn("Failed to warm TractStacks", "tenantId", tenantID, "error", err)
	}
	if _, err := NewStoryFragmentService(ws.logger, ws.perfTracker, contentMapSvc).GetAllIDs(tenantCtx); err != nil {
		reporter.LogWarning("Failed to warm StoryFragments: %v", err)
		ws.logger.Cache().Warn("Failed to warm StoryFragments", "tenantId", tenantID, "error", err)
	}
	if _, err := NewPaneService(ws.logger, ws.perfTracker, contentMapSvc).GetAllIDs(tenantCtx); err != nil {
		reporter.LogWarning("Failed to warm Panes: %v", err)
		ws.logger.Cache().Warn("Failed to warm Panes", "tenantId", tenantID, "error", err)
	}
	if _, err := NewMenuService(ws.logger, ws.perfTracker, contentMapSvc).GetAllIDs(tenantCtx); err != nil {
		reporter.LogWarning("Failed to warm Menus: %v", err)
		ws.logger.Cache().Warn("Failed to warm Menus", "tenantId", tenantID, "error", err)
	}
	if _, err := NewResourceService(ws.logger, ws.perfTracker, contentMapSvc).GetAllIDs(tenantCtx); err != nil {
		reporter.LogWarning("Failed to warm Resources: %v", err)
		ws.logger.Cache().Warn("Failed to warm Resources", "tenantId", tenantID, "error", err)
	}
	if _, err := NewBeliefService(ws.logger, ws.perfTracker, contentMapSvc).GetAllIDs(tenantCtx); err != nil {
		reporter.LogWarning("Failed to warm Beliefs: %v", err)
		ws.logger.Cache().Warn("Failed to warm Beliefs", "tenantId", tenantID, "error", err)
	}
	if _, err := NewEpinetService(ws.logger, ws.perfTracker, contentMapSvc).GetAllIDs(tenantCtx); err != nil {
		reporter.LogWarning("Failed to warm Epinets: %v", err)
		ws.logger.Cache().Warn("Failed to warm Epinets", "tenantId", tenantID, "error", err)
	}
	if _, err := NewImageFileService(ws.logger, ws.perfTracker, contentMapSvc).GetAllIDs(tenantCtx); err != nil {
		reporter.LogWarning("Failed to warm ImageFiles: %v", err)
		ws.logger.Cache().Warn("Failed to warm ImageFiles", "tenantId", tenantID, "error", err)
	}
	reporter.LogStepSuccess("Content Repositories Warmed")
	ws.logger.Cache().Debug("Content repositories warmed", "tenantId", tenantID)

	// Build Belief Registries for all Storyfragments
	storyFragmentIDs, err := NewStoryFragmentService(ws.logger, ws.perfTracker, contentMapSvc).GetAllIDs(tenantCtx)
	if err != nil {
		reporter.LogWarning("Could not retrieve StoryFragment IDs for belief registry warming: %v", err)
		ws.logger.Cache().Warn("Could not retrieve StoryFragment IDs for belief registry warming", "tenantId", tenantID, "error", err)
	} else {
		paneService := NewPaneService(ws.logger, ws.perfTracker, contentMapSvc)
		for _, sfID := range storyFragmentIDs {
			sf, err := NewStoryFragmentService(ws.logger, ws.perfTracker, contentMapSvc).GetByID(tenantCtx, sfID)
			if err != nil || sf == nil {
				continue
			}
			// Load only the panes required for this specific storyfragment.
			if len(sf.PaneIDs) > 0 {
				panes, err := paneService.GetByIDs(tenantCtx, sf.PaneIDs)
				if err != nil {
					log.Printf("Warning: Failed to load panes for storyfragment %s during warming: %v", sfID, err)
					ws.logger.Cache().Warn("Failed to load panes for storyfragment during warming", "tenantId", tenantID, "storyFragmentId", sfID, "error", err)
					continue
				}
				// Build and cache the registry.
				if _, err := beliefRegistrySvc.BuildRegistryFromLoadedPanes(tenantCtx, sfID, panes); err != nil {
					log.Printf("Warning: Failed to build belief registry for storyfragment %s: %v", sfID, err)
					ws.logger.Cache().Warn("Failed to build belief registry for storyfragment", "tenantId", tenantID, "storyFragmentId", sfID, "error", err)
				}
			}
		}
		reporter.LogStepSuccess("%d StoryFragment<>Beliefs registries cached", len(storyFragmentIDs))
		ws.logger.Cache().Debug("StoryFragment belief registries cached", "tenantId", tenantID, "count", len(storyFragmentIDs))
	}

	duration := time.Since(start)
	durationMs := float64(duration) / float64(time.Millisecond)
	reporter.LogStepSuccess("Tenant %s strategically warmed in %.2fms", tenantID, durationMs)
	ws.logger.Cache().Info("Strategic warming completed for tenant", "tenantId", tenantID, "duration", duration)

	return nil
}

func (ws *WarmingService) WarmHourlyEpinetData(tenantCtx *tenant.Context, cache interfaces.WriteOnlyAnalyticsCache, hoursBack int) error {
	const fullAnalyticsRange = 674

	log.Printf("Starting analytics cache warming for tenant '%s' - full %d hour range (requested: %d)",
		tenantCtx.TenantID, fullAnalyticsRange, hoursBack)
	ws.logger.Cache().Info("Starting analytics cache warming", "tenantId", tenantCtx.TenantID, "range", fullAnalyticsRange, "requestedHours", hoursBack)

	epinets, err := ws.getEpinets(tenantCtx)
	if err != nil || len(epinets) == 0 {
		log.Printf("No epinets found for tenant '%s'. Aborting analytics warming task.", tenantCtx.TenantID)
		ws.logger.Cache().Info("No epinets found for tenant. Aborting analytics warming task.", "tenantId", tenantCtx.TenantID)
		return nil
	}
	contentItems, err := ws.getContentItems(tenantCtx)
	if err != nil {
		ws.logger.Cache().Error("Could not pre-fetch content items for analytics warming", "tenantId", tenantCtx.TenantID, "error", err)
		return fmt.Errorf("could not pre-fetch content items: %w", err)
	}

	now := time.Now().UTC()
	fullRangeStartTime := now.Add(-time.Duration(fullAnalyticsRange) * time.Hour)

	estimatedEvents, err := ws.countEventsInRange(tenantCtx, fullRangeStartTime, now)
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

		analysis := ws.analyzeEpinet(epinets[0])
		allActionEvents, err := ws.getActionEventsForRange(tenantCtx, batchStartTime, batchEndTime, analysis)
		if err != nil {
			ws.logger.Cache().Error("Analytics warming batch failed: could not get action events", "tenantId", tenantCtx.TenantID, "error", err)
			return fmt.Errorf("batch failed for tenant '%s': could not get action events: %w", tenantCtx.TenantID, err)
		}
		allBeliefEvents, err := ws.getBeliefEventsForRange(tenantCtx, batchStartTime, batchEndTime, analysis)
		if err != nil {
			ws.logger.Cache().Error("Analytics warming batch failed: could not get belief events", "tenantId", tenantCtx.TenantID, "error", err)
			return fmt.Errorf("batch failed for tenant '%s': could not get belief events: %w", tenantCtx.TenantID, err)
		}

		eventsByHour := ws.groupEventsByHour(allActionEvents, allBeliefEvents)
		batchHourKeys := ws.getHourKeysForBatch(startHourOffset, endHourOffset)

		for _, hourKey := range batchHourKeys {
			for _, epinet := range epinets {
				events, hasEvents := eventsByHour[hourKey]

				var steps map[string]*types.HourlyEpinetStepData
				var transitions map[string]map[string]*types.HourlyEpinetTransitionData

				if hasEvents {
					steps = ws.buildStepsFromEvents(epinet, events.ActionEvents, events.BeliefEvents, contentItems)
					transitions = ws.buildTransitionsFromSteps(steps)
				} else {
					steps = make(map[string]*types.HourlyEpinetStepData)
					transitions = make(map[string]map[string]*types.HourlyEpinetTransitionData)
				}

				bin := &types.HourlyEpinetBin{
					Data: &types.HourlyEpinetData{
						Steps:       steps,
						Transitions: transitions,
					},
					ComputedAt: time.Now().UTC(),
					TTL:        ws.getTTLForHour(hourKey),
				}
				cache.SetHourlyEpinetBin(tenantCtx.TenantID, epinet.ID, hourKey, bin)
			}
		}
	}

	log.Printf("Analytics cache warming process for tenant '%s' completed successfully.", tenantCtx.TenantID)
	ws.logger.Cache().Info("Analytics cache warming process completed successfully", "tenantId", tenantCtx.TenantID)
	return nil
}

func (ws *WarmingService) WarmRecentHours(tenantCtx *tenant.Context, cache interfaces.WriteOnlyAnalyticsCache, missingHourKeys []string) error {
	if len(missingHourKeys) == 0 {
		return nil
	}

	log.Printf("Rapid catch-up refresh for tenant '%s' - %d hours", tenantCtx.TenantID, len(missingHourKeys))
	ws.logger.Cache().Info("Starting rapid catch-up refresh", "tenantId", tenantCtx.TenantID, "missingHours", len(missingHourKeys))

	epinets, err := ws.getEpinets(tenantCtx)
	if err != nil || len(epinets) == 0 {
		ws.logger.Cache().Info("No epinets found for tenant. Aborting rapid catch-up.", "tenantId", tenantCtx.TenantID)
		return nil
	}

	contentItems, err := ws.getContentItems(tenantCtx)
	if err != nil {
		ws.logger.Cache().Error("Failed to get content items for rapid catch-up", "tenantId", tenantCtx.TenantID, "error", err)
		return fmt.Errorf("failed to get content items: %w", err)
	}

	now := time.Now().UTC()
	oldestHour, err := utilities.ParseHourKeyToDate(missingHourKeys[len(missingHourKeys)-1])
	if err != nil {
		ws.logger.Cache().Error("Invalid hour key during rapid catch-up", "tenantId", tenantCtx.TenantID, "hourKey", missingHourKeys[len(missingHourKeys)-1], "error", err)
		return fmt.Errorf("invalid hour key: %w", err)
	}

	analysis := ws.analyzeEpinet(epinets[0])
	allActionEvents, err := ws.getActionEventsForRange(tenantCtx, oldestHour, now, analysis)
	if err != nil {
		ws.logger.Cache().Error("Failed to get action events for rapid catch-up", "tenantId", tenantCtx.TenantID, "error", err)
		return fmt.Errorf("failed to get action events: %w", err)
	}
	allBeliefEvents, err := ws.getBeliefEventsForRange(tenantCtx, oldestHour, now, analysis)
	if err != nil {
		ws.logger.Cache().Error("Failed to get belief events for rapid catch-up", "tenantId", tenantCtx.TenantID, "error", err)
		return fmt.Errorf("failed to get belief events: %w", err)
	}

	eventsByHour := ws.groupEventsByHour(allActionEvents, allBeliefEvents)

	for _, hourKey := range missingHourKeys {
		for _, epinet := range epinets {
			events, hasEvents := eventsByHour[hourKey]

			var steps map[string]*types.HourlyEpinetStepData
			var transitions map[string]map[string]*types.HourlyEpinetTransitionData

			if hasEvents {
				steps = ws.buildStepsFromEvents(epinet, events.ActionEvents, events.BeliefEvents, contentItems)
				transitions = ws.buildTransitionsFromSteps(steps)
			} else {
				steps = make(map[string]*types.HourlyEpinetStepData)
				transitions = make(map[string]map[string]*types.HourlyEpinetTransitionData)
			}

			bin := &types.HourlyEpinetBin{
				Data: &types.HourlyEpinetData{
					Steps:       steps,
					Transitions: transitions,
				},
				ComputedAt: time.Now().UTC(),
				TTL:        ws.getTTLForHour(hourKey),
			}
			cache.SetHourlyEpinetBin(tenantCtx.TenantID, epinet.ID, hourKey, bin)
		}
	}

	log.Printf("Rapid catch-up completed for tenant '%s'", tenantCtx.TenantID)
	ws.logger.Cache().Info("Rapid catch-up completed", "tenantId", tenantCtx.TenantID)
	return nil
}

func (ws *WarmingService) warmContentMap(tenantCtx *tenant.Context, contentMapSvc *ContentMapService, cache interfaces.Cache) error {
	_, _, err := contentMapSvc.GetContentMap(tenantCtx, "", cache)
	if err != nil {
		return fmt.Errorf("failed to warm content map: %w", err)
	}
	return nil
}

func (ws *WarmingService) getActiveTenants() ([]string, error) {
	registry, err := tenant.LoadTenantRegistry()
	if err != nil {
		return nil, err
	}
	activeTenants := make([]string, 0)
	for tenantID, tenantInfo := range registry.Tenants {
		if tenantInfo.Status == "active" {
			activeTenants = append(activeTenants, tenantID)
		}
	}
	return activeTenants, nil
}

type hourlyEvents struct {
	ActionEvents []analytics.ActionEvent
	BeliefEvents []analytics.BeliefEvent
}

func (ws *WarmingService) groupEventsByHour(actions []analytics.ActionEvent, beliefs []analytics.BeliefEvent) map[string]hourlyEvents {
	grouped := make(map[string]hourlyEvents)
	for _, event := range actions {
		hourKey := utilities.FormatHourKey(event.CreatedAt)
		data := grouped[hourKey]
		data.ActionEvents = append(data.ActionEvents, event)
		grouped[hourKey] = data
	}
	for _, event := range beliefs {
		hourKey := utilities.FormatHourKey(event.UpdatedAt)
		data := grouped[hourKey]
		data.BeliefEvents = append(data.BeliefEvents, event)
		grouped[hourKey] = data
	}
	return grouped
}

func (ws *WarmingService) countEventsInRange(tenantCtx *tenant.Context, startTime, endTime time.Time) (int, error) {
	var actionCount, beliefCount int
	actionQuery := `SELECT COUNT(*) FROM actions WHERE created_at >= ? AND created_at < ?`
	err := tenantCtx.Database.Conn.QueryRow(actionQuery, startTime, endTime).Scan(&actionCount)
	if err != nil {
		return 0, fmt.Errorf("failed to count action events: %w", err)
	}
	beliefQuery := `SELECT COUNT(*) FROM heldbeliefs WHERE updated_at >= ? AND updated_at < ?`
	err = tenantCtx.Database.Conn.QueryRow(beliefQuery, startTime, endTime).Scan(&beliefCount)
	if err != nil {
		return 0, fmt.Errorf("failed to count belief events: %w", err)
	}
	return actionCount + beliefCount, nil
}

func (ws *WarmingService) parseTimestamp(timestampStr string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, timestampStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse timestamp format: %s", timestampStr)
}

func (ws *WarmingService) getActionEventsForRange(tenantCtx *tenant.Context, startTime, endTime time.Time, analysis *EpinetAnalysis) ([]analytics.ActionEvent, error) {
	var events []analytics.ActionEvent
	if len(analysis.ActionVerbs) == 0 {
		return events, nil
	}

	verbPlaceholders := strings.Repeat("?,", len(analysis.ActionVerbs)-1) + "?"
	args := []any{startTime, endTime}
	for verb := range analysis.ActionVerbs {
		args = append(args, verb)
	}

	query := fmt.Sprintf(`
        SELECT object_id, object_type, verb, fingerprint_id, created_at
        FROM actions
        WHERE created_at >= ? AND created_at < ? AND verb IN (%s)
    `, verbPlaceholders)

	rows, err := tenantCtx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query action events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var event analytics.ActionEvent
		var createdAtStr string
		if err := rows.Scan(&event.ObjectID, &event.ObjectType, &event.Verb, &event.FingerprintID, &createdAtStr); err != nil {
			log.Printf("WARN: Failed to scan action event row: %v", err)
			ws.logger.Cache().Warn("Failed to scan action event row", "error", err)
			continue
		}
		createdAt, err := ws.parseTimestamp(createdAtStr)
		if err != nil {
			log.Printf("WARN: Failed to parse created_at timestamp '%s': %v", createdAtStr, err)
			ws.logger.Cache().Warn("Failed to parse created_at timestamp", "timestamp", createdAtStr, "error", err)
			continue
		}
		event.CreatedAt = createdAt
		events = append(events, event)
	}
	return events, nil
}

func (ws *WarmingService) getBeliefEventsForRange(tenantCtx *tenant.Context, startTime, endTime time.Time, analysis *EpinetAnalysis) ([]analytics.BeliefEvent, error) {
	var events []analytics.BeliefEvent
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
	args := []any{startTime, endTime}
	for _, val := range allValues {
		args = append(args, val)
	}

	query := fmt.Sprintf(`
        SELECT belief_id, fingerprint_id, verb, object, updated_at
        FROM heldbeliefs
        WHERE updated_at >= ? AND updated_at < ? AND object IN (%s)
    `, valuePlaceholders)

	rows, err := tenantCtx.Database.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query belief events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var event analytics.BeliefEvent
		var updatedAtStr string
		if err := rows.Scan(&event.BeliefID, &event.FingerprintID, &event.Verb, &event.Object, &updatedAtStr); err != nil {
			log.Printf("WARN: Failed to scan belief event row: %v", err)
			ws.logger.Cache().Warn("Failed to scan belief event row", "error", err)
			continue
		}
		updatedAt, err := ws.parseTimestamp(updatedAtStr)
		if err != nil {
			log.Printf("WARN: Failed to parse updated_at timestamp '%s': %v", updatedAtStr, err)
			ws.logger.Cache().Warn("Failed to parse updated_at timestamp", "timestamp", updatedAtStr, "error", err)
			continue
		}
		event.UpdatedAt = updatedAt
		events = append(events, event)
	}
	return events, nil
}

func (ws *WarmingService) getEpinets(tenantCtx *tenant.Context) ([]types.EpinetConfig, error) {
	epinetRepo := tenantCtx.EpinetRepo()
	epinetNodes, err := epinetRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, err
	}

	var epinets []types.EpinetConfig
	for _, node := range epinetNodes {
		var steps []types.EpinetStep
		for _, nodeStep := range node.Steps {
			step := types.EpinetStep{
				GateType:  nodeStep.GateType,
				Title:     nodeStep.Title,
				Values:    nodeStep.Values,
				ObjectIds: nodeStep.ObjectIDs,
			}
			if nodeStep.ObjectType != nil {
				step.ObjectType = *nodeStep.ObjectType
			}
			steps = append(steps, step)
		}
		epinets = append(epinets, types.EpinetConfig{
			ID:    node.ID,
			Title: node.Title,
			Steps: steps,
		})
	}
	return epinets, nil
}

func (ws *WarmingService) getContentItems(tenantCtx *tenant.Context) (map[string]types.ContentItem, error) {
	contentItems := make(map[string]types.ContentItem)

	sfRepo := tenantCtx.StoryFragmentRepo()
	storyFragments, err := sfRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, err
	}
	for _, sf := range storyFragments {
		contentItems[sf.ID] = types.ContentItem{Title: sf.Title, Slug: sf.Slug}
	}

	paneRepo := tenantCtx.PaneRepo()
	panes, err := paneRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, err
	}
	for _, pane := range panes {
		contentItems[pane.ID] = types.ContentItem{Title: pane.Title, Slug: pane.Slug}
	}
	return contentItems, nil
}

func (ws *WarmingService) analyzeEpinet(epinet types.EpinetConfig) *EpinetAnalysis {
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

func (ws *WarmingService) buildStepsFromEvents(epinet types.EpinetConfig, actionEvents []analytics.ActionEvent, beliefEvents []analytics.BeliefEvent, contentItems map[string]types.ContentItem) map[string]*types.HourlyEpinetStepData {
	steps := make(map[string]*types.HourlyEpinetStepData)
	for _, event := range actionEvents {
		for stepIndex, step := range epinet.Steps {
			if ws.eventMatchesStep(event, step) {
				nodeID := ws.getStepNodeID(step, event.ObjectID, event.Verb)
				if steps[nodeID] == nil {
					steps[nodeID] = &types.HourlyEpinetStepData{
						Visitors:  make(map[string]bool),
						Name:      ws.getNodeName(step, event.ObjectID, contentItems, event.Verb),
						StepIndex: stepIndex + 1,
					}
				}
				steps[nodeID].Visitors[event.FingerprintID] = true
			}
		}
	}
	for _, event := range beliefEvents {
		for stepIndex, step := range epinet.Steps {
			if ws.beliefEventMatchesStep(event, step) {
				nodeID := ws.getStepNodeID(step, "", *event.Object)
				if steps[nodeID] == nil {
					steps[nodeID] = &types.HourlyEpinetStepData{
						Visitors:  make(map[string]bool),
						Name:      ws.getNodeName(step, "", contentItems, *event.Object),
						StepIndex: stepIndex + 1,
					}
				}
				steps[nodeID].Visitors[event.FingerprintID] = true
			}
		}
	}
	return steps
}

func (ws *WarmingService) buildTransitionsFromSteps(steps map[string]*types.HourlyEpinetStepData) map[string]map[string]*types.HourlyEpinetTransitionData {
	transitions := make(map[string]map[string]*types.HourlyEpinetTransitionData)
	visitorSteps := make(map[string][]string)
	for nodeID, stepData := range steps {
		for visitor := range stepData.Visitors {
			visitorSteps[visitor] = append(visitorSteps[visitor], nodeID)
		}
	}
	for visitor, nodeIDs := range visitorSteps {
		sort.Slice(nodeIDs, func(i, j int) bool {
			return steps[nodeIDs[i]].StepIndex < steps[nodeIDs[j]].StepIndex
		})
		for i := 0; i < len(nodeIDs)-1; i++ {
			fromNode, toNode := nodeIDs[i], nodeIDs[i+1]
			if transitions[fromNode] == nil {
				transitions[fromNode] = make(map[string]*types.HourlyEpinetTransitionData)
			}
			if transitions[fromNode][toNode] == nil {
				transitions[fromNode][toNode] = &types.HourlyEpinetTransitionData{
					Visitors: make(map[string]bool),
				}
			}
			transitions[fromNode][toNode].Visitors[visitor] = true
		}
	}
	return transitions
}

func (ws *WarmingService) eventMatchesStep(event analytics.ActionEvent, step types.EpinetStep) bool {
	if step.GateType != "commitmentAction" && step.GateType != "conversionAction" {
		return false
	}
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

func (ws *WarmingService) beliefEventMatchesStep(event analytics.BeliefEvent, step types.EpinetStep) bool {
	if step.GateType != "belief" && step.GateType != "identifyAs" {
		return false
	}
	for _, val := range step.Values {
		if event.Object != nil && val == *event.Object {
			return true
		}
	}
	return false
}

func (ws *WarmingService) getStepNodeID(step types.EpinetStep, contentID string, matchedValue string) string {
	parts := []string{step.GateType}
	switch step.GateType {
	case "belief", "identifyAs":
		parts = append(parts, matchedValue)
	case "commitmentAction", "conversionAction":
		parts = append(parts, step.ObjectType, matchedValue, contentID)
	}
	return strings.Join(parts, "_")
}

func (ws *WarmingService) getNodeName(step types.EpinetStep, contentID string, contentItems map[string]types.ContentItem, matchedValue string) string {
	var parts []string
	switch step.GateType {
	case "belief", "identifyAs":
		parts = append(parts, step.Title)
	case "commitmentAction", "conversionAction":
		parts = append(parts, step.Title, matchedValue)
		if item, exists := contentItems[contentID]; exists {
			parts = append(parts, item.Title)
		}
	}
	return strings.Join(parts, " - ")
}

func (ws *WarmingService) getTTLForHour(hourKey string) time.Duration {
	now := time.Now().UTC()
	currentHour := utilities.FormatHourKey(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC))
	if hourKey == currentHour {
		return 15 * time.Minute
	}
	return 24 * time.Hour
}

func (ws *WarmingService) getHourKeysForBatch(startHourOffset, endHourOffset int) []string {
	now := time.Now().UTC()
	var hourKeys []string
	for i := startHourOffset; i < endHourOffset; i++ {
		hourTime := now.Add(-time.Duration(i) * time.Hour)
		hourKey := utilities.FormatHourKey(hourTime)
		hourKeys = append(hourKeys, hourKey)
	}
	return hourKeys
}

// WarmHTMLFragmentWithBeliefEvaluation generates and caches HTML with belief evaluation
// This replicates the warmHTMLFragmentForPane logic but adds the missing belief evaluation
func (ws *WarmingService) WarmHTMLFragmentWithBeliefEvaluation(
	tenantCtx *tenant.Context,
	paneNode *content.PaneNode,
	storyFragmentID string,
	beliefRegistry *types.StoryfragmentBeliefRegistry,
) {
	start := time.Now()

	// Generate base HTML (same as legacy warmHTMLFragmentForPane)
	nodesData, parentChildMap, err := templates.ExtractNodesFromPane(paneNode)
	if err != nil {
		ws.logger.Cache().Warn("Failed to parse pane nodes during warming", "paneId", paneNode.ID, "error", err)
		return
	}

	paneNodeData := &rendering.NodeRenderData{
		ID:       paneNode.ID,
		NodeType: "Pane",
		PaneData: &rendering.PaneRenderData{
			Title:           paneNode.Title,
			Slug:            paneNode.Slug,
			IsDecorative:    paneNode.IsDecorative,
			BgColour:        paneNode.BgColour,
			HeldBeliefs:     ws.convertStringMapToInterface(paneNode.HeldBeliefs),
			WithheldBeliefs: ws.convertStringMapToInterface(paneNode.WithheldBeliefs),
			CodeHookTarget:  paneNode.CodeHookTarget,
			CodeHookPayload: ws.convertStringMapToInterfaceMap(paneNode.CodeHookPayload),
		},
	}
	nodesData[paneNode.ID] = paneNodeData

	renderCtx := &rendering.RenderContext{
		AllNodes:         nodesData,
		ParentNodes:      parentChildMap,
		TenantID:         tenantCtx.TenantID,
		SessionID:        "", // No session during warming
		StoryfragmentID:  storyFragmentID,
		ContainingPaneID: paneNode.ID,
		WidgetContext:    nil,
	}

	generator := templates.NewGenerator(renderCtx)
	htmlContent := generator.RenderPaneFragment(paneNode.ID)

	if beliefRegistry != nil {
		if paneBeliefs, exists := beliefRegistry.PaneBeliefPayloads[paneNode.ID]; exists {
			emptyUserBeliefs := make(map[string][]string) // Anonymous user = empty beliefs
			visibility := ws.beliefEvaluationService.EvaluatePaneVisibility(paneBeliefs, emptyUserBeliefs)
			htmlContent = ws.applyVisibilityWrapper(htmlContent, visibility)

			ws.logger.Cache().Debug("Applied belief evaluation during warming",
				"paneId", paneNode.ID,
				"visibility", visibility,
				"heldRequirements", len(paneBeliefs.HeldBeliefs),
				"withheldRequirements", len(paneBeliefs.WithheldBeliefs))
		}
	}

	// Cache the properly evaluated HTML
	variant := types.PaneVariant{
		BeliefMode:      "default",
		HeldBeliefs:     []string{},
		WithheldBeliefs: []string{},
	}

	dependencies := []string{paneNode.ID}
	tenantCtx.CacheManager.SetHTMLChunk(tenantCtx.TenantID, paneNode.ID, variant, htmlContent, dependencies)

	ws.logger.Cache().Debug("HTML fragment warmed with belief evaluation",
		"paneId", paneNode.ID,
		"duration", time.Since(start),
		"htmlLength", len(htmlContent))
}

// Helper methods for new warming method
func (ws *WarmingService) convertStringMapToInterface(input map[string][]string) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range input {
		result[k] = v
	}
	return result
}

func (ws *WarmingService) convertStringMapToInterfaceMap(input map[string]string) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range input {
		result[k] = v
	}
	return result
}

// applyVisibilityWrapper wraps content based on visibility state
func (ws *WarmingService) applyVisibilityWrapper(htmlContent, visibility string) string {
	switch visibility {
	case "visible":
		return htmlContent
	case "hidden":
		// Use legacy-compatible wrapper with !important specificity
		return fmt.Sprintf(`<div style="display:none !important;">%s</div>`, htmlContent)
	case "empty":
		// Support for future heldBadges feature
		return `<div style="display:none !important;"></div>`
	default:
		return htmlContent
	}
}
