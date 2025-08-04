// Package handlers provides HTTP request handlers for the presentation layer.
package handlers

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// VisitHandlers contains all visit and session-related HTTP handlers
type VisitHandlers struct {
	sessionService *services.SessionService
	authService    *services.AuthService
	logger         *logging.ChanneledLogger
	perfTracker    *performance.Tracker
}

// ProfileRequest represents the structure for profile requests
type ProfileRequest struct {
	SessionID      *string `json:"sessionId,omitempty"`
	EncryptedEmail *string `json:"encryptedEmail,omitempty"`
	EncryptedCode  *string `json:"encryptedCode,omitempty"`
	Email          string  `json:"email,omitempty"`
	Codeword       string  `json:"codeword,omitempty"`
	FirstName      string  `json:"firstName,omitempty"`
	ContactPersona string  `json:"contactPersona,omitempty"`
	ShortBio       string  `json:"shortBio,omitempty"`
}

// ProfileResponse represents the response structure for profile requests
type ProfileResponse struct {
	Success        bool          `json:"success"`
	Profile        *user.Profile `json:"profile,omitempty"`
	Token          string        `json:"token,omitempty"`
	EncryptedEmail string        `json:"encryptedEmail,omitempty"`
	EncryptedCode  string        `json:"encryptedCode,omitempty"`
	Fingerprint    string        `json:"fingerprint,omitempty"`
	VisitID        string        `json:"visitId,omitempty"`
	HasProfile     bool          `json:"hasProfile"`
	Consent        string        `json:"consent,omitempty"`
	Error          string        `json:"error,omitempty"`
}

// VisitResponse represents the response structure for visit requests
type VisitResponse struct {
	Fingerprint string        `json:"fingerprint"`
	VisitID     string        `json:"visitId"`
	HasProfile  bool          `json:"hasProfile"`
	Profile     *user.Profile `json:"profile,omitempty"`
	Token       string        `json:"token,omitempty"`
	Consent     string        `json:"consent"`
}

// SSEMessage represents the structure for SSE messages
type SSEMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId,omitempty"`
	Data      any    `json:"data,omitempty"`
	Timestamp string `json:"timestamp"`
}

// NewVisitHandlers creates visit handlers with injected dependencies
func NewVisitHandlers(sessionService *services.SessionService, authService *services.AuthService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *VisitHandlers {
	return &VisitHandlers{
		sessionService: sessionService,
		authService:    authService,
		logger:         logger,
		perfTracker:    perfTracker,
	}
}

// Global SSE connection tracking
var (
	activeSSEConnections int64
	maxSSEConnections    = int64(1000) // Default max connections
)

// safeSSEConnection wraps SSE channel with safe closing
type safeSSEConnection struct {
	ch     chan string
	closed int32
}

func (sc *safeSSEConnection) SafeClose() bool {
	if atomic.CompareAndSwapInt32(&sc.closed, 0, 1) {
		close(sc.ch)
		return true
	}
	return false
}

// PostVisit handles POST /api/v1/auth/visit - creates/updates visits and sessions
func (h *VisitHandlers) PostVisit(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_visit_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Auth().Debug("Received post visit request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Parse visit request
	var req services.VisitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Auth().Error("Visit request JSON binding failed", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	// Log the request details for debugging
	h.logger.Auth().Debug("Processing visit request",
		"tenantId", tenantCtx.TenantID,
		"hasSessionId", req.SessionID != nil,
		"hasEncryptedEmail", req.EncryptedEmail != nil,
		"hasEncryptedCode", req.EncryptedCode != nil,
		"hasConsent", req.Consent != nil)

	// Use session service to process the visit request
	result := h.sessionService.ProcessVisitRequest(&req, tenantCtx)

	// Handle the result
	if !result.Success {
		h.logger.Auth().Error("Visit processing failed",
			"tenantId", tenantCtx.TenantID,
			"error", result.Error,
			"duration", time.Since(start))
		marker.SetSuccess(false)
		h.logger.Perf().Info("Performance for PostVisit request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", false)

		// Return appropriate HTTP status based on error type
		switch result.Error {
		case "session ID required":
			c.JSON(http.StatusBadRequest, gin.H{"error": result.Error})
		case "invalid credentials":
			c.JSON(http.StatusUnauthorized, gin.H{"error": result.Error})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error})
		}
		return
	}

	// Build response
	response := gin.H{
		"fingerprint": result.FingerprintID,
		"visitId":     result.VisitID,
		"hasProfile":  result.HasProfile,
		"consent":     result.Consent,
	}

	// Add profile and token if present
	if result.HasProfile && result.Profile != nil {
		response["profile"] = result.Profile
		if result.Token != "" {
			response["token"] = result.Token
		}
	}

	h.logger.Auth().Info("Visit processing completed",
		"tenantId", tenantCtx.TenantID,
		"fingerprintId", result.FingerprintID,
		"visitId", result.VisitID,
		"hasProfile", result.HasProfile,
		"duration", time.Since(start))

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for PostVisit request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	c.JSON(http.StatusOK, response)
}

// GetSSE handles GET /api/v1/auth/sse - establishes Server-Sent Events connection
func (h *VisitHandlers) GetSSE(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("get_sse_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.SSE().Debug("Received SSE connection request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	sessionID := c.Query("sessionId")
	if sessionID == "" {
		h.logger.SSE().Error("SSE connection request missing session ID", "tenantId", tenantCtx.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID required for SSE connection"})
		return
	}

	// Check connection limits
	currentConnections := atomic.LoadInt64(&activeSSEConnections)
	if currentConnections >= maxSSEConnections {
		h.logger.SSE().Warn("SSE connection limit reached",
			"tenantId", tenantCtx.TenantID,
			"sessionId", sessionID,
			"currentConnections", currentConnections,
			"maxConnections", maxSSEConnections)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "SSE connection limit reached. Please try again later.",
		})
		return
	}

	h.logger.SSE().Debug("Starting SSE connection setup",
		"tenantId", tenantCtx.TenantID,
		"sessionId", sessionID,
		"currentConnections", currentConnections)

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// Create SSE connection
	connection := &safeSSEConnection{
		ch: make(chan string, 100), // Buffered channel
	}

	// Increment connection count
	atomic.AddInt64(&activeSSEConnections, 1)
	defer func() {
		atomic.AddInt64(&activeSSEConnections, -1)
		connection.SafeClose()
	}()

	// Send initial connection confirmation
	select {
	case connection.ch <- fmt.Sprintf("data: {\"type\":\"connected\",\"sessionId\":\"%s\",\"timestamp\":\"%s\"}\n\n", sessionID, time.Now().Format(time.RFC3339)):
	default:
		// Channel full, connection likely dead
		h.logger.SSE().Warn("SSE initial message failed - channel full", "tenantId", tenantCtx.TenantID, "sessionId", sessionID)
		return
	}

	// Get client context for connection management
	clientCtx := c.Request.Context()

	// Register connection with broadcaster (if you have one)
	// This would typically register the connection.ch with your SSE broadcaster

	h.logger.SSE().Info("SSE connection established",
		"tenantId", tenantCtx.TenantID,
		"sessionId", sessionID,
		"totalConnections", atomic.LoadInt64(&activeSSEConnections),
		"setupDuration", time.Since(start))

	marker.SetSuccess(true)
	h.logger.Perf().Info("Performance for GetSSE request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

	// Keep connection alive and handle messages
	ticker := time.NewTicker(30 * time.Second) // Heartbeat every 30 seconds
	defer ticker.Stop()

	connectionStart := time.Now()
	for {
		select {
		case <-clientCtx.Done():
			// Client disconnected
			h.logger.SSE().Info("SSE client disconnected",
				"tenantId", tenantCtx.TenantID,
				"sessionId", sessionID,
				"connectionDuration", time.Since(connectionStart))
			return

		case message, ok := <-connection.ch:
			if !ok {
				// Channel closed
				h.logger.SSE().Info("SSE connection channel closed",
					"tenantId", tenantCtx.TenantID,
					"sessionId", sessionID,
					"connectionDuration", time.Since(connectionStart))
				return
			}

			// Send message to client
			if _, err := c.Writer.WriteString(message); err != nil {
				h.logger.SSE().Error("SSE write failed",
					"tenantId", tenantCtx.TenantID,
					"sessionId", sessionID,
					"error", err.Error())
				return
			}
			c.Writer.Flush()

		case <-ticker.C:
			// Send heartbeat
			heartbeat := fmt.Sprintf("data: {\"type\":\"heartbeat\",\"timestamp\":\"%s\"}\n\n", time.Now().Format(time.RFC3339))
			if _, err := c.Writer.WriteString(heartbeat); err != nil {
				h.logger.SSE().Error("SSE heartbeat failed",
					"tenantId", tenantCtx.TenantID,
					"sessionId", sessionID,
					"error", err.Error())
				return
			}
			c.Writer.Flush()
		}
	}
}

// PostProfile handles POST /api/v1/auth/profile - creates/updates user profiles
func (h *VisitHandlers) PostProfile(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}

	start := time.Now()
	marker := h.perfTracker.StartOperation("post_profile_request", tenantCtx.TenantID)
	defer marker.Complete()
	h.logger.Auth().Debug("Received post profile request", "method", c.Request.Method, "path", c.Request.URL.Path, "tenantId", tenantCtx.TenantID)

	// Parse profile request
	var req ProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Auth().Error("Profile request JSON binding failed", "tenantId", tenantCtx.TenantID, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate required fields
	if req.SessionID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID is required"})
		return
	}

	h.logger.Auth().Debug("Processing profile request",
		"tenantId", tenantCtx.TenantID,
		"sessionId", *req.SessionID,
		"hasEncryptedEmail", req.EncryptedEmail != nil,
		"hasEncryptedCode", req.EncryptedCode != nil,
		"isUpdate", req.Email != "")

	// Handle profile validation/authentication with encrypted credentials
	if req.EncryptedEmail != nil && req.EncryptedCode != nil {
		result := h.handleProfileValidation(&req, tenantCtx)

		if !result.Success {
			h.logger.Auth().Error("Profile validation failed",
				"tenantId", tenantCtx.TenantID,
				"sessionId", *req.SessionID,
				"error", result.Error,
				"duration", time.Since(start))
			marker.SetSuccess(false)

			if result.Error == "Invalid credentials" {
				c.JSON(http.StatusUnauthorized, result)
			} else {
				c.JSON(http.StatusInternalServerError, result)
			}
			return
		}

		h.logger.Auth().Info("Profile validation completed",
			"tenantId", tenantCtx.TenantID,
			"sessionId", *req.SessionID,
			"duration", time.Since(start))
		marker.SetSuccess(true)
		h.logger.Perf().Info("Performance for PostProfile request", "duration", marker.Duration, "tenantId", tenantCtx.TenantID, "success", true)

		c.JSON(http.StatusOK, result)
		return
	}

	// Handle profile creation/update (this would require more complex logic)
	// For now, return not implemented
	h.logger.Auth().Warn("Profile creation/update not yet implemented", "tenantId", tenantCtx.TenantID, "sessionId", *req.SessionID)
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Profile creation/update not yet implemented"})
}

// validateEncryptedCredentials validates encrypted email and code
func (h *VisitHandlers) validateEncryptedCredentials(encryptedEmail, encryptedCode string, tenantCtx *tenant.Context) *user.Profile {
	return h.authService.ValidateEncryptedCredentials(encryptedEmail, encryptedCode, tenantCtx)
}

// handleProfileValidation handles profile validation with encrypted credentials
func (h *VisitHandlers) handleProfileValidation(req *ProfileRequest, tenantCtx *tenant.Context) *ProfileResponse {
	// Use auth service to validate encrypted credentials
	profile := h.validateEncryptedCredentials(*req.EncryptedEmail, *req.EncryptedCode, tenantCtx)
	if profile == nil {
		return &ProfileResponse{
			Success: false,
			Error:   "Invalid credentials",
		}
	}

	// Handle session creation/update
	sessionResponse, err := h.sessionService.HandleProfileSession(tenantCtx, profile, *req.SessionID)
	if err != nil {
		return &ProfileResponse{
			Success: false,
			Error:   fmt.Sprintf("Session handling failed: %v", err),
		}
	}

	// Generate JWT token
	token, err := h.authService.GenerateJWT(map[string]any{
		"leadId":         profile.LeadID,
		"fingerprint":    profile.Fingerprint,
		"email":          profile.Email,
		"firstName":      profile.Firstname,
		"contactPersona": profile.ContactPersona,
		"type":           "profile",
		"exp":            time.Now().Add(7 * 24 * time.Hour).Unix(), // 7 days
		"iat":            time.Now().Unix(),
	}, tenantCtx.Config.JWTSecret)
	if err != nil {
		return &ProfileResponse{
			Success: false,
			Error:   "Token generation failed",
		}
	}

	return &ProfileResponse{
		Success:        true,
		Profile:        profile,
		Token:          token,
		EncryptedEmail: *req.EncryptedEmail,
		EncryptedCode:  *req.EncryptedCode,
		Fingerprint:    sessionResponse.Fingerprint,
		VisitID:        sessionResponse.VisitID,
		HasProfile:     true,
		Consent:        "1",
	}
}
