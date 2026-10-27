package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

const (
	sourceMaxFileBytes  = 2 << 20 // 2 MiB
	sourceMaxTreeEntries = 20000
)

var (
	ErrSourceNotFound     = errors.New("source file not found")
	ErrSourceTooLarge     = errors.New("source file too large")
	ErrSourceInvalidPath  = errors.New("invalid source path")
	ErrSourceForbidden    = errors.New("source path forbidden")
	ErrSourceNotText      = errors.New("not a text source file")
	ErrSourceTreeTooLarge = errors.New("source tree exceeds limit")
	ErrExtendDisabled     = errors.New("extend source root not available")
)

// SourceNode is one entry in GET /extend/source/tree.
type SourceNode struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Mtime    string `json:"mtime"`
	IsDir    bool   `json:"isDir"`
	Flag     string `json:"flag,omitempty"`
	TooLarge bool   `json:"tooLarge,omitempty"`
}

// ExtendSourceService serves allowlisted source under SourceRoot.
type ExtendSourceService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewExtendSourceService creates the source file service.
func NewExtendSourceService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ExtendSourceService {
	return &ExtendSourceService{logger: logger, perfTracker: perfTracker}
}

func (s *ExtendSourceService) sourceRoot() (string, error) {
	root := SourceRoot(config.BackendPath)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", ErrExtendDisabled
	}
	return root, nil
}

func (s *ExtendSourceService) loadRouteNames(root string) map[string]struct{} {
	names := make(map[string]struct{})
	b, err := os.ReadFile(filepath.Join(root, "src", "custom", "extend.json"))
	if err != nil {
		return names
	}
	var reg struct {
		Routes []struct {
			Name string `json:"name"`
		} `json:"routes"`
	}
	if json.Unmarshal(b, &reg) != nil {
		return names
	}
	for _, r := range reg.Routes {
		if r.Name != "" {
			names[r.Name] = struct{}{}
		}
	}
	return names
}

// Tree walks the invariant-2 / §5 Source tree allowlist. Ephemeral — never writes extend.json.
func (s *ExtendSourceService) Tree() ([]SourceNode, error) {
	root, err := s.sourceRoot()
	if err != nil {
		return nil, err
	}
	routeNames := s.loadRouteNames(root)
	var nodes []SourceNode
	seen := make(map[string]struct{})

	addNode := func(rel string, info os.FileInfo, isDir bool) error {
		rel = filepath.ToSlash(rel)
		if _, ok := seen[rel]; ok {
			return nil
		}
		seen[rel] = struct{}{}
		n := SourceNode{
			Name:  filepath.Base(rel),
			Path:  rel,
			Size:  info.Size(),
			Mtime: info.ModTime().UTC().Format(time.RFC3339),
			IsDir: isDir,
			Flag:  SourcePathFlag(rel),
		}
		if !isDir && info.Size() > sourceMaxFileBytes {
			n.TooLarge = true
		}
		nodes = append(nodes, n)
		if len(nodes) > sourceMaxTreeEntries {
			return ErrSourceTreeTooLarge
		}
		return nil
	}

	// Ensure parent dirs appear when listing files.
	ensureParents := func(rel string) error {
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for i := 1; i < len(parts); i++ {
			dirRel := strings.Join(parts[:i], "/")
			if _, ok := seen[dirRel]; ok {
				continue
			}
			abs := filepath.Join(root, filepath.FromSlash(dirRel))
			info, err := os.Stat(abs)
			if err != nil {
				continue
			}
			if err := addNode(dirRel, info, true); err != nil {
				return err
			}
		}
		return nil
	}

	// 1) Walk src/custom/**
	customRoot := filepath.Join(root, "src", "custom")
	if info, err := os.Stat(customRoot); err == nil && info.IsDir() {
		err = filepath.WalkDir(customRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if name == "node_modules" && d.IsDir() {
				return fs.SkipDir
			}
			if d.Type()&os.ModeSymlink != 0 {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			if IsDeniedSourcePath(relSlash) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			if err := ensureParents(relSlash); err != nil {
				return err
			}
			return addNode(relSlash, fi, d.IsDir())
		})
		if err != nil {
			return nil, err
		}
	}

	// 2) src/pages/codehooks/*.astro minus core ids
	codehooksDir := filepath.Join(root, "src", "pages", "codehooks")
	if entries, err := os.ReadDir(codehooksDir); err == nil {
		_ = ensureParents("src/pages/codehooks")
		if info, err := os.Stat(codehooksDir); err == nil {
			_ = addNode("src/pages/codehooks", info, true)
		}
		for _, e := range entries {
			if e.IsDir() || e.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".astro") {
				continue
			}
			id := strings.TrimSuffix(name, ".astro")
			if IsCoreBladeID(id) {
				continue
			}
			rel := "src/pages/codehooks/" + name
			fi, err := e.Info()
			if err != nil {
				continue
			}
			if err := addNode(rel, fi, false); err != nil {
				return nil, err
			}
		}
	}

	// 3) src/pages/<name>/[param1].astro for extend.json routes
	for name := range routeNames {
		if IsPlatformDirectory(name) {
			continue
		}
		rel := "src/pages/" + name + "/[param1].astro"
		abs := filepath.Join(root, filepath.FromSlash(rel))
		fi, err := os.Stat(abs)
		if err != nil || fi.IsDir() {
			continue
		}
		if err := ensureParents(rel); err != nil {
			return nil, err
		}
		if err := addNode(rel, fi, false); err != nil {
			return nil, err
		}
	}

	return nodes, nil
}

// GetFile returns text content for an allowlisted source file.
func (s *ExtendSourceService) GetFile(rel string) (string, error) {
	root, err := s.sourceRoot()
	if err != nil {
		return "", err
	}
	routeNames := s.loadRouteNames(root)
	abs, err := GuardAllowlistedSourcePath(root, rel, routeNames)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSourceInvalidPath, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrSourceNotFound
		}
		return "", err
	}
	if info.IsDir() {
		return "", ErrSourceNotFound
	}
	if info.Size() > sourceMaxFileBytes {
		return "", ErrSourceTooLarge
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", ErrSourceNotText
	}
	return string(b), nil
}

// PutFile writes text to an allowlisted source file. Refuses core blades.
func (s *ExtendSourceService) PutFile(rel, content string) error {
	root, err := s.sourceRoot()
	if err != nil {
		return err
	}
	if IsCoreCodehookBladePath(rel) {
		return fmt.Errorf("%w: core blade", ErrSourceForbidden)
	}
	routeNames := s.loadRouteNames(root)
	abs, err := GuardAllowlistedSourcePath(root, rel, routeNames)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSourceInvalidPath, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			// Allow write only if parent exists and path is allowlisted (create missing free-text files under custom).
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return err
			}
		} else {
			return err
		}
	} else if info.IsDir() {
		return ErrSourceNotFound
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}
