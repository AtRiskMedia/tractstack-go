// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"golang.org/x/sync/singleflight"
)

const (
	// BunnyVideoCategory is the knownResources category holding one resource per Bunny video.
	// It is a separate namespace from the "bunny-video" codehook id.
	BunnyVideoCategory = "bunny-video"

	bunnyCoreAPIBase   = "https://api.bunny.net"
	bunnyStreamAPIBase = "https://video.bunnycdn.com"

	bunnyLibrariesPerPage = 100
	bunnyVideosPerPage    = 25
)

// Bunny pipeline and encode states stored on bunny-video resources.
const (
	BunnyPipelineStatusIdle = "idle"

	BunnyEncodeStatusUploading  = "uploading"
	BunnyEncodeStatusProcessing = "processing"
	BunnyEncodeStatusFinished   = "finished"
	BunnyEncodeStatusError      = "error"
)

// BunnyVideoFixedFields are set once at creation and never change afterwards.
var BunnyVideoFixedFields = []string{"guid", "libraryId"}

// BunnyVideoSystemFields are the optionsPayload fields of a bunny-video resource that
// Go owns. Editor writes can never change them; background work changes them through
// ResourceService.MergeSystemFields.
var BunnyVideoSystemFields = []string{
	"bunnyData",
	"lengthSeconds",
	"thumbnailUrl",
	"encodeStatus",
	"mediaUrl",
	"pipelineStatus",
	"pipelineError",
	"transcriptId",
	"pageStoryFragmentId",
}

var (
	bunnyGUIDPattern      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	bunnyLibraryIDPattern = regexp.MustCompile(`^[0-9]+$`)

	bunnySlugStripPattern  = regexp.MustCompile(`[^a-z0-9\s-]`)
	bunnySlugSpacePattern  = regexp.MustCompile(`\s+`)
	bunnySlugHyphenPattern = regexp.MustCompile(`-+`)
)

// BunnyLibrarySummary is the only library data ever returned to the browser.
type BunnyLibrarySummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BunnyLibrary is the Bunny Core API VideoLibraryModel (PascalCase). It carries the
// library keys and must never be serialized into an API response.
type BunnyLibrary struct {
	ID                               int64    `json:"Id"`
	Name                             string   `json:"Name"`
	APIKey                           string   `json:"ApiKey"`
	ReadOnlyAPIKey                   string   `json:"ReadOnlyApiKey"`
	PullZoneID                       int64    `json:"PullZoneId"`
	EnableMP4Fallback                bool     `json:"EnableMP4Fallback"`
	KeepOriginalFiles                bool     `json:"KeepOriginalFiles"`
	ExposeOriginals                  bool     `json:"ExposeOriginals"`
	EnableDRM                        bool     `json:"EnableDRM"`
	PlayerTokenAuthenticationEnabled bool     `json:"PlayerTokenAuthenticationEnabled"`
	WebhookURL                       string   `json:"WebhookUrl"`
	AllowedReferrers                 []string `json:"AllowedReferrers"`
	BlockedReferrers                 []string `json:"BlockedReferrers"`
	BlockNoneReferrer                bool     `json:"BlockNoneReferrer"`
}

// BunnyPullZone is the part of the Bunny Core API PullZoneModel this integration reads.
type BunnyPullZone struct {
	ID        int64 `json:"Id"`
	Hostnames []struct {
		Value            string `json:"Value"`
		IsSystemHostname bool   `json:"IsSystemHostname"`
	} `json:"Hostnames"`
	ZoneSecurityEnabled bool     `json:"ZoneSecurityEnabled"`
	AllowedReferrers    []string `json:"AllowedReferrers"`
	BlockedReferrers    []string `json:"BlockedReferrers"`
	BlockNoneReferrer   bool     `json:"BlockNoneReferrer"`
}

// BunnyResolvedLibrary holds the server-side values resolved for a selected library.
type BunnyResolvedLibrary struct {
	LibraryID      string
	LibraryName    string
	LibraryAPIKey  string
	ReadOnlyAPIKey string
	CDNHostname    string
}

// BunnyLibraryStatus reports the library-readiness flags. It contains no keys.
type BunnyLibraryStatus struct {
	LibraryID                        string `json:"libraryId"`
	LibraryName                      string `json:"libraryName"`
	EnableMP4Fallback                bool   `json:"enableMP4Fallback"`
	KeepOriginalFiles                bool   `json:"keepOriginalFiles"`
	ExposeOriginals                  bool   `json:"exposeOriginals"`
	EnableDRM                        bool   `json:"enableDRM"`
	PlayerTokenAuthenticationEnabled bool   `json:"playerTokenAuthenticationEnabled"`
	WebhookURL                       string `json:"webhookUrl"`
	ZoneSecurityEnabled              bool   `json:"zoneSecurityEnabled"`
	BlockNoneReferrer                bool   `json:"blockNoneReferrer"`
	HasReferrerRules                 bool   `json:"hasReferrerRules"`
}

// BunnySkippedSetting names a recommended setting that was not applied, and why.
type BunnySkippedSetting struct {
	Setting string `json:"setting"`
	Reason  string `json:"reason"`
}

// BunnyApplyResult is the outcome of ApplyRecommendedSettings.
type BunnyApplyResult struct {
	Applied []string              `json:"applied"`
	Skipped []BunnySkippedSetting `json:"skipped"`
	Status  *BunnyLibraryStatus   `json:"status"`
}

// BunnyVideo is the Bunny Stream API VideoModel (camelCase). Raw keeps the full
// vendor document, which is stored stringified as bunnyData.
type BunnyVideo struct {
	GUID           string `json:"guid"`
	VideoLibraryID int64  `json:"videoLibraryId"`
	Title          string `json:"title"`
	Length         int    `json:"length"`
	Status         int    `json:"status"`
	ThumbnailURL   string `json:"thumbnailUrl"`

	Raw json.RawMessage `json:"-"`
}

// BunnyService talks to the Bunny Core and Stream APIs and manages bunny-video resources.
type BunnyService struct {
	logger          *logging.ChanneledLogger
	tenantManager   *tenant.Manager
	resourceService *ResourceService
	requestGroup    singleflight.Group
}

// NewBunnyService creates a new Bunny service instance.
func NewBunnyService(logger *logging.ChanneledLogger, tenantManager *tenant.Manager, resourceService *ResourceService) *BunnyService {
	return &BunnyService{
		logger:          logger,
		tenantManager:   tenantManager,
		resourceService: resourceService,
	}
}

// doRequest performs one Bunny API call and returns the response body.
// The key is sent as the AccessKey header for both the Core and the Stream API.
func (s *BunnyService) doRequest(method, requestURL, accessKey string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to encode bunny request: %w", err)
		}
		reader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, requestURL, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create bunny request: %w", err)
	}
	req.Header.Set("AccessKey", accessKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: config.BunnyRequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bunny request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.logger.System().Warn("Failed to close Bunny response body", "error", closeErr)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read bunny response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("bunny rejected the API key (%s)", resp.Status)
		case http.StatusNotFound:
			return nil, fmt.Errorf("bunny could not find it (%s)", resp.Status)
		}
		detail := strings.TrimSpace(string(respBody))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return nil, fmt.Errorf("bunny api error: %s %s", resp.Status, detail)
	}

	return respBody, nil
}

// listLibraries pages through every video library the account key can see.
func (s *BunnyService) listLibraries(accountKey string) ([]BunnyLibrary, error) {
	if accountKey == "" {
		return nil, fmt.Errorf("bunny account API key is not configured")
	}

	libraries := make([]BunnyLibrary, 0)
	for page := 1; ; page++ {
		requestURL := fmt.Sprintf("%s/videolibrary?page=%d&perPage=%d", bunnyCoreAPIBase, page, bunnyLibrariesPerPage)
		body, err := s.doRequest(http.MethodGet, requestURL, accountKey, nil)
		if err != nil {
			return nil, err
		}

		var result struct {
			Items        []BunnyLibrary `json:"Items"`
			HasMoreItems bool           `json:"HasMoreItems"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to decode bunny libraries: %w", err)
		}

		libraries = append(libraries, result.Items...)
		if !result.HasMoreItems || len(result.Items) == 0 {
			break
		}
	}
	return libraries, nil
}

// ListLibraries returns the id and name of every library in the tenant's Bunny account.
// It works before a library is selected, and never returns the keys Bunny sends.
func (s *BunnyService) ListLibraries(tenantCtx *tenant.Context) ([]BunnyLibrarySummary, error) {
	libraries, err := s.listLibraries(tenantCtx.Config.BunnyAPIKey)
	if err != nil {
		return nil, err
	}

	summaries := make([]BunnyLibrarySummary, 0, len(libraries))
	for _, library := range libraries {
		summaries = append(summaries, BunnyLibrarySummary{
			ID:   strconv.FormatInt(library.ID, 10),
			Name: library.Name,
		})
	}
	return summaries, nil
}

// GetLibrary fetches one video library with the account key.
func (s *BunnyService) GetLibrary(accountKey, libraryID string) (*BunnyLibrary, error) {
	if accountKey == "" {
		return nil, fmt.Errorf("bunny account API key is not configured")
	}
	if !bunnyLibraryIDPattern.MatchString(libraryID) {
		return nil, fmt.Errorf("invalid bunny library id")
	}

	requestURL := fmt.Sprintf("%s/videolibrary/%s", bunnyCoreAPIBase, libraryID)
	body, err := s.doRequest(http.MethodGet, requestURL, accountKey, nil)
	if err != nil {
		return nil, err
	}

	var library BunnyLibrary
	if err := json.Unmarshal(body, &library); err != nil {
		return nil, fmt.Errorf("failed to decode bunny library: %w", err)
	}
	if library.ID == 0 {
		return nil, fmt.Errorf("bunny returned no library for id %s", libraryID)
	}
	return &library, nil
}

// getPullZone fetches a pull zone with the account key.
func (s *BunnyService) getPullZone(accountKey string, pullZoneID int64) (*BunnyPullZone, error) {
	if accountKey == "" {
		return nil, fmt.Errorf("bunny account API key is not configured")
	}
	if pullZoneID <= 0 {
		return nil, fmt.Errorf("bunny library has no pull zone")
	}

	requestURL := fmt.Sprintf("%s/pullzone/%d", bunnyCoreAPIBase, pullZoneID)
	body, err := s.doRequest(http.MethodGet, requestURL, accountKey, nil)
	if err != nil {
		return nil, err
	}

	var zone BunnyPullZone
	if err := json.Unmarshal(body, &zone); err != nil {
		return nil, fmt.Errorf("failed to decode bunny pull zone: %w", err)
	}
	return &zone, nil
}

// pullZoneHost returns the pull zone's system (*.b-cdn.net) hostname: the CDN host.
func pullZoneHost(zone *BunnyPullZone) (string, error) {
	for _, hostname := range zone.Hostnames {
		if hostname.IsSystemHostname && hostname.Value != "" {
			return hostname.Value, nil
		}
	}
	return "", fmt.Errorf("bunny pull zone %d has no system hostname", zone.ID)
}

// GetPullZoneHost returns the CDN hostname of a pull zone.
func (s *BunnyService) GetPullZoneHost(accountKey string, pullZoneID int64) (string, error) {
	zone, err := s.getPullZone(accountKey, pullZoneID)
	if err != nil {
		return "", err
	}
	return pullZoneHost(zone)
}

// ResolveLibrary fetches the library and its pull zone and returns the values to store.
// It changes nothing, so a failure leaves the tenant config untouched.
func (s *BunnyService) ResolveLibrary(accountKey, libraryID string) (*BunnyResolvedLibrary, error) {
	library, err := s.GetLibrary(accountKey, libraryID)
	if err != nil {
		return nil, err
	}
	if library.APIKey == "" {
		return nil, fmt.Errorf("bunny returned no API key for library %s", libraryID)
	}

	host, err := s.GetPullZoneHost(accountKey, library.PullZoneID)
	if err != nil {
		return nil, err
	}

	return &BunnyResolvedLibrary{
		LibraryID:      strconv.FormatInt(library.ID, 10),
		LibraryName:    library.Name,
		LibraryAPIKey:  library.APIKey,
		ReadOnlyAPIKey: library.ReadOnlyAPIKey,
		CDNHostname:    host,
	}, nil
}

// StoreResolvedLibrary applies a resolved library to the tenant's in-memory config.
// The caller persists it with ConfigService.SaveAdvancedConfig.
func (s *BunnyService) StoreResolvedLibrary(tenantCtx *tenant.Context, resolved *BunnyResolvedLibrary) {
	tenantCtx.Config.BunnyLibraryID = resolved.LibraryID
	tenantCtx.Config.BunnyLibraryName = resolved.LibraryName
	tenantCtx.Config.BunnyLibraryAPIKey = resolved.LibraryAPIKey
	tenantCtx.Config.BunnyReadOnlyAPIKey = resolved.ReadOnlyAPIKey
	tenantCtx.Config.BunnyCDNHostname = resolved.CDNHostname
}

// ResolveAndStoreLibrary resolves a library and, only on success, applies it to the
// tenant's in-memory config.
func (s *BunnyService) ResolveAndStoreLibrary(tenantCtx *tenant.Context, accountKey, libraryID string) error {
	resolved, err := s.ResolveLibrary(accountKey, libraryID)
	if err != nil {
		return err
	}
	s.StoreResolvedLibrary(tenantCtx, resolved)
	return nil
}

// LibraryStatus reports the readiness flags of the tenant's selected library.
func (s *BunnyService) LibraryStatus(tenantCtx *tenant.Context) (*BunnyLibraryStatus, error) {
	accountKey := tenantCtx.Config.BunnyAPIKey
	libraryID := tenantCtx.Config.BunnyLibraryID
	if libraryID == "" {
		return nil, fmt.Errorf("no bunny library is selected for tenant %s", tenantCtx.TenantID)
	}

	library, err := s.GetLibrary(accountKey, libraryID)
	if err != nil {
		return nil, err
	}
	zone, err := s.getPullZone(accountKey, library.PullZoneID)
	if err != nil {
		return nil, err
	}
	return buildBunnyLibraryStatus(library, zone), nil
}

func buildBunnyLibraryStatus(library *BunnyLibrary, zone *BunnyPullZone) *BunnyLibraryStatus {
	return &BunnyLibraryStatus{
		LibraryID:                        strconv.FormatInt(library.ID, 10),
		LibraryName:                      library.Name,
		EnableMP4Fallback:                library.EnableMP4Fallback,
		KeepOriginalFiles:                library.KeepOriginalFiles,
		ExposeOriginals:                  library.ExposeOriginals,
		EnableDRM:                        library.EnableDRM,
		PlayerTokenAuthenticationEnabled: library.PlayerTokenAuthenticationEnabled,
		WebhookURL:                       library.WebhookURL,
		ZoneSecurityEnabled:              zone.ZoneSecurityEnabled,
		BlockNoneReferrer:                zone.BlockNoneReferrer,
		HasReferrerRules:                 len(zone.AllowedReferrers) > 0 || len(zone.BlockedReferrers) > 0,
	}
}

// ApplyRecommendedSettings turns on MP4 fallback, Keep original files and Serve
// original files (ExposeOriginals) on the tenant's library. Bunny produces those files only for videos
// uploaded afterwards. DRM, token authentication and referrer rules are reported by
// LibraryStatus and never changed here.
//
// goBackend is StoryKeep's PUBLIC_GO_BACKEND. It is accepted and not used yet.
func (s *BunnyService) ApplyRecommendedSettings(tenantCtx *tenant.Context, goBackend string) (*BunnyApplyResult, error) {
	_ = goBackend

	accountKey := tenantCtx.Config.BunnyAPIKey
	libraryID := tenantCtx.Config.BunnyLibraryID
	if libraryID == "" {
		return nil, fmt.Errorf("no bunny library is selected for tenant %s", tenantCtx.TenantID)
	}

	library, err := s.GetLibrary(accountKey, libraryID)
	if err != nil {
		return nil, err
	}

	update := map[string]any{
		"EnableMP4Fallback": true,
		"KeepOriginalFiles": true,
	}
	result := &BunnyApplyResult{
		Applied: []string{"EnableMP4Fallback", "KeepOriginalFiles"},
		Skipped: []BunnySkippedSetting{},
	}
	if library.EnableDRM {
		// The Bunny docs say ExposeOriginals "cannot be true while EnableDRM is true".
		result.Skipped = append(result.Skipped, BunnySkippedSetting{
			Setting: "ExposeOriginals",
			Reason:  "Serve original files cannot be turned on while DRM is enabled on the library.",
		})
	} else {
		update["ExposeOriginals"] = true
		result.Applied = append(result.Applied, "ExposeOriginals")
	}

	requestURL := fmt.Sprintf("%s/videolibrary/%s", bunnyCoreAPIBase, libraryID)
	if _, err := s.doRequest(http.MethodPost, requestURL, accountKey, update); err != nil {
		return nil, err
	}

	status, err := s.LibraryStatus(tenantCtx)
	if err != nil {
		return nil, err
	}
	result.Status = status

	s.logger.System().Info("Applied recommended Bunny library settings",
		"tenantId", tenantCtx.TenantID, "libraryId", libraryID, "applied", result.Applied, "skipped", len(result.Skipped))
	return result, nil
}

// libraryCredentials returns the selected library id and its API key.
func libraryCredentials(tenantCtx *tenant.Context) (string, string, error) {
	libraryID := tenantCtx.Config.BunnyLibraryID
	libraryKey := tenantCtx.Config.BunnyLibraryAPIKey
	if libraryID == "" || libraryKey == "" {
		return "", "", fmt.Errorf("bunny credentials missing for tenant %s", tenantCtx.TenantID)
	}
	if !bunnyLibraryIDPattern.MatchString(libraryID) {
		return "", "", fmt.Errorf("invalid bunny library id for tenant %s", tenantCtx.TenantID)
	}
	return libraryID, libraryKey, nil
}

// SearchVideos lists the videos of the tenant's Bunny library, newest first, filtered
// by a title substring. It uses ephemeral caching and singleflight, as FetchProducts does.
func (s *BunnyService) SearchVideos(tenantCtx *tenant.Context, queryStr string, page int) (map[string]any, error) {
	if page < 1 {
		page = 1
	}
	libraryID, libraryKey, err := libraryCredentials(tenantCtx)
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("bunny_videos_%s_l:%s_q:%s_p:%d", tenantCtx.TenantID, libraryID, queryStr, page)

	if cached, found := tenantCtx.CacheManager.GetGeneric(tenantCtx.TenantID, cacheKey); found {
		if result, ok := cached.(map[string]any); ok {
			return result, nil
		}
	}

	v, err, _ := s.requestGroup.Do(cacheKey, func() (any, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("itemsPerPage", strconv.Itoa(bunnyVideosPerPage))
		params.Set("orderBy", "date")
		if queryStr != "" {
			params.Set("search", queryStr)
		}
		requestURL := fmt.Sprintf("%s/library/%s/videos?%s", bunnyStreamAPIBase, libraryID, params.Encode())

		body, err := s.doRequest(http.MethodGet, requestURL, libraryKey, nil)
		if err != nil {
			return nil, err
		}

		var result struct {
			TotalItems   int64            `json:"totalItems"`
			CurrentPage  int64            `json:"currentPage"`
			ItemsPerPage int              `json:"itemsPerPage"`
			Items        []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to decode bunny videos: %w", err)
		}
		if result.Items == nil {
			result.Items = []map[string]any{}
		}

		responseEnvelope := map[string]any{
			"videos":       result.Items,
			"totalItems":   result.TotalItems,
			"currentPage":  result.CurrentPage,
			"itemsPerPage": result.ItemsPerPage,
		}

		tenantCtx.CacheManager.SetGenericWithTTL(tenantCtx.TenantID, cacheKey, responseEnvelope, 60*time.Second)
		return responseEnvelope, nil
	})
	if err != nil {
		return nil, err
	}

	return v.(map[string]any), nil
}

// GetVideo fetches one video's VideoModel from the tenant's Bunny library.
func (s *BunnyService) GetVideo(tenantCtx *tenant.Context, guid string) (*BunnyVideo, error) {
	if !bunnyGUIDPattern.MatchString(guid) {
		return nil, fmt.Errorf("invalid bunny video guid")
	}
	libraryID, libraryKey, err := libraryCredentials(tenantCtx)
	if err != nil {
		return nil, err
	}

	requestURL := fmt.Sprintf("%s/library/%s/videos/%s", bunnyStreamAPIBase, libraryID, guid)
	body, err := s.doRequest(http.MethodGet, requestURL, libraryKey, nil)
	if err != nil {
		return nil, err
	}

	var video BunnyVideo
	if err := json.Unmarshal(body, &video); err != nil {
		return nil, fmt.Errorf("failed to decode bunny video: %w", err)
	}
	if video.GUID == "" {
		return nil, fmt.Errorf("bunny returned no video for guid %s", guid)
	}
	video.Raw = json.RawMessage(body)
	return &video, nil
}

// EncodeStatusFromVideoStatus maps Bunny's VideoModel.status to encodeStatus.
// The webhook Status (plan 02) uses a different numbering and has its own mapping.
func EncodeStatusFromVideoStatus(status int) string {
	switch status {
	case 0: // Created
		return BunnyEncodeStatusUploading
	case 1, 2, 3, 7, 8: // Uploaded, Processing, Transcoding, JitSegmenting, JitPlaylistsCreated
		return BunnyEncodeStatusProcessing
	case 4: // Finished
		return BunnyEncodeStatusFinished
	case 5, 6: // Error, UploadFailed
		return BunnyEncodeStatusError
	default:
		return BunnyEncodeStatusProcessing
	}
}

// slugifyBunnyTitle applies the same rules as the frontend's resource slug helper:
// lower-case, drop everything but a-z, 0-9, spaces and hyphens, spaces to hyphens,
// collapse hyphens.
func slugifyBunnyTitle(title string) string {
	slug := strings.ToLower(title)
	slug = bunnySlugStripPattern.ReplaceAllString(slug, "")
	slug = bunnySlugSpacePattern.ReplaceAllString(slug, "-")
	slug = bunnySlugHyphenPattern.ReplaceAllString(slug, "-")
	return strings.TrimSpace(slug)
}

// BunnyVideoSlug builds the slug set once at creation:
// "bunny-video-" + slugified title + the first 8 characters of the guid.
func BunnyVideoSlug(title, guid string) string {
	shortGUID := strings.ToLower(guid)
	if len(shortGUID) > 8 {
		shortGUID = shortGUID[:8]
	}
	parts := []string{BunnyVideoCategory}
	if titleSlug := strings.Trim(slugifyBunnyTitle(title), "-"); titleSlug != "" {
		parts = append(parts, titleSlug)
	}
	parts = append(parts, shortGUID)
	return bunnySlugHyphenPattern.ReplaceAllString(strings.Join(parts, "-"), "-")
}

// TransformVideo converts a Bunny VideoModel into a bunny-video resource node.
func (s *BunnyService) TransformVideo(video *BunnyVideo) *content.ResourceNode {
	category := BunnyVideoCategory

	title := strings.TrimSpace(video.Title)
	if title == "" {
		title = video.GUID
	}

	optionsPayload := make(map[string]any)
	optionsPayload["guid"] = video.GUID
	optionsPayload["libraryId"] = strconv.FormatInt(video.VideoLibraryID, 10)
	optionsPayload["bunnyData"] = string(video.Raw)
	optionsPayload["lengthSeconds"] = float64(video.Length)
	optionsPayload["encodeStatus"] = EncodeStatusFromVideoStatus(video.Status)
	if video.ThumbnailURL != "" {
		optionsPayload["thumbnailUrl"] = video.ThumbnailURL
	}

	return &content.ResourceNode{
		Title:          title,
		Slug:           BunnyVideoSlug(title, video.GUID),
		CategorySlug:   &category,
		NodeType:       "Resource",
		OptionsPayload: optionsPayload,
	}
}

// UpsertVideoResource creates the bunny-video resource for a video, or updates the
// existing one found by guid. The merge is non-destructive, as for Shopify: an existing
// key absent from the incoming payload is kept, and incoming keys win. The slug, the
// oneliner and the action of an existing resource are kept.
func (s *BunnyService) UpsertVideoResource(tenantCtx *tenant.Context, resource *content.ResourceNode) (*content.ResourceNode, error) {
	guid, ok := resource.OptionsPayload["guid"].(string)
	if !ok || guid == "" {
		return nil, fmt.Errorf("incoming bunny video resource missing required 'guid' in optionsPayload")
	}

	existing, err := s.resourceService.GetByBunnyGUID(tenantCtx, guid)
	if err != nil {
		return nil, err
	}

	category := BunnyVideoCategory
	resource.CategorySlug = &category

	if existing != nil {
		// Cached resources are shared pointers: build a new node, never mutate existing.
		merged := make(map[string]any, len(existing.OptionsPayload)+len(resource.OptionsPayload))
		for k, v := range existing.OptionsPayload {
			merged[k] = v
		}
		for k, v := range resource.OptionsPayload {
			merged[k] = v
		}
		// guid and libraryId are fixed after creation.
		for _, field := range BunnyVideoFixedFields {
			if v, found := existing.OptionsPayload[field]; found {
				merged[field] = v
			}
		}
		if status, _ := merged["pipelineStatus"].(string); status == "" {
			merged["pipelineStatus"] = BunnyPipelineStatusIdle
		}

		resource.ID = existing.ID
		resource.Slug = existing.Slug
		resource.OneLiner = existing.OneLiner
		resource.ActionLisp = existing.ActionLisp
		resource.OptionsPayload = merged

		if err := s.resourceService.Update(tenantCtx, resource, nil); err != nil {
			return nil, fmt.Errorf("failed to update bunny video resource: %w", err)
		}
		s.logger.Content().Info("Bunny video resource updated", "tenantId", tenantCtx.TenantID, "guid", guid, "resourceId", resource.ID)
		return resource, nil
	}

	if status, _ := resource.OptionsPayload["pipelineStatus"].(string); status == "" {
		resource.OptionsPayload["pipelineStatus"] = BunnyPipelineStatusIdle
	}
	if err := s.resourceService.Create(tenantCtx, resource, nil); err != nil {
		return nil, fmt.Errorf("failed to create bunny video resource: %w", err)
	}
	s.logger.Content().Info("Bunny video resource created", "tenantId", tenantCtx.TenantID, "guid", guid, "resourceId", resource.ID)
	return resource, nil
}

// ImportVideo fetches a video from Bunny and upserts its resource.
func (s *BunnyService) ImportVideo(tenantCtx *tenant.Context, guid string) (*content.ResourceNode, error) {
	video, err := s.GetVideo(tenantCtx, guid)
	if err != nil {
		return nil, err
	}
	return s.UpsertVideoResource(tenantCtx, s.TransformVideo(video))
}
