// Package tenant provides tenant-specific infrastructure components.
package tenant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pkgconfig "github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// EmailConfigLoader defines the interface for loading and saving email template configurations,
// abstracting the underlying filesystem or storage layer.
type EmailConfigLoader interface {
	ReadTemplate(tenantID, category, filename string) ([]byte, error)
	WriteTemplate(tenantID, category, filename string, data []byte) error
	ListTemplates() (map[string][]string, error)
}

// LocalEmailConfigLoader implements EmailConfigLoader using the local filesystem.
type LocalEmailConfigLoader struct{}

// NewLocalEmailConfigLoader creates a new instance of LocalEmailConfigLoader.
func NewLocalEmailConfigLoader() *LocalEmailConfigLoader {
	return &LocalEmailConfigLoader{}
}

// ReadTemplate fetches the requested template, strictly enforcing the fallback hierarchy.
// It checks the tenant's specific configuration directory first, then falls back to the system default.
func (l *LocalEmailConfigLoader) ReadTemplate(tenantID, category, filename string) ([]byte, error) {
	tenantPath := filepath.Join(pkgconfig.BackendPath, "config", tenantID, "emails", category, filename)
	fallbackPath := filepath.Join("pkg", "emails", category, filename)

	data, err := os.ReadFile(tenantPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Fallback to system default
			fallbackData, fallbackErr := os.ReadFile(fallbackPath)
			if fallbackErr != nil {
				return nil, fmt.Errorf("template not found in tenant or fallback paths: %w", fallbackErr)
			}
			return fallbackData, nil
		}
		return nil, fmt.Errorf("failed to read tenant template: %w", err)
	}

	return data, nil
}

// WriteTemplate persists an email template payload to the tenant's specific configuration directory.
func (l *LocalEmailConfigLoader) WriteTemplate(tenantID, category, filename string, data []byte) error {
	dirPath := filepath.Join(pkgconfig.BackendPath, "config", tenantID, "emails", category)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("failed to create tenant email directory: %w", err)
	}

	filePath := filepath.Join(dirPath, filename)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write template file: %w", err)
	}

	return nil
}

// ListTemplates discovers all available templates by inspecting the fallback directory structure.
func (l *LocalEmailConfigLoader) ListTemplates() (map[string][]string, error) {
	basePath := filepath.Join("pkg", "emails")
	templates := make(map[string][]string)

	entries, err := os.ReadDir(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return templates, nil
		}
		return nil, fmt.Errorf("failed to read base emails directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			category := entry.Name()
			categoryPath := filepath.Join(basePath, category)

			files, err := os.ReadDir(categoryPath)
			if err != nil {
				continue // Skip unreadable subdirectories
			}

			var tplNames []string
			for _, file := range files {
				if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
					name := strings.TrimSuffix(file.Name(), ".json")
					tplNames = append(tplNames, name)
				}
			}
			if len(tplNames) > 0 {
				templates[category] = tplNames
			}
		}
	}

	return templates, nil
}
