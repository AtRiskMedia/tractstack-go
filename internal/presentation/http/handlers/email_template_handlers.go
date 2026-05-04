// Package handlers provides HTTP route handlers for the presentation layer.
package handlers

import (
	"html/template"
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// EmailTemplateHandlers provides REST endpoints for the parallel email builder UI.
type EmailTemplateHandlers struct {
	emailTemplateService *services.EmailTemplateService
	logger               *logging.ChanneledLogger
	perfTracker          *performance.Tracker
}

// NewEmailTemplateHandlers creates a new instance of EmailTemplateHandlers.
func NewEmailTemplateHandlers(
	emailTemplateService *services.EmailTemplateService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *EmailTemplateHandlers {
	return &EmailTemplateHandlers{
		emailTemplateService: emailTemplateService,
		logger:               logger,
		perfTracker:          perfTracker,
	}
}

// HandleListTemplates returns merged system + tenant email manifests (names and administrative titles).
func (h *EmailTemplateHandlers) HandleListTemplates(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		h.logger.System().Error("EMAIL ABORT: No tenant context found", "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("handler_email_list", tenantCtx.TenantID)
	defer marker.Complete()

	templates, err := h.emailTemplateService.ListTemplates(tenantCtx.TenantID)
	if err != nil {
		h.logger.System().Error("Failed to list email templates", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list templates"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"data": templates})
}

// HandleGetTemplate fetches a specific template configuration, returning either the tenant override or the system fallback.
func (h *EmailTemplateHandlers) HandleGetTemplate(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	category := c.Param("category")
	templateName := c.Param("template")

	marker := h.perfTracker.StartOperation("handler_email_get", tenantCtx.TenantID)
	defer marker.Complete()

	template, err := h.emailTemplateService.GetTemplate(tenantCtx.TenantID, category, templateName)
	if err != nil {
		h.logger.System().Error("Failed to get email template", "error", err, "tenantId", tenantCtx.TenantID, "category", category, "template", templateName)
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"data": template})
}

// HandleSaveTemplate stores a modified email template configuration directly to the tenant's configuration directory.
func (h *EmailTemplateHandlers) HandleSaveTemplate(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	category := c.Param("category")
	templateName := c.Param("template")

	marker := h.perfTracker.StartOperation("handler_email_save", tenantCtx.TenantID)
	defer marker.Complete()

	var templateData services.EmailTemplate // Renamed to templateData to avoid shadowing the "html/template" package
	if err := c.ShouldBindJSON(&templateData); err != nil {
		h.logger.System().Warn("Invalid email template payload", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload structure"})
		return
	}

	siteURL := tenantCtx.Config.BrandConfig.SiteURL
	rawHTML, err := services.ParseEmailBlocks(templateData.Blocks, siteURL, map[string]any{})
	if err != nil {
		h.logger.System().Warn("Pre-flight validation failed: invalid blocks", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template blocks"})
		return
	}

	if _, err := template.New("validate").Parse(rawHTML); err != nil {
		h.logger.System().Warn("Pre-flight validation failed: malformed HTML/template", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "template compilation error"})
		return
	}

	if err := h.emailTemplateService.SaveTemplate(tenantCtx.TenantID, category, templateName, &templateData); err != nil {
		h.logger.System().Error("Failed to save email template", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist template"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// HandlePreviewTemplate takes an unsaved template and mock data to return a compiled HTML preview.
func (h *EmailTemplateHandlers) HandlePreviewTemplate(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	marker := h.perfTracker.StartOperation("handler_email_preview", tenantCtx.TenantID)
	defer marker.Complete()

	var req struct {
		Template services.EmailTemplate `json:"template"`
		Data     map[string]any         `json:"data"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.System().Warn("Invalid email preview payload", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload structure"})
		return
	}

	siteURL := tenantCtx.Config.BrandConfig.SiteURL
	subject, body, err := h.emailTemplateService.Compile(&req.Template, req.Data, siteURL)
	if err != nil {
		h.logger.System().Error("Failed to compile email preview", "error", err, "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compile preview"})
		return
	}

	marker.SetSuccess(true)
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"subject": subject,
			"html":    body,
		},
	})
}
