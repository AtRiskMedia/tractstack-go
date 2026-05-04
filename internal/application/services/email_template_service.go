// Package services provides business logic and orchestration for the application.
package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	texttemplate "text/template"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/email/templates"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// EmailTemplate represents the root schema of the parallel email builder.
// The Blocks array uses json.RawMessage to defer polymorphic type evaluation
// until the template is explicitly compiled by the execution engine.
type EmailTemplate struct {
	Subject string            `json:"subject"`
	Blocks  []json.RawMessage `json:"blocks"`
}

// EmailTemplateService manages the storage, retrieval, and compilation of isolated email JSON schemas.
type EmailTemplateService struct {
	loader      tenant.EmailConfigLoader
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewEmailTemplateService creates a new instance of the email template service.
func NewEmailTemplateService(loader tenant.EmailConfigLoader, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *EmailTemplateService {
	return &EmailTemplateService{
		loader:      loader,
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// GetTemplate fetches the requested template, strictly enforcing the fallback hierarchy.
// If a tenant corrupts or deletes their local override, the system automatically
// degrades to the physical codebase template to guarantee transactional delivery.
func (s *EmailTemplateService) GetTemplate(tenantID, category, templateName string) (*EmailTemplate, error) {
	marker := s.perfTracker.StartOperation("email_template_get", tenantID)
	defer marker.Complete()

	filename := fmt.Sprintf("%s.json", templateName)

	data, err := s.loader.ReadTemplate(tenantID, category, filename)
	if err != nil {
		marker.SetError(err)
		return nil, fmt.Errorf("failed to read template data: %w", err)
	}

	var tmpl EmailTemplate
	if err := json.Unmarshal(data, &tmpl); err != nil {
		marker.SetError(err)
		return nil, fmt.Errorf("failed to parse template JSON: %w", err)
	}

	marker.SetSuccess(true)
	return &tmpl, nil
}

// SaveTemplate persists an EmailTemplate to the tenant's specific configuration directory,
// functioning as an override to the codebase fallback.
func (s *EmailTemplateService) SaveTemplate(tenantID, category, templateName string, template *EmailTemplate) error {
	marker := s.perfTracker.StartOperation("email_template_save", tenantID)
	defer marker.Complete()

	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		marker.SetError(err)
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	filename := fmt.Sprintf("%s.json", templateName)
	if err := s.loader.WriteTemplate(tenantID, category, filename, data); err != nil {
		marker.SetError(err)
		return fmt.Errorf("failed to write template file: %w", err)
	}

	marker.SetSuccess(true)
	return nil
}

// ListTemplates returns merged system + tenant email manifests (administrative titles and names).
func (s *EmailTemplateService) ListTemplates(tenantID string) (map[string][]tenant.EmailTemplateListEntry, error) {
	marker := s.perfTracker.StartOperation("email_template_list", tenantID)
	defer marker.Complete()

	templates, err := s.loader.ListTemplates(tenantID)
	if err != nil {
		marker.SetError(err)
		return nil, fmt.Errorf("failed to list templates via loader: %w", err)
	}

	marker.SetSuccess(true)
	return templates, nil
}

// Compile renders the EmailTemplate into its final subject and HTML body strings.
// This serves as the single source of truth for rendering logic used by both
// the background worker and the real-time previewer.
func (s *EmailTemplateService) Compile(tmpl *EmailTemplate, data map[string]any, siteURL string) (string, string, error) {
	if siteURL == "" {
		siteURL = "https://tractstack.com"
	}

	// 1. Compile Subject (text/template)
	subjectTmpl, err := texttemplate.New("subject").Option("missingkey=error").Parse(tmpl.Subject)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse subject template string: %w", err)
	}

	var subjectBuf bytes.Buffer
	if err := subjectTmpl.Execute(&subjectBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to execute subject template: %w", err)
	}
	compiledSubject := subjectBuf.String()

	// 2. Compile Polymorphic JSON Blocks to Base HTML
	rawHTML, err := ParseEmailBlocks(tmpl.Blocks, siteURL, data)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse JSON blocks to HTML: %w", err)
	}

	// 3. Inject variables securely using context-aware html/template escaping
	bodyTmpl, err := template.New("body").Option("missingkey=default").Parse(rawHTML)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse html template string: %w", err)
	}

	var bodyBuf bytes.Buffer
	if err := bodyTmpl.Execute(&bodyBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to execute html template: %w", err)
	}
	compiledBody := bodyBuf.String()

	// 4. Wrap inside the standard TractStack HTML layout
	finalHTML := templates.GetEmailLayout(templates.EmailLayoutProps{
		Content: compiledBody,
	})

	return compiledSubject, finalHTML, nil
}
