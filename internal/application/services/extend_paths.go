package services

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// StateDir is the concierge state directory (t8k-concierge.sh hardcodes it).
const StateDir = "/home/t8k/state"

var dedicatedSitePath = regexp.MustCompile(`^/home/t8k/sites/([a-z0-9-]{3,12})$`)

// PublishTarget describes where rebuild csvs should land for this install.
// Type is "main", "dedicated", or empty when publishing is unsupported.
type PublishTarget struct {
	Type string
	Site string
}

// InstallBase returns the install root: parent of BackendPath (trailing slash stripped).
func InstallBase(backendPath string) string {
	return filepath.Dir(strings.TrimSuffix(backendPath, "/"))
}

// SourceRoot is <InstallBase>/src/my-tractstack.
func SourceRoot(backendPath string) string {
	return filepath.Join(InstallBase(backendPath), "src", "my-tractstack")
}

// AssetRoot is BackendPath + "config/" + tenantID + "/custom".
func AssetRoot(backendPath, tenantID string) string {
	base := strings.TrimSuffix(backendPath, "/") + "/"
	return base + "config/" + tenantID + "/custom"
}

// ResolvePublishTarget maps InstallBase to a publish target or none.
func ResolvePublishTarget(installBase string) (PublishTarget, bool) {
	if installBase == "/home/t8k" {
		return PublishTarget{Type: "main"}, true
	}
	if m := dedicatedSitePath.FindStringSubmatch(installBase); m != nil {
		return PublishTarget{Type: "dedicated", Site: m[1]}, true
	}
	return PublishTarget{}, false
}

// PublishTargetFromBackend derives PublishTarget from GO_BACKEND_PATH.
func PublishTargetFromBackend(backendPath string) (PublishTarget, bool) {
	return ResolvePublishTarget(InstallBase(backendPath))
}

// ExtendEnabled is true when SourceRoot exists.
func ExtendEnabled(backendPath string) bool {
	info, err := os.Stat(SourceRoot(backendPath))
	return err == nil && info.IsDir()
}

// PublishEnabled is true when PublishTarget is set and StateDir is a writable directory.
func PublishEnabled(backendPath string) bool {
	if _, ok := PublishTargetFromBackend(backendPath); !ok {
		return false
	}
	return isWritableDir(StateDir)
}

func isWritableDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".t8k-write-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// ResolveUnderRoot cleans relPath, rejects absolute paths and "..", EvalSymlinks the
// root, and requires the result to stay under that root.
func ResolveUnderRoot(root, relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("path must be relative")
	}
	cleaned := filepath.Clean(relPath)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must not contain ..")
	}
	if strings.Contains(relPath, "..") {
		// Catch .. before Clean collapses (e.g. foo/../bar still has .. in input intent);
		// after Clean, only paths that still escape are rejected above.
		parts := strings.Split(filepath.ToSlash(relPath), "/")
		for _, p := range parts {
			if p == ".." {
				return "", fmt.Errorf("path must not contain ..")
			}
		}
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	abs := filepath.Join(resolvedRoot, cleaned)
	// If the target exists, resolve symlinks on the full path; otherwise resolve
	// existing parents so a symlink escape in an intermediate segment is caught.
	resolved, err := evalExistingPrefix(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("path escapes root")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return resolved, nil
}

func evalExistingPrefix(path string) (string, error) {
	cur := path
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			if len(missing) == 0 {
				return resolved, nil
			}
			return filepath.Join(append([]string{resolved}, reverse(missing)...)...), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("resolve path: %w", err)
		}
		missing = append(missing, filepath.Base(cur))
		cur = parent
	}
}

func reverse(s []string) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = s[len(s)-1-i]
	}
	return out
}

// IsDeniedSourcePath reports whether a cleaned relative source path hits a denied prefix.
func IsDeniedSourcePath(relPath string) bool {
	p := filepath.ToSlash(filepath.Clean(relPath))
	if p == "." || p == "" {
		return false
	}

	base := filepath.Base(p)
	if strings.HasPrefix(base, ".env") {
		return true
	}
	// Also deny .env* at any path segment (e.g. src/custom/.env.local).
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".env") {
			return true
		}
	}

	exactOrPrefix := []string{
		"node_modules",
		"dist",
		"releases",
		"astro.config.mjs",
		"package.json",
		"pnpm-lock.yaml",
		"tailwind.config.cjs",
		"src/layouts",
		"src/lib",
		"src/components",
		"src/utils",
		"src/stores",
		"src/pages/context",
		"src/pages/api",
		"src/pages/media",
		"src/pages/custom",
		"public",
	}
	for _, d := range exactOrPrefix {
		if p == d || strings.HasPrefix(p, d+"/") {
			return true
		}
	}

	// src/pages/[...slug]* and src/pages/storykeep*
	if strings.HasPrefix(p, "src/pages/[...slug]") || strings.HasPrefix(p, "src/pages/storykeep") {
		return true
	}

	return false
}

// GuardSourcePath resolves relPath under sourceRoot and rejects denied prefixes.
func GuardSourcePath(sourceRoot, relPath string) (string, error) {
	if IsDeniedSourcePath(relPath) {
		return "", fmt.Errorf("path denied: %s", relPath)
	}
	return ResolveUnderRoot(sourceRoot, relPath)
}
