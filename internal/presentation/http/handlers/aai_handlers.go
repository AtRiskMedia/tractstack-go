// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// AAIHandlers contains all Assembly AI-related HTTP handlers
type AAIHandlers struct {
	aaiService  *services.AAIService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewAAIHandlers creates AAI handlers with injected dependencies
func NewAAIHandlers(aaiService *services.AAIService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *AAIHandlers {
	return &AAIHandlers{
		aaiService:  aaiService,
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// AaiRequest represents the request structure for LeMUR API calls
type AaiRequest struct {
	Prompt      string  `json:"prompt" binding:"required"`
	InputText   string  `json:"input_text" binding:"required"`
	FinalModel  string  `json:"final_model,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// AaiResponse represents the response structure for LeMUR API calls
type AaiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// PostAai handles POST /api/v1/auth/aai/aai - calls Assembly AI LeMUR API via the AAIService
func (h *AAIHandlers) PostAai(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_ask_lemur_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received ask LeMUR request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	var req AaiRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.System().Warn("Invalid ask LeMUR request", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusBadRequest, AaiResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	serviceRequest := services.AaiRequest{
		Prompt:      req.Prompt,
		InputText:   req.InputText,
		FinalModel:  req.FinalModel,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	response, err := h.aaiService.Aai(tenantCtx, serviceRequest)
	if err != nil {
		h.logger.System().Error("AAI service call failed", "tenantId", tenantCtx.TenantID, "error", err.Error(), "duration", time.Since(start))
		c.JSON(http.StatusInternalServerError, AaiResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.logger.System().Info("AAI service call successful", "tenantId", tenantCtx.TenantID, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostAai request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, AaiResponse{
		Success: true,
		Data: gin.H{
			"response": response,
		},
	})
}
