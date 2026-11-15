package services

import (
	"bytes"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

func withTempBackend(t *testing.T) (cleanup func(), sourceRoot string) {
	t.Helper()
	tmp := t.TempDir()
	backend := filepath.Join(tmp, "t8k-go-server") + "/"
	sourceRoot = filepath.Join(tmp, "src", "my-tractstack")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "src", "custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := config.BackendPath
	config.BackendPath = backend
	return func() { config.BackendPath = prev }, sourceRoot
}

func TestAssetUploadTreeDeleteReference(t *testing.T) {
	cleanup, sourceRoot := withTempBackend(t)
	defer cleanup()

	svc := NewExtendAssetService(nil, nil)
	tenant := "default"

	// referencing source line
	refFile := filepath.Join(sourceRoot, "src", "custom", "banner.astro")
	if err := os.WriteFile(refFile, []byte("<img src=\"/custom/class-banners/paradiso.webp\" />\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "class-banners/paradiso.webp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("webp-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(&body, w.Boundary())
	if err := svc.UploadFromMultipart(tenant, mr); err != nil {
		t.Fatalf("upload: %v", err)
	}

	expected := filepath.Join(strings.TrimSuffix(config.BackendPath, "/"), "config", "default", "custom", "class-banners", "paradiso.webp")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("file missing at %s: %v", expected, err)
	}
	probe := filepath.Join(strings.TrimSuffix(config.BackendPath, "/"), "config", "default", "custom", "t8k-probe-default")
	if _, err := os.Stat(probe); err != nil {
		t.Fatalf("probe missing: %v", err)
	}

	tree, err := svc.Tree(tenant)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range tree {
		if n.Path == "class-banners/paradiso.webp" {
			found = true
		}
		if n.Path == "t8k-probe-default" || strings.HasPrefix(n.Name, "t8k-probe-") {
			t.Fatalf("probe leaked into tree: %+v", n)
		}
	}
	if !found {
		t.Fatalf("uploaded file not in tree: %+v", tree)
	}

	err = svc.Delete(tenant, "class-banners/paradiso.webp", false)
	if err == nil {
		t.Fatal("expected 409-style in-use error")
	}
	var inUse *AssetInUseError
	if !asAssetInUse(err, &inUse) {
		t.Fatalf("want AssetInUseError, got %T %v", err, err)
	}
	if len(inUse.References) == 0 {
		t.Fatal("expected references")
	}
}

func asAssetInUse(err error, target **AssetInUseError) bool {
	if e, ok := err.(*AssetInUseError); ok {
		*target = e
		return true
	}
	return false
}
