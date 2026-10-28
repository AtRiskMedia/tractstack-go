package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

var (
	codehooksManagedRe = regexp.MustCompile(`(?s)^\s*export\s+const\s+availableCodeHookIds\s*:\s*string\[\]\s*=\s*\[(.*?)\];\s*$`)
	stringLiteralRe    = regexp.MustCompile(`['"]([^'"]+)['"]`)
	importFromRe       = regexp.MustCompile(`(?m)^\s*import\s+(?:[\w*{}\s,]+\s+from\s+)?['"]([^'"]+)['"]\s*;?\s*$`)
	clientDirectiveRe = regexp.MustCompile(`client:(?:load|only|idle|visible)`)
)

// ExtendRegistry is the on-disk / in-memory extend.json shape (version 1).
type ExtendRegistry struct {
	Version  int                 `json:"version"`
	Manifest ExtendManifestEntry `json:"manifest"`
	Hooks    []ExtendHookEntry   `json:"hooks"`
	Routes   []ExtendRouteEntry  `json:"routes"`
}

// ExtendManifestEntry describes codehooks.ts mode.
type ExtendManifestEntry struct {
	File string `json:"file"`
	Mode string `json:"mode"` // managed | manual
}

// ExtendHookEntry is one discovered or registered hook.
type ExtendHookEntry struct {
	ID     string   `json:"id"`
	Kind   string   `json:"kind"` // astro | astro+jsx
	Blade  *string  `json:"blade"`
	Files  []string `json:"files"`
	Origin string   `json:"origin"`
}

// ExtendRouteEntry is one discovered or registered route.
type ExtendRouteEntry struct {
	Name      string `json:"name"`
	Shell     string `json:"shell"`
	View      string `json:"view,omitempty"`
	Origin    string `json:"origin"`
	ShellMode string `json:"shellMode"`
}

// RegistryJSONPath is GO_BACKEND_PATH/config/<tenantID>/extend.json.
func RegistryJSONPath(tenantID string) string {
	if tenantID == "" {
		tenantID = "default"
	}
	return filepath.Join(config.BackendPath, "config", tenantID, "extend.json")
}

// ExtendRegistryService serves GET /source/registry (discovery + read) and registry writes.
type ExtendRegistryService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
	usage       *ExtendUsageService
}

// NewExtendRegistryService creates the registry service.
func NewExtendRegistryService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ExtendRegistryService {
	return &ExtendRegistryService{logger: logger, perfTracker: perfTracker}
}

func (s *ExtendRegistryService) sourceRoot() (string, error) {
	root := SourceRoot(config.BackendPath)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", ErrExtendDisabled
	}
	return root, nil
}

// GetRegistry returns the registry. If config/<tenant>/extend.json is absent, runs discovery and writes it.
// Core blade ids are never returned (invariant 3), even if present in an older extend.json.
// Old src/custom/extend.json is ignored.
func (s *ExtendRegistryService) GetRegistry(tenantID string) (*ExtendRegistry, error) {
	root, err := s.sourceRoot()
	if err != nil {
		return nil, err
	}
	path := RegistryJSONPath(tenantID)
	if b, err := os.ReadFile(path); err == nil {
		var reg ExtendRegistry
		if err := json.Unmarshal(b, &reg); err != nil {
			return nil, fmt.Errorf("parse extend.json: %w", err)
		}
		stripCoreHooks(&reg)
		return &reg, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	reg, err := s.discover(root)
	if err != nil {
		return nil, err
	}
	if err := writeExtendJSON(path, reg); err != nil {
		return nil, err
	}
	stripCoreHooks(reg)
	return reg, nil
}

func stripCoreHooks(reg *ExtendRegistry) {
	if reg == nil {
		return
	}
	out := make([]ExtendHookEntry, 0, len(reg.Hooks))
	for _, h := range reg.Hooks {
		if IsCoreBladeID(h.ID) {
			continue
		}
		out = append(out, h)
	}
	reg.Hooks = out
}

func writeExtendJSON(path string, reg *ExtendRegistry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func (s *ExtendRegistryService) discover(root string) (*ExtendRegistry, error) {
	reg := &ExtendRegistry{
		Version: 1,
		Manifest: ExtendManifestEntry{
			File: "src/custom/codehooks.ts",
			Mode: "manual",
		},
		Hooks:  []ExtendHookEntry{},
		Routes: []ExtendRouteEntry{},
	}

	// codehooks.ts + blades
	codehooksPath := filepath.Join(root, "src", "custom", "codehooks.ts")
	if b, err := os.ReadFile(codehooksPath); err == nil {
		mode, ids := parseCodehooksTS(string(b))
		reg.Manifest.Mode = mode
		for _, id := range ids {
			if IsCoreBladeID(id) {
				continue
			}
			hook := ExtendHookEntry{
				ID:     id,
				Kind:   "astro",
				Files:  []string{},
				Origin: "discovered",
			}
			bladeRel := "src/pages/codehooks/" + id + ".astro"
			bladeAbs := filepath.Join(root, filepath.FromSlash(bladeRel))
			if _, err := os.Stat(bladeAbs); err != nil {
				hook.Blade = nil
				hook.Files = []string{}
			} else {
				hook.Blade = &bladeRel
				files, kind := analyzeBlade(root, bladeAbs)
				hook.Files = files
				hook.Kind = kind
			}
			reg.Hooks = append(reg.Hooks, hook)
		}
	}

	// src/pages/<dir>/[param1].astro minus PlatformDirectories
	pagesRoot := filepath.Join(root, "src", "pages")
	entries, err := os.ReadDir(pagesRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if IsPlatformDirectory(name) {
			continue
		}
		shellRel := "src/pages/" + name + "/[param1].astro"
		shellAbs := filepath.Join(root, filepath.FromSlash(shellRel))
		b, err := os.ReadFile(shellAbs)
		if err != nil {
			continue
		}
		shellMode := "manual"
		if ShellMatchesGenerated(string(b), name) {
			shellMode = "generated"
		}
		viewRel := RouteViewRelPath(name)
		viewAbs := filepath.Join(root, filepath.FromSlash(viewRel))
		route := ExtendRouteEntry{
			Name:      name,
			Shell:     shellRel,
			Origin:    "discovered",
			ShellMode: shellMode,
		}
		if _, err := os.Stat(viewAbs); err == nil {
			route.View = viewRel
		}
		reg.Routes = append(reg.Routes, route)
	}

	return reg, nil
}

// parseCodehooksTS returns mode and ids. managed only if the file is exactly the
// export const availableCodeHookIds: string[] = [ …string literals… ]; form.
func parseCodehooksTS(src string) (mode string, ids []string) {
	trimmed := stripTSComments(src)
	trimmed = strings.TrimSpace(trimmed)
	m := codehooksManagedRe.FindStringSubmatch(trimmed)
	if m == nil {
		// Conservative manual: still extract string literals from any array-looking content.
		ids = extractStringLiteralsInArray(src)
		return "manual", ids
	}
	inner := m[1]
	// Reject non-literal content (identifiers, expressions) inside the array.
	withoutLiterals := stringLiteralRe.ReplaceAllString(inner, "")
	withoutLiterals = strings.ReplaceAll(withoutLiterals, ",", "")
	withoutLiterals = strings.TrimSpace(withoutLiterals)
	if withoutLiterals != "" {
		return "manual", extractStringLiteralsInArray(src)
	}
	for _, sm := range stringLiteralRe.FindAllStringSubmatch(inner, -1) {
		ids = append(ids, sm[1])
	}
	return "managed", ids
}

func extractStringLiteralsInArray(src string) []string {
	// Prefer content inside availableCodeHookIds = [ ... ]
	idx := strings.Index(src, "availableCodeHookIds")
	if idx < 0 {
		return nil
	}
	rest := src[idx:]
	lb := strings.Index(rest, "[")
	rb := strings.Index(rest, "]")
	if lb < 0 || rb < 0 || rb <= lb {
		return nil
	}
	inner := rest[lb+1 : rb]
	var ids []string
	for _, sm := range stringLiteralRe.FindAllStringSubmatch(inner, -1) {
		ids = append(ids, sm[1])
	}
	return ids
}

func stripTSComments(src string) string {
	var b strings.Builder
	i := 0
	for i < len(src) {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// analyzeBlade computes hooks[].files and kind from first-level local imports under src/custom/.
func analyzeBlade(sourceRoot, bladeAbs string) (files []string, kind string) {
	kind = "astro"
	b, err := os.ReadFile(bladeAbs)
	if err != nil {
		return nil, kind
	}
	content := string(b)
	fm := astroFrontmatter(content)
	var resolved []string
	for _, sm := range importFromRe.FindAllStringSubmatch(fm, -1) {
		spec := sm[1]
		rel, ok := resolveLocalImport(sourceRoot, filepath.Dir(bladeAbs), spec)
		if !ok {
			continue
		}
		if !strings.HasPrefix(rel, "src/custom/") {
			continue
		}
		resolved = append(resolved, rel)
	}
	files = resolved
	checkClient := content
	for _, f := range files {
		fb, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(f)))
		if err == nil {
			checkClient += "\n" + string(fb)
		}
	}
	if clientDirectiveRe.MatchString(checkClient) {
		kind = "astro+jsx"
	}
	return files, kind
}

func astroFrontmatter(src string) string {
	if !strings.HasPrefix(strings.TrimSpace(src), "---") {
		return ""
	}
	rest := src
	if i := strings.Index(src, "---"); i >= 0 {
		rest = src[i+3:]
	}
	if j := strings.Index(rest, "---"); j >= 0 {
		return rest[:j]
	}
	return rest
}

func resolveLocalImport(sourceRoot, fromDir, spec string) (rel string, ok bool) {
	var abs string
	switch {
	case strings.HasPrefix(spec, "@/"):
		abs = filepath.Join(sourceRoot, "src", filepath.FromSlash(strings.TrimPrefix(spec, "@/")))
	case strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../"):
		abs = filepath.Join(fromDir, filepath.FromSlash(spec))
	default:
		return "", false
	}
	abs = filepath.Clean(abs)
	candidates := []string{abs}
	exts := []string{".astro", ".tsx", ".ts", ".jsx", ".js", ".mjs"}
	if filepath.Ext(abs) == "" {
		for _, e := range exts {
			candidates = append(candidates, abs+e)
		}
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			rel, err := filepath.Rel(sourceRoot, c)
			if err != nil {
				return "", false
			}
			return filepath.ToSlash(rel), true
		}
	}
	return "", false
}

