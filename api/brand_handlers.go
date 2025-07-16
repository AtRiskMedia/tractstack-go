// Package api provides brand configuration handlers
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
	"github.com/AtRiskMedia/tractstack-go/utils/images"
	"github.com/gin-gonic/gin"
)

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
}

// GetBrandConfigHandler returns tenant brand configuration
func GetBrandConfigHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if ctx.Config.BrandConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "brand configuration not loaded"})
		return
	}

	c.JSON(http.StatusOK, ctx.Config.BrandConfig)
}

// UpdateBrandConfigHandler handles PUT requests to update brand configuration
func UpdateBrandConfigHandler(c *gin.Context) {
	// 1. Admin Authentication
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Try admin cookie first
	adminCookie, err := c.Cookie("admin_auth")
	if err == nil {
		if claims, err := utils.ValidateJWT(adminCookie, ctx.Config.JWTSecret); err == nil {
			if role, ok := claims["role"].(string); ok && role == "admin" {
				// Admin authenticated - continue
			} else {
				c.JSON(http.StatusForbidden, gin.H{"error": "Admin or editor access required"})
				return
			}
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
			return
		}
	} else {
		// Try editor cookie
		editorCookie, err := c.Cookie("editor_auth")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Admin or editor authentication required"})
			return
		}
		if claims, err := utils.ValidateJWT(editorCookie, ctx.Config.JWTSecret); err == nil {
			if role, ok := claims["role"].(string); ok && role == "editor" {
				// Editor authenticated - continue
			} else {
				c.JSON(http.StatusForbidden, gin.H{"error": "Admin or editor access required"})
				return
			}
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
			return
		}
	}

	// 2. Request Parsing
	var request BrandConfigUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// 3. Image Processor Initialization
	mediaPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", ctx.Config.TenantID, "media")
	processor := images.NewImageProcessor(mediaPath)

	// 4. Base64 Image Processing
	currentConfig := ctx.Config.BrandConfig
	if currentConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Brand configuration not loaded"})
		return
	}

	// Process base64 assets
	updatedConfig, err := processBase64Assets(processor, &request, currentConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Image processing failed: %v", err)})
		return
	}

	// 5. Configuration Update
	finalConfig := updateBrandConfigFields(updatedConfig, &request)

	// 6. Configuration Save
	if err := saveBrandConfig(ctx.Config.TenantID, finalConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save configuration: %v", err)})
		return
	}

	// Update in-memory config
	ctx.Config.BrandConfig = finalConfig

	// 7. Response
	c.JSON(http.StatusOK, finalConfig)
}

// processBase64Assets processes uploaded base64 images and returns updated config
func processBase64Assets(processor *images.ImageProcessor, request *BrandConfigUpdateRequest, currentConfig *tenant.BrandConfig) (*tenant.BrandConfig, error) {
	// Create a copy of current config
	config := *currentConfig

	// Process direct save assets (logo, wordmark, favicon)
	if request.LogoBase64 != "" {
		_, err := processor.ProcessBase64Image(request.LogoBase64, "logo", "images/brand")
		if err != nil {
			return nil, fmt.Errorf("failed to process logo: %w", err)
		}
		config.Logo = "/media/images/brand/logo" + getExtensionFromBase64(request.LogoBase64)
	}

	if request.WordmarkBase64 != "" {
		_, err := processor.ProcessBase64Image(request.WordmarkBase64, "wordmark", "images/brand")
		if err != nil {
			return nil, fmt.Errorf("failed to process wordmark: %w", err)
		}
		config.Wordmark = "/media/images/brand/wordmark" + getExtensionFromBase64(request.WordmarkBase64)
	}

	if request.FaviconBase64 != "" {
		_, err := processor.ProcessBase64Image(request.FaviconBase64, "favicon", "images/brand")
		if err != nil {
			return nil, fmt.Errorf("failed to process favicon: %w", err)
		}
		config.Favicon = "/media/images/brand/favicon" + getExtensionFromBase64(request.FaviconBase64)
	}

	// Process versioned assets (og, oglogo)
	if request.OGBase64 != "" {
		// Get current version (if any) - we'll use STYLES_VER as a proxy since there's no separate OG version
		currentVersion := config.StylesVer
		relativePath, newVersion, err := processor.ProcessVersionedImage(request.OGBase64, "og", "images/brand", currentVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to process OG image: %w", err)
		}
		config.OG = relativePath
		// Update version for cache busting
		config.StylesVer = newVersion
	}

	if request.OGLogoBase64 != "" {
		// Get current version
		currentVersion := config.StylesVer
		relativePath, newVersion, err := processor.ProcessVersionedImage(request.OGLogoBase64, "oglogo", "images/brand", currentVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to process OG logo: %w", err)
		}
		config.OGLogo = relativePath
		// Update version for cache busting
		config.StylesVer = newVersion
	}

	return &config, nil
}

// updateBrandConfigFields updates all non-empty text fields from request
func updateBrandConfigFields(config *tenant.BrandConfig, request *BrandConfigUpdateRequest) *tenant.BrandConfig {
	// Update brand styling fields
	if request.BrandColours != "" {
		config.BrandColours = request.BrandColours
	}
	if request.Theme != "" {
		config.Theme = request.Theme
	}

	// Update site configuration fields
	if request.WordmarkMode != "" {
		config.WordmarkMode = request.WordmarkMode
	}
	if request.HomeSlug != "" {
		config.HomeSlug = request.HomeSlug
	}
	if request.TractStackHomeSlug != "" {
		config.TractStackHomeSlug = request.TractStackHomeSlug
	}
	if request.SiteURL != "" {
		config.SiteURL = request.SiteURL
	}
	if request.Slogan != "" {
		config.Slogan = request.Slogan
	}
	if request.Footer != "" {
		config.Footer = request.Footer
	}

	// Update SEO and social fields
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

	// Update asset URLs only when not uploading new files
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

	// Update version tracking (CSS cache busting)
	if request.StylesVer > 0 {
		config.StylesVer = request.StylesVer
	}

	// Update boolean fields
	config.SiteInit = request.SiteInit
	config.OpenDemo = request.OpenDemo

	return config
}

// saveBrandConfig saves the brand configuration to disk
func saveBrandConfig(tenantID string, config *tenant.BrandConfig) error {
	configPath := filepath.Join(os.Getenv("HOME"), "t8k-go-server", "config", tenantID)

	// Ensure config directory exists
	if err := os.MkdirAll(configPath, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write brand config
	brandPath := filepath.Join(configPath, "brand.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal brand config: %w", err)
	}

	if err := os.WriteFile(brandPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write brand config: %w", err)
	}

	return nil
}

// getExtensionFromBase64 extracts file extension from base64 data URI
func getExtensionFromBase64(data string) string {
	if data == "" {
		return ""
	}

	if data[:25] == "data:image/svg+xml;base64" {
		return ".svg"
	} else if data[:22] == "data:image/png;base64" {
		return ".png"
	} else if data[:23] == "data:image/jpeg;base64" || data[:22] == "data:image/jpg;base64" {
		return ".jpg"
	} else if data[:23] == "data:image/x-icon;base64" || data[:35] == "data:image/vnd.microsoft.icon;base64" {
		return ".ico"
	} else if data[:23] == "data:image/webp;base64" {
		return ".webp"
	}

	return ".png" // fallback
}
