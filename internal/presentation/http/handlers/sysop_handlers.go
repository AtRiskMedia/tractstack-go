package handlers

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/AtRiskMedia/tractstack-go/internal/application/container"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
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

// AuthCheck checks if SYSOP_PASSWORD is set and validates session
func (h *SysOpHandlers) AuthCheck(c *gin.Context) {
	sysopPassword := os.Getenv("SYSOP_PASSWORD")
	response := map[string]any{
		"passwordRequired": sysopPassword != "",
		"authenticated":    false,
	}
	if sysopPassword == "" {
		response["message"] = "Welcome to your story keep. Set SYSOP_PASSWORD to protect the system"
		response["docsLink"] = "https://tractstack.org"
	} else {
		auth := c.GetHeader("Authorization")
		if auth == "Bearer "+sysopPassword {
			response["authenticated"] = true
		}
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
	sysopPassword := os.Getenv("SYSOP_PASSWORD")
	if sysopPassword == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "token": "no-auth-required"})
		return
	}
	if request.Password != sysopPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "token": sysopPassword})
}

// GetTenants returns available tenants
func (h *SysOpHandlers) GetTenants(c *gin.Context) {
	tenants := []string{"default", "love"} // Simplified
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

	// *** START OF THE FIX ***
	// Send an initial comment to establish the connection and trigger the 'onopen' event in the browser.
	fmt.Fprintf(c.Writer, ": connection established\n\n")
	c.Writer.Flush()
	// *** END OF THE FIX ***

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
