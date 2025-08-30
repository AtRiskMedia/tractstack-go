// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/AssemblyAI/assemblyai-go-sdk"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// AAIHandlers contains all Assembly AI-related HTTP handlers
type AAIHandlers struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewAAIHandlers creates AAI handlers with injected dependencies
func NewAAIHandlers(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *AAIHandlers {
	return &AAIHandlers{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// AskLemurRequest represents the request structure for LeMUR API calls
type AskLemurRequest struct {
	Prompt      string  `json:"prompt" binding:"required"`
	InputText   string  `json:"input_text" binding:"required"`
	FinalModel  string  `json:"final_model,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// AskLemurResponse represents the response structure for LeMUR API calls
type AskLemurResponse struct {
	Success bool       `json:"success"`
	Data    *LemurData `json:"data,omitempty"`
	Error   string     `json:"error,omitempty"`
}

// LemurData represents the data structure from Assembly AI LeMUR response
type LemurData struct {
	Response any `json:"response"`
}

// PostAskLemur handles POST /api/v1/auth/aai/askLemur - calls Assembly AI LeMUR API
func (h *AAIHandlers) PostAskLemur(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_ask_lemur_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.System().Debug("Received ask LeMUR request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Check if AAI API key is configured
	if tenantCtx.Config.AAIAPIKey == "" {
		h.logger.System().Warn("AAI API key not configured", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusServiceUnavailable, AskLemurResponse{
			Success: false,
			Error:   "Assembly AI API key not configured",
		})
		return
	}

	// Parse request body
	var req AskLemurRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.System().Warn("Invalid ask LeMUR request", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusBadRequest, AskLemurResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	// Validate input text
	if req.InputText == "" || req.InputText == "..." {
		h.logger.System().Warn("Empty input text provided", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, AskLemurResponse{
			Success: false,
			Error:   "Input text is required",
		})
		return
	}

	// Set defaults
	if req.FinalModel == "" {
		req.FinalModel = "anthropic/claude-3-5-sonnet"
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4000
	}
	if req.Temperature == 0 {
		req.Temperature = 0.0
	}

	// Initialize Assembly AI client
	client := assemblyai.NewClient(tenantCtx.Config.AAIAPIKey)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Prepare LeMUR request using the correct Go SDK API
	var lemurRequest assemblyai.LeMURTaskParams
	lemurRequest.Prompt = assemblyai.String(req.Prompt)
	lemurRequest.InputText = assemblyai.String(req.InputText)
	lemurRequest.FinalModel = assemblyai.LeMURModel(req.FinalModel)
	lemurRequest.MaxOutputSize = assemblyai.Int64(int64(req.MaxTokens))
	lemurRequest.Temperature = assemblyai.Float64(req.Temperature)

	h.logger.System().Debug("Calling Assembly AI LeMUR API", "tenantId", tenantCtx.TenantID, "model", req.FinalModel)

	// Call Assembly AI LeMUR API
	response, err := client.LeMUR.Task(ctx, lemurRequest)
	if err != nil {
		h.logger.System().Error("Assembly AI LeMUR API call failed", "tenantId", tenantCtx.TenantID, "error", err.Error(), "duration", time.Since(start))
		c.JSON(http.StatusInternalServerError, AskLemurResponse{
			Success: false,
			Error:   "Assembly AI API call failed",
		})
		return
	}

	// Parse the response - it might be a JSON string that needs parsing
	var parsedResponse any
	if response.Response != nil && *response.Response != "" {
		// Try to parse as JSON first
		if err := json.Unmarshal([]byte(*response.Response), &parsedResponse); err != nil {
			// If JSON parsing fails, use the raw string
			parsedResponse = *response.Response
		}
	} else {
		parsedResponse = ""
	}

	h.logger.System().Info("Assembly AI LeMUR API call successful", "tenantId", tenantCtx.TenantID, "duration", time.Since(start))
	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostAskLemur request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, AskLemurResponse{
		Success: true,
		Data: &LemurData{
			Response: parsedResponse,
		},
	})
}
