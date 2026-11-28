package services

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	_ "github.com/mattn/go-sqlite3"
)

const bunnyTestTenant = "test-tenant"

func setupBunnyResourceTest(t *testing.T) (*ResourceService, *tenant.Context) {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// One connection, so every query sees the same in-memory database.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS resources (id TEXT PRIMARY KEY, title TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, category_slug TEXT, oneliner TEXT NOT NULL, options_payload TEXT NOT NULL, action_lisp TEXT)`,
		`CREATE TABLE IF NOT EXISTS file_resources (id TEXT PRIMARY KEY, file_id TEXT, resource_id TEXT)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}

	logger, err := logging.NewChanneledLogger(&logging.LoggerConfig{
		OutputToFile:    false,
		OutputToConsole: false,
		DefaultLevel:    slog.LevelError,
		ChannelLevels:   make(map[logging.Channel]slog.Level),
		TimestampFormat: time.RFC3339,
	})
	if err != nil {
		t.Fatal(err)
	}
	perfTracker := performance.NewTracker(nil)
	cacheManager := manager.NewManager(logger)
	cacheManager.InitializeTenant(bunnyTestTenant)

	tenantCtx := &tenant.Context{
		TenantID:     bunnyTestTenant,
		Config:       &tenant.Config{TenantID: bunnyTestTenant},
		Database:     &tenant.Database{Conn: db, TenantID: bunnyTestTenant},
		CacheManager: cacheManager,
		Logger:       logger,
	}

	contentMapService := NewContentMapService(logger, perfTracker)
	resourceService := NewResourceService(logger, perfTracker, contentMapService, nil)
	return resourceService, tenantCtx
}

func insertBunnyTestResource(t *testing.T, tenantCtx *tenant.Context, id, slug, category string, payload map[string]any) {
	t.Helper()
	optionsJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tenantCtx.Database.Conn.Exec(
		`INSERT INTO resources (id, title, slug, category_slug, oneliner, action_lisp, options_payload) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, "A video", slug, category, "", "", string(optionsJSON),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func loadBunnyTestPayload(t *testing.T, tenantCtx *tenant.Context, id string) map[string]any {
	t.Helper()
	var raw string
	if err := tenantCtx.Database.Conn.QueryRow(`SELECT options_payload FROM resources WHERE id = ?`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("options_payload is not valid JSON after merge: %v (%s)", err, raw)
	}
	return payload
}

func TestMergeSystemFields(t *testing.T) {
	resourceService, tenantCtx := setupBunnyResourceTest(t)
	db := tenantCtx.Database.Conn

	insertBunnyTestResource(t, tenantCtx, "res-1", "bunny-video-a-video-12345678", BunnyVideoCategory, map[string]any{
		"guid":           "12345678-aaaa-bbbb-cccc-1234567890ab",
		"libraryId":      "42",
		"bunnyData":      `{"title":"A video"}`,
		"pipelineStatus": "idle",
		"localNote":      "keep me",
	})
	if _, err := db.Exec(`INSERT INTO file_resources (id, file_id, resource_id) VALUES ('fr-1', 'file-1', 'res-1')`); err != nil {
		t.Fatal(err)
	}

	// Warm the cache the way a read does, and keep the shared pointer.
	cachedBefore, err := resourceService.GetByID(tenantCtx, "res-1")
	if err != nil || cachedBefore == nil {
		t.Fatalf("failed to load resource: %v", err)
	}

	// A sentinel content map: any refresh would rebuild it from the database and drop it.
	sentinel := []types.FullContentMapItem{{ID: "sentinel", Title: "sentinel", Slug: "sentinel", Type: "Sentinel"}}
	tenantCtx.CacheManager.SetFullContentMap(bunnyTestTenant, sentinel)

	tenantCtx.CacheManager.TouchResourcesVersion(bunnyTestTenant)
	versionBefore, ok := tenantCtx.CacheManager.GetResourcesVersion(bunnyTestTenant)
	if !ok {
		t.Fatal("resources version not set")
	}
	time.Sleep(2 * time.Millisecond)

	err = resourceService.MergeSystemFields(tenantCtx, "res-1", map[string]any{
		"pipelineStatus": "encoding",
		"lengthSeconds":  125,
		"pipelineError":  `quote " and 'apostrophe' and {"json":true}`,
	})
	if err != nil {
		t.Fatalf("MergeSystemFields failed: %v", err)
	}

	// The database holds the new values, as the right JSON types, with other keys kept.
	payload := loadBunnyTestPayload(t, tenantCtx, "res-1")
	if payload["pipelineStatus"] != "encoding" {
		t.Errorf("pipelineStatus = %v, want encoding", payload["pipelineStatus"])
	}
	if payload["lengthSeconds"] != float64(125) {
		t.Errorf("lengthSeconds = %#v, want number 125", payload["lengthSeconds"])
	}
	if payload["pipelineError"] != `quote " and 'apostrophe' and {"json":true}` {
		t.Errorf("pipelineError was not stored as a plain string: %#v", payload["pipelineError"])
	}
	if payload["guid"] != "12345678-aaaa-bbbb-cccc-1234567890ab" || payload["localNote"] != "keep me" || payload["bunnyData"] != `{"title":"A video"}` {
		t.Errorf("untouched keys changed: %#v", payload)
	}

	// File links are kept.
	var links int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_resources WHERE resource_id = 'res-1'`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Errorf("file links = %d, want 1", links)
	}

	// No content-map refresh happened.
	contentMap, found := tenantCtx.CacheManager.GetFullContentMap(bunnyTestTenant)
	if !found || len(contentMap) != 1 || contentMap[0].ID != "sentinel" {
		t.Errorf("content map was refreshed or cleared: found=%v map=%#v", found, contentMap)
	}

	// The resources version moved, so a bulk read with If-None-Match gets 200, not 304.
	versionAfter, ok := tenantCtx.CacheManager.GetResourcesVersion(bunnyTestTenant)
	if !ok || !versionAfter.After(versionBefore) {
		t.Errorf("resources version was not touched: before=%v after=%v", versionBefore, versionAfter)
	}

	// The next bulk read returns the new values.
	results, err := resourceService.GetByFilters(tenantCtx, nil, []string{BunnyVideoCategory}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("bulk read returned %d resources, want 1", len(results))
	}
	if results[0].OptionsPayload["pipelineStatus"] != "encoding" || results[0].OptionsPayload["lengthSeconds"] != float64(125) {
		t.Errorf("bulk read returned stale values: %#v", results[0].OptionsPayload)
	}
	if results[0].OptionsPayload["localNote"] != "keep me" {
		t.Errorf("bulk read lost a local field: %#v", results[0].OptionsPayload)
	}

	// The previously cached pointer was replaced by a copy, not mutated.
	if cachedBefore.OptionsPayload["pipelineStatus"] != "idle" {
		t.Errorf("the shared cached resource was mutated in place")
	}
	if _, present := cachedBefore.OptionsPayload["lengthSeconds"]; present {
		t.Errorf("the shared cached resource was mutated in place")
	}
}

func TestMergeSystemFieldsRejectsBadInput(t *testing.T) {
	resourceService, tenantCtx := setupBunnyResourceTest(t)

	insertBunnyTestResource(t, tenantCtx, "res-video", "bunny-video-a-12345678", BunnyVideoCategory, map[string]any{"guid": "g", "libraryId": "1"})
	insertBunnyTestResource(t, tenantCtx, "res-other", "other-thing", "product", map[string]any{"gid": "x"})

	for _, field := range []string{"guid", "libraryId", "title", "x') , ('$.y"} {
		if err := resourceService.MergeSystemFields(tenantCtx, "res-video", map[string]any{field: "v"}); err == nil {
			t.Errorf("expected %q to be rejected as a system field", field)
		}
	}
	if err := resourceService.MergeSystemFields(tenantCtx, "res-other", map[string]any{"pipelineStatus": "ready"}); err == nil {
		t.Error("expected a non bunny-video resource to be rejected")
	}
	if err := resourceService.MergeSystemFields(tenantCtx, "missing", map[string]any{"pipelineStatus": "ready"}); err == nil {
		t.Error("expected a missing resource to be rejected")
	}

	payload := loadBunnyTestPayload(t, tenantCtx, "res-video")
	if payload["guid"] != "g" || payload["libraryId"] != "1" || len(payload) != 2 {
		t.Errorf("rejected merges changed the payload: %#v", payload)
	}
}

func TestGetByBunnyGUID(t *testing.T) {
	resourceService, tenantCtx := setupBunnyResourceTest(t)

	insertBunnyTestResource(t, tenantCtx, "res-a", "bunny-video-a-aaaaaaaa", BunnyVideoCategory, map[string]any{"guid": "aaaaaaaa-0000-0000-0000-000000000000"})
	insertBunnyTestResource(t, tenantCtx, "res-b", "bunny-video-b-bbbbbbbb", BunnyVideoCategory, map[string]any{"guid": "bbbbbbbb-0000-0000-0000-000000000000"})
	// The same value under another category must not match.
	insertBunnyTestResource(t, tenantCtx, "res-c", "product-c", "product", map[string]any{"guid": "cccccccc-0000-0000-0000-000000000000"})

	found, err := resourceService.GetByBunnyGUID(tenantCtx, "bbbbbbbb-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.ID != "res-b" {
		t.Errorf("GetByBunnyGUID returned %#v, want res-b", found)
	}

	for _, guid := range []string{"cccccccc-0000-0000-0000-000000000000", "dddddddd-0000-0000-0000-000000000000"} {
		missing, err := resourceService.GetByBunnyGUID(tenantCtx, guid)
		if err != nil {
			t.Fatal(err)
		}
		if missing != nil {
			t.Errorf("GetByBunnyGUID(%s) returned %s, want nil", guid, missing.ID)
		}
	}
}

func TestGuardBunnyVideoUpdate(t *testing.T) {
	videoCategory := BunnyVideoCategory
	existing := &content.ResourceNode{
		ID:           "res-1",
		Title:        "Old title",
		Slug:         "bunny-video-old-title-12345678",
		CategorySlug: &videoCategory,
		OneLiner:     "old",
		OptionsPayload: map[string]any{
			"guid":           "12345678-aaaa-bbbb-cccc-1234567890ab",
			"libraryId":      "42",
			"bunnyData":      `{"title":"Old title"}`,
			"pipelineStatus": "ready",
			"transcriptId":   "tr-1",
		},
	}

	otherCategory := "product"
	incoming := &content.ResourceNode{
		ID:           "res-1",
		Title:        "New title",
		Slug:         "hijacked-slug",
		CategorySlug: &otherCategory,
		OneLiner:     "new",
		OptionsPayload: map[string]any{
			"guid":           "ffffffff-aaaa-bbbb-cccc-1234567890ab",
			"libraryId":      "99",
			"pipelineStatus": "idle",
			"mediaUrl":       "https://example.com/injected.mp4",
			// bunnyData and transcriptId are omitted by the editor.
		},
	}

	if !GuardBunnyVideoUpdate(existing, incoming) {
		t.Fatal("guard did not apply to a bunny-video resource")
	}

	if incoming.Title != "New title" || incoming.OneLiner != "new" {
		t.Errorf("title and oneliner must stay editable: %q %q", incoming.Title, incoming.OneLiner)
	}
	if incoming.Slug != existing.Slug {
		t.Errorf("slug changed to %q", incoming.Slug)
	}
	if incoming.CategorySlug == nil || *incoming.CategorySlug != BunnyVideoCategory {
		t.Errorf("category changed")
	}
	if incoming.CategorySlug == existing.CategorySlug {
		t.Errorf("category pointer is shared with the cached resource")
	}
	want := map[string]any{
		"guid":           "12345678-aaaa-bbbb-cccc-1234567890ab",
		"libraryId":      "42",
		"bunnyData":      `{"title":"Old title"}`,
		"pipelineStatus": "ready",
		"transcriptId":   "tr-1",
	}
	if len(incoming.OptionsPayload) != len(want) {
		t.Errorf("payload = %#v, want %#v", incoming.OptionsPayload, want)
	}
	for k, v := range want {
		if incoming.OptionsPayload[k] != v {
			t.Errorf("payload[%s] = %#v, want %#v", k, incoming.OptionsPayload[k], v)
		}
	}
	if len(existing.OptionsPayload) != 5 || existing.Title != "Old title" {
		t.Errorf("the existing (cached) resource was mutated")
	}

	// Other categories are left alone.
	product := &content.ResourceNode{ID: "p", Slug: "product-a", CategorySlug: &otherCategory, OptionsPayload: map[string]any{"gid": "1"}}
	edit := &content.ResourceNode{ID: "p", Slug: "product-b", CategorySlug: &otherCategory, OptionsPayload: map[string]any{"gid": "2"}}
	if GuardBunnyVideoUpdate(product, edit) {
		t.Error("guard applied to a non bunny-video resource")
	}
	if edit.Slug != "product-b" || edit.OptionsPayload["gid"] != "2" {
		t.Error("guard changed a non bunny-video resource")
	}
}

func TestBunnyVideoSlug(t *testing.T) {
	cases := []struct {
		title string
		guid  string
		want  string
	}{
		{"Healing Crusade 2026", "A1B2C3D4-aaaa-bbbb-cccc-1234567890ab", "bunny-video-healing-crusade-2026-a1b2c3d4"},
		{"  Monday -- Night: Part #2!  ", "a1b2c3d4-aaaa-bbbb-cccc-1234567890ab", "bunny-video-monday-night-part-2-a1b2c3d4"},
		{"Café Ünïcode.mp4", "a1b2c3d4-aaaa-bbbb-cccc-1234567890ab", "bunny-video-caf-ncodemp4-a1b2c3d4"},
		{"!!!", "a1b2c3d4-aaaa-bbbb-cccc-1234567890ab", "bunny-video-a1b2c3d4"},
		{"", "a1b2c3d4-aaaa-bbbb-cccc-1234567890ab", "bunny-video-a1b2c3d4"},
	}
	for _, tc := range cases {
		got := BunnyVideoSlug(tc.title, tc.guid)
		if got != tc.want {
			t.Errorf("BunnyVideoSlug(%q) = %q, want %q", tc.title, got, tc.want)
		}
		if !bunnyTestSlugPattern(got) {
			t.Errorf("BunnyVideoSlug(%q) = %q does not match ^[a-z0-9-]+$", tc.title, got)
		}
	}
}

func bunnyTestSlugPattern(slug string) bool {
	if slug == "" {
		return false
	}
	for _, r := range slug {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}

func TestEncodeStatusFromVideoStatus(t *testing.T) {
	want := map[int]string{
		0: "uploading",
		1: "processing",
		2: "processing",
		3: "processing",
		4: "finished",
		5: "error",
		6: "error",
		7: "processing",
		8: "processing",
	}
	for status, expected := range want {
		if got := EncodeStatusFromVideoStatus(status); got != expected {
			t.Errorf("EncodeStatusFromVideoStatus(%d) = %q, want %q", status, got, expected)
		}
	}
}

func TestTransformVideo(t *testing.T) {
	service := NewBunnyService(nil, nil, nil)
	raw := `{"guid":"a1b2c3d4-aaaa-bbbb-cccc-1234567890ab","videoLibraryId":42,"title":"My Video","length":3725,"status":4,"thumbnailUrl":"https://vz-x.b-cdn.net/a1b2c3d4/thumbnail.jpg","extra":"kept"}`
	var video BunnyVideo
	if err := json.Unmarshal([]byte(raw), &video); err != nil {
		t.Fatal(err)
	}
	video.Raw = json.RawMessage(raw)

	node := service.TransformVideo(&video)
	if node.Title != "My Video" || node.Slug != "bunny-video-my-video-a1b2c3d4" || node.NodeType != "Resource" {
		t.Errorf("unexpected node: %#v", node)
	}
	if node.CategorySlug == nil || *node.CategorySlug != BunnyVideoCategory {
		t.Errorf("category not set")
	}
	payload := node.OptionsPayload
	if payload["guid"] != "a1b2c3d4-aaaa-bbbb-cccc-1234567890ab" || payload["libraryId"] != "42" {
		t.Errorf("guid/libraryId wrong: %#v", payload)
	}
	if payload["bunnyData"] != raw {
		t.Errorf("bunnyData must be the stringified VideoModel as Bunny sent it")
	}
	if payload["lengthSeconds"] != float64(3725) || payload["encodeStatus"] != "finished" {
		t.Errorf("length/status wrong: %#v", payload)
	}
	if payload["thumbnailUrl"] != "https://vz-x.b-cdn.net/a1b2c3d4/thumbnail.jpg" {
		t.Errorf("thumbnailUrl wrong: %#v", payload["thumbnailUrl"])
	}
}
