// Package services provides application-level orchestration services
package services

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// ConfigService handles configuration management operations
type ConfigService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewConfigService creates a new configuration service
func NewConfigService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *ConfigService {
	return &ConfigService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// BrandConfigUpdateRequest holds the request structure for brand config updates
type BrandConfigUpdateRequest struct {
	// Brand Styling Fields
	BrandColours string `json:"BRAND_COLOURS,omitempty"`
	Theme        string `json:"THEME,omitempty"`

	// Site Configuration Fields
	SiteInit           bool   `json:"SITE_INIT,omitempty"`
	WordmarkMode       string `json:"WORDMARK_MODE,omitempty"`
	HomeSlug           string `json:"HOME_SLUG,omitempty"`
	TractStackHomeSlug string `json:"TRACTSTACK_HOME_SLUG,omitempty"`
	OpenDemo           bool   `json:"OPEN_DEMO,omitempty"`
	SiteURL            string `json:"SITE_URL,omitempty"`
	Slogan             string `json:"SLOGAN,omitempty"`
	Footer             string `json:"FOOTER,omitempty"`

	// SEO and Social Fields
	OGTitle  string `json:"OGTITLE,omitempty"`
	OGAuthor string `json:"OGAUTHOR,omitempty"`
	OGDesc   string `json:"OGDESC,omitempty"`
	GTag     string `json:"GTAG,omitempty"`
	Socials  string `json:"SOCIALS,omitempty"`

	// Existing Asset URL Fields (when not uploading new)
	Logo     string `json:"LOGO,omitempty"`
	Wordmark string `json:"WORDMARK,omitempty"`
	OG       string `json:"OG,omitempty"`
	OGLogo   string `json:"OGLOGO,omitempty"`
	Favicon  string `json:"FAVICON,omitempty"`

	// Base64 Upload Fields (new uploads)
	LogoBase64     string `json:"LOGO_BASE64,omitempty"`
	WordmarkBase64 string `json:"WORDMARK_BASE64,omitempty"`
	OGBase64       string `json:"OG_BASE64,omitempty"`
	OGLogoBase64   string `json:"OGLOGO_BASE64,omitempty"`
	FaviconBase64  string `json:"FAVICON_BASE64,omitempty"`

	// Version Tracking Field
	StylesVer int64 `json:"STYLES_VER,omitempty"`

	// Known Resources Field
	KnownResources *types.KnownResourcesConfig `json:"KNOWN_RESOURCES,omitempty"`
}

// AdvancedConfigUpdateRequest holds the request structure for advanced config updates
type AdvancedConfigUpdateRequest struct {
	TursoDatabaseURL string `json:"turso_database_url,omitempty"`
	TursoAuthToken   string `json:"turso_auth_token,omitempty"`
	EmailHost        string `json:"email_host,omitempty"`
	EmailPort        int    `json:"email_port,omitempty"`
	EmailUser        string `json:"email_user,omitempty"`
	EmailPass        string `json:"email_pass,omitempty"`
	EmailFrom        string `json:"email_from,omitempty"`
	AdminPassword    string `json:"admin_password,omitempty"`
	EditorPassword   string `json:"editor_password,omitempty"`
	AAIAPIKey        string `json:"aai_api_key,omitempty"`
	TursoEnabled     bool   `json:"turso_enabled,omitempty"`
}

// ValidateAdminPermissions validates admin-only authentication
func (c *ConfigService) ValidateAdminPermissions(authHeader string, tenantCtx *tenant.Context) error {
	if authHeader == "" {
		return fmt.Errorf("authorization header required")
	}

	// Extract token from Bearer header
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		return fmt.Errorf("invalid authorization format")
	}

	// Validate JWT token
	claims, err := security.ValidateJWT(token, tenantCtx.Config.JWTSecret)
	if err != nil {
		return fmt.Errorf("invalid JWT token: %w", err)
	}

	// Check role
	role, ok := claims["role"].(string)
	if !ok || role != "admin" {
		return fmt.Errorf("admin permissions required")
	}

	return nil
}

// ValidateEditorPermissions validates admin or editor authentication
func (c *ConfigService) ValidateEditorPermissions(authHeader string, tenantCtx *tenant.Context) error {
	if authHeader == "" {
		return fmt.Errorf("authorization header required")
	}

	// Extract token from Bearer header
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		return fmt.Errorf("invalid authorization format")
	}

	// Validate JWT token
	claims, err := security.ValidateJWT(token, tenantCtx.Config.JWTSecret)
	if err != nil {
		return fmt.Errorf("invalid JWT token: %w", err)
	}

	// Check role
	role, ok := claims["role"].(string)
	if !ok || (role != "admin" && role != "editor") {
		return fmt.Errorf("admin or editor permissions required")
	}

	return nil
}

// ProcessBrandConfigUpdate processes brand configuration updates including base64 assets
func (c *ConfigService) ProcessBrandConfigUpdate(
	mediaPath string,
	request *BrandConfigUpdateRequest,
	currentConfig *types.BrandConfig,
) (*types.BrandConfig, error) {
	updatedConfig := *currentConfig

	// Process base64 assets
	processedConfig, err := c.processBase64Assets(mediaPath, request, &updatedConfig)
	if err != nil {
		return nil, err
	}

	// Update configuration fields
	finalConfig := c.updateBrandConfigFields(processedConfig, request)

	// Update version tracking
	if request.StylesVer > 0 {
		finalConfig.StylesVer = request.StylesVer
	} else {
		finalConfig.StylesVer = time.Now().Unix()
	}

	// Always set SiteInit to true on update
	finalConfig.SiteInit = true

	return finalConfig, nil
}

// ProcessAdvancedConfigUpdate processes advanced configuration updates
func (c *ConfigService) ProcessAdvancedConfigUpdate(
	request *AdvancedConfigUpdateRequest,
	tenantCtx *tenant.Context,
) error {
	// Update fields from request directly on tenant config
	if request.TursoDatabaseURL != "" {
		tenantCtx.Config.TursoDatabase = request.TursoDatabaseURL
	}
	if request.TursoAuthToken != "" {
		tenantCtx.Config.TursoToken = request.TursoAuthToken
	}
	if request.AdminPassword != "" {
		tenantCtx.Config.AdminPassword = request.AdminPassword
	}
	if request.EditorPassword != "" {
		tenantCtx.Config.EditorPassword = request.EditorPassword
	}
	if request.AAIAPIKey != "" {
		tenantCtx.Config.AAIAPIKey = request.AAIAPIKey
	}
	tenantCtx.Config.TursoEnabled = request.TursoEnabled

	return nil
}

// TestTursoConnection tests connectivity to a Turso database
func (c *ConfigService) TestTursoConnection(databaseURL, authToken string) error {
	if databaseURL == "" || authToken == "" {
		return fmt.Errorf("database URL and auth token are required")
	}

	// Create connection string
	connectionString := fmt.Sprintf("%s?authToken=%s", databaseURL, authToken)

	// Test connection
	db, err := sql.Open("libsql", connectionString)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	// Test with simple query
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected test result: %d", result)
	}

	return nil
}

// SaveBrandConfig saves brand configuration to disk
func (c *ConfigService) SaveBrandConfig(tenantID string, config *types.BrandConfig) error {
	configPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantID)

	// Ensure config directory exists
	if err := os.MkdirAll(configPath, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Save knownResources separately
	if err := c.saveKnownResources(tenantID, config.KnownResources); err != nil {
		return err
	}

	// Create copy without KnownResources for brand.json
	brandConfigForFile := *config
	brandConfigForFile.KnownResources = nil

	// Write brand config
	brandPath := filepath.Join(configPath, "brand.json")
	data, err := json.MarshalIndent(brandConfigForFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal brand config: %w", err)
	}

	if err := os.WriteFile(brandPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write brand config: %w", err)
	}

	return nil
}

// SaveAdvancedConfig saves advanced configuration to disk
func (c *ConfigService) SaveAdvancedConfig(tenantCtx *tenant.Context) error {
	configPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantCtx.Config.TenantID, "env.json")

	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal and save configuration
	data, err := json.MarshalIndent(tenantCtx.Config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// Private helper methods

func (c *ConfigService) processBase64Assets(mediaPath string, request *BrandConfigUpdateRequest, currentConfig *types.BrandConfig) (*types.BrandConfig, error) {
	updatedConfig := *currentConfig

	// Process each base64 asset if provided
	if request.LogoBase64 != "" {
		filename, err := c.processBase64Image(mediaPath, request.LogoBase64, "logo", "images/brand")
		if err != nil {
			return nil, err
		}
		updatedConfig.Logo = filename
	}

	if request.WordmarkBase64 != "" {
		filename, err := c.processBase64Image(mediaPath, request.WordmarkBase64, "wordmark", "images/brand")
		if err != nil {
			return nil, err
		}
		updatedConfig.Wordmark = filename
	}

	if request.OGBase64 != "" {
		filename, err := c.processBase64Image(mediaPath, request.OGBase64, "og", "images/brand")
		if err != nil {
			return nil, err
		}
		updatedConfig.OG = filename
	}

	if request.OGLogoBase64 != "" {
		filename, err := c.processBase64Image(mediaPath, request.OGLogoBase64, "og-logo", "images/brand")
		if err != nil {
			return nil, err
		}
		updatedConfig.OGLogo = filename
	}

	if request.FaviconBase64 != "" {
		filename, err := c.processBase64Image(mediaPath, request.FaviconBase64, "favicon", "images/brand")
		if err != nil {
			return nil, err
		}
		updatedConfig.Favicon = filename
	}

	return &updatedConfig, nil
}

func (c *ConfigService) processBase64Image(basePath, data, filename, subdir string) (string, error) {
	if data == "" {
		return "", fmt.Errorf("empty base64 data")
	}

	// Extract file extension from MIME type
	ext := c.extractExtension(data)
	if ext == "" {
		return "", fmt.Errorf("unsupported image format")
	}

	// Construct filename with extension
	fullFilename := fmt.Sprintf("%s.%s", filename, ext)

	// Create target directory
	targetDir := filepath.Join(basePath, subdir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Route to appropriate processor based on format
	if strings.Contains(data, "image/svg+xml") {
		return c.processSVG(data, fullFilename, targetDir)
	} else {
		return c.processBinaryImage(data, fullFilename, targetDir)
	}
}

func (c *ConfigService) extractExtension(data string) string {
	if strings.Contains(data, "data:image/svg+xml") {
		return "svg"
	} else if strings.Contains(data, "data:image/png") {
		return "png"
	} else if strings.Contains(data, "data:image/jpeg") || strings.Contains(data, "data:image/jpg") {
		return "jpg"
	} else if strings.Contains(data, "data:image/x-icon") || strings.Contains(data, "data:image/vnd.microsoft.icon") {
		return "ico"
	} else if strings.Contains(data, "data:image/webp") {
		return "webp"
	}
	return "png" // Fallback
}

func (c *ConfigService) processSVG(data, filename, targetDir string) (string, error) {
	svgPattern := regexp.MustCompile(`^data:image/svg\+xml;base64,`)
	if !svgPattern.MatchString(data) {
		return "", fmt.Errorf("invalid SVG base64 format")
	}

	// Strip prefix and decode
	b64Data := svgPattern.ReplaceAllString(data, "")
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Write as UTF-8 text
	fullPath := filepath.Join(targetDir, filename)
	if err := os.WriteFile(fullPath, decoded, 0644); err != nil {
		return "", fmt.Errorf("failed to write SVG file: %w", err)
	}

	return "/media/" + filepath.Join("images/brand", filename), nil
}

func (c *ConfigService) processBinaryImage(data, filename, targetDir string) (string, error) {
	binaryPattern := regexp.MustCompile(`^data:image/\w+;base64,`)
	if !binaryPattern.MatchString(data) {
		return "", fmt.Errorf("invalid binary image base64 format")
	}

	// Strip prefix and decode
	b64Data := binaryPattern.ReplaceAllString(data, "")
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Write as binary
	fullPath := filepath.Join(targetDir, filename)
	if err := os.WriteFile(fullPath, decoded, 0644); err != nil {
		return "", fmt.Errorf("failed to write binary file: %w", err)
	}

	return "/media/" + filepath.Join("images/brand", filename), nil
}

func (c *ConfigService) updateBrandConfigFields(config *types.BrandConfig, request *BrandConfigUpdateRequest) *types.BrandConfig {
	if request.SiteURL != "" {
		config.SiteURL = request.SiteURL
	}
	if request.Slogan != "" {
		config.Slogan = request.Slogan
	}
	if request.BrandColours != "" {
		config.BrandColours = request.BrandColours
	}
	if request.Theme != "" {
		config.Theme = request.Theme
	}
	if request.WordmarkMode != "" {
		config.WordmarkMode = request.WordmarkMode
	}
	if request.HomeSlug != "" {
		config.HomeSlug = request.HomeSlug
	}
	if request.TractStackHomeSlug != "" {
		config.TractStackHomeSlug = request.TractStackHomeSlug
	}
	if request.Footer != "" {
		config.Footer = request.Footer
	}
	if request.OGTitle != "" {
		config.OGTitle = request.OGTitle
	}
	if request.OGAuthor != "" {
		config.OGAuthor = request.OGAuthor
	}
	if request.OGDesc != "" {
		config.OGDesc = request.OGDesc
	}
	if request.GTag != "" {
		config.Gtag = request.GTag
	}
	if request.Socials != "" {
		config.Socials = request.Socials
	}

	// Handle asset URLs only when not uploading new files
	if request.Logo != "" && request.LogoBase64 == "" {
		config.Logo = request.Logo
	}
	if request.Wordmark != "" && request.WordmarkBase64 == "" {
		config.Wordmark = request.Wordmark
	}
	if request.OG != "" && request.OGBase64 == "" {
		config.OG = request.OG
	}
	if request.OGLogo != "" && request.OGLogoBase64 == "" {
		config.OGLogo = request.OGLogo
	}
	if request.Favicon != "" && request.FaviconBase64 == "" {
		config.Favicon = request.Favicon
	}

	// Update boolean fields and known resources
	config.SiteInit = request.SiteInit
	config.OpenDemo = request.OpenDemo
	if request.KnownResources != nil {
		config.KnownResources = request.KnownResources
	}

	return config
}

func (c *ConfigService) saveKnownResources(tenantID string, knownResources *types.KnownResourcesConfig) error {
	if knownResources == nil {
		return nil // Nothing to save
	}

	configPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantID)

	// Ensure config directory exists
	if err := os.MkdirAll(configPath, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write known resources config
	knownResourcesPath := filepath.Join(configPath, "knownResources.json")
	data, err := json.MarshalIndent(knownResources, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal known resources config: %w", err)
	}

	if err := os.WriteFile(knownResourcesPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write known resources config: %w", err)
	}

	return nil
}
