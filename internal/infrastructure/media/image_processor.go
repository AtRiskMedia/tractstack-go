// Package media provides image processing utilities
package media

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
)

// ImageProcessor handles image processing operations for a specific tenant
type ImageProcessor struct {
	basePath string // Points to ~/t8k-go-server/config/{tenantId}/media
}

// NewImageProcessor creates a new ImageProcessor instance
func NewImageProcessor(basePath string) *ImageProcessor {
	return &ImageProcessor{
		basePath: basePath,
	}
}

// ProcessBase64Image handles any base64 image upload with automatic format detection
// Returns the full file path on disk
func (p *ImageProcessor) ProcessBase64Image(data, filename, subdir string) (string, error) {
	if data == "" {
		return "", fmt.Errorf("empty base64 data")
	}

	// Extract file extension from MIME type
	ext := extractExtension(data)
	if ext == "" {
		return "", fmt.Errorf("unsupported image format")
	}

	// Construct filename with extension
	fullFilename := fmt.Sprintf("%s.%s", filename, ext)

	// Create target directory
	targetDir := filepath.Join(p.basePath, subdir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Route to appropriate processor based on format
	if strings.Contains(data, "image/svg+xml") {
		return processSVG(data, fullFilename, targetDir)
	} else {
		return processBinaryImage(data, fullFilename, targetDir)
	}
}

// ProcessVersionedImage handles brand asset uploads with timestamp versioning and cleanup
// Used for og and oglogo assets only - returns relative URL path and new version timestamp
func (p *ImageProcessor) ProcessVersionedImage(data, baseFilename, subdir string, currentVersion int64) (string, int64, error) {
	if data == "" {
		return "", 0, fmt.Errorf("empty base64 data")
	}

	// Extract file extension
	ext := extractExtension(data)
	if ext == "" {
		return "", 0, fmt.Errorf("unsupported image format")
	}

	// Create target directory
	targetDir := filepath.Join(p.basePath, subdir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Delete old versioned file if exists
	if currentVersion > 0 {
		deleteVersionedFile(baseFilename, currentVersion, targetDir)
	}

	// Generate new timestamp
	newVersion := time.Now().Unix()
	versionedFilename := fmt.Sprintf("%s-%d.%s", baseFilename, newVersion, ext)

	// Process the image
	var err error
	if strings.Contains(data, "image/svg+xml") {
		_, err = processSVG(data, versionedFilename, targetDir)
	} else {
		_, err = processBinaryImage(data, versionedFilename, targetDir)
	}

	if err != nil {
		return "", 0, err
	}

	// Return relative URL path for serving by nginx
	relativePath := filepath.Join("/media", subdir, versionedFilename)
	// Ensure forward slashes for URLs
	relativePath = strings.ReplaceAll(relativePath, "\\", "/")

	return relativePath, newVersion, nil
}

// processSVG handles SVG-specific base64 processing
func processSVG(data, filename, targetDir string) (string, error) {
	// SVG regex pattern
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

	return fullPath, nil
}

// extractExtension auto-detects file extension from MIME type
func extractExtension(data string) string {
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
	// Fallback to PNG
	return "png"
}

// deleteVersionedFile cleans up old versioned files before new upload
func deleteVersionedFile(baseFilename string, version int64, targetDir string) {
	// Try common extensions
	extensions := []string{"svg", "png", "jpg", "jpeg", "ico", "webp"}

	for _, ext := range extensions {
		oldFilename := fmt.Sprintf("%s-%d.%s", baseFilename, version, ext)
		oldPath := filepath.Join(targetDir, oldFilename)

		// Ignore errors - file might not exist or might have different extension
		os.Remove(oldPath)
	}
}

// DeleteOGImageAndThumbnails removes OG image and associated thumbnails
func (p *ImageProcessor) DeleteOGImageAndThumbnails(imagePath string) error {
	if imagePath == "" {
		return fmt.Errorf("empty image path")
	}

	// Extract filename and base name
	filename := filepath.Base(imagePath)
	basename := filename
	if dotIndex := strings.LastIndex(filename, "."); dotIndex != -1 {
		basename = filename[:dotIndex]
	}

	// Remove original image
	originalPath := filepath.Join(p.basePath, strings.TrimPrefix(imagePath, "/media/"))

	if err := os.Remove(originalPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove original image: %w", err)
	}

	// Remove thumbnails (1200px, 600px, 300px WebP)
	thumbsDir := filepath.Join(p.basePath, "images", "thumbs")
	thumbnailSizes := []string{"1200px", "600px", "300px"}

	for _, size := range thumbnailSizes {
		thumbPath := filepath.Join(thumbsDir, fmt.Sprintf("%s_%s.webp", basename, size))

		if err := os.Remove(thumbPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to remove thumbnail %s: %v\n", thumbPath, err)
		}
	}

	return nil
}

// generateWebPThumbnails creates 1200px, 600px, and 300px WebP thumbnails
func (p *ImageProcessor) generateWebPThumbnails(originalPath, nodeID string, timestamp int64, thumbsDir string) ([]string, error) {
	// Open and decode the original image
	originalFile, err := os.Open(originalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open original file: %w", err)
	}
	defer originalFile.Close()

	// Decode the image
	img, err := imaging.Decode(originalFile)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Extract base name for thumbnails
	basename := fmt.Sprintf("%s-%d", nodeID, timestamp)
	sizes := []int{1200, 600, 300}
	thumbnailPaths := make([]string, len(sizes))

	for i, width := range sizes {
		// Resize image maintaining aspect ratio
		resized := imaging.Resize(img, width, 0, imaging.Lanczos)

		// Create WebP filename
		thumbFilename := fmt.Sprintf("%s_%dpx.webp", basename, width)
		thumbPath := filepath.Join(thumbsDir, thumbFilename)

		// Save as WebP using webp library, NOT imaging.Save()
		err := webp.Save(thumbPath, resized, &webp.Options{Quality: 85})
		if err != nil {
			// Clean up any previously created thumbnails - FIXED THE BUG HERE
			for j := range i {
				os.Remove(thumbnailPaths[j])
			}
			return nil, fmt.Errorf("failed to save WebP thumbnail %s: %w", thumbFilename, err)
		}

		thumbnailPaths[i] = thumbPath
	}

	return thumbnailPaths, nil
}

// ProcessOGImageWithThumbnails handles OG image uploads for StoryFragments
// Saves original to /images/og/ and generates WebP thumbnails to /images/thumbs/
// Returns original path and thumbnail paths
func (p *ImageProcessor) ProcessOGImageWithThumbnails(data, nodeID string) (string, []string, error) {
	if data == "" {
		return "", nil, fmt.Errorf("empty base64 data")
	}

	// Extract file extension from MIME type
	ext := extractExtension(data)
	if ext == "" {
		return "", nil, fmt.Errorf("unsupported image format")
	}

	// Generate timestamped filename: {nodeID}-{timestamp}.{ext}
	timestamp := time.Now().UnixMilli()
	filename := fmt.Sprintf("%s-%d.%s", nodeID, timestamp, ext)

	// Create directories
	ogDir := filepath.Join(p.basePath, "images", "og")
	thumbsDir := filepath.Join(p.basePath, "images", "thumbs")

	if err := os.MkdirAll(ogDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create og directory: %w", err)
	}
	if err := os.MkdirAll(thumbsDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create thumbs directory: %w", err)
	}

	// Save original image to /images/og/
	originalPath, err := processBinaryImage(data, filename, ogDir)
	if err != nil {
		return "", nil, fmt.Errorf("failed to save original image: %w", err)
	}

	// Generate WebP thumbnails (1200px, 600px, 300px)
	thumbnailPaths, err := p.generateWebPThumbnails(originalPath, nodeID, timestamp, thumbsDir)
	if err != nil {
		// If thumbnail generation fails, clean up original and return error
		os.Remove(originalPath)
		return "", nil, fmt.Errorf("failed to generate thumbnails: %w", err)
	}

	relativeOriginal := fmt.Sprintf("/media/images/og/%s", filename)
	relativeThumbnails := make([]string, len(thumbnailPaths))
	for i, thumbPath := range thumbnailPaths {
		relativeThumbnails[i] = fmt.Sprintf("/media/images/thumbs/%s", filepath.Base(thumbPath))
	}

	return relativeOriginal, relativeThumbnails, nil
}

// processBinaryImage handles binary image processing (PNG, JPG, ICO, WebP)
func processBinaryImage(data, filename, targetDir string) (string, error) {
	// Binary image regex pattern
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

	// Verify file was actually written
	if _, err := os.Stat(fullPath); err != nil {
		return "", fmt.Errorf("file verification failed: %w", err)
	}

	return fullPath, nil
}

// ProcessContentImageWithSizes handles content image processing with responsive sizes
// Creates responsive WebP versions for raster images or saves SVG as-is
// Returns main src path and srcSet string (srcSet is nil for SVGs)
func (p *ImageProcessor) ProcessContentImageWithSizes(data, fileID string) (string, *string, error) {
	if data == "" {
		return "", nil, fmt.Errorf("empty base64 data")
	}

	// Extract file extension from MIME type
	ext := extractExtension(data)
	if ext == "" {
		return "", nil, fmt.Errorf("unsupported image format")
	}

	// Get current month path for organization
	monthPath := getMonthPath()

	// Create month-based directory
	monthDir := filepath.Join(p.basePath, "images", monthPath)
	if err := os.MkdirAll(monthDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create month directory: %w", err)
	}

	// Handle SVG files (no resizing needed)
	if ext == "svg" {
		filename := fmt.Sprintf("%s.%s", fileID, ext)
		_, err := processSVG(data, filename, monthDir)
		if err != nil {
			fmt.Printf("[ERROR] Failed to save SVG: %v\n", err)
			return "", nil, fmt.Errorf("failed to save SVG: %w", err)
		}

		relativePath := fmt.Sprintf("/media/images/%s/%s", monthPath, filename)
		return relativePath, nil, nil
	}

	// Handle raster images (PNG, JPG, WebP) - create responsive versions
	filename := fmt.Sprintf("%s.%s", fileID, ext)
	originalPath, err := processBinaryImage(data, filename, monthDir)
	if err != nil {
		fmt.Printf("[ERROR] Failed to save original image: %v\n", err)
		return "", nil, fmt.Errorf("failed to save original image: %w", err)
	}

	// Generate responsive WebP versions
	responsivePaths, err := p.generateContentImageSizes(originalPath, fileID, monthDir)
	if err != nil {
		// If responsive generation fails, clean up original and return error
		fmt.Printf("[ERROR] Responsive image generation failed, cleaning up original: %s\n", originalPath)
		os.Remove(originalPath)
		return "", nil, fmt.Errorf("failed to generate responsive images: %w", err)
	}

	// Build srcSet string and determine main src
	srcSet := p.buildContentImageSrcSet(responsivePaths, monthPath)
	mainSrc := fmt.Sprintf("/media/images/%s/%s_1920px.webp", monthPath, fileID)

	return mainSrc, &srcSet, nil
}

// generateContentImageSizes creates 1920px, 1080px, and 600px WebP versions
func (p *ImageProcessor) generateContentImageSizes(originalPath, fileID, monthDir string) ([]string, error) {
	// Open and decode the original image
	originalFile, err := os.Open(originalPath)
	if err != nil {
		fmt.Printf("[ERROR] Failed to open original file: %v\n", err)
		return nil, fmt.Errorf("failed to open original file: %w", err)
	}
	defer originalFile.Close()

	// Decode the image
	img, err := imaging.Decode(originalFile)
	if err != nil {
		fmt.Printf("[ERROR] Failed to decode image: %v\n", err)
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Content image responsive sizes (different from OG thumbnail sizes)
	sizes := []int{1920, 1080, 600}
	responsivePaths := make([]string, len(sizes))

	for i, width := range sizes {
		// Resize image maintaining aspect ratio
		resized := imaging.Resize(img, width, 0, imaging.Lanczos)

		// Create WebP filename with content image naming pattern
		webpFilename := fmt.Sprintf("%s_%dpx.webp", fileID, width)
		webpPath := filepath.Join(monthDir, webpFilename)

		// Save as WebP using webp library
		err := webp.Save(webpPath, resized, &webp.Options{Quality: 85})
		if err != nil {
			// Clean up any previously created responsive images
			for j := range i {
				os.Remove(responsivePaths[j])
			}
			return nil, fmt.Errorf("failed to save WebP responsive image %s: %w", webpFilename, err)
		}

		responsivePaths[i] = webpPath
	}

	// Remove original image after successful WebP generation
	if err := os.Remove(originalPath); err != nil {
		fmt.Printf("[WARN] Failed to remove original image: %v\n", err)
	}

	return responsivePaths, nil
}

// buildContentImageSrcSet generates the srcSet string for responsive images
func (p *ImageProcessor) buildContentImageSrcSet(responsivePaths []string, monthPath string) string {
	sizes := []int{1920, 1080, 600}
	srcSetParts := make([]string, len(sizes))

	for i, width := range sizes {
		// Extract filename from full path
		filename := filepath.Base(responsivePaths[i])
		relativePath := fmt.Sprintf("/media/images/%s/%s", monthPath, filename)
		srcSetParts[i] = fmt.Sprintf("%s %dw", relativePath, width)
	}

	return strings.Join(srcSetParts, ", ")
}

// getMonthPath returns current month in YYYY-MM format for directory organization
func getMonthPath() string {
	now := time.Now()
	return now.Format("2006-01")
}
