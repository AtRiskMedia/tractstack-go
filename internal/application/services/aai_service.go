// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AssemblyAI/assemblyai-go-sdk"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// AAIService provides a reusable service for interacting with the AssemblyAI API.
type AAIService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewAAIService creates a new AAI service singleton.
func NewAAIService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *AAIService {
	return &AAIService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// AskLemurRequest defines the parameters for a LeMUR task.
type AskLemurRequest struct {
	Prompt      string
	InputText   string
	FinalModel  string
	MaxTokens   int
	Temperature float64
}

// AskLemur performs a task using the AssemblyAI LeMUR API.
func (s *AAIService) AskLemur(tenantCtx *tenant.Context, request AskLemurRequest) (any, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("aai_service_ask_lemur", tenantCtx.TenantID)
	defer marker.Complete()
	s.logger.System().Debug("Executing AskLemur task", "tenantId", tenantCtx.TenantID)

	if tenantCtx.Config.AAIAPIKey == "" {
		s.logger.System().Warn("AAI API key not configured", "tenantId", tenantCtx.TenantID)
		return nil, fmt.Errorf("assembly AI API key not configured")
	}

	if request.InputText == "" || request.InputText == "..." {
		return nil, fmt.Errorf("input text is required")
	}

	// Set defaults
	if request.FinalModel == "" {
		request.FinalModel = "anthropic/claude-3-5-sonnet"
	}
	if request.MaxTokens == 0 {
		request.MaxTokens = 4000
	}
	if request.Temperature == 0 {
		request.Temperature = 0.0
	}

	client := assemblyai.NewClient(tenantCtx.Config.AAIAPIKey)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var lemurRequest assemblyai.LeMURTaskParams
	lemurRequest.Prompt = assemblyai.String(request.Prompt)
	lemurRequest.InputText = assemblyai.String(request.InputText)
	lemurRequest.FinalModel = assemblyai.LeMURModel(request.FinalModel)
	lemurRequest.MaxOutputSize = assemblyai.Int64(int64(request.MaxTokens))
	lemurRequest.Temperature = assemblyai.Float64(request.Temperature)

	s.logger.System().Debug("Calling Assembly AI LeMUR API", "tenantId", tenantCtx.TenantID, "model", request.FinalModel)
	response, err := client.LeMUR.Task(ctx, lemurRequest)
	if err != nil {
		s.logger.System().Error("Assembly AI LeMUR API call failed", "tenantId", tenantCtx.TenantID, "error", err.Error(), "duration", time.Since(start))
		return nil, fmt.Errorf("assembly AI API call failed: %w", err)
	}

	var parsedResponse any
	if response.Response != nil && *response.Response != "" {
		if err := json.Unmarshal([]byte(*response.Response), &parsedResponse); err != nil {
			parsedResponse = *response.Response
		}
	} else {
		parsedResponse = ""
	}

	s.logger.System().Info("Assembly AI LeMUR API call successful", "tenantId", tenantCtx.TenantID, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for AskLemur service call", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return parsedResponse, nil
}
