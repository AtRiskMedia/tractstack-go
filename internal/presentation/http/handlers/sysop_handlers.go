// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/container"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/messaging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

// SysOpHandlers handles SysOp dashboard authentication and data streaming
type SysOpHandlers struct {
	container        *container.Container
	sysOpBroadcaster *messaging.SysOpBroadcaster
}

// NewSysOpHandlers creates new SysOp handlers
func NewSysOpHandlers(container *container.Container) *SysOpHandlers {
	return &SysOpHandlers{
		container:        container,
		sysOpBroadcaster: container.SysOpBroadcaster,
	}
}

// GetTenants returns available tenants
func (h *SysOpHandlers) GetTenants(c *gin.Context) {
	registry := h.container.TenantManager.GetDetector().GetRegistry()
	if registry == nil || registry.Tenants == nil {
		c.JSON(http.StatusOK, map[string]any{"tenants": []string{}})
		return
	}

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
	defer func() {
		_ = tenantCtx.Close()
	}()

	claims := map[string]any{
		"role":     "admin",
		"tenantId": tenantCtx.Config.TenantID,
		"type":     "admin_auth",
		"exp":      time.Now().Add(1 * time.Hour).Unix(),
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

// AuthCheck validates the current system operator's session and permissions.
func (h *SysOpHandlers) AuthCheck(c *gin.Context) {
	sysopPassword := config.SysopPassword

	var fallbackHash string
	tenantCtx, err := h.container.TenantManager.NewContextFromID("default")
	if err == nil {
		fallbackHash = tenantCtx.Config.AdminPasswordHash
		_ = tenantCtx.Close()
	}

	var effectivePassword string
	var passwordRequired bool
	var message string
	var docsLink string

	switch {
	case sysopPassword == "storykeep":
		passwordRequired = true
		message = "Default SYSOP_PASSWORD 'storykeep' is not secure. Please set SYSOP_PASSWORD or configure tenant admin password."
	case sysopPassword != "":
		effectivePassword = sysopPassword
		passwordRequired = true
	case fallbackHash != "":
		passwordRequired = true
		message = "Using tenant admin password for SysOp access. Set SYSOP_PASSWORD to restrict SysOp authentication."
	default:
		passwordRequired = false
		message = "Welcome to your story keep. Set SYSOP_PASSWORD to protect the system"
		docsLink = "https://tractstack.org"
	}

	response := map[string]any{
		"passwordRequired": passwordRequired,
		"authenticated":    false,
	}

	if message != "" {
		response["message"] = message
	}
	if docsLink != "" {
		response["docsLink"] = docsLink
	}

	auth := c.GetHeader("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")

	if passwordRequired && token != "" {
		if effectivePassword != "" {
			if token == effectivePassword {
				response["authenticated"] = true
			}
		} else if fallbackHash != "" {
			if err := bcrypt.CompareHashAndPassword([]byte(fallbackHash), []byte(token)); err == nil {
				response["authenticated"] = true
			} else if token == fallbackHash {
				response["authenticated"] = true
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// Login handles system operator authentication requests.
func (h *SysOpHandlers) Login(c *gin.Context) {
	var request struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	sysopPassword := config.SysopPassword

	var fallbackHash string
	tenantCtx, err := h.container.TenantManager.NewContextFromID("default")
	if err == nil {
		fallbackHash = tenantCtx.Config.AdminPasswordHash
		_ = tenantCtx.Close()
	}

	if sysopPassword == "storykeep" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Default password 'storykeep' is not secure. Please set SYSOP_PASSWORD or configure tenant admin password."})
		return
	}

	if sysopPassword == "" && fallbackHash == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "token": "no-auth-required"})
		return
	}

	if sysopPassword != "" && sysopPassword != "storykeep" {
		if request.Password == sysopPassword {
			c.JSON(http.StatusOK, gin.H{"success": true, "token": sysopPassword})
			return
		}
	}

	if fallbackHash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(fallbackHash), []byte(request.Password)); err == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "token": fallbackHash})
			return
		}
	}

	c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
}

// SysOpAuthMiddleware provides a GIN middleware for protecting system operator routes.
func (h *SysOpHandlers) SysOpAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sysopPassword := config.SysopPassword

		var fallbackHash string
		tenantCtx, err := h.container.TenantManager.NewContextFromID("default")
		if err == nil {
			fallbackHash = tenantCtx.Config.AdminPasswordHash
			_ = tenantCtx.Close()
		}

		if (sysopPassword == "" || sysopPassword == "storykeep") && fallbackHash == "" {
			if sysopPassword == "storykeep" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Default password 'storykeep' is not secure"})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		token := ""
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) > 7 && strings.HasPrefix(authHeader, "Bearer ") {
			token = authHeader[7:]
		} else {
			token = c.Query("token")
		}

		if sysopPassword != "" && sysopPassword != "storykeep" {
			if token == sysopPassword {
				c.Next()
				return
			}
		}

		if fallbackHash != "" {
			if token == fallbackHash {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		c.Abort()
	}
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

	_, _ = fmt.Fprintf(c.Writer, ": connection established\n\n")
	c.Writer.Flush()

	c.Stream(func(w io.Writer) bool {
		select {
		case message, ok := <-client.Channel:
			if !ok {
				return false
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", message)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// GetLogLevels handles GET /sysop-logs/levels
func (h *SysOpHandlers) GetLogLevels(c *gin.Context) {
	logger := h.container.Logger
	if logger == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Logger not available"})
		return
	}
	levels := logger.GetChannelLevels()
	c.JSON(http.StatusOK, levels)
}

// SetLogLevel handles POST /sysop-logs/levels
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

// HandleSessionMapStream handles the WebSocket connection for the session map.
func (h *SysOpHandlers) HandleSessionMapStream(c *gin.Context) {
	tenantID := c.Query("tenant")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant query parameter is required"})
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")

			// Allow localhost origins
			if strings.HasPrefix(origin, "http://localhost") ||
				strings.HasPrefix(origin, "http://127.0.0.1") ||
				strings.HasPrefix(origin, "https://localhost") ||
				strings.HasPrefix(origin, "https://127.0.0.1") {
				return true
			}

			// For production origins, validate against tenant domains
			if origin != "" {
				if u, err := url.Parse(origin); err == nil {
					hostname := u.Hostname()
					detector := h.container.TenantManager.GetDetector()
					return detector.ValidateDomain("default", hostname)
				}
			}

			return false
		},
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade to websocket: %v", err)
		return
	}

	client := &messaging.SysOpClient{
		Conn:     conn,
		TenantID: tenantID,
		Send:     make(chan []byte, 256),
	}

	h.container.SysOpBroadcaster.Register(client)

	go h.clientWritePump(client)
	go h.clientReadPump(client)
}

// clientReadPump handles incoming messages from the client (primarily for disconnection detection).
func (h *SysOpHandlers) clientReadPump(client *messaging.SysOpClient) {
	defer func() {
		h.container.SysOpBroadcaster.Unregister(client)
		_ = client.Conn.Close()
	}()
	client.Conn.SetReadLimit(512)
	_ = client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Conn.SetPongHandler(func(string) error { _ = client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })

	for {
		_, _, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}
	}
}

// clientWritePump handles pushing messages from the broadcaster to the client.
func (h *SysOpHandlers) clientWritePump(client *messaging.SysOpClient) {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		_ = client.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-client.Send:
			_ = client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := client.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			_ = client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// GetActivityGraph returns time-series data for system activity visualization.
func (h *SysOpHandlers) GetActivityGraph(c *gin.Context) {
	tenantID := c.Query("tenant")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant query parameter is required"})
		return
	}

	h.container.Logger.System().Debug("Received activity graph request",
		"tenantId", tenantID,
		"method", c.Request.Method,
		"path", c.Request.URL.Path)

	// Delegate to SysOpService for business logic
	result, err := h.container.SysOpService.GetActivityGraph(tenantID)
	if err != nil {
		h.container.Logger.System().Error("Activity graph generation failed",
			"tenantId", tenantID,
			"error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return the enhanced graph data
	c.JSON(http.StatusOK, result)
}
