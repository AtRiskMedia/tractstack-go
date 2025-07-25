// Package images provides multi-size image generation for responsive images
package images

import (
	"bytes"
	"fmt"
	"html/template"
	"image"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/oklog/ulid/v2"
)

// imageTemplates holds pre-compiled, secure templates for generating responsive HTML.
var imageTemplates = template.Must(template.New("responsiveImage").Parse(
	`{{define "imgTag"}}<img src="{{.Src}}" srcset="{{.SrcSet}}" alt="{{.Alt}}" class="{{.ClassName}}" loading="lazy">{{end}}` +
		`{{define "pictureTag"}}<picture><source srcset="{{.WebPSrcSet}}" type="image/webp"><source srcset="{{.JpegSrcSet}}" type="image/jpeg"><img src="{{.JpegSrc}}" alt="{{.Alt}}" class="{{.ClassName}}" loading="lazy"></picture>{{end}}`,
))

// Data structs for template execution
type imgTagData struct {
	Src       string
	SrcSet    string
	Alt       string
	ClassName string
}

type pictureTagData struct {
	WebPSrcSet string
	JpegSrcSet string
	JpegSrc    string
	Alt        string
	ClassName  string
}

// MultiSizeConfig holds configuration for multi-size generation
type MultiSizeConfig struct {
	Widths  []int
	Quality int
	Format  string
}

// MultiSizeResult holds the results of multi-size generation
type MultiSizeResult struct {
	MainPath string
	SrcSet   string
	Paths    []string
}

// Predefined configurations matching V1 patterns
var (
	ContentImageConfig = MultiSizeConfig{
		Widths:  []int{1920, 1080, 600},
		Quality: 80,
		Format:  "webp",
	}
	ResourceImageConfig = MultiSizeConfig{
		Widths:  []int{1080, 600, 400},
		Quality: 80,
		Format:  "webp",
	}
	OGThumbnailConfig = MultiSizeConfig{
		Widths:  []int{1200, 600, 300},
		Quality: 80,
		Format:  "webp",
	}
)

// ProcessMultiSize generates multiple responsive image sizes from a source image
func (p *ImageProcessor) ProcessMultiSize(sourcePath, fileID, subdir string, config MultiSizeConfig) (*MultiSizeResult, error) {
	if fileID == "" {
		fileID = ulid.Make().String()
	}

	src, err := imaging.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source image: %w", err)
	}

	targetDir := filepath.Join(p.basePath, subdir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	var result MultiSizeResult
	var srcSetParts []string

	for _, width := range config.Widths {
		resized := imaging.Resize(src, width, 0, imaging.Lanczos)
		filename := fmt.Sprintf("%s_%dpx.%s", fileID, width, config.Format)
		targetPath := filepath.Join(targetDir, filename)

		if err := p.saveWithQuality(resized, targetPath, config.Format, config.Quality); err != nil {
			return nil, fmt.Errorf("failed to save %dpx image: %w", width, err)
		}

		result.Paths = append(result.Paths, targetPath)
		relativeURL := filepath.Join("/media", subdir, filename)
		relativeURL = strings.ReplaceAll(relativeURL, "\\", "/")
		srcSetParts = append(srcSetParts, fmt.Sprintf("%s %dw", relativeURL, width))

		if len(result.Paths) == 1 {
			result.MainPath = relativeURL
		}
	}

	result.SrcSet = strings.Join(srcSetParts, ", ")
	return &result, nil
}

// saveWithQuality saves an image with format-specific quality settings
func (p *ImageProcessor) saveWithQuality(img *image.NRGBA, path, format string, quality int) error {
	switch strings.ToLower(format) {
	case "webp":
		return imaging.Save(img, path, imaging.JPEGQuality(quality))
	case "jpeg", "jpg":
		return imaging.Save(img, path, imaging.JPEGQuality(quality))
	case "png":
		return imaging.Save(img, path)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// GenerateResponsiveHTML generates a secure HTML img tag with srcSet
func GenerateResponsiveHTML(result *MultiSizeResult, alt, className string) string {
	data := imgTagData{
		Src:       result.MainPath,
		SrcSet:    result.SrcSet,
		Alt:       alt,
		ClassName: className,
	}
	var buf bytes.Buffer
	err := imageTemplates.ExecuteTemplate(&buf, "imgTag", data)
	if err != nil {
		log.Printf("ERROR: Failed to execute imgTag template: %v", err)
		return "<!-- template error -->"
	}
	return buf.String()
}

// GenerateResponsivePicture generates a secure HTML picture element with multiple formats
func GenerateResponsivePicture(webpResult, jpegResult *MultiSizeResult, alt, className string) string {
	data := pictureTagData{
		WebPSrcSet: webpResult.SrcSet,
		JpegSrcSet: jpegResult.SrcSet,
		JpegSrc:    jpegResult.MainPath,
		Alt:        alt,
		ClassName:  className,
	}
	var buf bytes.Buffer
	err := imageTemplates.ExecuteTemplate(&buf, "pictureTag", data)
	if err != nil {
		log.Printf("ERROR: Failed to execute pictureTag template: %v", err)
		return "<!-- template error -->"
	}
	return buf.String()
}
