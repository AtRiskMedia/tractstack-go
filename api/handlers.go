// Package api provides HTTP handlers and database connectivity for the application's API.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// getTenantContext is a helper to extract tenant context from gin context
func getTenantContext(c *gin.Context) (*tenant.Context, error) {
	tenantCtx, exists := c.Get("tenant")
	if !exists {
		return nil, fmt.Errorf("no tenant context")
	}
	return tenantCtx.(*tenant.Context), nil
}

// getTenantManager is a helper to extract tenant manager from gin context
func getTenantManager(c *gin.Context) (*tenant.Manager, error) {
	manager, exists := c.Get("tenantManager")
	if !exists {
		return nil, fmt.Errorf("no tenant manager")
	}
	return manager.(*tenant.Manager), nil
}

// generateULID creates a new ULID
func generateULID() string {
	return ulid.Make().String()
}

// DBStatusHandler checks tenant status and activates if needed
func DBStatusHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Only activate if status is inactive
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}

		// Update detector's cached registry after successful activation
		manager, err := getTenantManager(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Determine database type for cache update
		dbType := "sqlite3"
		if ctx.Database.UseTurso {
			dbType = "turso"
		}

		manager.GetDetector().UpdateTenantStatus(ctx.TenantID, "active", dbType)
	}

	// If we reach here and status is active, tables are guaranteed to exist
	allTablesExist := (ctx.Status == "active")

	c.JSON(http.StatusOK, gin.H{
		"tenantId":       ctx.TenantID,
		"status":         ctx.Status,
		"database":       ctx.Database.GetConnectionInfo(),
		"allTablesExist": allTablesExist,
	})
}

func VisitHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Log the raw request for debugging
	body, _ := c.GetRawData()

	// Reset the body for binding
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// Parse form data instead of JSON
	var req models.VisitRequest
	if c.GetHeader("Content-Type") == "application/x-www-form-urlencoded" {
		// Parse form data
		encryptedEmail := c.PostForm("encryptedEmail")
		encryptedCode := c.PostForm("encryptedCode")
		consent := c.PostForm("consent")
		sessionId := c.PostForm("sessionId")

		if encryptedEmail != "" {
			req.EncryptedEmail = &encryptedEmail
		}
		if encryptedCode != "" {
			req.EncryptedCode = &encryptedCode
		}
		if consent != "" {
			req.Consent = &consent
		}
		if sessionId != "" {
			req.SessionID = &sessionId
		}
	} else {
		// Try JSON binding for other content types
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
			return
		}
	}

	// Check for existing JWT token instead of cookies
	authHeader := c.GetHeader("Authorization")
	var profile *models.Profile
	if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token := authHeader[7:]
		claims, err := utils.ValidateJWT(token, ctx.Config.JWTSecret)
		if err == nil {
			profile = utils.GetProfileFromClaims(claims)
		}
	}

	// Try encrypted credentials if no JWT profile
	if profile == nil && req.EncryptedEmail != nil && req.EncryptedCode != nil {
		profile = validateEncryptedCredentials(*req.EncryptedEmail, *req.EncryptedCode, ctx)
	}

	hasProfile := profile != nil
	consentValue := ""
	if hasProfile {
		consentValue = "1"
	} else if req.Consent != nil {
		consentValue = *req.Consent
	}

	// Session ID is required for secure operation
	if req.SessionID == nil || *req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
		return
	}

	sessionID := *req.SessionID

	// Session-based locking
	if !cache.TryAcquireSessionLock(ctx.TenantID, sessionID) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "session lock busy"})
		return
	}
	defer cache.ReleaseSessionLock(ctx.TenantID, sessionID)

	// Check for existing session in cache
	sessionData, sessionExists := cache.GetGlobalManager().GetSession(ctx.TenantID, sessionID)

	var finalFpID, finalVisitID string
	var leadID *string

	if sessionExists {
		// Use existing session data
		finalFpID = sessionData.FingerprintID
		finalVisitID = sessionData.VisitID
		leadID = sessionData.LeadID

		// If session has lead_id but we don't have profile, restore it
		if sessionData.LeadID != nil && profile == nil {
			if restoredProfile := getProfileFromLeadID(*sessionData.LeadID, ctx); restoredProfile != nil {
				profile = restoredProfile
				hasProfile = true
				consentValue = "1"
				leadID = sessionData.LeadID
			}
		}

		// Update session activity
		sessionData.UpdateActivity()
		cache.GetGlobalManager().SetSession(ctx.TenantID, sessionData)

	} else {
		// Create new session with backend-generated IDs
		visitService := NewVisitService(ctx, nil)

		if hasProfile && profile != nil {
			// For authenticated users, try to find existing fingerprint
			if existingFpID := visitService.FindFingerprintByLeadID(profile.LeadID); existingFpID != nil {
				finalFpID = *existingFpID
			} else {
				finalFpID = generateULID()
			}
			leadID = &profile.LeadID
		} else {
			// Generate new fingerprint for anonymous users
			finalFpID = generateULID()
		}

		// Create fingerprint if it doesn't exist
		if exists, err := visitService.FingerprintExists(finalFpID); err == nil && !exists {
			if err := visitService.CreateFingerprint(finalFpID, leadID); err != nil {
				log.Printf("Fingerprint creation error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "fingerprint creation failed"})
				return
			}
			cache.GetGlobalManager().SetKnownFingerprint(ctx.TenantID, finalFpID, leadID != nil)
		}

		// Handle visit creation/reuse
		var err error
		finalVisitID, err = visitService.HandleVisitCreation(finalFpID, hasProfile)
		if err != nil {
			log.Printf("Visit creation error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "session handling failed"})
			return
		}

		// Create new session data
		sessionData = &models.SessionData{
			SessionID:     sessionID,
			FingerprintID: finalFpID,
			VisitID:       finalVisitID,
			LeadID:        leadID,
			HasConsent:    consentValue == "1",
			LastActivity:  time.Now(),
			CreatedAt:     time.Now(),
		}

		// Store session in cache (ephemeral)
		cache.GetGlobalManager().SetSession(ctx.TenantID, sessionData)
	}

	// Update visit state cache
	visitState := &models.VisitState{
		VisitID:       finalVisitID,
		FingerprintID: finalFpID,
		StartTime:     time.Now(),
		LastActivity:  time.Now(),
		CurrentPage:   c.Request.Header.Get("Referer"),
	}
	cache.GetGlobalManager().SetVisitState(ctx.TenantID, visitState)

	fingerprintState := &models.FingerprintState{
		FingerprintID: finalFpID,
		HeldBeliefs:   make(map[string]models.BeliefValue),
		HeldBadges:    make(map[string]string),
		LastActivity:  time.Now(),
	}
	cache.GetGlobalManager().SetFingerprintState(ctx.TenantID, fingerprintState)

	// Return JWT response instead of cookies
	response := gin.H{
		"fingerprint": finalFpID,
		"visitId":     finalVisitID,
		"hasProfile":  hasProfile,
		"consent":     consentValue,
	}

	if hasProfile && profile != nil {
		token, err := utils.GenerateProfileToken(profile, ctx.Config.JWTSecret, ctx.Config.AESKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate profile token"})
			return
		}
		response["token"] = token
		response["profile"] = profile
	}

	c.JSON(http.StatusOK, response)
}

func SseHandler(c *gin.Context) {
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := models.Broadcaster.AddClient()
	defer models.Broadcaster.RemoveClient(ch)

	flusher, ok := w.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	for {
		select {
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

func StateHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		Events []models.Event `json:"events"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Process events with tenant context
	for _, event := range req.Events {
		if event.Type == "Pane" && event.Verb == "CLICKED" {
			paneIDs := []string{"pane-123", "pane-456"}
			data, _ := json.Marshal(paneIDs)
			models.Broadcaster.Broadcast("reload_panes", string(data))
		}
		// TODO: Add belief events and database persistence
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "tenantId": ctx.TenantID})
}

func DecodeProfileHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check Authorization header for JWT instead of cookies
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || len(authHeader) <= 7 || authHeader[:7] != "Bearer " {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	token := authHeader[7:]
	claims, err := utils.ValidateJWT(token, ctx.Config.JWTSecret)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	profile := claims["profile"]
	c.JSON(http.StatusOK, gin.H{"profile": profile})
}

func LoginHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req models.LoginRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if validateAdminLogin(req.TenantID, req.Password, ctx) {
		c.SetCookie("auth_token", "admin", 24*3600, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	}
}

func validateEncryptedCredentials(email, code string, ctx *tenant.Context) *models.Profile {
	// TODO: Implement database lookup with tenant context
	return nil
}

func getProfileFromLeadID(leadID string, ctx *tenant.Context) *models.Profile {
	// TODO: Implement database lookup for profile data from lead_id
	return nil
}

func validateAdminLogin(tenantID, password string, ctx *tenant.Context) bool {
	// TODO: Implement proper admin validation with tenant context
	return password == "admin" && tenantID == ctx.TenantID
}
