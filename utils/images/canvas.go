// Package images provides canvas-based image generation for OG images
package images

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
)

// OG image standard dimensions
const (
	OGImageWidth  = 1200
	OGImageHeight = 630
)

// CanvasConfig holds configuration for canvas-based image generation
type CanvasConfig struct {
	Width           int
	Height          int
	BackgroundColor string // Hex color string
	TextColor       string // Hex color string
	FontSize        float64
	FontPath        string // Path to font file (optional)
}

// DefaultOGConfig provides sensible defaults for OG image generation
var DefaultOGConfig = CanvasConfig{
	Width:           OGImageWidth,
	Height:          OGImageHeight,
	BackgroundColor: "#1a1a1a",
	TextColor:       "#ffffff",
	FontSize:        64,
	FontPath:        "", // Will use system default
}

// GenerateOGImage creates a text-based OG image with title and optional subtitle
// Saves to the specified path and returns the relative URL path
func (p *ImageProcessor) GenerateOGImage(title, subtitle, outputFilename, subdir string, config CanvasConfig) (string, error) {
	// Create target directory
	targetDir := filepath.Join(p.basePath, subdir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Create drawing context
	dc := gg.NewContext(config.Width, config.Height)

	// Parse and set background color
	bgColor, err := parseHexColor(config.BackgroundColor)
	if err != nil {
		return "", fmt.Errorf("invalid background color: %w", err)
	}
	dc.SetRGBA255(int(bgColor.R), int(bgColor.G), int(bgColor.B), int(bgColor.A))
	dc.Clear()

	// Load font
	if config.FontPath != "" && fileExists(config.FontPath) {
		if err := dc.LoadFontFace(config.FontPath, config.FontSize); err != nil {
			// Fall back to default if custom font fails
			dc.LoadFontFace("", config.FontSize)
		}
	} else {
		// Use system default font
		if err := dc.LoadFontFace("", config.FontSize); err != nil {
			return "", fmt.Errorf("failed to load font: %w", err)
		}
	}

	// Parse and set text color
	textColor, err := parseHexColor(config.TextColor)
	if err != nil {
		return "", fmt.Errorf("invalid text color: %w", err)
	}
	dc.SetRGBA255(int(textColor.R), int(textColor.G), int(textColor.B), int(textColor.A))

	// Calculate text positioning
	margin := float64(config.Width) * 0.1 // 10% margin
	maxWidth := float64(config.Width) - (2 * margin)

	// Wrap and render title
	titleLines := wrapText(dc, title, maxWidth)
	titleHeight := float64(len(titleLines)) * config.FontSize * 1.2

	// Calculate starting Y position for vertical centering
	totalTextHeight := titleHeight
	if subtitle != "" {
		subtitleFontSize := config.FontSize * 0.6
		dc.LoadFontFace("", subtitleFontSize)
		subtitleLines := wrapText(dc, subtitle, maxWidth)
		subtitleHeight := float64(len(subtitleLines)) * subtitleFontSize * 1.2
		totalTextHeight += subtitleHeight + (config.FontSize * 0.5) // Add spacing
	}

	startY := (float64(config.Height) - totalTextHeight) / 2

	// Render title
	dc.LoadFontFace("", config.FontSize)
	y := startY
	for _, line := range titleLines {
		dc.DrawStringAnchored(line, float64(config.Width)/2, y, 0.5, 0.5)
		y += config.FontSize * 1.2
	}

	// Render subtitle if provided
	if subtitle != "" {
		subtitleFontSize := config.FontSize * 0.6
		dc.LoadFontFace("", subtitleFontSize)

		// Add spacing between title and subtitle
		y += config.FontSize * 0.5

		subtitleLines := wrapText(dc, subtitle, maxWidth)
		for _, line := range subtitleLines {
			dc.DrawStringAnchored(line, float64(config.Width)/2, y, 0.5, 0.5)
			y += subtitleFontSize * 1.2
		}
	}

	// Save image
	fullPath := filepath.Join(targetDir, outputFilename)
	if err := dc.SavePNG(fullPath); err != nil {
		return "", fmt.Errorf("failed to save OG image: %w", err)
	}

	// Return relative URL path
	relativePath := filepath.Join("/media", subdir, outputFilename)
	relativePath = strings.ReplaceAll(relativePath, "\\", "/")

	return relativePath, nil
}

// parseHexColor converts hex color strings to RGBA values
func parseHexColor(hex string) (color.RGBA, error) {
	// Remove # prefix if present
	hex = strings.TrimPrefix(hex, "#")

	// Ensure we have 6 characters
	if len(hex) != 6 {
		return color.RGBA{}, fmt.Errorf("invalid hex color length: %s", hex)
	}

	var r, g, b uint8
	_, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("failed to parse hex color: %w", err)
	}

	return color.RGBA{R: r, G: g, B: b, A: 255}, nil
}

// wrapText breaks text into lines that fit within the specified width
func wrapText(dc *gg.Context, text string, maxWidth float64) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{}
	}

	var lines []string
	var currentLine strings.Builder

	for i, word := range words {
		// Test if adding this word would exceed the width
		testLine := currentLine.String()
		if testLine != "" {
			testLine += " "
		}
		testLine += word

		width, _ := dc.MeasureString(testLine)

		if width <= maxWidth || currentLine.Len() == 0 {
			// Word fits, add it to current line
			if currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)
		} else {
			// Word doesn't fit, start new line
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			currentLine.WriteString(word)
		}

		// If this is the last word, add the current line
		if i == len(words)-1 {
			lines = append(lines, currentLine.String())
		}
	}

	return lines
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GenerateSimpleOGImage is a convenience function for basic OG image generation
func (p *ImageProcessor) GenerateSimpleOGImage(title, filename, subdir string) (string, error) {
	return p.GenerateOGImage(title, "", filename, subdir, DefaultOGConfig)
}

// GenerateOGImageWithSubtitle is a convenience function for OG images with subtitle
func (p *ImageProcessor) GenerateOGImageWithSubtitle(title, subtitle, filename, subdir string) (string, error) {
	return p.GenerateOGImage(title, subtitle, filename, subdir, DefaultOGConfig)
}

// GenerateCustomOGImage allows full customization of the OG image
func (p *ImageProcessor) GenerateCustomOGImage(title, subtitle, filename, subdir, bgColor, textColor string, fontSize float64) (string, error) {
	config := CanvasConfig{
		Width:           OGImageWidth,
		Height:          OGImageHeight,
		BackgroundColor: bgColor,
		TextColor:       textColor,
		FontSize:        fontSize,
		FontPath:        "",
	}

	return p.GenerateOGImage(title, subtitle, filename, subdir, config)
}
