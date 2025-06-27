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
		fingerprint := c.PostForm("fingerprint")
		visitId := c.PostForm("visitId")
		encryptedEmail := c.PostForm("encryptedEmail")
		encryptedCode := c.PostForm("encryptedCode")
		consent := c.PostForm("consent")
		sessionId := c.PostForm("sessionId")

		if fingerprint != "" {
			req.Fingerprint = &fingerprint
		}
		if visitId != "" {
			req.VisitID = &visitId
		}
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

	// Session-based locking using SSR session ID
	sessionID := ""
	if req.SessionID != nil && *req.SessionID != "" {
		sessionID = *req.SessionID
	} else {
		// Fallback for requests without session ID
		sessionID = utils.GenerateULID()
	}

	if !cache.TryAcquireSessionLock(ctx.TenantID, sessionID) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "session lock busy"})
		return
	}
	defer cache.ReleaseSessionLock(ctx.TenantID, sessionID)

	visitService := NewVisitService(ctx, nil)

	var requestFpID, requestVisitID *string
	if req.Fingerprint != nil && *req.Fingerprint != "" {
		requestFpID = req.Fingerprint
	}

	if req.VisitID != nil && *req.VisitID != "" {
		requestVisitID = req.VisitID
	}

	finalFpID, finalVisitID, leadID, err := visitService.HandleVisitSession(requestFpID, requestVisitID, hasProfile)
	if err != nil {
		log.Printf("Visit session error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session handling failed"})
		return
	}

	// Profile restoration from lead_id
	if profile == nil && leadID != nil {
		profile = getProfileFromLeadID(*leadID, ctx)
		hasProfile = profile != nil
		if hasProfile {
			consentValue = "1"
		}
	}

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

	if hasProfile {
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
