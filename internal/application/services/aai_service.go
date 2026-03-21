// Package services provides application-level services that orchestrate
// business logic and coordinate between repositories and domain entities.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

type AAIService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

func NewAAIService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *AAIService {
	return &AAIService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

type AaiRequest struct {
	Prompt      string
	InputText   string
	FinalModel  string
	MaxTokens   int
	Temperature float64
}

type aaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aaiPayload struct {
	Model       string       `json:"model"`
	Messages    []aaiMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens"`
	Temperature float64      `json:"temperature"`
}

type aaiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (s *AAIService) Aai(tenantCtx *tenant.Context, request AaiRequest) (any, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("aai_service_aai", tenantCtx.TenantID)
	defer marker.Complete()
	s.logger.System().Debug("Executing AAI task", "tenantId", tenantCtx.TenantID)

	if tenantCtx.Config.AAIAPIKey == "" {
		s.logger.System().Warn("AAI API key not configured", "tenantId", tenantCtx.TenantID)
		return nil, fmt.Errorf("assembly AI API key not configured")
	}

	if request.InputText == "" || request.InputText == "..." {
		return nil, fmt.Errorf("input text is required")
	}

	if request.FinalModel == "" {
		request.FinalModel = "claude-sonnet-4-6"
	}
	if request.MaxTokens == 0 {
		request.MaxTokens = 4000
	}
	if request.Temperature == 0 {
		request.Temperature = 0.0
	}

	payload := aaiPayload{
		Model: request.FinalModel,
		Messages: []aaiMessage{
			{
				Role:    "user",
				Content: fmt.Sprintf("%s\n\n%s", request.Prompt, request.InputText),
			},
		},
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal AAI request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://llm-gateway.assemblyai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Authorization", tenantCtx.Config.AAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	s.logger.System().Debug("Calling Assembly AI LLM Gateway", "tenantId", tenantCtx.TenantID, "model", request.FinalModel)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.System().Error("Assembly AI LLM Gateway call failed", "tenantId", tenantCtx.TenantID, "error", err.Error(), "duration", time.Since(start))
		return nil, fmt.Errorf("assembly AI API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		s.logger.System().Error("Assembly AI API returned non-200", "status", resp.Status, "body", string(respBody))
		return nil, fmt.Errorf("assembly AI API returned status: %s", resp.Status)
	}

	var gatewayResp aaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gatewayResp); err != nil {
		return nil, fmt.Errorf("failed to decode gateway response: %w", err)
	}

	var responseContent string
	if len(gatewayResp.Choices) > 0 {
		responseContent = gatewayResp.Choices[0].Message.Content
	}

	var parsedResponse any
	if responseContent != "" {
		if err := json.Unmarshal([]byte(responseContent), &parsedResponse); err != nil {
			parsedResponse = responseContent
		}
	} else {
		parsedResponse = ""
	}

	s.logger.System().Info("Assembly AI LLM Gateway call successful", "tenantId", tenantCtx.TenantID, "duration", time.Since(start))
	marker.SetSuccess(true)
	s.logger.Perf().Info("Performance for AAI service call", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	return parsedResponse, nil
}
