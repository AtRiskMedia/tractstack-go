// Package tenant provides tenant-specific infrastructure components.
package tenant

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	pkgconfig "github.com/AtRiskMedia/tractstack-go/pkg/config"
)

const emailManifestFilename = "manifest.json"

// emailManifestEntry is one row in emails/manifest.json (system or tenant).
type emailManifestEntry struct {
	Category   string `json:"category"`
	Name       string `json:"name"`
	AdminTitle string `json:"adminTitle"`
}

type emailManifestDocument struct {
	Templates []emailManifestEntry `json:"templates"`
}

// EmailTemplateListEntry is one template row for GET /emails/templates.
type EmailTemplateListEntry struct {
	Name       string `json:"name"`
	AdminTitle string `json:"adminTitle"`
}

// EmailConfigLoader defines the interface for loading and saving email template configurations,
// abstracting the underlying filesystem or storage layer.
type EmailConfigLoader interface {
	ReadTemplate(tenantID, category, filename string) ([]byte, error)
	WriteTemplate(tenantID, category, filename string, data []byte) error
	ListTemplates(tenantID string) (map[string][]EmailTemplateListEntry, error)
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

// ListTemplates loads the system manifest, merges the tenant manifest when present,
// and returns only entries whose template JSON exists (tenant override or pkg fallback).
func (l *LocalEmailConfigLoader) ListTemplates(tenantID string) (map[string][]EmailTemplateListEntry, error) {
	systemPath := filepath.Join("pkg", "emails", emailManifestFilename)
	systemEntries, err := l.readManifestFile(systemPath)
	if err != nil {
		return nil, fmt.Errorf("read system email manifest: %w", err)
	}

	tenantManifestPath := filepath.Join(pkgconfig.BackendPath, "config", tenantID, "emails", emailManifestFilename)
	var tenantEntries []emailManifestEntry
	if data, err := os.ReadFile(tenantManifestPath); err == nil {
		var mf emailManifestDocument
		if err := json.Unmarshal(data, &mf); err != nil {
			return nil, fmt.Errorf("parse tenant email manifest: %w", err)
		}
		tenantEntries = mf.Templates
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read tenant email manifest: %w", err)
	}

	merged := mergeEmailManifests(systemEntries, tenantEntries)
	merged = l.filterResolvableTemplates(tenantID, merged)

	out := make(map[string][]EmailTemplateListEntry)
	for _, e := range merged {
		if e.Category == "" || e.Name == "" {
			continue
		}
		out[e.Category] = append(out[e.Category], EmailTemplateListEntry{
			Name:       e.Name,
			AdminTitle: e.AdminTitle,
		})
	}
	return out, nil
}

func (l *LocalEmailConfigLoader) readManifestFile(path string) ([]emailManifestEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mf emailManifestDocument
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, err
	}
	return mf.Templates, nil
}

type manifestKey struct {
	category string
	name     string
}

func mergeEmailManifests(system, tenant []emailManifestEntry) []emailManifestEntry {
	mergedByKey := make(map[manifestKey]emailManifestEntry)
	for _, e := range system {
		k := manifestKey{e.Category, e.Name}
		mergedByKey[k] = e
	}
	for _, e := range tenant {
		k := manifestKey{e.Category, e.Name}
		mergedByKey[k] = e
	}

	systemKeys := make(map[manifestKey]bool)
	for _, e := range system {
		systemKeys[manifestKey{e.Category, e.Name}] = true
	}

	out := make([]emailManifestEntry, 0, len(mergedByKey))
	for _, e := range system {
		k := manifestKey{e.Category, e.Name}
		out = append(out, mergedByKey[k])
	}
	for _, e := range tenant {
		k := manifestKey{e.Category, e.Name}
		if !systemKeys[k] {
			out = append(out, mergedByKey[k])
		}
	}
	return out
}

func (l *LocalEmailConfigLoader) filterResolvableTemplates(tenantID string, entries []emailManifestEntry) []emailManifestEntry {
	var out []emailManifestEntry
	for _, e := range entries {
		filename := fmt.Sprintf("%s.json", e.Name)
		if _, err := l.ReadTemplate(tenantID, e.Category, filename); err == nil {
			out = append(out, e)
		}
	}
	return out
}
