package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

func withHistoryFixture(t *testing.T) (backend, srcRoot, tenantID string, cleanup func()) {
	t.Helper()
	tmp := t.TempDir()
	backend = filepath.Join(tmp, "t8k-go-server")
	srcRoot = filepath.Join(tmp, "src", "my-tractstack")
	tenantID = "default"
	if err := os.MkdirAll(filepath.Join(backend, "config", tenantID, "custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(backend, "config", tenantID, "media", "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcRoot, "src", "custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcRoot, "src", "pages", "codehooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcRoot, "src", "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeStamp(t, srcRoot, "stamp-S", "2026-11-07T10:00:00Z")

	prev := config.BackendPath
	config.BackendPath = backend + "/"
	prevFile := config.ExtendFileHistorySlots
	prevSite := config.ExtendSiteHistorySlots
	config.ExtendFileHistorySlots = 3
	config.ExtendSiteHistorySlots = 3

	cleanup = func() {
		config.BackendPath = prev
		config.ExtendFileHistorySlots = prevFile
		config.ExtendSiteHistorySlots = prevSite
	}
	return backend, srcRoot, tenantID, cleanup
}

func TestHistoryNsNoCollision(t *testing.T) {
	if HistoryNsSource("src/custom/x.ts") == HistoryNsAsset("x.ts") {
		t.Fatal("source and asset namespaces collided")
	}
	if HistoryNsCustomCSS() == HistoryNsAsset("media/css/custom.css") {
		t.Fatal("css and asset namespaces collided")
	}
	if HistoryNsSource("src/custom/a") == HistoryNsCustomCSS() {
		t.Fatal("source and css namespaces collided")
	}
}

func TestSnapshotBeforeWrite_EpochIdempotent(t *testing.T) {
	_, srcRoot, tenantID, cleanup := withHistoryFixture(t)
	defer cleanup()

	svc := NewExtendHistoryService(nil, nil)
	path := filepath.Join(srcRoot, "src", "custom", "HeaderWidget.astro")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	ns := HistoryNsSource("src/custom/HeaderWidget.astro")

	if err := svc.SnapshotBeforeWrite(tenantID, ns, path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.SnapshotBeforeWrite(tenantID, ns, path); err != nil {
		t.Fatal(err)
	}

	slots, err := svc.ListFileSlots(tenantID, ns)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	if slots[0].JobID != "stamp-S" || !slots[0].Existed {
		t.Fatalf("slot %+v", slots[0])
	}
	stored, err := os.ReadFile(filepath.Join(historyRoot(tenantID), "files", "stamp-S", "src", "custom", "HeaderWidget.astro"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != "v1" {
		t.Fatalf("stored %q want v1", stored)
	}
}

func TestSnapshotBeforeWrite_TombstoneAndPrune(t *testing.T) {
	_, srcRoot, tenantID, cleanup := withHistoryFixture(t)
	defer cleanup()
	config.ExtendFileHistorySlots = 2

	svc := NewExtendHistoryService(nil, nil)
	path := filepath.Join(srcRoot, "src", "custom", "New.astro")
	ns := HistoryNsSource("src/custom/New.astro")

	// create under stamp-S (did not exist)
	if err := svc.SnapshotBeforeWrite(tenantID, ns, path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("created"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeStamp(t, srcRoot, "stamp-T", "2026-11-07T11:00:00Z")
	if err := svc.SnapshotBeforeWrite(tenantID, ns, path); err != nil {
		t.Fatal(err)
	}
	writeStamp(t, srcRoot, "stamp-U", "2026-11-07T12:00:00Z")
	if err := svc.SnapshotBeforeWrite(tenantID, ns, path); err != nil {
		t.Fatal(err)
	}

	slots, err := svc.ListFileSlots(tenantID, ns)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 2 {
		t.Fatalf("want 2 slots after prune, got %d %+v", len(slots), slots)
	}
	if slots[0].JobID != "stamp-U" || slots[1].JobID != "stamp-T" {
		t.Fatalf("unexpected order %+v", slots)
	}
	// oldest tombstone (stamp-S) pruned
	for _, s := range slots {
		if s.JobID == "stamp-S" {
			t.Fatal("stamp-S should be pruned")
		}
	}
}

func TestBeginCommitAbortSitePoint(t *testing.T) {
	_, srcRoot, tenantID, cleanup := withHistoryFixture(t)
	defer cleanup()
	config.ExtendSiteHistorySlots = 2

	svc := NewExtendHistoryService(nil, nil)
	if err := os.WriteFile(filepath.Join(srcRoot, "src", "custom", "A.astro"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(config.BackendPath, "config", tenantID, "custom", "logo.png")
	if err := os.WriteFile(asset, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	css := CustomCSSPath(config.BackendPath, tenantID)
	if err := os.WriteFile(css, []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.BeginSitePoint(tenantID, "job-1"); err != nil {
		t.Fatal(err)
	}
	m, err := svc.ReadManifest(tenantID, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if m.JobID != "job-1" || m.BuiltAt != "" || m.FileCount < 3 {
		t.Fatalf("manifest %+v", m)
	}
	found := map[string]bool{}
	for _, f := range m.Files {
		found[f.Path] = true
		if f.SHA256 == "" || f.Size == 0 {
			t.Fatalf("entry %+v missing hash/size", f)
		}
	}
	if !found["src/custom/A.astro"] || !found["custom/logo.png"] || !found["media/css/custom.css"] {
		t.Fatalf("paths missing: %+v", found)
	}
	points, err := svc.ListSitePoints(tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 0 {
		t.Fatalf("provisional must not list in Sites, got %d", len(points))
	}

	if err := svc.CommitSitePoint(tenantID, "job-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CommitSitePoint(tenantID, "job-1"); err != nil {
		t.Fatal(err) // idempotent
	}
	points, err = svc.ListSitePoints(tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].JobID != "job-1" {
		t.Fatalf("want committed job-1, got %+v", points)
	}

	if err := svc.BeginSitePoint(tenantID, "job-2"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CommitSitePoint(tenantID, "job-2"); err != nil {
		t.Fatal(err)
	}
	if err := svc.BeginSitePoint(tenantID, "job-3"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CommitSitePoint(tenantID, "job-3"); err != nil {
		t.Fatal(err)
	}
	points, err = svc.ListSitePoints(tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("want 2 points, got %d", len(points))
	}
	if _, err := svc.ReadManifest(tenantID, "job-1"); err == nil {
		t.Fatal("job-1 should be pruned")
	}

	if err := svc.BeginSitePoint(tenantID, "job-abort"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AbortSitePoint(tenantID, "job-abort"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReadManifest(tenantID, "job-abort"); err == nil {
		t.Fatal("aborted seal should be gone")
	}
	points, err = svc.ListSitePoints(tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("abort of provisional must not change Sites, got %d", len(points))
	}
}

func TestRestoreSitePoint_SwapAndDeleteExtras(t *testing.T) {
	_, srcRoot, tenantID, cleanup := withHistoryFixture(t)
	defer cleanup()

	svc := NewExtendHistoryService(nil, nil)
	// minimal css service stub: leave nil and omit css from seal by not writing it... wait restore needs cssService only if css in manifest.
	if err := os.WriteFile(filepath.Join(srcRoot, "src", "custom", "Keep.astro"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.BeginSitePoint(tenantID, "job-seal"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CommitSitePoint(tenantID, "job-seal"); err != nil {
		t.Fatal(err)
	}

	// mutate live: add Extra.astro, change Keep.astro
	if err := os.WriteFile(filepath.Join(srcRoot, "src", "custom", "Extra.astro"), []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, "src", "custom", "Keep.astro"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	blade := filepath.Join(srcRoot, "src", "pages", "codehooks", "author-hook.astro")
	if err := os.WriteFile(blade, []byte("blade"), 0o644); err != nil {
		t.Fatal(err)
	}

	tenantCtx := &tenant.Context{TenantID: tenantID}
	res, err := svc.RestoreSitePoint(tenantCtx, "job-seal")
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("nil result")
	}

	keep, err := os.ReadFile(filepath.Join(srcRoot, "src", "custom", "Keep.astro"))
	if err != nil {
		t.Fatal(err)
	}
	if string(keep) != "keep" {
		t.Fatalf("Keep.astro = %q", keep)
	}
	if _, err := os.Stat(filepath.Join(srcRoot, "src", "custom", "Extra.astro")); !os.IsNotExist(err) {
		t.Fatalf("Extra.astro should be gone after custom dir swap, err=%v", err)
	}
	if _, err := os.Stat(blade); !os.IsNotExist(err) {
		t.Fatalf("author blade not in manifest should be deleted, err=%v", err)
	}
}

func TestSnapshot_ExistedFalse(t *testing.T) {
	_, srcRoot, tenantID, cleanup := withHistoryFixture(t)
	defer cleanup()
	svc := NewExtendHistoryService(nil, nil)
	path := filepath.Join(srcRoot, "src", "custom", "Missing.astro")
	ns := HistoryNsSource("src/custom/Missing.astro")
	if err := svc.SnapshotBeforeWrite(tenantID, ns, path); err != nil {
		t.Fatal(err)
	}
	slots, _ := svc.ListFileSlots(tenantID, ns)
	if len(slots) != 1 || slots[0].Existed {
		t.Fatalf("want existed=false tombstone, got %+v", slots)
	}
}
