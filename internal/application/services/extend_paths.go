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

// JobsDir is StateDir/jobs (concierge job JSON + logs).
func JobsDir() string {
	return filepath.Join(StateDir, "jobs")
}

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

// PlatformDirectories is the §7 set: reservedSlugs after Phase 0 plus media/codehooks/custom
// (already in that list) and any directory whose name begins with "[".
var PlatformDirectories = map[string]struct{}{
	"api": {}, "privacy": {}, "terms": {}, "collections": {},
	"create": {}, "edit": {}, "concierge": {}, "context": {},
	"products": {}, "storykeep": {}, "cart": {}, "404": {},
	"transcribe": {}, "sitemap": {}, "sandbox": {}, "robots": {},
	"llm": {}, "custom": {}, "media": {}, "codehooks": {},
}

// IsPlatformDirectory reports whether name is a platform page directory (§7).
func IsPlatformDirectory(name string) bool {
	if strings.HasPrefix(name, "[") {
		return true
	}
	_, ok := PlatformDirectories[name]
	return ok
}

// CoreBladeIDs are refused for create/edit/delete (invariant 3).
var CoreBladeIDs = map[string]struct{}{
	"featured-article": {},
	"list-content":     {},
	"search-widget":    {},
	"bunny-video":      {},
	"epinet":           {},
	"[...hookId]":      {},
}

// IsCoreBladeID reports whether id is a core (inject-overwritten) codehook blade.
func IsCoreBladeID(id string) bool {
	_, ok := CoreBladeIDs[id]
	return ok
}

// SourcePathFlag is the tree flag for a relative source path.
// Values: "", "protected", "platform".
func SourcePathFlag(relPath string) string {
	p := filepath.ToSlash(filepath.Clean(relPath))
	switch p {
	case "src/custom/codehooks.ts",
		"src/custom/HeaderWidget.astro",
		"src/custom/customHelpers.ts":
		return "protected"
	}
	if p == "src/pages/codehooks/shopify-product-grid.astro" ||
		p == "src/pages/codehooks/shopify-service-list.astro" ||
		p == "src/custom/shopify" ||
		strings.HasPrefix(p, "src/custom/shopify/") {
		return "platform"
	}
	return ""
}

// IsCoreCodehookBladePath reports whether rel is a core blade under src/pages/codehooks/.
func IsCoreCodehookBladePath(relPath string) bool {
	p := filepath.ToSlash(filepath.Clean(relPath))
	const prefix = "src/pages/codehooks/"
	if !strings.HasPrefix(p, prefix) || !strings.HasSuffix(p, ".astro") {
		return false
	}
	base := strings.TrimSuffix(strings.TrimPrefix(p, prefix), ".astro")
	return IsCoreBladeID(base) || base == "[...hookId]"
}

// IsAllowlistedSourcePath reports whether rel is admitted by invariant 2 / §5 Source tree
// for GET/PUT file (positive allowlist, not merely "not denied").
// routeNames are extend.json routes[].name values (may be nil/empty).
func IsAllowlistedSourcePath(relPath string, routeNames map[string]struct{}) bool {
	p := filepath.ToSlash(filepath.Clean(relPath))
	if p == "." || p == "" || IsDeniedSourcePath(p) {
		return false
	}
	// Excluded even though allowlist names it: concierge-only stamp.
	if p == "src/generated/buildStamp.ts" || strings.HasPrefix(p, "src/generated/") {
		return false
	}
	if p == "src/custom/extend.json" {
		return false
	}
	if p == "src/custom" || strings.HasPrefix(p, "src/custom/") {
		return true
	}
	if strings.HasPrefix(p, "src/pages/codehooks/") && strings.HasSuffix(p, ".astro") {
		base := strings.TrimSuffix(filepath.Base(p), ".astro")
		if IsCoreBladeID(base) || base == "[...hookId]" {
			return false
		}
		// Only direct children of codehooks/ (no nested dirs in allowlist for blades).
		rel := strings.TrimPrefix(p, "src/pages/codehooks/")
		return !strings.Contains(rel, "/")
	}
	// src/pages/<name>/[param1].astro for registry routes
	if strings.HasPrefix(p, "src/pages/") && strings.HasSuffix(p, "/[param1].astro") {
		rest := strings.TrimPrefix(p, "src/pages/")
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && parts[1] == "[param1].astro" {
			name := parts[0]
			if IsPlatformDirectory(name) {
				return false
			}
			if routeNames == nil {
				return false
			}
			_, ok := routeNames[name]
			return ok
		}
	}
	return false
}

// GuardAllowlistedSourcePath resolves an allowlisted source path under sourceRoot.
func GuardAllowlistedSourcePath(sourceRoot, relPath string, routeNames map[string]struct{}) (string, error) {
	if !IsAllowlistedSourcePath(relPath, routeNames) {
		return "", fmt.Errorf("path denied: %s", relPath)
	}
	return ResolveUnderRoot(sourceRoot, relPath)
}
