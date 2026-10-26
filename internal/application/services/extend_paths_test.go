package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishTarget_Main(t *testing.T) {
	backend := "/home/t8k/t8k-go-server/"
	got, ok := PublishTargetFromBackend(backend)
	if !ok {
		t.Fatalf("expected publish target for %s", backend)
	}
	if got.Type != "main" || got.Site != "" {
		t.Fatalf("got %+v, want type=main", got)
	}
	if InstallBase(backend) != "/home/t8k" {
		t.Fatalf("InstallBase = %q, want /home/t8k", InstallBase(backend))
	}
}

func TestPublishTarget_Dedicated(t *testing.T) {
	id := "sneaky-psg1"
	backend := "/home/t8k/sites/" + id + "/t8k-go-server/"
	got, ok := PublishTargetFromBackend(backend)
	if !ok {
		t.Fatalf("expected publish target for %s", backend)
	}
	if got.Type != "dedicated" || got.Site != id {
		t.Fatalf("got %+v, want dedicated,site=%s", got, id)
	}
}

func TestPublishTarget_None_QuickInstall(t *testing.T) {
	backend := "/home/adon/t8k/t8k-go-server/"
	got, ok := PublishTargetFromBackend(backend)
	if ok {
		t.Fatalf("expected none for quick install, got %+v", got)
	}
	if InstallBase(backend) != "/home/adon/t8k" {
		t.Fatalf("InstallBase = %q", InstallBase(backend))
	}
}

func TestPublishTarget_DedicatedInvalidSiteID(t *testing.T) {
	// Outside ^[a-z0-9-]{3,12}$ → none
	backend := "/home/t8k/sites/AB/t8k-go-server/"
	if _, ok := PublishTargetFromBackend(backend); ok {
		t.Fatal("expected none for invalid site id")
	}
}

func TestSourceRootAndAssetRoot(t *testing.T) {
	backend := "/home/t8k/t8k-go-server/"
	if got := SourceRoot(backend); got != "/home/t8k/src/my-tractstack" {
		t.Fatalf("SourceRoot = %q", got)
	}
	if got := AssetRoot(backend, "default"); got != "/home/t8k/t8k-go-server/config/default/custom" {
		t.Fatalf("AssetRoot = %q", got)
	}
}

func TestResolveUnderRoot_AbsolutePath(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveUnderRoot(root, "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestResolveUnderRoot_Traversal(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveUnderRoot(root, "../outside")
	if err == nil {
		t.Fatal("expected error for .. traversal")
	}
	_, err = ResolveUnderRoot(root, "foo/../../outside")
	if err == nil {
		t.Fatal("expected error for nested .. traversal")
	}
}

func TestResolveUnderRoot_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveUnderRoot(root, "escape/secret.txt")
	if err == nil {
		t.Fatal("expected error for symlink escape")
	}
}

func TestResolveUnderRoot_OK(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "src", "custom")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveUnderRoot(root, "src/custom/HeaderWidget.astro")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "src", "custom", "HeaderWidget.astro")
	if got != want {
		// EvalSymlinks may canonicalize; compare cleaned
		gotReal, _ := filepath.EvalSymlinks(filepath.Dir(got))
		wantReal, _ := filepath.EvalSymlinks(filepath.Dir(want))
		if filepath.Join(gotReal, filepath.Base(got)) != filepath.Join(wantReal, filepath.Base(want)) {
			t.Fatalf("got %q want %q", got, want)
		}
	}
}

func TestIsDeniedSourcePath(t *testing.T) {
	denied := []string{
		".env",
		".env.local",
		"node_modules/pkg",
		"dist/server/entry.mjs",
		"releases/legacy",
		"astro.config.mjs",
		"package.json",
		"pnpm-lock.yaml",
		"tailwind.config.cjs",
		"src/layouts/Layout.astro",
		"src/lib/session.ts",
		"src/components/Menu.tsx",
		"src/utils/api.ts",
		"src/stores/resources.ts",
		"src/pages/[...slug].astro",
		"src/pages/[...slug]/edit.astro",
		"src/pages/context/[...contextSlug].astro",
		"src/pages/storykeep/advanced.astro",
		"src/pages/api/extend/stamp.ts",
		"src/pages/media/[...slug].astro",
		"src/pages/custom/[...slug].astro",
		"public/client/view.js",
	}
	for _, p := range denied {
		if !IsDeniedSourcePath(p) {
			t.Errorf("expected denied: %s", p)
		}
	}
	allowed := []string{
		"src/custom/HeaderWidget.astro",
		"src/custom/codehooks.ts",
		"src/pages/codehooks/sneakyfolio.astro",
		"src/pages/animals/[param1].astro",
		"src/generated/buildStamp.ts",
	}
	for _, p := range allowed {
		if IsDeniedSourcePath(p) {
			t.Errorf("expected allowed: %s", p)
		}
	}
}

func TestGuardSourcePath_Denied(t *testing.T) {
	root := t.TempDir()
	_, err := GuardSourcePath(root, "src/lib/foo.ts")
	if err == nil {
		t.Fatal("expected denied for src/lib")
	}
}

func TestGuardSourcePath_OK(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := GuardSourcePath(root, "src/custom/HeaderWidget.astro")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "HeaderWidget.astro" {
		t.Fatalf("got %q", got)
	}
}
