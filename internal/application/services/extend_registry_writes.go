package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var extendIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

var (
	ErrExtendBadID       = errors.New("invalid id or name")
	ErrExtendCoreID      = errors.New("core blade id refused")
	ErrExtendCollision   = errors.New("id or name collides")
	ErrExtendTrunkExists = errors.New("trunk already exists")
	ErrExtendNotFound    = errors.New("registry entry not found")
)

// HookCreateResult is the POST /hooks response body.
type HookCreateResult struct {
	Hook           ExtendHookEntry `json:"hook"`
	ManifestManual bool            `json:"manifestManual,omitempty"`
}

// RouteCreateResult is the POST /routes response body.
type RouteCreateResult struct {
	Route ExtendRouteEntry `json:"route"`
}

// RegistryRemoveResult is the DELETE response for hooks/routes.
type RegistryRemoveResult struct {
	Removed        []string `json:"removed"`
	Leftovers      []string `json:"leftovers"`
	ManifestManual bool     `json:"manifestManual,omitempty"`
}

// SetUsageService attaches the §7 usage scanner.
func (s *ExtendRegistryService) SetUsageService(u *ExtendUsageService) {
	s.usage = u
}

func (s *ExtendRegistryService) loadOrDiscover(root, tenantID string) (*ExtendRegistry, string, error) {
	path := RegistryJSONPath(tenantID)
	if b, err := os.ReadFile(path); err == nil {
		var reg ExtendRegistry
		if err := json.Unmarshal(b, &reg); err != nil {
			return nil, path, fmt.Errorf("parse extend.json: %w", err)
		}
		return &reg, path, nil
	} else if !os.IsNotExist(err) {
		return nil, path, err
	}
	reg, err := s.discover(root)
	if err != nil {
		return nil, path, err
	}
	if err := writeExtendJSON(path, reg); err != nil {
		return nil, path, err
	}
	return reg, path, nil
}

// CreateHook implements POST /source/hooks.
func (s *ExtendRegistryService) CreateHook(tenantID, id, kind string) (*HookCreateResult, error) {
	if kind == "" {
		kind = "astro"
	}
	if kind != "astro" && kind != "astro+jsx" {
		return nil, fmt.Errorf("%w: kind must be astro or astro+jsx", ErrExtendBadID)
	}
	if !extendIDRe.MatchString(id) {
		return nil, ErrExtendBadID
	}
	if IsCoreBladeID(id) {
		return nil, ErrExtendCoreID
	}
	if IsPlatformDirectory(id) {
		return nil, ErrExtendCollision
	}

	root, err := s.sourceRoot()
	if err != nil {
		return nil, err
	}
	reg, regPath, err := s.loadOrDiscover(root, tenantID)
	if err != nil {
		return nil, err
	}
	for _, h := range reg.Hooks {
		if h.ID == id {
			return nil, ErrExtendCollision
		}
	}
	for _, r := range reg.Routes {
		if r.Name == id {
			return nil, ErrExtendCollision
		}
	}

	bladeRel := "src/pages/codehooks/" + id + ".astro"
	bladeAbs := filepath.Join(root, filepath.FromSlash(bladeRel))
	if _, err := os.Stat(bladeAbs); err == nil {
		return nil, ErrExtendTrunkExists
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(bladeAbs), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(bladeAbs, []byte(GenerateHookScaffold(id, kind)), 0o644); err != nil {
		return nil, err
	}

	hook := ExtendHookEntry{
		ID:     id,
		Kind:   kind,
		Blade:  &bladeRel,
		Files:  []string{},
		Origin: "extend",
	}
	reg.Hooks = append(reg.Hooks, hook)

	manifestManual := reg.Manifest.Mode != "managed"
	if !manifestManual {
		if err := s.writeManagedManifest(root, reg); err != nil {
			return nil, err
		}
	}
	if err := writeExtendJSON(regPath, reg); err != nil {
		return nil, err
	}
	return &HookCreateResult{Hook: hook, ManifestManual: manifestManual}, nil
}

// DeleteHook implements DELETE /source/hooks/:id.
func (s *ExtendRegistryService) DeleteHook(requestTenantID, id string) (*RegistryRemoveResult, error) {
	if IsCoreBladeID(id) {
		return nil, ErrExtendCoreID
	}
	if s.usage == nil {
		return nil, fmt.Errorf("usage service not configured")
	}
	usages, err := s.usage.FindHookUsages(requestTenantID, id)
	if err != nil {
		return nil, err
	}
	if len(usages) > 0 {
		return nil, &ErrHookInUse{Usages: usages}
	}

	root, err := s.sourceRoot()
	if err != nil {
		return nil, err
	}
	reg, regPath, err := s.loadOrDiscover(root, requestTenantID)
	if err != nil {
		return nil, err
	}
	idx := -1
	var blade string
	for i, h := range reg.Hooks {
		if h.ID == id {
			idx = i
			if h.Blade != nil {
				blade = *h.Blade
			}
			break
		}
	}
	if idx < 0 {
		return nil, ErrExtendNotFound
	}
	if blade == "" {
		blade = "src/pages/codehooks/" + id + ".astro"
	}
	_ = os.Remove(filepath.Join(root, filepath.FromSlash(blade)))

	reg.Hooks = append(reg.Hooks[:idx], reg.Hooks[idx+1:]...)

	manifestManual := reg.Manifest.Mode != "managed"
	if !manifestManual {
		if err := s.writeManagedManifest(root, reg); err != nil {
			return nil, err
		}
	}
	if err := writeExtendJSON(regPath, reg); err != nil {
		return nil, err
	}
	return &RegistryRemoveResult{
		Removed:        []string{blade},
		Leftovers:      []string{},
		ManifestManual: manifestManual,
	}, nil
}

// CreateRoute implements POST /source/routes.
func (s *ExtendRegistryService) CreateRoute(requestTenantID, name string) (*RouteCreateResult, error) {
	if !extendIDRe.MatchString(name) {
		return nil, ErrExtendBadID
	}
	if IsCoreBladeID(name) {
		return nil, ErrExtendCoreID
	}
	if IsPlatformDirectory(name) {
		return nil, ErrExtendCollision
	}

	root, err := s.sourceRoot()
	if err != nil {
		return nil, err
	}
	reg, regPath, err := s.loadOrDiscover(root, requestTenantID)
	if err != nil {
		return nil, err
	}
	for _, h := range reg.Hooks {
		if h.ID == name {
			return nil, ErrExtendCollision
		}
	}
	for _, r := range reg.Routes {
		if r.Name == name {
			return nil, ErrExtendCollision
		}
	}
	pageDir := filepath.Join(root, "src", "pages", name)
	if info, err := os.Stat(pageDir); err == nil && info.IsDir() {
		return nil, ErrExtendCollision
	}
	if s.usage != nil {
		hit, err := s.usage.HasFragmentSlug(requestTenantID, name)
		if err != nil {
			return nil, err
		}
		if hit {
			return nil, ErrExtendCollision
		}
	}

	shellRel := "src/pages/" + name + "/[param1].astro"
	shellAbs := filepath.Join(root, filepath.FromSlash(shellRel))
	if _, err := os.Stat(shellAbs); err == nil {
		return nil, ErrExtendTrunkExists
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(shellAbs), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(shellAbs, []byte(GenerateShell(name)), 0o644); err != nil {
		return nil, err
	}

	viewRel := RouteViewRelPath(name)
	viewAbs := filepath.Join(root, filepath.FromSlash(viewRel))
	if err := os.MkdirAll(filepath.Dir(viewAbs), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(viewAbs); os.IsNotExist(err) {
		if err := os.WriteFile(viewAbs, []byte(GenerateRouteView(name)), 0o644); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	route := ExtendRouteEntry{
		Name:      name,
		Shell:     shellRel,
		View:      viewRel,
		Origin:    "extend",
		ShellMode: "generated",
	}
	reg.Routes = append(reg.Routes, route)

	if err := writeExtendJSON(regPath, reg); err != nil {
		return nil, err
	}
	return &RouteCreateResult{Route: route}, nil
}

// DeleteRoute implements DELETE /source/routes/:name.
// Removes shell and view trunks; does not cascade into imports.
func (s *ExtendRegistryService) DeleteRoute(requestTenantID, name string) (*RegistryRemoveResult, error) {
	if s.usage == nil {
		return nil, fmt.Errorf("usage service not configured")
	}
	usages, err := s.usage.FindRouteUsages(requestTenantID, name)
	if err != nil {
		return nil, err
	}
	if len(usages) > 0 {
		return nil, &ErrRouteInUse{Usages: usages}
	}

	root, err := s.sourceRoot()
	if err != nil {
		return nil, err
	}
	reg, regPath, err := s.loadOrDiscover(root, requestTenantID)
	if err != nil {
		return nil, err
	}
	idx := -1
	var shellRel, viewRel string
	for i, r := range reg.Routes {
		if r.Name == name {
			idx = i
			shellRel = r.Shell
			viewRel = r.View
			break
		}
	}
	if idx < 0 {
		return nil, ErrExtendNotFound
	}
	if shellRel == "" {
		shellRel = "src/pages/" + name + "/[param1].astro"
	}
	if viewRel == "" {
		viewRel = RouteViewRelPath(name)
	}

	removed := []string{}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(shellRel))); err == nil {
		removed = append(removed, shellRel)
	}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(viewRel))); err == nil {
		removed = append(removed, viewRel)
	}

	dirAbs := filepath.Join(root, "src", "pages", name)
	_ = os.Remove(dirAbs) // rmdir if empty

	reg.Routes = append(reg.Routes[:idx], reg.Routes[idx+1:]...)

	if err := writeExtendJSON(regPath, reg); err != nil {
		return nil, err
	}
	leftovers := listLeftovers(dirAbs, root)
	return &RegistryRemoveResult{
		Removed:   removed,
		Leftovers: leftovers,
	}, nil
}

func listLeftovers(dirAbs, sourceRoot string) []string {
	entries, err := os.ReadDir(dirAbs)
	if err != nil {
		return []string{}
	}
	var out []string
	for _, e := range entries {
		rel, err := filepath.Rel(sourceRoot, filepath.Join(dirAbs, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func (s *ExtendRegistryService) writeManagedManifest(root string, reg *ExtendRegistry) error {
	ids := make([]string, 0, len(reg.Hooks))
	for _, h := range reg.Hooks {
		ids = append(ids, h.ID)
	}
	return os.WriteFile(
		filepath.Join(root, "src", "custom", "codehooks.ts"),
		[]byte(GenerateManagedManifest(ids)),
		0o644,
	)
}
