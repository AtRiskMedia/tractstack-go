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
	fmt.Printf("[DEBUG] DeleteOGImageAndThumbnails called with imagePath: %s\n", imagePath)

	if imagePath == "" {
		return fmt.Errorf("empty image path")
	}

	// Extract filename and base name
	filename := filepath.Base(imagePath)
	basename := filename
	if dotIndex := strings.LastIndex(filename, "."); dotIndex != -1 {
		basename = filename[:dotIndex]
	}

	fmt.Printf("[DEBUG] filename: %s, basename: %s\n", filename, basename)

	// Remove original image
	originalPath := filepath.Join(p.basePath, strings.TrimPrefix(imagePath, "/media/"))
	fmt.Printf("[DEBUG] Attempting to remove original image: %s\n", originalPath)

	if err := os.Remove(originalPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("[ERROR] Failed to remove original image: %v\n", err)
		return fmt.Errorf("failed to remove original image: %w", err)
	} else if err == nil {
		fmt.Printf("[DEBUG] Successfully removed original image: %s\n", originalPath)
	} else {
		fmt.Printf("[DEBUG] Original image didn't exist: %s\n", originalPath)
	}

	// Remove thumbnails (1200px, 600px, 300px WebP)
	thumbsDir := filepath.Join(p.basePath, "images", "thumbs")
	thumbnailSizes := []string{"1200px", "600px", "300px"}

	for _, size := range thumbnailSizes {
		thumbPath := filepath.Join(thumbsDir, fmt.Sprintf("%s_%s.webp", basename, size))
		fmt.Printf("[DEBUG] Attempting to remove thumbnail: %s\n", thumbPath)

		if err := os.Remove(thumbPath); err != nil && !os.IsNotExist(err) {
			// Log warning but don't fail - continue removing other thumbnails
			fmt.Printf("Warning: failed to remove thumbnail %s: %v\n", thumbPath, err)
		} else if err == nil {
			fmt.Printf("[DEBUG] Successfully removed thumbnail: %s\n", thumbPath)
		} else {
			fmt.Printf("[DEBUG] Thumbnail didn't exist: %s\n", thumbPath)
		}
	}

	fmt.Printf("[DEBUG] DeleteOGImageAndThumbnails completed\n")
	return nil
}

// generateWebPThumbnails creates 1200px, 600px, and 300px WebP thumbnails
func (p *ImageProcessor) generateWebPThumbnails(originalPath, nodeID string, timestamp int64, thumbsDir string) ([]string, error) {
	fmt.Printf("[DEBUG] generateWebPThumbnails: originalPath=%s, nodeID=%s, timestamp=%d, thumbsDir=%s\n",
		originalPath, nodeID, timestamp, thumbsDir)

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

	fmt.Printf("[DEBUG] Original image decoded successfully, bounds: %v\n", img.Bounds())

	// Extract base name for thumbnails
	basename := fmt.Sprintf("%s-%d", nodeID, timestamp)
	sizes := []int{1200, 600, 300}
	thumbnailPaths := make([]string, len(sizes))

	fmt.Printf("[DEBUG] Creating thumbnails with basename: %s\n", basename)

	for i, width := range sizes {
		fmt.Printf("[DEBUG] Processing thumbnail %d/%d: %dpx width\n", i+1, len(sizes), width)

		// Resize image maintaining aspect ratio
		resized := imaging.Resize(img, width, 0, imaging.Lanczos)
		fmt.Printf("[DEBUG] Resized image bounds: %v\n", resized.Bounds())

		// Create WebP filename
		thumbFilename := fmt.Sprintf("%s_%dpx.webp", basename, width)
		thumbPath := filepath.Join(thumbsDir, thumbFilename)
		fmt.Printf("[DEBUG] Saving thumbnail to: %s\n", thumbPath)

		// Save as WebP using webp library, NOT imaging.Save()
		err := webp.Save(thumbPath, resized, &webp.Options{Quality: 85})
		if err != nil {
			fmt.Printf("[ERROR] Failed to save WebP thumbnail %s: %v\n", thumbFilename, err)
			// Clean up any previously created thumbnails - FIXED THE BUG HERE
			for j := range i {
				fmt.Printf("[DEBUG] Cleaning up thumbnail: %s\n", thumbnailPaths[j])
				os.Remove(thumbnailPaths[j])
			}
			return nil, fmt.Errorf("failed to save WebP thumbnail %s: %w", thumbFilename, err)
		}

		thumbnailPaths[i] = thumbPath
		fmt.Printf("[DEBUG] Successfully saved thumbnail: %s\n", thumbPath)
	}

	fmt.Printf("[DEBUG] All thumbnails created successfully: %v\n", thumbnailPaths)
	return thumbnailPaths, nil
}

// ProcessOGImageWithThumbnails handles OG image uploads for StoryFragments
// Saves original to /images/og/ and generates WebP thumbnails to /images/thumbs/
// Returns original path and thumbnail paths
func (p *ImageProcessor) ProcessOGImageWithThumbnails(data, nodeID string) (string, []string, error) {
	fmt.Printf("[DEBUG] ProcessOGImageWithThumbnails: nodeID=%s, basePath=%s\n", nodeID, p.basePath)

	if data == "" {
		return "", nil, fmt.Errorf("empty base64 data")
	}

	// Extract file extension from MIME type
	ext := extractExtension(data)
	if ext == "" {
		return "", nil, fmt.Errorf("unsupported image format")
	}
	fmt.Printf("[DEBUG] Detected extension: %s\n", ext)

	// Generate timestamped filename: {nodeID}-{timestamp}.{ext}
	timestamp := time.Now().UnixMilli()
	filename := fmt.Sprintf("%s-%d.%s", nodeID, timestamp, ext)
	fmt.Printf("[DEBUG] Generated filename: %s\n", filename)

	// Create directories
	ogDir := filepath.Join(p.basePath, "images", "og")
	thumbsDir := filepath.Join(p.basePath, "images", "thumbs")

	fmt.Printf("[DEBUG] Creating directories: ogDir=%s, thumbsDir=%s\n", ogDir, thumbsDir)

	if err := os.MkdirAll(ogDir, 0755); err != nil {
		fmt.Printf("[ERROR] Failed to create og directory %s: %v\n", ogDir, err)
		return "", nil, fmt.Errorf("failed to create og directory: %w", err)
	}
	if err := os.MkdirAll(thumbsDir, 0755); err != nil {
		fmt.Printf("[ERROR] Failed to create thumbs directory %s: %v\n", thumbsDir, err)
		return "", nil, fmt.Errorf("failed to create thumbs directory: %w", err)
	}

	fmt.Printf("[DEBUG] Directories created successfully\n")

	// Save original image to /images/og/
	originalPath, err := processBinaryImage(data, filename, ogDir)
	if err != nil {
		fmt.Printf("[ERROR] Failed to save original image: %v\n", err)
		return "", nil, fmt.Errorf("failed to save original image: %w", err)
	}
	fmt.Printf("[DEBUG] Original image saved to: %s\n", originalPath)

	// Generate WebP thumbnails (1200px, 600px, 300px)
	thumbnailPaths, err := p.generateWebPThumbnails(originalPath, nodeID, timestamp, thumbsDir)
	if err != nil {
		// If thumbnail generation fails, clean up original and return error
		fmt.Printf("[ERROR] Thumbnail generation failed, cleaning up original: %s\n", originalPath)
		os.Remove(originalPath)
		return "", nil, fmt.Errorf("failed to generate thumbnails: %w", err)
	}

	relativeOriginal := fmt.Sprintf("/media/images/og/%s", filename)
	relativeThumbnails := make([]string, len(thumbnailPaths))
	for i, thumbPath := range thumbnailPaths {
		relativeThumbnails[i] = fmt.Sprintf("/media/images/thumbs/%s", filepath.Base(thumbPath))
	}

	fmt.Printf("[DEBUG] Success! Original: %s, Thumbnails: %v\n", relativeOriginal, relativeThumbnails)
	return relativeOriginal, relativeThumbnails, nil
}

// processBinaryImage handles binary image processing (PNG, JPG, ICO, WebP)
func processBinaryImage(data, filename, targetDir string) (string, error) {
	fmt.Printf("[DEBUG] processBinaryImage: filename=%s, targetDir=%s\n", filename, targetDir)

	// Binary image regex pattern
	binaryPattern := regexp.MustCompile(`^data:image/\w+;base64,`)
	if !binaryPattern.MatchString(data) {
		fmt.Printf("[ERROR] Invalid binary image base64 format: %s\n", data[:50])
		return "", fmt.Errorf("invalid binary image base64 format")
	}

	// Strip prefix and decode
	b64Data := binaryPattern.ReplaceAllString(data, "")
	fmt.Printf("[DEBUG] Base64 data length after stripping prefix: %d\n", len(b64Data))

	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		fmt.Printf("[ERROR] Failed to decode base64: %v\n", err)
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}
	fmt.Printf("[DEBUG] Decoded binary data length: %d bytes\n", len(decoded))

	// Write as binary
	fullPath := filepath.Join(targetDir, filename)
	fmt.Printf("[DEBUG] Writing file to: %s\n", fullPath)

	if err := os.WriteFile(fullPath, decoded, 0644); err != nil {
		fmt.Printf("[ERROR] Failed to write binary file: %v\n", err)
		return "", fmt.Errorf("failed to write binary file: %w", err)
	}

	// Verify file was actually written
	if info, err := os.Stat(fullPath); err != nil {
		fmt.Printf("[ERROR] File verification failed: %v\n", err)
		return "", fmt.Errorf("file verification failed: %w", err)
	} else {
		fmt.Printf("[DEBUG] File written successfully: %s (size: %d bytes)\n", fullPath, info.Size())
	}

	return fullPath, nil
}
