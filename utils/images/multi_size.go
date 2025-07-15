// Package images provides multi-size image generation for responsive images
package images

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/oklog/ulid/v2"
)

// MultiSizeConfig holds configuration for multi-size generation
type MultiSizeConfig struct {
	Widths  []int  // Target widths for responsive images
	Quality int    // JPEG/WebP quality (1-100)
	Format  string // Output format: "webp", "jpeg", "png"
}

// MultiSizeResult holds the results of multi-size generation
type MultiSizeResult struct {
	MainPath string   // Path to the largest/main image
	SrcSet   string   // Complete srcSet string for HTML
	Paths    []string // All generated file paths
}

// Predefined configurations matching V1 patterns
var (
	// ContentImageConfig for content/article images
	ContentImageConfig = MultiSizeConfig{
		Widths:  []int{1920, 1080, 600},
		Quality: 80,
		Format:  "webp",
	}

	// ResourceImageConfig for resource thumbnails
	ResourceImageConfig = MultiSizeConfig{
		Widths:  []int{1080, 600, 400},
		Quality: 80,
		Format:  "webp",
	}

	// OGThumbnailConfig for social media thumbnails
	OGThumbnailConfig = MultiSizeConfig{
		Widths:  []int{1200, 600, 300},
		Quality: 80,
		Format:  "webp",
	}
)

// ProcessMultiSize generates multiple responsive image sizes from a source image
// sourcePath: path to the source image file
// fileID: unique identifier for the file (if empty, generates ULID)
// subdir: subdirectory within media path (e.g., "images/content")
// config: MultiSizeConfig specifying widths, quality, and format
func (p *ImageProcessor) ProcessMultiSize(sourcePath, fileID, subdir string, config MultiSizeConfig) (*MultiSizeResult, error) {
	// Generate ULID if fileID not provided
	if fileID == "" {
		fileID = ulid.Make().String()
	}

	// Load source image
	src, err := imaging.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source image: %w", err)
	}

	// Create target directory
	targetDir := filepath.Join(p.basePath, subdir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	var result MultiSizeResult
	var srcSetParts []string

	// Process each target width
	for _, width := range config.Widths {
		// Resize image using Lanczos algorithm for best quality
		resized := imaging.Resize(src, width, 0, imaging.Lanczos)

		// Generate filename
		filename := fmt.Sprintf("%s_%dpx.%s", fileID, width, config.Format)
		targetPath := filepath.Join(targetDir, filename)

		// Save with format-specific options
		if err := p.saveWithQuality(resized, targetPath, config.Format, config.Quality); err != nil {
			return nil, fmt.Errorf("failed to save %dpx image: %w", width, err)
		}

		// Add to results
		result.Paths = append(result.Paths, targetPath)

		// Build srcSet entry - use relative URL path
		relativeURL := filepath.Join("/media", subdir, filename)
		relativeURL = strings.ReplaceAll(relativeURL, "\\", "/") // Ensure forward slashes
		srcSetParts = append(srcSetParts, fmt.Sprintf("%s %dw", relativeURL, width))

		// Set main path to largest image
		if len(result.Paths) == 1 {
			result.MainPath = relativeURL
		}
	}

	// Build complete srcSet string
	result.SrcSet = strings.Join(srcSetParts, ", ")

	return &result, nil
}

// saveWithQuality saves an image with format-specific quality settings
func (p *ImageProcessor) saveWithQuality(img *image.NRGBA, path, format string, quality int) error {
	switch strings.ToLower(format) {
	case "webp":
		// WebP with quality setting
		return imaging.Save(img, path, imaging.JPEGQuality(quality))
	case "jpeg", "jpg":
		// JPEG with quality setting
		return imaging.Save(img, path, imaging.JPEGQuality(quality))
	case "png":
		// PNG (lossless, quality parameter ignored)
		return imaging.Save(img, path)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// GenerateResponsiveHTML generates HTML img tag with srcSet
func GenerateResponsiveHTML(result *MultiSizeResult, alt, className string) string {
	return fmt.Sprintf(
		`<img src="%s" srcset="%s" alt="%s" class="%s" loading="lazy">`,
		result.MainPath,
		result.SrcSet,
		alt,
		className,
	)
}

// GenerateResponsivePicture generates HTML picture element with multiple formats
func GenerateResponsivePicture(webpResult, jpegResult *MultiSizeResult, alt, className string) string {
	return fmt.Sprintf(`<picture>
  <source srcset="%s" type="image/webp">
  <source srcset="%s" type="image/jpeg">
  <img src="%s" alt="%s" class="%s" loading="lazy">
</picture>`,
		webpResult.SrcSet,
		jpegResult.SrcSet,
		jpegResult.MainPath,
		alt,
		className,
	)
}
