// Package types defines configuration cache data structures
package types

import (
	"sync"
	"time"
)

// TenantConfigCache stores configuration data for a tenant
type TenantConfigCache struct {
	// Brand configuration
	BrandConfig            *BrandConfig `json:"brandConfig"`
	BrandConfigLastUpdated time.Time    `json:"brandConfigLastUpdated"`

	// Advanced configuration
	AdvancedConfig            *AdvancedConfig `json:"advancedConfig"`
	AdvancedConfigLastUpdated time.Time       `json:"advancedConfigLastUpdated"`

	// Cache metadata
	LastUpdated time.Time    `json:"lastUpdated"`
	Mu          sync.RWMutex `json:"-"`
}

// TemplateNode represents a node within a template structure configuration.
type TemplateNode struct {
	NodeType        string          `json:"nodeType"`
	TagName         string          `json:"tagName,omitempty"`
	Copy            string          `json:"copy,omitempty"`
	Href            string          `json:"href,omitempty"`
	Src             string          `json:"src,omitempty"`
	Alt             string          `json:"alt,omitempty"`
	ButtonPayload   string          `json:"buttonPayload,omitempty"`
	CodeHookParams  []string        `json:"codeHookParams,omitempty"`
	OverrideClasses []string        `json:"overrideClasses,omitempty"`
	Nodes           []*TemplateNode `json:"nodes,omitempty"`
}

// TemplateMarkdown represents markdown content configuration within a template.
type TemplateMarkdown struct {
	NodeType string          `json:"nodeType"`
	Nodes    []*TemplateNode `json:"nodes"`
}

// TemplatePane represents a pane configuration within a template.
type TemplatePane struct {
	NodeType      string            `json:"nodeType"`
	Title         string            `json:"title"`
	Slug          string            `json:"slug,omitempty"`
	IsContextPane bool              `json:"isContextPane,omitempty"`
	Markdown      *TemplateMarkdown `json:"markdown,omitempty"`
}

// StorageButtonPayload defines the data structure for button interactions in storage.
type StorageButtonPayload struct {
	ButtonClasses      map[string][]string `json:"buttonClasses"`
	ButtonHoverClasses map[string][]string `json:"buttonHoverClasses"`
	CallbackPayload    string              `json:"callbackPayload"`
}

// StorageNode represents a content node as stored in the persistence layer.
type StorageNode struct {
	NodeType        string                       `json:"nodeType"`
	TagName         string                       `json:"tagName,omitempty"`
	Copy            string                       `json:"copy,omitempty"`
	Href            string                       `json:"href,omitempty"`
	Src             string                       `json:"src,omitempty"`
	Alt             string                       `json:"alt,omitempty"`
	ButtonPayload   *StorageButtonPayload        `json:"buttonPayload,omitempty"`
	CodeHookParams  []any                        `json:"codeHookParams,omitempty"`
	OverrideClasses map[string]map[string]string `json:"overrideClasses,omitempty"`
	Nodes           []*StorageNode               `json:"nodes,omitempty"`
}

// StorageMarkdown represents markdown content as stored in the persistence layer.
type StorageMarkdown struct {
	NodeType       string           `json:"nodeType"`
	Type           string           `json:"type"`
	DefaultClasses map[string]any   `json:"defaultClasses"`
	ParentClasses  []map[string]any `json:"parentClasses"`
	Nodes          []*StorageNode   `json:"nodes"`
}

// GridColumns defines the column count for grid layouts at different breakpoints.
type GridColumns struct {
	Mobile  int `json:"mobile"`
	Tablet  int `json:"tablet"`
	Desktop int `json:"desktop"`
}

// StorageGridLayoutNode represents a grid layout configuration stored in the database.
type StorageGridLayoutNode struct {
	NodeType       string            `json:"nodeType"`
	Type           string            `json:"type"`
	DefaultClasses map[string]any    `json:"defaultClasses"`
	ParentClasses  []map[string]any  `json:"parentClasses"`
	GridColumns    GridColumns       `json:"gridColumns"`
	Nodes          []StorageMarkdown `json:"nodes,omitempty"`
}

// VisualBreakData contains configuration for a visual break element.
type VisualBreakData struct {
	Collection string `json:"collection"`
	Image      string `json:"image"`
	SvgFill    string `json:"svgFill"`
}

// StorageBgPane represents a background pane configuration stored in the database.
type StorageBgPane struct {
	NodeType              string           `json:"nodeType"`
	Type                  string           `json:"type"`
	Collection            string           `json:"collection,omitempty"`
	Image                 string           `json:"image,omitempty"`
	Src                   string           `json:"src,omitempty"`
	SrcSet                string           `json:"srcSet,omitempty"`
	Alt                   string           `json:"alt,omitempty"`
	FileID                string           `json:"fileId,omitempty"`
	ObjectFit             string           `json:"objectFit,omitempty"`
	Position              string           `json:"position,omitempty"`
	Size                  string           `json:"size,omitempty"`
	BreakDesktop          *VisualBreakData `json:"breakDesktop,omitempty"`
	BreakTablet           *VisualBreakData `json:"breakTablet,omitempty"`
	BreakMobile           *VisualBreakData `json:"breakMobile,omitempty"`
	HiddenViewportDesktop bool             `json:"hiddenViewportDesktop,omitempty"`
	HiddenViewportTablet  bool             `json:"hiddenViewportTablet,omitempty"`
	HiddenViewportMobile  bool             `json:"hiddenViewportMobile,omitempty"`
}

// StoragePane represents a pane configuration stored in the database.
type StoragePane struct {
	NodeType     string                 `json:"nodeType"`
	Title        string                 `json:"title"`
	Slug         string                 `json:"slug,omitempty"`
	BgColour     string                 `json:"bgColour,omitempty"`
	IsDecorative bool                   `json:"isDecorative"`
	Markdowns    []*StorageMarkdown     `json:"markdowns,omitempty"`
	GridLayout   *StorageGridLayoutNode `json:"gridLayout,omitempty"`
	BgPane       *StorageBgPane         `json:"bgPane,omitempty"`
}

// DesignLibraryEntry represents a reusable design template in the library.
type DesignLibraryEntry struct {
	Category      string      `json:"category"`
	Title         string      `json:"title"`
	MarkdownCount int         `json:"markdownCount"`
	Retain        bool        `json:"retain,omitempty"`
	Locked        bool        `json:"locked,omitempty"`
	Template      StoragePane `json:"template"`
}

// DesignLibraryConfig is a collection of design library entries.
type DesignLibraryConfig []DesignLibraryEntry

// KnownResourcesConfig holds resource category definitions
type KnownResourcesConfig map[string]map[string]FieldDefinition

// FieldDefinition represents a field definition in known resources
type FieldDefinition struct {
	Type              string `json:"type"`
	Optional          bool   `json:"optional"`
	DefaultValue      any    `json:"defaultValue,omitempty"`
	BelongsToCategory string `json:"belongsToCategory,omitempty"`
	MinNumber         *int   `json:"minNumber,omitempty"`
	MaxNumber         *int   `json:"maxNumber,omitempty"`
}

// TimeBlock represents a specific window of time for availability calculations.
// For BusinessHours, Start/End are time strings (e.g., "09:00", "17:00").
// For UnavailableHours, Start/End are full RFC3339 date-time strings (e.g., "2026-12-25T09:00:00Z").
type TimeBlock struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Scheduling defines the availability and rules for the native booking engine.
type Scheduling struct {
	Timezone          string               `json:"timezone"`
	BufferGapsMinutes int                  `json:"bufferGapsMinutes"`
	MaxLengthMinutes  int                  `json:"maxLengthMinutes"`
	AllowRemote       bool                 `json:"allowRemote"`
	RemoteOnly        bool                 `json:"remoteOnly"`
	BusinessHours     map[string]TimeBlock `json:"businessHours"`
	UnavailableHours  []TimeBlock          `json:"unavailableHours"`
}

// BrandConfig holds tenant-specific branding configuration
type BrandConfig struct {
	SiteInit           bool                  `json:"SITE_INIT"`
	WordmarkMode       string                `json:"WORDMARK_MODE"`
	OpenDemo           bool                  `json:"OPEN_DEMO"`
	HomeSlug           string                `json:"HOME_SLUG"`
	TractStackHomeSlug string                `json:"TRACTSTACK_HOME_SLUG"`
	Theme              string                `json:"THEME"`
	BrandColours       string                `json:"BRAND_COLOURS"`
	Socials            string                `json:"SOCIALS"`
	SiteURL            string                `json:"SITE_URL"`
	Slogan             string                `json:"SLOGAN"`
	Footer             string                `json:"FOOTER"`
	OGTitle            string                `json:"OGTITLE"`
	OGAuthor           string                `json:"OGAUTHOR"`
	OGDesc             string                `json:"OGDESC"`
	Gtag               string                `json:"GTAG"`
	StylesVer          int64                 `json:"STYLES_VER"`
	Logo               string                `json:"LOGO"`
	Wordmark           string                `json:"WORDMARK"`
	Favicon            string                `json:"FAVICON"`
	OG                 string                `json:"OG"`
	OGLogo             string                `json:"OGLOGO"`
	LogoBase64         string                `json:"LOGO_BASE64,omitempty"`
	WordmarkBase64     string                `json:"WORDMARK_BASE64,omitempty"`
	OGBase64           string                `json:"OG_BASE64,omitempty"`
	OGLogoBase64       string                `json:"OGLOGO_BASE64,omitempty"`
	FaviconBase64      string                `json:"FAVICON_BASE64,omitempty"`
	KnownResources     *KnownResourcesConfig `json:"KNOWN_RESOURCES,omitempty"`
	HasAAI             bool                  `json:"HAS_AAI"`
	HasShopify         bool                  `json:"HAS_SHOPIFY"`
	ShowShopifyHelper  bool                  `json:"SHOW_SHOPIFY_HELPER"`
	HasResend          bool                  `json:"HAS_RESEND"`
	HasHydrationToken  bool                  `json:"HAS_HYDRATION_TOKEN"`
	DesignLibrary      *DesignLibraryConfig  `json:"DESIGN_LIBRARY,omitempty"`
	Scheduling         Scheduling            `json:"SCHEDULING"`
	AdminEmail         string                `json:"ADMIN_EMAIL"`
}

// AdvancedConfig represents advanced configuration from main.go
type AdvancedConfig struct {
	// Multi-tenant settings
	MultiTenant    bool   `json:"multiTenant"`
	HydrationToken string `json:"hydrationToken,omitempty"`

	// Database settings
	DatabaseMode string `json:"databaseMode"` // "sqlite" or "turso"
	TursoURL     string `json:"tursoUrl,omitempty"`
	TursoToken   string `json:"tursoToken,omitempty"`

	// Server settings
	Port           string `json:"port"`
	AllowedOrigins string `json:"allowedOrigins"`
	CORSMaxAge     int    `json:"corsMaxAge"`

	// Feature flags
	EnableAnalytics bool `json:"enableAnalytics"`
	EnableSSE       bool `json:"enableSSE"`
	DebugMode       bool `json:"debugMode"`

	// Performance settings
	CacheSize      int `json:"cacheSize"`
	RequestTimeout int `json:"requestTimeout"`
	MaxConnections int `json:"maxConnections"`

	// Security settings
	JWTSecret          string `json:"jwtSecret,omitempty"`
	SessionTimeout     int    `json:"sessionTimeout"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute"`

	// External service settings
	EmailProvider string `json:"emailProvider,omitempty"`
	EmailAPIKey   string `json:"emailApiKey,omitempty"`
	CDNBaseURL    string `json:"cdnBaseUrl,omitempty"`
	MediaPath     string `json:"mediaPath"`

	// Monitoring settings
	EnableMetrics   bool   `json:"enableMetrics"`
	MetricsEndpoint string `json:"metricsEndpoint,omitempty"`
	LogLevel        string `json:"logLevel"`

	// Backup settings
	BackupEnabled   bool `json:"backupEnabled"`
	BackupInterval  int  `json:"backupInterval"`
	BackupRetention int  `json:"backupRetention"`
}
