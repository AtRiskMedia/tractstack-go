// Package services provides business logic and orchestration for the application.
package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

const googleCalendarEventsScope = "https://www.googleapis.com/auth/calendar.events"

type oauthStateRecord struct {
	TenantID  string
	ExpiresAt time.Time
	Used      bool
}

// GoogleTokenPayload captures oauth token response fields needed by the system.
type GoogleTokenPayload struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// GoogleOAuthService encapsulates OAuth start/callback token exchange logic.
type GoogleOAuthService struct {
	logger   *logging.ChanneledLogger
	mu       sync.Mutex
	stateMap map[string]oauthStateRecord
}

// NewGoogleOAuthService constructs oauth service.
func NewGoogleOAuthService(logger *logging.ChanneledLogger) *GoogleOAuthService {
	return &GoogleOAuthService{
		logger:   logger,
		stateMap: map[string]oauthStateRecord{},
	}
}

// GenerateState creates single-use state bound to tenant with TTL.
func (s *GoogleOAuthService) GenerateState(tenantID string) (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate oauth state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(randomBytes)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateMap[state] = oauthStateRecord{
		TenantID:  tenantID,
		ExpiresAt: time.Now().Add(config.GoogleOAuthStateTTL),
		Used:      false,
	}
	return state, nil
}

// ValidateAndConsumeState enforces single-use + ttl.
func (s *GoogleOAuthService) ValidateAndConsumeState(tenantID, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.stateMap[state]
	if !ok {
		return fmt.Errorf("invalid oauth state")
	}
	if record.Used {
		return fmt.Errorf("oauth state already consumed")
	}
	if time.Now().After(record.ExpiresAt) {
		delete(s.stateMap, state)
		return fmt.Errorf("oauth state expired")
	}
	if record.TenantID != tenantID {
		return fmt.Errorf("oauth state tenant mismatch")
	}

	record.Used = true
	s.stateMap[state] = record
	return nil
}

// BuildAuthorizeURL builds Google oauth consent URL.
func (s *GoogleOAuthService) BuildAuthorizeURL(tenantCtx *tenant.Context, redirectURI, state string) (string, error) {
	clientID := tenantCtx.Config.GoogleOAuthClientID
	if clientID == "" {
		return "", fmt.Errorf("google oauth client id not configured")
	}

	query := url.Values{}
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", googleCalendarEventsScope)
	query.Set("access_type", "offline")
	query.Set("include_granted_scopes", "true")
	query.Set("prompt", "consent")
	query.Set("state", state)

	return "https://accounts.google.com/o/oauth2/v2/auth?" + query.Encode(), nil
}

// ExchangeCode exchanges authorization code for token payload.
func (s *GoogleOAuthService) ExchangeCode(ctx context.Context, tenantCtx *tenant.Context, code, redirectURI string) (*GoogleTokenPayload, error) {
	client := &http.Client{Timeout: config.GoogleOAuthHTTPTimeout}
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", tenantCtx.Config.GoogleOAuthClientID)
	form.Set("client_secret", tenantCtx.Config.GoogleOAuthClientSecret)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create oauth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth token exchange failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read oauth response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("oauth token exchange failed: %s", string(body))
	}

	var payload GoogleTokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to decode oauth response: %w", err)
	}
	if payload.AccessToken == "" {
		return nil, fmt.Errorf("oauth response missing access token")
	}
	if payload.Scope != "" && payload.Scope != googleCalendarEventsScope {
		s.logger.System().Warn("OAuth scope differs from expected", "scope", payload.Scope)
	}
	return &payload, nil
}

// RefreshAccessToken refreshes Google access token using refresh token.
func (s *GoogleOAuthService) RefreshAccessToken(ctx context.Context, tenantCtx *tenant.Context) (*GoogleTokenPayload, error) {
	client := &http.Client{Timeout: config.GoogleOAuthHTTPTimeout}
	form := url.Values{}
	form.Set("client_id", tenantCtx.Config.GoogleOAuthClientID)
	form.Set("client_secret", tenantCtx.Config.GoogleOAuthClientSecret)
	form.Set("refresh_token", tenantCtx.Config.GoogleRefreshToken)
	form.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create oauth refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth refresh failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read oauth refresh response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("oauth refresh failed: %s", string(body))
	}

	var payload GoogleTokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to decode oauth refresh payload: %w", err)
	}
	if payload.AccessToken == "" {
		return nil, fmt.Errorf("oauth refresh missing access token")
	}
	if payload.RefreshToken == "" {
		payload.RefreshToken = tenantCtx.Config.GoogleRefreshToken
	}
	return &payload, nil
}
