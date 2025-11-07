// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/repositories"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/services/markdown"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/mitchellh/mapstructure"
)

// PaneService orchestrates pane operations with cache-first repository pattern
type PaneService struct {
	logger               *logging.ChanneledLogger
	perfTracker          *performance.Tracker
	contentMapService    *ContentMapService
	registryOrchestrator *RegistryRebuildOrchestrator
	markdownConverter    *markdown.Converter
	aaiService           *AAIService
	resourceService      *ResourceService
}

// PaneTemplatePayload represents the template format for a pane
type PaneTemplatePayload struct {
	PaneNode   *content.PaneNode `json:"paneNode"`
	ChildNodes []any             `json:"childNodes"`
}

// AITitleSlug represents the validated and sanitized struct from an AI response.
type AITitleSlug struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

// paneNeedingAI is a helper struct to track panes that need AI processing.
type paneNeedingAI struct {
	payload  map[string]any
	markdown string
	index    int
}

// a regex to validate the slug format strictly.
var slugRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// NewPaneService creates a new pane service singleton
func NewPaneService(
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
	contentMapService *ContentMapService,
	registryOrchestrator *RegistryRebuildOrchestrator,
	aaiService *AAIService,
	resourceService *ResourceService,
) *PaneService {
	return &PaneService{
		logger:               logger,
		perfTracker:          perfTracker,
		contentMapService:    contentMapService,
		registryOrchestrator: registryOrchestrator,
		markdownConverter:    markdown.NewConverter(),
		aaiService:           aaiService,
		resourceService:      resourceService,
	}
}

// StringToTimeHookFunc is a mapstructure hook that converts string to time.Time.
func StringToTimeHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data any,
	) (any, error) {
		if t != reflect.TypeOf(time.Time{}) {
			return data, nil
		}
		if f.Kind() != reflect.String {
			return data, nil
		}
		return time.Parse(time.RFC3339Nano, data.(string))
	}
}

func (s *PaneService) generateMarkdownFromNodes(paneID string, optionsPayload map[string]any) (string, error) {
	if optionsPayload == nil {
		return "", nil
	}
	nodesData, exists := optionsPayload["nodes"]
	if !exists {
		return "", nil
	}
	nodes, ok := nodesData.([]any)
	if !ok {
		s.logger.Content().Warn("Pane optionsPayload 'nodes' field is not an array, cannot generate markdown",
			"paneId", paneID, "type", fmt.Sprintf("%T", nodesData))
		return "", nil
	}
	if len(nodes) == 0 {
		return "", nil
	}

	nodeMap := make(map[string]map[string]any)
	childMap := make(map[string][]string)

	for _, nodeInterface := range nodes {
		node, ok := nodeInterface.(map[string]any)
		if !ok {
			continue
		}
		id, idOk := node["id"].(string)
		if !idOk {
			continue
		}
		nodeMap[id] = node
		if parentID, parentIDOk := node["parentId"].(string); parentIDOk {
			childMap[parentID] = append(childMap[parentID], id)
		}
	}

	gatherDescendants := func(startNodeID string) []any {
		var result []any
		var queue []string
		queue = append(queue, startNodeID)
		visited := make(map[string]bool)

		for len(queue) > 0 {
			currentID := queue[0]
			queue = queue[1:]

			if visited[currentID] {
				continue
			}
			visited[currentID] = true

			if startNode, ok := nodeMap[currentID]; ok {
				result = append(result, startNode)
			}

			if children, ok := childMap[currentID]; ok {
				queue = append(queue, children...)
			}
		}
		return result
	}

	for _, nodeInterface := range nodes {
		node, _ := nodeInterface.(map[string]any)
		id, _ := node["id"].(string)
		if strings.HasPrefix(id, "fts-markdown-") {
			nodesForConversion := gatherDescendants(id)
			return s.markdownConverter.ConvertNodesToMarkdown(nodesForConversion)
		}
	}

	var rootContentNodeIDs []string
	for _, nodeInterface := range nodes {
		node, _ := nodeInterface.(map[string]any)
		id, _ := node["id"].(string)
		parentID, _ := node["parentId"].(string)
		if parentID == paneID {
			nodeType, _ := node["nodeType"].(string)
			if nodeType == "Markdown" || nodeType == "GridLayoutNode" {
				rootContentNodeIDs = append(rootContentNodeIDs, id)
			}
		}
	}

	var markdownStrings []string
	for _, rootID := range rootContentNodeIDs {
		rootNode := nodeMap[rootID]
		nodeType, _ := rootNode["nodeType"].(string)

		if nodeType == "Markdown" {
			nodesForConversion := gatherDescendants(rootID)
			markdownBody, err := s.markdownConverter.ConvertNodesToMarkdown(nodesForConversion)
			if err != nil {
				s.logger.Content().Error("Failed to generate markdown from a direct MarkdownNode",
					"error", err, "paneId", paneID, "markdownNodeId", rootID)
				continue
			}
			markdownStrings = append(markdownStrings, markdownBody)
		} else if nodeType == "GridLayoutNode" {
			columnIDs := childMap[rootID]
			for _, columnID := range columnIDs {
				columnNode, exists := nodeMap[columnID]
				if !exists {
					continue
				}
				if colNodeType, _ := columnNode["nodeType"].(string); colNodeType == "Markdown" {
					nodesForConversion := gatherDescendants(columnID)
					markdownBody, err := s.markdownConverter.ConvertNodesToMarkdown(nodesForConversion)
					if err != nil {
						s.logger.Content().Error("Failed to generate markdown from a GridLayout column",
							"error", err, "paneId", paneID, "gridLayoutId", rootID, "columnId", columnID)
						continue
					}
					markdownStrings = append(markdownStrings, markdownBody)
				}
			}
		}
	}

	return strings.Join(markdownStrings, "\n\n"), nil
}

// isSystemGenerated checks if a title and slug match the system-generated pattern.
func isSystemGenerated(title, slug any) bool {
	titleStr, titleOk := title.(string)
	slugStr, slugOk := slug.(string)

	if !titleOk || !slugOk {
		return false
	}

	titleParts := strings.Split(titleStr, "-")
	slugParts := strings.Split(slugStr, "-")

	if len(titleParts) < 2 || len(slugParts) < 2 {
		return false
	}

	lastTitlePart := titleParts[len(titleParts)-1]
	lastSlugPart := slugParts[len(slugParts)-1]

	return len(lastTitlePart) == 4 && len(lastSlugPart) == 4
}

// convertPayloadToPaneNode safely converts a map to a PaneNode struct.
func convertPayloadToPaneNode(payload map[string]any) (*content.PaneNode, error) {
	var pane content.PaneNode
	config := &mapstructure.DecoderConfig{
		Result:           &pane,
		WeaklyTypedInput: true,
		DecodeHook:       StringToTimeHookFunc(),
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}
	if err := decoder.Decode(payload); err != nil {
		return nil, fmt.Errorf("failed to decode payload into PaneNode: %w", err)
	}
	return &pane, nil
}

// ensureUniqueSlug checks if a slug is unique and appends a suffix if it's not.
func (s *PaneService) ensureUniqueSlug(tenantCtx *tenant.Context, desiredSlug string, paneIDToExclude string) (string, error) {
	paneRepo := tenantCtx.PaneRepo()
	currentSlug := desiredSlug
	counter := 2

	for {
		existingPane, err := paneRepo.FindBySlug(tenantCtx.TenantID, currentSlug)
		if err != nil {
			return "", fmt.Errorf("failed to check for existing slug '%s': %w", currentSlug, err)
		}

		if existingPane == nil || existingPane.ID == paneIDToExclude {
			return currentSlug, nil
		}

		s.logger.Content().Info("Slug conflict found, generating new slug.", "originalSlug", desiredSlug, "conflictingId", existingPane.ID, "attempt", counter)
		currentSlug = fmt.Sprintf("%s-%d", desiredSlug, counter)
		counter++
	}
}

// BulkProcessPanes handles the efficient processing of multiple panes,
// including batch AI title/slug generation for system-generated panes.
func (s *PaneService) BulkProcessPanes(tenantCtx *tenant.Context, panePayloads []map[string]any, originalReq *http.Request) ([]string, error) {
	start := time.Now()
	var panesNeedingAI []paneNeedingAI

	for i, payload := range panePayloads {
		id, _ := payload["id"].(string)
		title, _ := payload["title"].(string)
		slug, _ := payload["slug"].(string)
		optionsPayload, _ := payload["optionsPayload"].(map[string]any)

		if isSystemGenerated(title, slug) {
			markdownBody, _ := s.generateMarkdownFromNodes(id, optionsPayload)
			if markdownBody != "" {
				panesNeedingAI = append(panesNeedingAI, paneNeedingAI{payload, markdownBody, i})
			}
		}
	}

	if len(panesNeedingAI) > 0 {
		s.logger.Content().Info("Found system-generated panes, attempting batch AI title generation.", "count", len(panesNeedingAI), "tenantId", tenantCtx.TenantID)
		aiResults, err := s.batchGetAITitles(tenantCtx, panesNeedingAI)
		if err != nil {
			s.logger.System().Warn("Batch AI title generation failed. Proceeding with system-generated names.", "error", err, "tenantId", tenantCtx.TenantID)
		} else {
			for i, result := range aiResults {
				originalIndex := panesNeedingAI[i].index
				panePayloads[originalIndex]["title"] = result.Title
				panePayloads[originalIndex]["slug"] = result.Slug
			}
			s.logger.Content().Info("Successfully updated panes with AI-generated titles and slugs.", "count", len(aiResults), "tenantId", tenantCtx.TenantID)
		}
	}

	var processedIDs []string
	paneRepo := tenantCtx.PaneRepo()
	for _, payload := range panePayloads {
		pane, err := convertPayloadToPaneNode(payload)
		if err != nil {
			s.logger.Content().Error("Failed to convert payload to PaneNode, failing batch.", "error", err, "payloadId", payload["id"], "tenantId", tenantCtx.TenantID)
			return nil, err
		}

		existing, err := paneRepo.FindByID(tenantCtx.TenantID, pane.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check existence for pane %s: %w", pane.ID, err)
		}

		if existing != nil {
			err = s.Update(tenantCtx, pane)
		} else {
			err = s.Create(tenantCtx, pane)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to process pane %s: %w", pane.ID, err)
		}
		processedIDs = append(processedIDs, pane.ID)
	}

	s.logger.Content().Info("Successfully processed bulk panes", "tenantId", tenantCtx.TenantID, "count", len(processedIDs), "duration", time.Since(start))
	s.logger.Perf().Info("Performance for BulkProcessPanes", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true, "paneCount", len(panePayloads))
	return processedIDs, nil
}

// batchGetAITitles gets titles and slugs from an LLM and validates the output.
func (s *PaneService) batchGetAITitles(tenantCtx *tenant.Context, panes []paneNeedingAI) ([]AITitleSlug, error) {
	type lemurInput struct {
		Index    int    `json:"index"`
		Markdown string `json:"markdown"`
	}
	batchInput := make([]lemurInput, len(panes))
	for i, pane := range panes {
		batchInput[i] = lemurInput{Index: i, Markdown: pane.markdown}
	}

	batchJSON, err := json.Marshal(batchInput)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch input for LeMUR: %w", err)
	}

	prompt := `You are an expert technical content editor responsible for creating web-friendly metadata. I will provide you with a JSON array of markdown content snippets. For each snippet, you must generate a concise, descriptive title and a URL-safe slug.
STRICT RULES:
1. Title must be max 50 characters.
2. Slug must be lowercase kebab-case (e.g., 'this-is-a-good-slug'), containing only a-z, 0-9, and hyphens. It should be concise, ideally under 40 characters, and MUST NOT exceed 50 characters.
3. Your response MUST be ONLY a single, valid JSON array of objects, each with "title" and "slug" keys.
4. The output array order MUST exactly match the input array order.
5. Do not include any other text, explanations, or markdown formatting in your response.
`

	serviceRequest := AskLemurRequest{
		Prompt:    prompt,
		InputText: string(batchJSON),
	}

	lemurResponse, err := s.aaiService.AskLemur(tenantCtx, serviceRequest)
	if err != nil {
		return nil, fmt.Errorf("AAIService AskLemur call failed: %w", err)
	}

	lemurResponseStr, ok := lemurResponse.(string)
	if !ok {
		if marshaled, err := json.Marshal(lemurResponse); err == nil {
			lemurResponseStr = string(marshaled)
		} else {
			return nil, fmt.Errorf("askLemur response was not a string or marshallable object, was type %T", lemurResponse)
		}
	}

	var results []AITitleSlug
	if err := json.Unmarshal([]byte(lemurResponseStr), &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal askLemur JSON response: %w. Response: %s", err, lemurResponseStr)
	}

	if len(results) != len(panes) {
		return nil, fmt.Errorf("askLemur returned %d results, but expected %d", len(results), len(panes))
	}

	for i := range results {
		if len(results[i].Title) == 0 {
			return nil, fmt.Errorf("item %d has an empty title", i)
		}
		if len(results[i].Title) > 60 {
			results[i].Title = results[i].Title[:60]
		}
		if len(results[i].Slug) == 0 {
			return nil, fmt.Errorf("item %d has an empty slug", i)
		}
		sanitizedSlug := strings.ToLower(results[i].Slug)
		sanitizedSlug = strings.ReplaceAll(sanitizedSlug, " ", "-")
		sanitizedSlug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(sanitizedSlug, "")
		sanitizedSlug = regexp.MustCompile(`--+`).ReplaceAllString(sanitizedSlug, "-")
		sanitizedSlug = strings.Trim(sanitizedSlug, "-")

		if len(sanitizedSlug) > 50 {
			sanitizedSlug = sanitizedSlug[:50]
			sanitizedSlug = strings.Trim(sanitizedSlug, "-")
		}

		if !slugRegex.MatchString(sanitizedSlug) || len(sanitizedSlug) == 0 {
			return nil, fmt.Errorf("slug for item %d ('%s') is invalid after sanitization", i, results[i].Slug)
		}
		results[i].Slug = sanitizedSlug
	}

	return results, nil
}

// GetAllIDs returns all pane IDs for a tenant by leveraging the robust repository.
func (s *PaneService) GetAllIDs(tenantCtx *tenant.Context) ([]string, error) {
	start := time.Now()
	paneRepo := tenantCtx.PaneRepo()

	panes, err := paneRepo.FindAll(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all panes from repository: %w", err)
	}

	ids := make([]string, len(panes))
	for i, pane := range panes {
		ids[i] = pane.ID
	}

	s.logger.Content().Info("Successfully retrieved all pane IDs", "tenantId", tenantCtx.TenantID, "count", len(ids), "duration", time.Since(start))
	s.logger.Perf().Info("Performance for GetAllPaneIDs", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true)

	return ids, nil
}

// GetByID returns a pane by ID (cache-first via repository)
func (s *PaneService) GetByID(tenantCtx *tenant.Context, id string) (*content.PaneNode, error) {
	start := time.Now()
	if id == "" {
		return nil, fmt.Errorf("pane ID cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane %s: %w", id, err)
	}

	s.logger.Content().Info("Successfully retrieved pane by ID", "tenantId", tenantCtx.TenantID, "paneId", id, "found", pane != nil, "duration", time.Since(start))
	s.logger.Perf().Info("Performance for GetPaneByID", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true, "paneId", id)

	return pane, nil
}

// GetByIDs returns multiple panes by IDs (cache-first with bulk loading via repository)
func (s *PaneService) GetByIDs(tenantCtx *tenant.Context, ids []string) ([]*content.PaneNode, error) {
	start := time.Now()
	if len(ids) == 0 {
		return []*content.PaneNode{}, nil
	}

	paneRepo := tenantCtx.PaneRepo()
	panes, err := paneRepo.FindByIDs(tenantCtx.TenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get panes by IDs from repository: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved panes by IDs", "tenantId", tenantCtx.TenantID, "requestedCount", len(ids), "foundCount", len(panes), "duration", time.Since(start))
	s.logger.Perf().Info("Performance for GetPanesByIDs", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true, "requestedCount", len(ids))

	return panes, nil
}

// GetBySlug returns a pane by slug (cache-first via repository)
func (s *PaneService) GetBySlug(tenantCtx *tenant.Context, slug string) (*content.PaneNode, error) {
	start := time.Now()
	if slug == "" {
		return nil, fmt.Errorf("pane slug cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()
	pane, err := paneRepo.FindBySlug(tenantCtx.TenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane by slug %s: %w", slug, err)
	}

	s.logger.Content().Info("Successfully retrieved pane by slug", "tenantId", tenantCtx.TenantID, "slug", slug, "found", pane != nil, "duration", time.Since(start))
	s.logger.Perf().Info("Performance for GetPaneBySlug", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true, "slug", slug)

	return pane, nil
}

// GetContextPanes returns all context panes (cache-first with filtering via repository)
func (s *PaneService) GetContextPanes(tenantCtx *tenant.Context) ([]*content.PaneNode, error) {
	start := time.Now()
	paneRepo := tenantCtx.PaneRepo()
	contextPanes, err := paneRepo.FindContext(tenantCtx.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get context panes: %w", err)
	}

	s.logger.Content().Info("Successfully retrieved context panes", "tenantId", tenantCtx.TenantID, "count", len(contextPanes), "duration", time.Since(start))
	s.logger.Perf().Info("Performance for GetContextPanes", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true)

	return contextPanes, nil
}

// Create creates a new pane
// Create creates a new pane
func (s *PaneService) Create(tenantCtx *tenant.Context, pane *content.PaneNode) error {
	start := time.Now()
	if pane.ID == "" {
		pane.ID = security.GenerateULID()
	}
	if pane == nil {
		return fmt.Errorf("pane cannot be nil")
	}
	if pane.Title == "" {
		return fmt.Errorf("pane title cannot be empty")
	}
	if pane.Slug == "" {
		return fmt.Errorf("pane slug cannot be empty")
	}

	uniqueSlug, err := s.ensureUniqueSlug(tenantCtx, pane.Slug, "")
	if err != nil {
		return err
	}
	pane.Slug = uniqueSlug

	markdownBody, err := s.generateMarkdownFromNodes(pane.ID, pane.OptionsPayload)
	if err != nil {
		return fmt.Errorf("markdown generation failed for new pane %s: %w", pane.ID, err)
	}

	paneRepo := tenantCtx.PaneRepo()
	err = paneRepo.Store(tenantCtx.TenantID, pane, markdownBody)
	if err != nil {
		return fmt.Errorf("failed to create pane %s: %w", pane.ID, err)
	}

	tenantCtx.CacheManager.SetPane(tenantCtx.TenantID, pane)
	tenantCtx.CacheManager.AddPaneID(tenantCtx.TenantID, pane.ID)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after pane creation",
			"error", err, "paneId", pane.ID, "tenantId", tenantCtx.TenantID)
	}

	s.triggerRebuildForPaneParents(tenantCtx, pane.ID)

	s.logger.Content().Info("Successfully created pane", "tenantId", tenantCtx.TenantID, "paneId", pane.ID, "title", pane.Title, "slug", pane.Slug, "duration", time.Since(start))
	s.logger.Perf().Info("Performance for CreatePane", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true, "paneId", pane.ID)

	return nil
}

// Update updates an existing pane
func (s *PaneService) Update(tenantCtx *tenant.Context, pane *content.PaneNode) error {
	start := time.Now()
	if pane == nil {
		return fmt.Errorf("pane cannot be nil")
	}
	if pane.ID == "" {
		return fmt.Errorf("pane ID cannot be empty")
	}
	if pane.Title == "" {
		return fmt.Errorf("pane title cannot be empty")
	}
	if pane.Slug == "" {
		return fmt.Errorf("pane slug cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()

	existing, err := paneRepo.FindByID(tenantCtx.TenantID, pane.ID)
	if err != nil {
		return fmt.Errorf("failed to verify pane %s exists: %w", pane.ID, err)
	}
	if existing == nil {
		return fmt.Errorf("pane %s not found", pane.ID)
	}

	uniqueSlug, err := s.ensureUniqueSlug(tenantCtx, pane.Slug, pane.ID)
	if err != nil {
		return err
	}
	pane.Slug = uniqueSlug

	markdownBody, err := s.generateMarkdownFromNodes(pane.ID, pane.OptionsPayload)
	if err != nil {
		return fmt.Errorf("markdown generation failed for pane update %s: %w", pane.ID, err)
	}

	err = paneRepo.Update(tenantCtx.TenantID, pane, markdownBody)
	if err != nil {
		return fmt.Errorf("failed to update pane %s: %w", pane.ID, err)
	}

	tenantCtx.CacheManager.SetPane(tenantCtx.TenantID, pane)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after pane update",
			"error", err, "paneId", pane.ID, "tenantId", tenantCtx.TenantID)
	}
	tenantCtx.CacheManager.InvalidateByDependency(tenantCtx.TenantID, pane.ID)

	s.triggerRebuildForPaneParents(tenantCtx, pane.ID)

	s.logger.Content().Info("Successfully updated pane", "tenantId", tenantCtx.TenantID, "paneId", pane.ID, "title", pane.Title, "slug", pane.Slug, "duration", time.Since(start))
	s.logger.Perf().Info("Performance for UpdatePane", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true, "paneId", pane.ID)

	return nil
}

// Delete deletes a pane
func (s *PaneService) Delete(tenantCtx *tenant.Context, id string) error {
	start := time.Now()
	if id == "" {
		return fmt.Errorf("pane ID cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()

	s.triggerRebuildForPaneParents(tenantCtx, id)

	existing, err := paneRepo.FindByID(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to verify pane %s exists: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("pane %s not found", id)
	}

	err = paneRepo.Delete(tenantCtx.TenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete pane %s: %w", id, err)
	}

	tenantCtx.CacheManager.InvalidatePane(tenantCtx.TenantID, id)
	tenantCtx.CacheManager.RemovePaneID(tenantCtx.TenantID, id)
	if err := s.contentMapService.RefreshContentMap(tenantCtx, tenantCtx.GetCacheManager()); err != nil {
		s.logger.Content().Error("Failed to refresh content map after pane deletion",
			"error", err, "paneId", id, "tenantId", tenantCtx.TenantID)
	}

	s.logger.Content().Info("Successfully deleted pane", "tenantId", tenantCtx.TenantID, "paneId", id, "duration", time.Since(start))
	s.logger.Perf().Info("Performance for DeletePane", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true, "paneId", id)

	return nil
}

// GetPaneTemplate returns a pane template in the same format as full-payload
func (s *PaneService) GetPaneTemplate(tenantCtx *tenant.Context, paneID string) (*PaneTemplatePayload, error) {
	start := time.Now()
	if paneID == "" {
		return nil, fmt.Errorf("pane ID cannot be empty")
	}

	pane, err := s.GetByID(tenantCtx, paneID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pane %s: %w", paneID, err)
	}
	if pane == nil {
		return nil, fmt.Errorf("pane not found with ID %s", paneID)
	}

	var childNodes []any
	if pane.OptionsPayload != nil {
		if nodes, exists := pane.OptionsPayload["nodes"]; exists {
			if nodesArray, ok := nodes.([]any); ok {
				childNodes = nodesArray
			}
		}
	}

	cleanedPane := *pane
	cleanedPane.OptionsPayload = make(map[string]any)
	if pane.OptionsPayload != nil {
		for k, v := range pane.OptionsPayload {
			if k != "nodes" {
				cleanedPane.OptionsPayload[k] = v
			}
		}
	}

	payload := &PaneTemplatePayload{
		PaneNode:   &cleanedPane,
		ChildNodes: childNodes,
	}

	s.logger.Content().Info("Successfully generated pane template", "tenantId", tenantCtx.TenantID, "paneId", paneID, "childNodeCount", len(childNodes), "duration", time.Since(start))
	s.logger.Perf().Info("Performance for GetPaneTemplate", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true, "paneId", paneID)

	return payload, nil
}

// BulkUpdateFilePaneRelationships updates file-pane relationships for multiple panes
func (s *PaneService) BulkUpdateFilePaneRelationships(tenantCtx *tenant.Context, relationships map[string][]string) error {
	start := time.Now()
	if len(relationships) == 0 {
		return fmt.Errorf("relationships map cannot be empty")
	}

	paneRepo := tenantCtx.PaneRepo()
	for paneID := range relationships {
		if paneID == "" {
			return fmt.Errorf("pane ID cannot be empty")
		}
		existing, err := paneRepo.FindByID(tenantCtx.TenantID, paneID)
		if err != nil {
			return fmt.Errorf("failed to verify pane %s exists: %w", paneID, err)
		}
		if existing == nil {
			return fmt.Errorf("pane %s not found", paneID)
		}
	}

	if err := paneRepo.UpdateFilePaneRelationships(tenantCtx.TenantID, relationships); err != nil {
		return fmt.Errorf("failed to bulk update file-pane relationships: %w", err)
	}

	for paneID := range relationships {
		tenantCtx.CacheManager.InvalidatePane(tenantCtx.TenantID, paneID)
	}

	s.logger.Content().Info("Successfully bulk updated file-pane relationships", "tenantId", tenantCtx.TenantID, "paneCount", len(relationships), "duration", time.Since(start))
	s.logger.Perf().Info("Performance for BulkUpdateFilePaneRelationships", "duration", time.Since(start), "tenantId", tenantCtx.TenantID, "success", true, "paneCount", len(relationships))

	return nil
}

// triggerRebuildForPaneParents finds all storyfragments containing a pane and enqueues them for a registry rebuild.
func (s *PaneService) triggerRebuildForPaneParents(tenantCtx *tenant.Context, paneID string) {
	storyFragmentRepo := tenantCtx.StoryFragmentRepo()
	parentSFIDs, err := storyFragmentRepo.FindIDsByPaneID(paneID)
	if err != nil {
		s.logger.Content().Error("Failed to find parent storyfragments for pane", "error", err, "paneId", paneID)
		return
	}

	if len(parentSFIDs) > 0 {
		s.logger.Cache().Info("Found parent storyfragments for pane update, enqueuing registry rebuilds", "paneId", paneID, "storyFragmentCount", len(parentSFIDs), "storyFragmentIds", parentSFIDs)
		for _, sfID := range parentSFIDs {
			s.registryOrchestrator.EnqueueRebuild(tenantCtx.TenantID, sfID)
		}
	}
}

// SearchContent calls the repository to perform a prefix search on pane markdown content.
func (s *PaneService) SearchContent(tenantCtx *tenant.Context, term string) ([]repositories.FTSResult, error) {
	repo := tenantCtx.PaneRepo()
	return repo.SearchContent(tenantCtx.TenantID, term)
}

// FindPaneContextStatus calls the repository to check the context status for a list of pane IDs.
func (s *PaneService) FindPaneContextStatus(tenantCtx *tenant.Context, paneIDs []string) (map[string]bool, error) {
	repo := tenantCtx.PaneRepo()
	return repo.FindPaneContextStatus(tenantCtx.TenantID, paneIDs)
}
