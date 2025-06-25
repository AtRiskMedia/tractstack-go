// Package api provides HTTP handlers and database connectivity for the application's API.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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

	var req models.VisitRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	fpID, _ := c.Cookie("fp_id")
	visitID, _ := c.Cookie("visit_id")
	consent, _ := c.Cookie("consent")
	profileToken, _ := c.Cookie("profile_token")

	var profile *models.Profile
	if profileToken != "" {
		claims, err := utils.ValidateJWT(profileToken, ctx.Config.JWTSecret)
		if err == nil {
			profile = utils.GetProfileFromClaims(claims)
		}
	}

	if profile == nil && req.EncryptedEmail != nil && req.EncryptedCode != nil {
		profile = validateEncryptedCredentials(*req.EncryptedEmail, *req.EncryptedCode, ctx)
	}

	hasProfile := profile != nil
	consentValue := consent
	if hasProfile {
		consentValue = "1"
	} else if req.Consent != nil {
		consentValue = *req.Consent
	}

	if fpID == "" || (hasProfile || consentValue == "1") {
		if req.Fingerprint != nil && *req.Fingerprint != "" {
			fpID = *req.Fingerprint
		} else {
			fpID = utils.GenerateULID()
		}
	}

	fpExpiry := time.Hour
	if hasProfile || consentValue == "1" {
		fpExpiry = 30 * 24 * time.Hour
	}

	c.SetCookie("fp_id", fpID, int(fpExpiry.Seconds()), "/", "", false, true)

	if visitID == "" {
		if req.VisitID != nil && *req.VisitID != "" {
			visitID = *req.VisitID
		} else {
			visitID = utils.GenerateULID()
		}
	}
	c.SetCookie("visit_id", visitID, 24*3600, "/", "", false, true)

	if hasProfile || consentValue == "1" {
		c.SetCookie("consent", "1", 30*24*3600, "/", "", false, true)
	}

	if hasProfile {
		token, err := utils.GenerateProfileToken(profile, ctx.Config.JWTSecret, ctx.Config.AESKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate profile token"})
			return
		}
		c.SetCookie("profile_token", token, 30*24*3600, "/", "", false, true)
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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

	profileToken, err := c.Cookie("profile_token")
	if err != nil || profileToken == "" {
		c.JSON(http.StatusOK, gin.H{"profile": nil})
		return
	}

	claims, err := utils.ValidateJWT(profileToken, ctx.Config.JWTSecret)
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

func validateAdminLogin(tenantID, password string, ctx *tenant.Context) bool {
	// TODO: Implement proper admin validation with tenant context
	return password == "admin" && tenantID == ctx.TenantID
}
