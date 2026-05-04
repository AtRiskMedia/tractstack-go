// Package services provides business logic and orchestration for the application.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/booking"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// TimeRange represents a busy interval from Google freebusy.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// GoogleCalendarService performs Google calendar/meet operations.
type GoogleCalendarService struct {
	logger        *logging.ChanneledLogger
	oauthService  *GoogleOAuthService
	configService *ConfigService
}

// NewGoogleCalendarService constructs calendar service.
func NewGoogleCalendarService(
	logger *logging.ChanneledLogger,
	oauthService *GoogleOAuthService,
	configService *ConfigService,
) *GoogleCalendarService {
	return &GoogleCalendarService{
		logger:        logger,
		oauthService:  oauthService,
		configService: configService,
	}
}

// IsConfigured indicates tenant has required oauth credentials and calendar linkage.
func (s *GoogleCalendarService) IsConfigured(tenantCtx *tenant.Context) bool {
	return tenantCtx.Config.GoogleOAuthClientID != "" &&
		tenantCtx.Config.GoogleOAuthClientSecret != "" &&
		tenantCtx.Config.GoogleCalendarID != "" &&
		tenantCtx.Config.GoogleRefreshToken != ""
}

func (s *GoogleCalendarService) ensureAccessToken(ctx context.Context, tenantCtx *tenant.Context) (string, error) {
	if !s.IsConfigured(tenantCtx) {
		return "", fmt.Errorf("google sync not configured")
	}

	needsRefresh := tenantCtx.Config.GoogleAccessToken == ""
	if !needsRefresh && tenantCtx.Config.GoogleTokenExpiry != "" {
		expiry, err := time.Parse(time.RFC3339, tenantCtx.Config.GoogleTokenExpiry)
		if err != nil || time.Now().Add(config.GoogleOAuthRefreshLead).After(expiry) {
			needsRefresh = true
		}
	}

	if !needsRefresh {
		return tenantCtx.Config.GoogleAccessToken, nil
	}

	var payload *GoogleTokenPayload
	var err error
	for attempt := 1; attempt <= config.GoogleOAuthRefreshRetries; attempt++ {
		payload, err = s.oauthService.RefreshAccessToken(ctx, tenantCtx)
		if err == nil {
			break
		}

		if strings.Contains(err.Error(), "invalid_grant") {
			tenantCtx.Config.GoogleAccessToken = ""
			tenantCtx.Config.GoogleRefreshToken = ""
			tenantCtx.Config.GoogleTokenExpiry = ""
			_ = s.configService.SaveAdvancedConfig(tenantCtx)
			return "", fmt.Errorf("google oauth disconnected: %w", err)
		}

		if attempt < config.GoogleOAuthRefreshRetries {
			backoff := float64(config.GoogleOAuthRefreshBackoffBase) * math.Pow(2, float64(attempt-1))
			if time.Duration(backoff) > config.GoogleOAuthRefreshBackoffMax {
				backoff = float64(config.GoogleOAuthRefreshBackoffMax)
			}
			time.Sleep(time.Duration(backoff))
		}
	}
	if err != nil {
		return "", err
	}

	expiry := time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UTC()
	tenantCtx.Config.GoogleAccessToken = payload.AccessToken
	tenantCtx.Config.GoogleRefreshToken = payload.RefreshToken
	tenantCtx.Config.GoogleTokenExpiry = expiry.Format(time.RFC3339)
	if saveErr := s.configService.SaveAdvancedConfig(tenantCtx); saveErr != nil {
		s.logger.System().Warn("Failed to persist refreshed google token", "error", saveErr, "tenantId", tenantCtx.TenantID)
	}

	return payload.AccessToken, nil
}

func (s *GoogleCalendarService) doJSON(
	ctx context.Context,
	tenantCtx *tenant.Context,
	method string,
	endpoint string,
	requestBody any,
	responseBody any,
) error {
	accessToken, err := s.ensureAccessToken(ctx, tenantCtx)
	if err != nil {
		return err
	}

	var body io.Reader
	if requestBody != nil {
		data, marshalErr := json.Marshal(requestBody)
		if marshalErr != nil {
			return marshalErr
		}
		body = bytes.NewBuffer(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: config.GoogleCalendarHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("google calendar api error: %s", string(raw))
	}
	if responseBody != nil {
		if err := json.Unmarshal(raw, responseBody); err != nil {
			return err
		}
	}
	return nil
}

// GetBusyRanges fetches Google busy windows for additive availability blocking.
func (s *GoogleCalendarService) GetBusyRanges(ctx context.Context, tenantCtx *tenant.Context, start, end time.Time) ([]TimeRange, error) {
	if !s.IsConfigured(tenantCtx) {
		return nil, nil
	}

	req := map[string]any{
		"timeMin": start.UTC().Format(time.RFC3339),
		"timeMax": end.UTC().Format(time.RFC3339),
		"items": []map[string]string{
			{"id": tenantCtx.Config.GoogleCalendarID},
		},
	}
	var resp struct {
		Calendars map[string]struct {
			Busy []struct {
				Start string `json:"start"`
				End   string `json:"end"`
			} `json:"busy"`
		} `json:"calendars"`
	}

	if err := s.doJSON(ctx, tenantCtx, http.MethodPost, "https://www.googleapis.com/calendar/v3/freeBusy", req, &resp); err != nil {
		return nil, err
	}

	calendar := resp.Calendars[tenantCtx.Config.GoogleCalendarID]
	out := make([]TimeRange, 0, len(calendar.Busy))
	for _, busy := range calendar.Busy {
		startAt, err1 := time.Parse(time.RFC3339, busy.Start)
		endAt, err2 := time.Parse(time.RFC3339, busy.End)
		if err1 == nil && err2 == nil {
			out = append(out, TimeRange{Start: startAt, End: endAt})
		}
	}
	return out, nil
}

// CreateBookingEvent creates a Google calendar event and optional Meet link.
// summary and description are caller-built calendar copy for the business owner.
func (s *GoogleCalendarService) CreateBookingEvent(ctx context.Context, tenantCtx *tenant.Context, b *booking.Booking, summary, description string) (*string, *string, error) {
	if !s.IsConfigured(tenantCtx) {
		return nil, nil, fmt.Errorf("google sync not configured")
	}

	payload := map[string]any{
		"summary":     summary,
		"description": description,
		"start": map[string]string{
			"dateTime": b.StartTime.UTC().Format(time.RFC3339),
		},
		"end": map[string]string{
			"dateTime": b.EndTime.UTC().Format(time.RFC3339),
		},
		"extendedProperties": map[string]any{
			"private": map[string]string{
				"bookingId": b.ID,
			},
		},
	}
	if b.ShopifyOrderID != nil {
		payload["extendedProperties"].(map[string]any)["private"].(map[string]string)["shopifyOrderId"] = *b.ShopifyOrderID
	}
	endpoint := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/%s/events",
		url.PathEscape(tenantCtx.Config.GoogleCalendarID),
	)
	if b.AppointmentMode == booking.AppointmentModeRemote {
		payload["conferenceData"] = map[string]any{
			"createRequest": map[string]any{
				"requestId": b.ID,
				"conferenceSolutionKey": map[string]string{
					"type": "hangoutsMeet",
				},
			},
		}
		endpoint += "?conferenceDataVersion=1"
	}

	var resp struct {
		ID             string `json:"id"`
		HangoutLink    string `json:"hangoutLink"`
		ConferenceData struct {
			EntryPoints []struct {
				EntryPointType string `json:"entryPointType"`
				URI            string `json:"uri"`
			} `json:"entryPoints"`
		} `json:"conferenceData"`
	}
	if err := s.doJSON(ctx, tenantCtx, http.MethodPost, endpoint, payload, &resp); err != nil {
		return nil, nil, err
	}

	var meetURL *string
	if resp.HangoutLink != "" {
		meetURL = &resp.HangoutLink
	} else {
		for _, entry := range resp.ConferenceData.EntryPoints {
			if entry.EntryPointType == "video" && entry.URI != "" {
				meetURL = &entry.URI
				break
			}
		}
	}
	if resp.ID == "" {
		return nil, meetURL, fmt.Errorf("google event create returned empty id")
	}
	return &resp.ID, meetURL, nil
}

// DeleteEvent deletes google event by explicit event ID.
func (s *GoogleCalendarService) DeleteEvent(ctx context.Context, tenantCtx *tenant.Context, eventID string) error {
	if !s.IsConfigured(tenantCtx) {
		return fmt.Errorf("google sync not configured")
	}
	endpoint := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/%s/events/%s",
		url.PathEscape(tenantCtx.Config.GoogleCalendarID),
		url.PathEscape(eventID),
	)
	return s.doJSON(ctx, tenantCtx, http.MethodDelete, endpoint, nil, nil)
}

// FindEventIDByBookingID resolves google event via booking id extended property.
func (s *GoogleCalendarService) FindEventIDByBookingID(ctx context.Context, tenantCtx *tenant.Context, bookingID string) (*string, error) {
	if !s.IsConfigured(tenantCtx) {
		return nil, fmt.Errorf("google sync not configured")
	}
	query := url.Values{}
	query.Set("privateExtendedProperty", "bookingId="+bookingID)
	query.Set("singleEvents", "true")
	query.Set("maxResults", "1")

	endpoint := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/%s/events?%s",
		url.PathEscape(tenantCtx.Config.GoogleCalendarID),
		query.Encode(),
	)

	var resp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := s.doJSON(ctx, tenantCtx, http.MethodGet, endpoint, nil, &resp); err != nil {
		return nil, err
	}
	if len(resp.Items) == 0 || resp.Items[0].ID == "" {
		return nil, nil
	}
	return &resp.Items[0].ID, nil
}
