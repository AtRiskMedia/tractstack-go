package services

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStamp(t *testing.T, root, jobID, builtAt string) {
	t.Helper()
	dir := filepath.Join(root, "src", "generated")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "export const buildStamp = { jobId: '" + jobID + "', builtAt: '" + builtAt + "' };\n"
	if err := os.WriteFile(filepath.Join(dir, "buildStamp.ts"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadBuildStamp_OK(t *testing.T) {
	tmp := t.TempDir()
	backend := filepath.Join(tmp, "t8k-go-server")
	src := filepath.Join(tmp, "src", "my-tractstack")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStamp(t, src, "01ABCDEF", "2026-11-07T12:00:00Z")

	got := ReadBuildStamp(backend + "/")
	if got.JobID != "01ABCDEF" || got.BuiltAt != "2026-11-07T12:00:00Z" {
		t.Fatalf("got %+v", got)
	}
}

func TestReadBuildStamp_Missing(t *testing.T) {
	tmp := t.TempDir()
	backend := filepath.Join(tmp, "t8k-go-server")
	_ = os.MkdirAll(filepath.Join(tmp, "src", "my-tractstack"), 0o755)
	_ = os.MkdirAll(backend, 0o755)

	got := ReadBuildStamp(backend)
	if got.JobID != "dev" || got.BuiltAt != "" {
		t.Fatalf("got %+v, want dev/empty", got)
	}
}

func TestReadBuildStamp_EmptyJobID(t *testing.T) {
	tmp := t.TempDir()
	backend := filepath.Join(tmp, "t8k-go-server")
	src := filepath.Join(tmp, "src", "my-tractstack")
	_ = os.MkdirAll(backend, 0o755)
	writeStamp(t, src, "", "x")

	got := ReadBuildStamp(backend)
	if got.JobID != "dev" || got.BuiltAt != "" {
		t.Fatalf("got %+v, want dev/empty", got)
	}
}

func TestReadBuildStamp_DevInject(t *testing.T) {
	tmp := t.TempDir()
	backend := filepath.Join(tmp, "t8k-go-server")
	src := filepath.Join(tmp, "src", "my-tractstack")
	_ = os.MkdirAll(backend, 0o755)
	writeStamp(t, src, "dev", "")

	got := ReadBuildStamp(backend)
	if got.JobID != "dev" || got.BuiltAt != "" {
		t.Fatalf("got %+v", got)
	}
}
