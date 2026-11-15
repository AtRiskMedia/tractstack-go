package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

const customCSSMaxBytes = 2 << 20 // 2 MiB

var (
	ErrCustomCSSNotFound = errors.New("custom.css not found")
	ErrCustomCSSTooLarge = errors.New("custom.css too large")
	ErrCustomCSSNotText  = errors.New("custom.css is not valid UTF-8")
)

// CustomCSSPublicPath is the URL Layout links for an initialized tenant.
const CustomCSSPublicPath = "/media/css/custom.css"

// CustomCSSPath is GO_BACKEND_PATH/config/<tenant>/media/css/custom.css.
func CustomCSSPath(backendPath, tenantID string) string {
	if tenantID == "" {
		tenantID = "default"
	}
	return filepath.Join(backendPath, "config", tenantID, "media", "css", "custom.css")
}

// ExtendCustomCSSService reads and writes the tenant custom.css.
type ExtendCustomCSSService struct {
	configService *ConfigService
	logger        *logging.ChanneledLogger
	perfTracker   *performance.Tracker
	history       *ExtendHistoryService
}

// NewExtendCustomCSSService creates the custom.css service.
func NewExtendCustomCSSService(
	configService *ConfigService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *ExtendCustomCSSService {
	return &ExtendCustomCSSService{
		configService: configService,
		logger:        logger,
		perfTracker:   perfTracker,
	}
}

// SetHistory attaches design-history snapshots for writes.
func (s *ExtendCustomCSSService) SetHistory(h *ExtendHistoryService) {
	s.history = h
}

// Get returns custom.css content and mtime (RFC3339).
func (s *ExtendCustomCSSService) Get(tenantID string) (string, string, error) {
	path := CustomCSSPath(config.BackendPath, tenantID)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", ErrCustomCSSNotFound
		}
		return "", "", err
	}
	if info.IsDir() {
		return "", "", ErrCustomCSSNotFound
	}
	if info.Size() > customCSSMaxBytes {
		return "", "", ErrCustomCSSTooLarge
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if !utf8.Valid(b) {
		return "", "", ErrCustomCSSNotText
	}
	return string(b), info.ModTime().UTC().Format(time.RFC3339), nil
}

// Put writes custom.css atomically and bumps STYLES_VER so stylesheet URLs change.
func (s *ExtendCustomCSSService) Put(tenantCtx *tenant.Context, content string) (int64, error) {
	if tenantCtx == nil || tenantCtx.Config == nil || tenantCtx.Config.BrandConfig == nil {
		return 0, fmt.Errorf("brand configuration not loaded")
	}
	if len(content) > customCSSMaxBytes {
		return 0, ErrCustomCSSTooLarge
	}
	if !utf8.ValidString(content) {
		return 0, ErrCustomCSSNotText
	}

	path := CustomCSSPath(config.BackendPath, tenantCtx.TenantID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, fmt.Errorf("create css directory: %w", err)
	}
	if s.history != nil {
		_ = s.history.SnapshotBeforeWrite(tenantCtx.TenantID, HistoryNsCustomCSS(), path)
	}

	tmpPath := path + ".tmp"
	goodPath := path + ".good"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return 0, fmt.Errorf("write temp custom.css: %w", err)
	}

	hadOriginal := false
	if _, err := os.Stat(path); err == nil {
		hadOriginal = true
		if err := os.Rename(path, goodPath); err != nil {
			_ = os.Remove(tmpPath)
			return 0, fmt.Errorf("backup custom.css: %w", err)
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if hadOriginal {
			_ = os.Rename(goodPath, path)
		}
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("activate custom.css: %w", err)
	}

	newVersion := time.Now().Unix()
	updated := *tenantCtx.Config.BrandConfig
	updated.StylesVer = newVersion
	if err := s.configService.SaveBrandConfig(tenantCtx.TenantID, &updated); err != nil {
		return 0, fmt.Errorf("update STYLES_VER: %w", err)
	}
	tenantCtx.Config.BrandConfig = &updated

	return newVersion, nil
}
