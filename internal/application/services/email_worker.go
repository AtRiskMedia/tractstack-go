// Package services provides business logic and orchestration for the application.
package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/email"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// EmailJob defines the payload required to securely dispatch a templated email.
// The Data map should be populated with the exact variables needed for the template.
type EmailJob struct {
	TenantID     string
	To           []string
	Category     string
	TemplateName string
	Data         map[string]any
}

// EmailWorker is a durable background processor that resolves email templates,
// executes context-aware variable substitution, and dispatches them via the email client.
type EmailWorker struct {
	jobQueue             chan EmailJob
	emailService         email.Service
	emailTemplateService *EmailTemplateService
	tenantManager        *tenant.Manager
	logger               *logging.ChanneledLogger
}

// NewEmailWorker initializes and starts the background email processing pipeline.
func NewEmailWorker(
	emailService email.Service,
	emailTemplateService *EmailTemplateService,
	tenantManager *tenant.Manager,
	logger *logging.ChanneledLogger,
) *EmailWorker {
	worker := &EmailWorker{
		jobQueue:             make(chan EmailJob, 100), // Buffered to handle traffic bursts without blocking checkout
		emailService:         emailService,
		emailTemplateService: emailTemplateService,
		tenantManager:        tenantManager,
		logger:               logger,
	}

	go worker.run()
	return worker
}

// Enqueue drops a non-blocking email dispatch request into the background pipeline.
func (w *EmailWorker) Enqueue(job EmailJob) {
	select {
	case w.jobQueue <- job:
		w.logger.System().Debug("Email job enqueued", "tenantId", job.TenantID, "template", job.TemplateName)
	default:
		w.logger.System().Error("Email worker queue is full; dropping email job", "tenantId", job.TenantID, "template", job.TemplateName)
	}
}

func (w *EmailWorker) run() {
	for job := range w.jobQueue {
		w.processJobWithRetries(job)
	}
}

// processJobWithRetries attempts to process and dispatch the email, backing off on failure.
// Deterministic errors (like missing or malformed templates) are dropped immediately.
func (w *EmailWorker) processJobWithRetries(job EmailJob) {
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := w.processJob(job)
		if err == nil {
			return
		}

		// Inspect error for fatal, non-retriable conditions
		errMsg := err.Error()
		if strings.Contains(errMsg, "template not found") ||
			strings.Contains(errMsg, "failed to parse") ||
			strings.Contains(errMsg, "failed to compile") {
			w.logger.System().Error("Email processing aborted due to deterministic error", "error", err, "tenantId", job.TenantID, "template", job.TemplateName)
			return
		}

		w.logger.System().Warn("Email processing failed", "attempt", attempt, "error", err, "tenantId", job.TenantID)

		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		} else {
			w.logger.System().Error("Email processing permanently abandoned after max retries", "tenantId", job.TenantID, "template", job.TemplateName)
		}
	}
}

func (w *EmailWorker) processJob(job EmailJob) error {
	ctx, err := w.tenantManager.NewContextFromID(job.TenantID)
	if err != nil {
		return fmt.Errorf("failed to acquire tenant context for email worker: %w", err)
	}
	defer func() {
		if closeErr := ctx.Close(); closeErr != nil {
			w.logger.System().Warn("Failed to close tenant context in email worker", "error", closeErr)
		}
	}()

	// Resolve Template Configuration
	templateConfig, err := w.emailTemplateService.GetTemplate(job.TenantID, job.Category, job.TemplateName)
	if err != nil {
		return fmt.Errorf("failed to retrieve template schema: %w", err)
	}

	// Compile high-fidelity subject and body using unified service logic
	siteURL := ctx.Config.BrandConfig.SiteURL
	compiledSubject, finalHTML, err := w.emailTemplateService.Compile(templateConfig, job.Data, siteURL)
	if err != nil {
		return fmt.Errorf("failed to compile email: %w", err)
	}

	if err := w.emailService.SendDynamicHTML(job.TenantID, job.To, compiledSubject, finalHTML); err != nil {
		return fmt.Errorf("dispatch failed: %w", err)
	}

	w.logger.System().Info("Email successfully dispatched", "tenantId", job.TenantID, "template", job.TemplateName)
	return nil
}
