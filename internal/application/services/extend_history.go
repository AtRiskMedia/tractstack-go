package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

const (
	historySchemaVersion = 1
	historyMetaFile      = "meta.json"
	historyManifestFile  = "MANIFEST.json"
	nsCustomCSS          = "media/css/custom.css"
	nsAssetPrefix        = "custom/"
	nsSourcePrefix       = "src/"
)

var (
	ErrHistoryNotFound      = errors.New("history slot or site point not found")
	ErrHistoryInvalidPath   = errors.New("invalid history path")
	ErrHistoryInvalidJob    = errors.New("invalid history job id")
	ErrHistorySealConflict  = errors.New("site history point already exists")
	ErrHistoryNoCustomCSS   = errors.New("custom.css service not configured")
)

// FileHistorySlot is one immutable per-file epoch snapshot.
type FileHistorySlot struct {
	JobID      string `json:"jobId"`
	BuiltAt    string `json:"builtAt"`
	CapturedAt string `json:"capturedAt"`
	Existed    bool   `json:"existed"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256,omitempty"`
}

// SiteHistoryMeta is a short index row for a sealed site point.
type SiteHistoryMeta struct {
	JobID      string `json:"jobId"`
	SealedAt   string `json:"sealedAt"`
	FileCount  int    `json:"fileCount"`
	TotalBytes int64  `json:"totalBytes"`
}

// HistoryMeta is config/<tenant>/extend-history/meta.json.
type HistoryMeta struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Files         map[string][]FileHistorySlot `json:"files"`
	Sites         []SiteHistoryMeta            `json:"sites"`
}

// ManifestFileEntry is one file in a sealed site MANIFEST.
type ManifestFileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// SiteManifest is site/<jobId>/MANIFEST.json.
type SiteManifest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	JobID         string              `json:"jobId"`
	BuiltAt       string              `json:"builtAt"`
	SealedAt      string              `json:"sealedAt"`
	TenantID      string              `json:"tenantId"`
	Stamp         BuildStamp          `json:"stamp"`
	Files         []ManifestFileEntry `json:"files"`
	FileCount     int                 `json:"fileCount"`
	TotalBytes    int64               `json:"totalBytes"`
}

// SitePointSummary is returned by ListSitePoints (optional job join fields filled by handler).
type SitePointSummary struct {
	JobID      string `json:"jobId"`
	SealedAt   string `json:"sealedAt"`
	FileCount  int    `json:"fileCount"`
	TotalBytes int64  `json:"totalBytes"`
	BuiltAt    string `json:"builtAt,omitempty"`
	Status     string `json:"status,omitempty"`
}

// SiteRestoreResult counts from RestoreSitePoint.
type SiteRestoreResult struct {
	Written []string `json:"written"`
	Deleted []string `json:"deleted"`
}

// ExtendHistoryService manages Extend design history under config/<tenant>/extend-history/.
type ExtendHistoryService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
	cssService  *ExtendCustomCSSService
	mu          sync.Map // tenantID -> *sync.Mutex
}

// NewExtendHistoryService creates the history service.
func NewExtendHistoryService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ExtendHistoryService {
	return &ExtendHistoryService{logger: logger, perfTracker: perfTracker}
}

// SetCustomCSSService attaches the css writer used on restore (STYLES_VER bump).
func (s *ExtendHistoryService) SetCustomCSSService(css *ExtendCustomCSSService) {
	s.cssService = css
}

func (s *ExtendHistoryService) lock(tenantID string) *sync.Mutex {
	v, _ := s.mu.LoadOrStore(tenantID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func historyRoot(tenantID string) string {
	if tenantID == "" {
		tenantID = "default"
	}
	return filepath.Join(config.BackendPath, "config", tenantID, "extend-history")
}

func HistoryNsSource(rel string) string {
	return filepath.ToSlash(filepath.Clean(rel))
}

func HistoryNsAsset(rel string) string {
	return nsAssetPrefix + filepath.ToSlash(filepath.Clean(rel))
}

func HistoryNsCustomCSS() string {
	return nsCustomCSS
}

func isSafeJobID(jobID string) bool {
	if jobID == "" || strings.Contains(jobID, "/") || strings.Contains(jobID, "..") || strings.Contains(jobID, "\\") {
		return false
	}
	for _, r := range jobID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func copyFileAtomic(src, dst string) (size int64, sum string, err error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, "", err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, "", err
	}
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), in)
	if err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return 0, "", err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return 0, "", err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return 0, "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return 0, "", err
	}
	_ = info
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func hashFile(path string) (size int64, sum string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, "", err
	}
	return info.Size(), hex.EncodeToString(h.Sum(nil)), nil
}

func (s *ExtendHistoryService) loadMeta(tenantID string) (*HistoryMeta, error) {
	path := filepath.Join(historyRoot(tenantID), historyMetaFile)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &HistoryMeta{
				SchemaVersion: historySchemaVersion,
				Files:         map[string][]FileHistorySlot{},
				Sites:         []SiteHistoryMeta{},
			}, nil
		}
		return nil, err
	}
	var meta HistoryMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, fmt.Errorf("parse history meta: %w", err)
	}
	if meta.Files == nil {
		meta.Files = map[string][]FileHistorySlot{}
	}
	if meta.Sites == nil {
		meta.Sites = []SiteHistoryMeta{}
	}
	if meta.SchemaVersion == 0 {
		meta.SchemaVersion = historySchemaVersion
	}
	return &meta, nil
}

func (s *ExtendHistoryService) saveMeta(tenantID string, meta *HistoryMeta) error {
	meta.SchemaVersion = historySchemaVersion
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return atomicWriteFile(filepath.Join(historyRoot(tenantID), historyMetaFile), b, 0o644)
}

// SnapshotBeforeWrite captures pre-write bytes for nsPath under the live stamp epoch.
// Idempotent: if a slot for the current stamp already exists, this is a no-op.
func (s *ExtendHistoryService) SnapshotBeforeWrite(tenantID, nsPath, absPath string) error {
	nsPath = filepath.ToSlash(filepath.Clean(nsPath))
	if nsPath == "." || nsPath == "" || strings.Contains(nsPath, "..") {
		return ErrHistoryInvalidPath
	}
	mu := s.lock(tenantID)
	mu.Lock()
	defer mu.Unlock()

	stamp := ReadBuildStamp(config.BackendPath)
	meta, err := s.loadMeta(tenantID)
	if err != nil {
		return err
	}
	slots := meta.Files[nsPath]
	for _, slot := range slots {
		if slot.JobID == stamp.JobID {
			return nil // epoch already captured
		}
	}

	slot := FileHistorySlot{
		JobID:      stamp.JobID,
		BuiltAt:    stamp.BuiltAt,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Existed:    false,
	}
	info, err := os.Stat(absPath)
	if err == nil && !info.IsDir() {
		slot.Existed = true
		dest := filepath.Join(historyRoot(tenantID), "files", stamp.JobID, filepath.FromSlash(nsPath))
		size, sum, copyErr := copyFileAtomic(absPath, dest)
		if copyErr != nil {
			return copyErr
		}
		slot.Size = size
		slot.SHA256 = sum
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	meta.Files[nsPath] = append([]FileHistorySlot{slot}, slots...)
	// prune
	maxSlots := config.ExtendFileHistorySlots
	if maxSlots < 1 {
		maxSlots = 1
	}
	for len(meta.Files[nsPath]) > maxSlots {
		old := meta.Files[nsPath][len(meta.Files[nsPath])-1]
		meta.Files[nsPath] = meta.Files[nsPath][:len(meta.Files[nsPath])-1]
		if old.Existed {
			_ = os.Remove(filepath.Join(historyRoot(tenantID), "files", old.JobID, filepath.FromSlash(nsPath)))
		}
	}
	return s.saveMeta(tenantID, meta)
}

// ListFileSlots returns newest-first slots for a namespaced path.
func (s *ExtendHistoryService) ListFileSlots(tenantID, nsPath string) ([]FileHistorySlot, error) {
	nsPath = filepath.ToSlash(filepath.Clean(nsPath))
	mu := s.lock(tenantID)
	mu.Lock()
	defer mu.Unlock()
	meta, err := s.loadMeta(tenantID)
	if err != nil {
		return nil, err
	}
	slots := meta.Files[nsPath]
	if slots == nil {
		return []FileHistorySlot{}, nil
	}
	out := make([]FileHistorySlot, len(slots))
	copy(out, slots)
	return out, nil
}

func (s *ExtendHistoryService) findSlot(meta *HistoryMeta, nsPath, jobID string) (FileHistorySlot, bool) {
	for _, slot := range meta.Files[nsPath] {
		if slot.JobID == jobID {
			return slot, true
		}
	}
	return FileHistorySlot{}, false
}

// RestoreFileSlot writes historical bytes live for one namespaced path.
// Source/asset restore is done here; custom.css uses cssService.Put when tenantCtx is set.
// Caller is responsible for RegenerateAll after source restores.
func (s *ExtendHistoryService) RestoreFileSlot(tenantCtx *tenant.Context, nsPath, jobID string) error {
	if tenantCtx == nil {
		return fmt.Errorf("tenant context required")
	}
	tenantID := tenantCtx.TenantID
	nsPath = filepath.ToSlash(filepath.Clean(nsPath))
	if !isSafeJobID(jobID) {
		return ErrHistoryInvalidJob
	}

	mu := s.lock(tenantID)
	mu.Lock()
	meta, err := s.loadMeta(tenantID)
	if err != nil {
		mu.Unlock()
		return err
	}
	slot, ok := s.findSlot(meta, nsPath, jobID)
	mu.Unlock()
	if !ok {
		return ErrHistoryNotFound
	}

	liveAbs, err := s.resolveLiveAbs(tenantID, nsPath)
	if err != nil {
		return err
	}
	// Capture current epoch before overwrite (idempotent).
	_ = s.SnapshotBeforeWrite(tenantID, nsPath, liveAbs)

	if nsPath == nsCustomCSS {
		if s.cssService == nil {
			return ErrHistoryNoCustomCSS
		}
		if !slot.Existed {
			// Leave file; no css delete API. Treat as restore of empty content.
			_, err := s.cssService.Put(tenantCtx, "")
			return err
		}
		b, err := os.ReadFile(filepath.Join(historyRoot(tenantID), "files", jobID, filepath.FromSlash(nsPath)))
		if err != nil {
			return err
		}
		_, err = s.cssService.Put(tenantCtx, string(b))
		return err
	}

	if !slot.Existed {
		if err := os.Remove(liveAbs); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	src := filepath.Join(historyRoot(tenantID), "files", jobID, filepath.FromSlash(nsPath))
	_, _, err = copyFileAtomic(src, liveAbs)
	return err
}

func (s *ExtendHistoryService) resolveLiveAbs(tenantID, nsPath string) (string, error) {
	nsPath = filepath.ToSlash(filepath.Clean(nsPath))
	switch {
	case nsPath == nsCustomCSS:
		return CustomCSSPath(config.BackendPath, tenantID), nil
	case strings.HasPrefix(nsPath, nsAssetPrefix):
		rel := strings.TrimPrefix(nsPath, nsAssetPrefix)
		if err := validateAssetRelPath(rel); err != nil {
			return "", fmt.Errorf("%w: %v", ErrHistoryInvalidPath, err)
		}
		root := AssetRoot(config.BackendPath, tenantID)
		return ResolveUnderRoot(root, rel)
	case strings.HasPrefix(nsPath, nsSourcePrefix):
		if IsDeniedSourcePath(nsPath) {
			return "", ErrHistoryInvalidPath
		}
		root := SourceRoot(config.BackendPath)
		return ResolveUnderRoot(root, nsPath)
	default:
		return "", ErrHistoryInvalidPath
	}
}

// designFile is one enumerated design-tree entry with live abs path.
type designFile struct {
	NS  string
	Abs string
}

// EnumerateDesignTree lists source + assets + custom.css for seal/restore.
func (s *ExtendHistoryService) EnumerateDesignTree(tenantID string) ([]designFile, error) {
	var out []designFile
	root := SourceRoot(config.BackendPath)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil, ErrExtendDisabled
	}

	// src/custom/**
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
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			if IsDeniedSourcePath(relSlash) || relSlash == "src/custom/extend.json" {
				return nil
			}
			out = append(out, designFile{NS: HistoryNsSource(relSlash), Abs: path})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// codehooks blades minus core
	codehooksDir := filepath.Join(root, "src", "pages", "codehooks")
	if entries, err := os.ReadDir(codehooksDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || e.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".astro") {
				continue
			}
			id := strings.TrimSuffix(name, ".astro")
			if IsCoreBladeID(id) {
				continue
			}
			rel := "src/pages/codehooks/" + name
			out = append(out, designFile{NS: HistoryNsSource(rel), Abs: filepath.Join(codehooksDir, name)})
		}
	}

	// route shells (discover rule: non-platform dirs with [param1].astro)
	pagesRoot := filepath.Join(root, "src", "pages")
	if entries, err := os.ReadDir(pagesRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if IsPlatformDirectory(name) {
				continue
			}
			rel := "src/pages/" + name + "/[param1].astro"
			abs := filepath.Join(root, filepath.FromSlash(rel))
			if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
				out = append(out, designFile{NS: HistoryNsSource(rel), Abs: abs})
			}
		}
	}

	// assets
	assetRoot := AssetRoot(config.BackendPath, tenantID)
	probe := probeMarkerName(tenantID)
	if info, err := os.Stat(assetRoot); err == nil && info.IsDir() {
		_ = filepath.WalkDir(assetRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}
			rel, err := filepath.Rel(assetRoot, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			if relSlash == probe || strings.HasPrefix(relSlash, probe+"/") {
				return nil
			}
			out = append(out, designFile{NS: HistoryNsAsset(relSlash), Abs: path})
			return nil
		})
	}

	cssPath := CustomCSSPath(config.BackendPath, tenantID)
	if fi, err := os.Stat(cssPath); err == nil && !fi.IsDir() {
		out = append(out, designFile{NS: HistoryNsCustomCSS(), Abs: cssPath})
	}

	return out, nil
}

// BeginSitePoint provisionally seals the design tree as site/<jobID>/.
// Does not append to meta.Sites (not restorable until CommitSitePoint).
// Immutable; fails if site/<jobID>/ already present.
func (s *ExtendHistoryService) BeginSitePoint(tenantID, jobID string) error {
	if !isSafeJobID(jobID) {
		return ErrHistoryInvalidJob
	}
	mu := s.lock(tenantID)
	mu.Lock()
	defer mu.Unlock()

	finalDir := filepath.Join(historyRoot(tenantID), "site", jobID)
	if _, err := os.Stat(finalDir); err == nil {
		return ErrHistorySealConflict
	} else if !os.IsNotExist(err) {
		return err
	}

	files, err := s.EnumerateDesignTree(tenantID)
	if err != nil {
		return err
	}
	stamp := ReadBuildStamp(config.BackendPath)
	tmpDir := filepath.Join(historyRoot(tenantID), "site", ".tmp-"+jobID)
	_ = os.RemoveAll(tmpDir)
	treeDir := filepath.Join(tmpDir, "tree")
	if err := os.MkdirAll(treeDir, 0o755); err != nil {
		return err
	}

	var entries []ManifestFileEntry
	var total int64
	for _, f := range files {
		dest := filepath.Join(treeDir, filepath.FromSlash(f.NS))
		size, sum, err := copyFileAtomic(f.Abs, dest)
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			return fmt.Errorf("seal copy %s: %w", f.NS, err)
		}
		entries = append(entries, ManifestFileEntry{Path: f.NS, Size: size, SHA256: sum})
		total += size
	}

	sealedAt := time.Now().UTC().Format(time.RFC3339)
	manifest := SiteManifest{
		SchemaVersion: historySchemaVersion,
		JobID:         jobID,
		BuiltAt:       "", // filled at commit / concierge success; empty while provisional
		SealedAt:      sealedAt,
		TenantID:      tenantID,
		Stamp:         stamp,
		Files:         entries,
		FileCount:     len(entries),
		TotalBytes:    total,
	}
	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}
	mb = append(mb, '\n')
	if err := atomicWriteFile(filepath.Join(tmpDir, historyManifestFile), mb, 0o644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}
	if d, err := os.Open(tmpDir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}
	if err := os.Rename(tmpDir, finalDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}
	return nil
}

// CommitSitePoint promotes a provisional site/<jobID>/ into meta.Sites (durable history).
// Idempotent if already listed. Requires the seal directory to exist.
func (s *ExtendHistoryService) CommitSitePoint(tenantID, jobID string) error {
	if !isSafeJobID(jobID) {
		return ErrHistoryInvalidJob
	}
	mu := s.lock(tenantID)
	mu.Lock()
	defer mu.Unlock()

	finalDir := filepath.Join(historyRoot(tenantID), "site", jobID)
	manifestPath := filepath.Join(finalDir, historyManifestFile)
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrHistoryNotFound
		}
		return err
	}
	var manifest SiteManifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}

	meta, err := s.loadMeta(tenantID)
	if err != nil {
		return err
	}
	for _, p := range meta.Sites {
		if p.JobID == jobID {
			return nil // already committed
		}
	}
	meta.Sites = append([]SiteHistoryMeta{{
		JobID:      jobID,
		SealedAt:   manifest.SealedAt,
		FileCount:  manifest.FileCount,
		TotalBytes: manifest.TotalBytes,
	}}, meta.Sites...)

	maxSites := config.ExtendSiteHistorySlots
	if maxSites < 1 {
		maxSites = 1
	}
	for len(meta.Sites) > maxSites {
		old := meta.Sites[len(meta.Sites)-1]
		meta.Sites = meta.Sites[:len(meta.Sites)-1]
		_ = os.RemoveAll(filepath.Join(historyRoot(tenantID), "site", old.JobID))
	}
	return s.saveMeta(tenantID, meta)
}

// AbortSitePoint discards a provisional (or committed) site/<jobID>/ and removes it from meta.Sites.
// Idempotent when the seal is already gone.
func (s *ExtendHistoryService) AbortSitePoint(tenantID, jobID string) error {
	if !isSafeJobID(jobID) {
		return ErrHistoryInvalidJob
	}
	mu := s.lock(tenantID)
	mu.Lock()
	defer mu.Unlock()

	_ = os.RemoveAll(filepath.Join(historyRoot(tenantID), "site", jobID))
	_ = os.RemoveAll(filepath.Join(historyRoot(tenantID), "site", ".tmp-"+jobID))

	meta, err := s.loadMeta(tenantID)
	if err != nil {
		return err
	}
	filtered := meta.Sites[:0]
	changed := false
	for _, p := range meta.Sites {
		if p.JobID == jobID {
			changed = true
			continue
		}
		filtered = append(filtered, p)
	}
	if !changed {
		return nil
	}
	meta.Sites = filtered
	return s.saveMeta(tenantID, meta)
}

// ListSitePoints returns newest-first site points from meta.
func (s *ExtendHistoryService) ListSitePoints(tenantID string) ([]SitePointSummary, error) {
	mu := s.lock(tenantID)
	mu.Lock()
	defer mu.Unlock()
	meta, err := s.loadMeta(tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]SitePointSummary, 0, len(meta.Sites))
	for _, p := range meta.Sites {
		out = append(out, SitePointSummary{
			JobID:      p.JobID,
			SealedAt:   p.SealedAt,
			FileCount:  p.FileCount,
			TotalBytes: p.TotalBytes,
		})
	}
	return out, nil
}

// ReadManifest reads site/<jobID>/MANIFEST.json.
func (s *ExtendHistoryService) ReadManifest(tenantID, jobID string) (*SiteManifest, error) {
	if !isSafeJobID(jobID) {
		return nil, ErrHistoryInvalidJob
	}
	b, err := os.ReadFile(filepath.Join(historyRoot(tenantID), "site", jobID, historyManifestFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrHistoryNotFound
		}
		return nil, err
	}
	var m SiteManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// RestoreSitePoint restores a sealed site point onto the live design tree.
// Caller must RegenerateAll + InvalidateTenant afterwards.
func (s *ExtendHistoryService) RestoreSitePoint(tenantCtx *tenant.Context, jobID string) (*SiteRestoreResult, error) {
	if tenantCtx == nil {
		return nil, fmt.Errorf("tenant context required")
	}
	tenantID := tenantCtx.TenantID
	if !isSafeJobID(jobID) {
		return nil, ErrHistoryInvalidJob
	}
	// Only committed points (meta.Sites) are restorable.
	mu := s.lock(tenantID)
	mu.Lock()
	meta, err := s.loadMeta(tenantID)
	if err != nil {
		mu.Unlock()
		return nil, err
	}
	committed := false
	for _, p := range meta.Sites {
		if p.JobID == jobID {
			committed = true
			break
		}
	}
	mu.Unlock()
	if !committed {
		return nil, ErrHistoryNotFound
	}
	manifest, err := s.ReadManifest(tenantID, jobID)
	if err != nil {
		return nil, err
	}
	treeRoot := filepath.Join(historyRoot(tenantID), "site", jobID, "tree")

	manifestSet := make(map[string]ManifestFileEntry, len(manifest.Files))
	for _, f := range manifest.Files {
		manifestSet[f.Path] = f
	}

	live, err := s.EnumerateDesignTree(tenantID)
	if err != nil && !errors.Is(err, ErrExtendDisabled) {
		return nil, err
	}
	liveSet := make(map[string]designFile, len(live))
	for _, f := range live {
		liveSet[f.NS] = f
	}

	// Snapshot every path that will change (epoch rule).
	for ns, f := range liveSet {
		if _, ok := manifestSet[ns]; !ok {
			_ = s.SnapshotBeforeWrite(tenantID, ns, f.Abs)
			continue
		}
		_ = s.SnapshotBeforeWrite(tenantID, ns, f.Abs)
	}
	for ns := range manifestSet {
		if _, ok := liveSet[ns]; ok {
			continue
		}
		abs, rerr := s.resolveLiveAbs(tenantID, ns)
		if rerr == nil {
			_ = s.SnapshotBeforeWrite(tenantID, ns, abs)
		}
	}

	result := &SiteRestoreResult{Written: []string{}, Deleted: []string{}}

	// --- source: src/custom dir swap ---
	srcRoot := SourceRoot(config.BackendPath)
	customLive := filepath.Join(srcRoot, "src", "custom")
	customTree := filepath.Join(treeRoot, "src", "custom")
	if err := s.swapDirFromTree(customTree, customLive); err != nil {
		return result, fmt.Errorf("restore src/custom: %w", err)
	}
	for ns := range manifestSet {
		if strings.HasPrefix(ns, "src/custom/") {
			result.Written = append(result.Written, ns)
		}
	}

	// --- codehooks + route shells: copy/delete individually ---
	for ns, entry := range manifestSet {
		if !strings.HasPrefix(ns, "src/pages/") {
			continue
		}
		if strings.HasPrefix(ns, "src/custom/") {
			continue
		}
		src := filepath.Join(treeRoot, filepath.FromSlash(ns))
		dst := filepath.Join(srcRoot, filepath.FromSlash(ns))
		if _, _, err := copyFileAtomic(src, dst); err != nil {
			return result, fmt.Errorf("restore %s: %w", ns, err)
		}
		_ = entry
		result.Written = append(result.Written, ns)
	}
	// delete live allowlisted blades/shells not in manifest
	for ns := range liveSet {
		if !strings.HasPrefix(ns, "src/pages/") {
			continue
		}
		if _, ok := manifestSet[ns]; ok {
			continue
		}
		abs := filepath.Join(srcRoot, filepath.FromSlash(ns))
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return result, err
		}
		result.Deleted = append(result.Deleted, ns)
		// rmdir empty route dir
		if strings.HasSuffix(ns, "/[param1].astro") {
			_ = os.Remove(filepath.Dir(abs))
		}
	}

	// --- assets dir swap ---
	assetLive := AssetRoot(config.BackendPath, tenantID)
	assetTree := filepath.Join(treeRoot, "custom")
	if err := s.swapDirFromTree(assetTree, assetLive); err != nil {
		return result, fmt.Errorf("restore assets: %w", err)
	}
	// re-create probe marker
	probe := filepath.Join(assetLive, probeMarkerName(tenantID))
	if _, err := os.Stat(probe); os.IsNotExist(err) {
		_ = os.WriteFile(probe, nil, 0o644)
	}
	for ns := range manifestSet {
		if strings.HasPrefix(ns, nsAssetPrefix) {
			result.Written = append(result.Written, ns)
		}
	}
	for ns := range liveSet {
		if strings.HasPrefix(ns, nsAssetPrefix) {
			if _, ok := manifestSet[ns]; !ok {
				result.Deleted = append(result.Deleted, ns)
			}
		}
	}

	// --- custom.css via Put when present in manifest ---
	if entry, ok := manifestSet[nsCustomCSS]; ok {
		if s.cssService == nil {
			return result, ErrHistoryNoCustomCSS
		}
		b, err := os.ReadFile(filepath.Join(treeRoot, filepath.FromSlash(nsCustomCSS)))
		if err != nil {
			return result, err
		}
		if _, err := s.cssService.Put(tenantCtx, string(b)); err != nil {
			return result, err
		}
		_ = entry
		result.Written = append(result.Written, nsCustomCSS)
	}

	return result, nil
}

// swapDirFromTree builds live from treeDir (may be missing → empty dir), atomic rename swap.
func (s *ExtendHistoryService) swapDirFromTree(treeDir, liveDir string) error {
	parent := filepath.Dir(liveDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging := liveDir + ".restore-new"
	old := liveDir + ".restore-old"
	_ = os.RemoveAll(staging)
	_ = os.RemoveAll(old)

	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(treeDir); err == nil && info.IsDir() {
		if err := copyDirContents(treeDir, staging); err != nil {
			_ = os.RemoveAll(staging)
			return err
		}
	}

	liveExists := false
	if _, err := os.Stat(liveDir); err == nil {
		liveExists = true
		if err := os.Rename(liveDir, old); err != nil {
			_ = os.RemoveAll(staging)
			return err
		}
	} else if !os.IsNotExist(err) {
		_ = os.RemoveAll(staging)
		return err
	}
	if err := os.Rename(staging, liveDir); err != nil {
		if liveExists {
			_ = os.Rename(old, liveDir)
		}
		_ = os.RemoveAll(staging)
		return err
	}
	_ = os.RemoveAll(old)
	return nil
}

func copyDirContents(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		_, _, err = copyFileAtomic(path, target)
		return err
	})
}

// CurrentStamp returns the live build stamp for API responses.
func (s *ExtendHistoryService) CurrentStamp() BuildStamp {
	return ReadBuildStamp(config.BackendPath)
}
