// Package images provides image processing utilities for TractStack
package images

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
