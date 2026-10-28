package services

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

const (
	assetMaxDepth         = 12
	assetMaxNameLen       = 128
	assetMaxUploadBytes   = 512 << 20 // 512 MiB
	assetMaxUnzipBytes    = 2 << 30   // 2 GiB
	assetMaxUnzipEntries  = 20000
	assetMaxTreeEntries   = 20000
	assetScanMaxFileBytes = 2 << 20 // 2 MiB
)

var (
	assetSegmentRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

	assetExtBlocklist = map[string]struct{}{
		".astro": {}, ".ts": {}, ".tsx": {}, ".jsx": {},
		".mjs": {}, ".cjs": {}, ".php": {}, ".sh": {},
	}

	assetTextExts = map[string]struct{}{
		".js": {}, ".css": {}, ".json": {}, ".txt": {},
		".md": {}, ".html": {}, ".svg": {}, ".csv": {},
	}

	ErrAssetNotFound      = errors.New("asset not found")
	ErrAssetNotText       = errors.New("not a text asset")
	ErrAssetInvalidPath   = errors.New("invalid asset path")
	ErrAssetForbidden     = errors.New("operation forbidden on this path")
	ErrAssetExists        = errors.New("asset already exists")
	ErrAssetTooLarge      = errors.New("payload too large")
	ErrAssetInUse         = errors.New("asset is referenced")
	ErrAssetTreeTooLarge  = errors.New("asset tree exceeds limit")
	ErrAssetUnzipRejected = errors.New("unzip rejected")
)

// AssetNode is one entry in GET /extend/assets/tree.
type AssetNode struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
	IsDir bool   `json:"isDir"`
}

// AssetReference is one hit from the section-7 asset reference scan.
type AssetReference struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// AssetInUseError carries 409 reference rows.
type AssetInUseError struct {
	References []AssetReference
}

func (e *AssetInUseError) Error() string {
	return ErrAssetInUse.Error()
}

func (e *AssetInUseError) Unwrap() error {
	return ErrAssetInUse
}

// ExtendAssetService manages files under config/<tenant>/custom.
type ExtendAssetService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewExtendAssetService creates the asset manager singleton.
func NewExtendAssetService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ExtendAssetService {
	return &ExtendAssetService{logger: logger, perfTracker: perfTracker}
}

func probeMarkerName(tenantID string) string {
	return "t8k-probe-" + tenantID
}

func (s *ExtendAssetService) root(tenantID string) string {
	return AssetRoot(config.BackendPath, tenantID)
}

func (s *ExtendAssetService) ensureRoot(tenantID string) (string, error) {
	root := s.root(tenantID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create asset root: %w", err)
	}
	marker := filepath.Join(root, probeMarkerName(tenantID))
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		if err := os.WriteFile(marker, nil, 0o644); err != nil {
			return "", fmt.Errorf("write probe marker: %w", err)
		}
	} else if err != nil {
		return "", err
	}
	return root, nil
}

func validateAssetRelPath(rel string) error {
	if rel == "" || rel == "." {
		return fmt.Errorf("%w: empty path", ErrAssetInvalidPath)
	}
	if filepath.IsAbs(rel) {
		return fmt.Errorf("%w: absolute path", ErrAssetInvalidPath)
	}
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%w: path escape", ErrAssetInvalidPath)
	}
	parts := strings.Split(cleaned, "/")
	if len(parts) > assetMaxDepth {
		return fmt.Errorf("%w: max depth %d", ErrAssetInvalidPath, assetMaxDepth)
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return fmt.Errorf("%w: bad segment", ErrAssetInvalidPath)
		}
		if strings.HasPrefix(p, ".") {
			return fmt.Errorf("%w: segment must not start with .", ErrAssetInvalidPath)
		}
		if len(p) > assetMaxNameLen {
			return fmt.Errorf("%w: name too long", ErrAssetInvalidPath)
		}
		if !assetSegmentRe.MatchString(p) {
			return fmt.Errorf("%w: invalid segment %q", ErrAssetInvalidPath, p)
		}
	}
	ext := strings.ToLower(filepath.Ext(cleaned))
	if ext != "" {
		if _, blocked := assetExtBlocklist[ext]; blocked {
			return fmt.Errorf("%w: extension %s blocked", ErrAssetInvalidPath, ext)
		}
	}
	return nil
}

func isTextAsset(rel string) bool {
	_, ok := assetTextExts[strings.ToLower(filepath.Ext(rel))]
	return ok
}

func (s *ExtendAssetService) resolve(tenantID, rel string) (root, abs string, err error) {
	if err := validateAssetRelPath(rel); err != nil {
		return "", "", err
	}
	root, err = s.ensureRoot(tenantID)
	if err != nil {
		return "", "", err
	}
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	if cleaned == probeMarkerName(tenantID) {
		return root, "", fmt.Errorf("%w: probe marker", ErrAssetForbidden)
	}
	abs, err = ResolveUnderRoot(root, cleaned)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrAssetInvalidPath, err)
	}
	return root, abs, nil
}

// Tree lists the asset tree, omitting the probe marker.
func (s *ExtendAssetService) Tree(tenantID string) ([]AssetNode, error) {
	root, err := s.ensureRoot(tenantID)
	if err != nil {
		return nil, err
	}
	probe := probeMarkerName(tenantID)
	var nodes []AssetNode
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == probe || strings.HasPrefix(relSlash, probe+"/") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		nodes = append(nodes, AssetNode{
			Name:  d.Name(),
			Path:  relSlash,
			Size:  info.Size(),
			Mtime: info.ModTime().UTC().Format(time.RFC3339),
			IsDir: d.IsDir(),
		})
		if len(nodes) > assetMaxTreeEntries {
			return ErrAssetTreeTooLarge
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

// GetFile returns text content for a text asset.
func (s *ExtendAssetService) GetFile(tenantID, rel string) (string, error) {
	_, abs, err := s.resolve(tenantID, rel)
	if err != nil {
		return "", err
	}
	if !isTextAsset(rel) {
		return "", ErrAssetNotText
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrAssetNotFound
		}
		return "", err
	}
	if info.IsDir() {
		return "", ErrAssetNotFound
	}
	if info.Size() > assetScanMaxFileBytes {
		return "", fmt.Errorf("%w: file exceeds read limit", ErrAssetTooLarge)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", ErrAssetNotText
	}
	return string(b), nil
}

// PutFile writes text content (creates parents).
func (s *ExtendAssetService) PutFile(tenantID, rel, content string) error {
	_, abs, err := s.resolve(tenantID, rel)
	if err != nil {
		return err
	}
	if !isTextAsset(rel) {
		return ErrAssetNotText
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

// Mkdir creates a directory under the asset root.
func (s *ExtendAssetService) Mkdir(tenantID, rel string) error {
	_, abs, err := s.resolve(tenantID, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	return nil
}

// Rename moves from → to under the asset root.
func (s *ExtendAssetService) Rename(tenantID, from, to string) error {
	_, fromAbs, err := s.resolve(tenantID, from)
	if err != nil {
		return err
	}
	_, toAbs, err := s.resolve(tenantID, to)
	if err != nil {
		return err
	}
	if _, err := os.Stat(fromAbs); err != nil {
		if os.IsNotExist(err) {
			return ErrAssetNotFound
		}
		return err
	}
	if _, err := os.Stat(toAbs); err == nil {
		return ErrAssetExists
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		return err
	}
	return os.Rename(fromAbs, toAbs)
}

// Delete removes a file or directory after the reference scan unless force=1.
// Directory deletes never accept force when referenced.
func (s *ExtendAssetService) Delete(tenantID, rel string, force bool) error {
	_, abs, err := s.resolve(tenantID, rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrAssetNotFound
		}
		return err
	}
	refs, err := s.FindReferences(tenantID, rel)
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		if info.IsDir() || !force {
			return &AssetInUseError{References: refs}
		}
	}
	if info.IsDir() {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("%w: directory not empty", ErrAssetForbidden)
		}
		return os.Remove(abs)
	}
	return os.Remove(abs)
}

// FindReferences scans allowlisted source for /custom/<rel> (or /custom/<dir>/).
func (s *ExtendAssetService) FindReferences(tenantID, rel string) ([]AssetReference, error) {
	if err := validateAssetRelPath(rel); err != nil {
		return nil, err
	}
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	needle := "/custom/" + cleaned
	root, err := s.ensureRoot(tenantID)
	if err != nil {
		return nil, err
	}
	abs, err := ResolveUnderRoot(root, cleaned)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAssetInvalidPath, err)
	}
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		needle = "/custom/" + cleaned + "/"
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	} else if strings.HasSuffix(cleaned, "/") {
		needle = "/custom/" + strings.TrimSuffix(cleaned, "/") + "/"
	}

	sourceRoot := SourceRoot(config.BackendPath)
	if !ExtendEnabled(config.BackendPath) {
		return nil, nil
	}
	var refs []AssetReference
	scanFile := func(absPath, relFromSource string) error {
		info, err := os.Stat(absPath)
		if err != nil {
			return err
		}
		if info.IsDir() || info.Size() > assetScanMaxFileBytes {
			return nil
		}
		f, err := os.Open(absPath)
		if err != nil {
			return err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, assetScanMaxFileBytes)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if !utf8.ValidString(line) {
				continue
			}
			if strings.Contains(line, needle) {
				refs = append(refs, AssetReference{
					File: filepath.ToSlash(relFromSource),
					Line: lineNo,
					Text: strings.TrimSpace(line),
				})
			}
		}
		return scanner.Err()
	}

	customRoot := filepath.Join(sourceRoot, "src", "custom")
	if err := walkSourceFiles(customRoot, "src/custom", scanFile); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	codehooksRoot := filepath.Join(sourceRoot, "src", "pages", "codehooks")
	if err := walkSourceFiles(codehooksRoot, "src/pages/codehooks", scanFile); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	pagesRoot := filepath.Join(sourceRoot, "src", "pages")
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
		dirAbs := filepath.Join(pagesRoot, name)
		relPrefix := filepath.ToSlash(filepath.Join("src/pages", name))
		if err := walkSourceFiles(dirAbs, relPrefix, scanFile); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func walkSourceFiles(absRoot, relPrefix string, fn func(abs, rel string) error) error {
	return filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}
		return fn(path, filepath.ToSlash(filepath.Join(relPrefix, rel)))
	})
}

// multipartRelPath returns the Content-Disposition filename as sent by the client.
// mime/multipart.Part.FileName strips directories (RFC 7578); folder uploads need the
// full relative path (webkitRelativePath), so parse the header without Base().
func multipartRelPath(part *multipart.Part) string {
	v := part.Header.Get("Content-Disposition")
	if v == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(v)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// UploadFromMultipart reads all file parts; each part filename is the relative path.
func (s *ExtendAssetService) UploadFromMultipart(tenantID string, mr *multipart.Reader) error {
	if _, err := s.ensureRoot(tenantID); err != nil {
		return err
	}
	remaining := int64(assetMaxUploadBytes)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := multipartRelPath(part)
		if name == "" {
			_ = part.Close()
			continue
		}
		rel := filepath.ToSlash(name)
		if err := s.streamUpload(tenantID, rel, part, &remaining); err != nil {
			_ = part.Close()
			return err
		}
		_ = part.Close()
	}
	return nil
}

func (s *ExtendAssetService) streamUpload(tenantID, rel string, r io.Reader, remaining *int64) error {
	_, abs, err := s.resolve(tenantID, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".upload-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()

	var written int64
	buf := make([]byte, 32*1024)
	for {
		nr, readErr := r.Read(buf)
		if nr > 0 {
			if written+int64(nr) > *remaining {
				return ErrAssetTooLarge
			}
			nw, werr := tmp.Write(buf[:nr])
			written += int64(nw)
			if werr != nil {
				return werr
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	*remaining -= written
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return err
	}
	ok = true
	return nil
}

// Unzip expands an already-uploaded zip at path into target.
func (s *ExtendAssetService) Unzip(tenantID, zipRel, targetRel string, keep bool) error {
	_, zipAbs, err := s.resolve(tenantID, zipRel)
	if err != nil {
		return err
	}
	root, err := s.ensureRoot(tenantID)
	if err != nil {
		return err
	}
	targetClean := filepath.ToSlash(filepath.Clean(targetRel))
	if targetRel == "" || targetRel == "." {
		targetClean = "."
	} else {
		if err := validateAssetRelPath(targetRel); err != nil {
			return err
		}
	}
	var targetAbs string
	if targetClean == "." {
		targetAbs = root
	} else {
		_, targetAbs, err = s.resolve(tenantID, targetClean)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(targetAbs, 0o755); err != nil {
			return err
		}
	}

	zr, err := zip.OpenReader(zipAbs)
	if err != nil {
		return err
	}
	defer zr.Close()

	var totalUncompressed int64
	if len(zr.File) > assetMaxUnzipEntries {
		return fmt.Errorf("%w: too many entries", ErrAssetUnzipRejected)
	}
	for _, f := range zr.File {
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink entry", ErrAssetUnzipRejected)
		}
		name := filepath.ToSlash(f.Name)
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		cleaned := filepath.Clean(name)
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || filepath.IsAbs(cleaned) {
			return fmt.Errorf("%w: zip-slip", ErrAssetUnzipRejected)
		}
		totalUncompressed += int64(f.UncompressedSize64)
		if totalUncompressed > assetMaxUnzipBytes {
			return fmt.Errorf("%w: uncompressed size", ErrAssetUnzipRejected)
		}
		destRel := cleaned
		if targetClean != "." {
			destRel = filepath.ToSlash(filepath.Join(targetClean, cleaned))
		}
		if err := validateAssetRelPath(destRel); err != nil {
			return fmt.Errorf("%w: %v", ErrAssetUnzipRejected, err)
		}
		destAbs, err := ResolveUnderRoot(root, destRel)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAssetUnzipRejected, err)
		}
		// Ensure dest stays under targetAbs
		relToTarget, err := filepath.Rel(targetAbs, destAbs)
		if err != nil || relToTarget == ".." || strings.HasPrefix(relToTarget, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: zip-slip target", ErrAssetUnzipRejected)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destAbs, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(destAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, assetMaxUnzipBytes+1))
		_ = out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	if !keep {
		_ = os.Remove(zipAbs)
	}
	return nil
}
