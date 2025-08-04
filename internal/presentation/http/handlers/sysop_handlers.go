// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/container"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"github.com/gin-gonic/gin"
)

// SysOpHandlers handles SysOp dashboard authentication and data streaming
type SysOpHandlers struct {
	container *container.Container
}

// NewSysOpHandlers creates new SysOp handlers
func NewSysOpHandlers(container *container.Container) *SysOpHandlers {
	return &SysOpHandlers{
		container: container,
	}
}

// AuthCheck checks if SysopPassword is set and validates session
func (h *SysOpHandlers) AuthCheck(c *gin.Context) {
	sysopPassword := config.SysopPassword
	response := map[string]any{
		"passwordRequired": sysopPassword != "",
		"authenticated":    false,
	}

	switch sysopPassword {
	case "":
		response["message"] = "Welcome to your story keep. Set SYSOP_PASSWORD to protect the system"
		response["docsLink"] = "https://tractstack.org"
	case "storykeep":
		response["message"] = "WARNING: Your Story Keep is not protected. Please change the default SYSOP_PASSWORD."
	}

	// Also check for a valid token in the header
	auth := c.GetHeader("Authorization")
	if sysopPassword != "" && auth == "Bearer "+sysopPassword {
		response["authenticated"] = true
	}

	c.JSON(http.StatusOK, response)
}

// Login handles SysOp authentication
func (h *SysOpHandlers) Login(c *gin.Context) {
	var request struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	sysopPassword := config.SysopPassword
	if sysopPassword == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "token": "no-auth-required"})
		return
	}
	if request.Password != sysopPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	response := gin.H{"success": true, "token": sysopPassword}
	if sysopPassword == "storykeep" {
		response["warning"] = "Default password is in use. Please change the SYSOP_PASSWORD environment variable for security."
	}
	c.JSON(http.StatusOK, response)
}

// GetTenants returns available tenants
func (h *SysOpHandlers) GetTenants(c *gin.Context) {
	registry := h.container.TenantManager.GetDetector().GetRegistry()
	if registry == nil || registry.Tenants == nil {
		c.JSON(http.StatusOK, map[string]any{"tenants": []string{}})
		return
	}

	// Extract the keys (tenant IDs) from the registry map
	tenants := make([]string, 0, len(registry.Tenants))
	for tenantID := range registry.Tenants {
		tenants = append(tenants, tenantID)
	}

	c.JSON(http.StatusOK, map[string]any{"tenants": tenants})
}

// GetActivityMetrics fetches live activity counts from the cache manager.
func (h *SysOpHandlers) GetActivityMetrics(c *gin.Context) {
	tenantID := c.Query("tenant")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant query parameter is required"})
		return
	}
	cacheManager := h.container.CacheManager
	sessions := len(cacheManager.GetAllSessionIDs(tenantID))
	fingerprints := len(cacheManager.GetAllFingerprintIDs(tenantID))
	visits := len(cacheManager.GetAllVisitIDs(tenantID))
	beliefMaps := len(cacheManager.GetAllStoryfragmentBeliefRegistryIDs(tenantID))
	fragments := len(cacheManager.GetAllHTMLChunkIDs(tenantID))
	c.JSON(http.StatusOK, gin.H{
		"sessions":     sessions,
		"fingerprints": fingerprints,
		"visits":       visits,
		"beliefMaps":   beliefMaps,
		"fragments":    fragments,
	})
}

// GetTenantToken is the secure token broker endpoint.
// It leverages the fact that the SysOp is already authenticated via middleware
// to generate a short-lived, admin-level token for the requested tenant.
func (h *SysOpHandlers) GetTenantToken(c *gin.Context) {
	var req struct {
		TenantID string `json:"tenantId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: tenantId is required"})
		return
	}

	tenantCtx, err := h.container.TenantManager.NewContextFromID(req.TenantID)
	if err != nil {
		h.container.Logger.System().Error("SysOp failed to create context for token generation", "error", err, "tenantId", req.TenantID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found or could not be initialized"})
		return
	}
	defer tenantCtx.Close()

	// Since the SysOp is already authenticated, we can directly generate an admin token.
	// This is the correct way to grant temporary, privileged access.
	claims := map[string]interface{}{
		"role":     "admin", // SysOp gets admin-level access for monitoring.
		"tenantId": tenantCtx.Config.TenantID,
		"type":     "admin_auth",
		"exp":      time.Now().Add(1 * time.Hour).Unix(), // Token is valid for 1 hour.
		"iat":      time.Now().Unix(),
	}

	token, err := h.container.AuthService.GenerateJWT(claims, tenantCtx.Config.JWTSecret)
	if err != nil {
		h.container.Logger.System().Error("SysOp failed to generate JWT for tenant", "error", err, "tenantId", req.TenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tenant token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token":   token,
		"role":    "admin",
	})
}

// SysOpAuthMiddleware protects SysOp-specific endpoints.
func (h *SysOpHandlers) SysOpAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sysopPassword := config.SysopPassword
		if sysopPassword == "" {
			c.Next() // No password set, allow access
			return
		}

		authHeader := c.GetHeader("Authorization")
		token := ""
		if len(authHeader) > 7 && strings.HasPrefix(authHeader, "Bearer ") {
			token = authHeader[7:]
		}

		if token != sysopPassword {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// StreamLogs, GetLogLevels, and SetLogLevel remain unchanged as they have separate auth logic if needed.

// StreamLogs handles the SSE connection for live log streaming.
func (h *SysOpHandlers) StreamLogs(c *gin.Context) {
	broadcaster := h.container.LogBroadcaster
	if broadcaster == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Log broadcaster not available"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	channelFilter := c.DefaultQuery("channel", "all")
	levelFilter := c.DefaultQuery("level", "INFO")
	var logLevel slog.Level
	switch levelFilter {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	filters := logging.AppliedFilters{
		Channel: logging.Channel(channelFilter),
		Level:   logLevel,
	}

	client := broadcaster.NewClient(filters)
	broadcaster.RegisterClient(client)
	defer broadcaster.UnregisterClient(client)

	fmt.Fprintf(c.Writer, ": connection established\n\n")
	c.Writer.Flush()

	c.Stream(func(w io.Writer) bool {
		select {
		case message, ok := <-client.Channel:
			if !ok {
				return false
			}
			fmt.Fprintf(w, "data: %s\n\n", message)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// GetLogLevels handles GET /sysop-logs/levels - returns current log levels for all channels.
func (h *SysOpHandlers) GetLogLevels(c *gin.Context) {
	logger := h.container.Logger
	if logger == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Logger not available"})
		return
	}
	levels := logger.GetChannelLevels()
	c.JSON(http.StatusOK, levels)
}

// SetLogLevel handles POST /sysop-logs/levels - sets the log level for a specific channel.
func (h *SysOpHandlers) SetLogLevel(c *gin.Context) {
	logger := h.container.Logger
	if logger == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Logger not available"})
		return
	}

	var req struct {
		Channel string `json:"channel" binding:"required"`
		Level   string `json:"level" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	var level slog.Level
	switch req.Level {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid log level specified"})
		return
	}

	if err := logger.SetChannelLevel(logging.Channel(req.Channel), level); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set log level", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": fmt.Sprintf("Log level for channel '%s' set to '%s'", req.Channel, req.Level)})
}
