// Package handlers provides HTTP handlers for the presentation layer.
package handlers

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/application/container"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/messaging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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
	defer tenantCtx.Close()

	claims := map[string]interface{}{
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

// SysOpAuthMiddleware protects SysOp-specific endpoints.
func (h *SysOpHandlers) SysOpAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sysopPassword := config.SysopPassword
		if sysopPassword == "" {
			c.Next()
			return
		}

		token := ""
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) > 7 && strings.HasPrefix(authHeader, "Bearer ") {
			token = authHeader[7:]
		} else {
			// Fallback for WebSocket authentication via query parameter
			token = c.Query("token")
		}

		if token != sysopPassword {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		c.Next()
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

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "http://127.0.0.1")
	},
}

// HandleSessionMapStream handles the WebSocket connection for the session map.
func (h *SysOpHandlers) HandleSessionMapStream(c *gin.Context) {
	tenantID := c.Query("tenant")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant query parameter is required"})
		return
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
		client.Conn.Close()
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
		client.Conn.Close()
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

// GetActivityGraph returns real-time session/fingerprint/belief graph data from cache (last hour)
func (h *SysOpHandlers) GetActivityGraph(c *gin.Context) {
	tenantID := c.Query("tenant")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant query parameter is required"})
		return
	}

	cacheManager := h.container.CacheManager
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

	// Get all session and fingerprint IDs from cache
	sessionIDs := cacheManager.GetAllSessionIDs(tenantID)
	fingerprintIDs := cacheManager.GetAllFingerprintIDs(tenantID)
	visitIDs := cacheManager.GetAllVisitIDs(tenantID)

	type GraphNode struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Label    string `json:"label"`
		Size     int    `json:"size"`
		LeadName string `json:"leadName,omitempty"`
	}

	type GraphLink struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Type   string `json:"type"`
	}

	nodes := []GraphNode{}
	links := []GraphLink{}
	nodeSet := make(map[string]bool) // Track unique nodes

	// Process sessions -> fingerprints (last hour only)
	for _, sessionID := range sessionIDs {
		if sessionData, exists := cacheManager.GetSession(tenantID, sessionID); exists {
			// Skip if session activity is older than 1 hour
			if sessionData.LastActivity.Before(oneHourAgo) {
				continue
			}

			// Add session node
			sessionLabel := sessionID
			if len(sessionID) > 8 {
				sessionLabel = sessionID[:8] + "..."
			}

			if !nodeSet[sessionID] {
				nodes = append(nodes, GraphNode{
					ID:    sessionID,
					Type:  "session",
					Label: sessionLabel,
					Size:  10,
				})
				nodeSet[sessionID] = true
			}

			// Add fingerprint node
			fingerprintID := sessionData.FingerprintID
			fingerprintLabel := fingerprintID
			if len(fingerprintID) > 8 {
				fingerprintLabel = fingerprintID[:8] + "..."
			}

			if !nodeSet[fingerprintID] {
				nodes = append(nodes, GraphNode{
					ID:    fingerprintID,
					Type:  "fingerprint",
					Label: fingerprintLabel,
					Size:  12,
				})
				nodeSet[fingerprintID] = true
			}

			// Link session to fingerprint
			links = append(links, GraphLink{
				Source: sessionID,
				Target: fingerprintID,
				Type:   "session_fingerprint",
			})

			// Add visit node and link fingerprint -> visit
			visitID := sessionData.VisitID
			if visitState, exists := cacheManager.GetVisitState(tenantID, visitID); exists {
				// Skip if visit activity is older than 1 hour
				if visitState.LastActivity.Before(oneHourAgo) {
					continue
				}

				visitLabel := visitID
				if len(visitID) > 8 {
					visitLabel = visitID[:8] + "..."
				}

				if !nodeSet[visitID] {
					nodes = append(nodes, GraphNode{
						ID:    visitID,
						Type:  "visit",
						Label: visitLabel,
						Size:  8,
					})
					nodeSet[visitID] = true
				}

				// Link fingerprint to visit
				links = append(links, GraphLink{
					Source: fingerprintID,
					Target: visitID,
					Type:   "fingerprint_visit",
				})

				// Add page node and link visit -> page
				currentPage := visitState.CurrentPage
				if currentPage == "" {
					currentPage = "/"
				}

				pageLabel := currentPage
				if len(currentPage) > 20 {
					pageLabel = "..." + currentPage[len(currentPage)-17:]
				}

				if !nodeSet[currentPage] {
					nodes = append(nodes, GraphNode{
						ID:    currentPage,
						Type:  "page",
						Label: pageLabel,
						Size:  14,
					})
					nodeSet[currentPage] = true
				}

				// Link visit to page
				links = append(links, GraphLink{
					Source: visitID,
					Target: currentPage,
					Type:   "visit_page",
				})
			}

			// Add lead node if this fingerprint has a lead (separate from fingerprint)
			if sessionData.LeadID != nil && *sessionData.LeadID != "" {
				leadID := *sessionData.LeadID

				// Try to get lead name from database
				leadName := "Unknown Lead"
				if tenantCtx, err := h.container.TenantManager.NewContextFromID(tenantID); err == nil {
					if lead, err := tenantCtx.LeadRepo().FindByID(leadID); err == nil && lead != nil {
						leadName = lead.FirstName
						if lead.Email != "" {
							leadName += " (" + lead.Email + ")"
						}
					}
					tenantCtx.Close()
				}

				leadLabel := leadName
				if len(leadLabel) > 20 {
					leadLabel = leadLabel[:17] + "..."
				}

				if !nodeSet[leadID] {
					nodes = append(nodes, GraphNode{
						ID:       leadID,
						Type:     "lead",
						Label:    leadLabel,
						Size:     12,
						LeadName: leadName,
					})
					nodeSet[leadID] = true
				}

				// Link fingerprint to lead
				links = append(links, GraphLink{
					Source: fingerprintID,
					Target: leadID,
					Type:   "fingerprint_lead",
				})
			}
		}
	}

	// Process fingerprints -> beliefs (last hour only)
	for _, fingerprintID := range fingerprintIDs {
		if fpState, exists := cacheManager.GetFingerprintState(tenantID, fingerprintID); exists {
			// Skip if fingerprint activity is older than 1 hour
			if fpState.LastActivity.Before(oneHourAgo) {
				continue
			}

			// Process held beliefs
			for beliefKey, beliefValues := range fpState.HeldBeliefs {
				for _, beliefValue := range beliefValues {
					beliefNodeID := beliefKey + ":" + beliefValue
					beliefLabel := beliefKey
					if len(beliefKey) > 12 {
						beliefLabel = beliefKey[:12] + "..."
					}

					if !nodeSet[beliefNodeID] {
						nodes = append(nodes, GraphNode{
							ID:    beliefNodeID,
							Type:  "belief",
							Label: beliefLabel,
							Size:  8,
						})
						nodeSet[beliefNodeID] = true
					}

					// Link fingerprint to belief
					links = append(links, GraphLink{
						Source: fingerprintID,
						Target: beliefNodeID,
						Type:   "fingerprint_belief",
					})
				}
			}
		}
	}

	// Calculate stats
	stats := gin.H{
		"sessions":     len(sessionIDs),
		"fingerprints": len(fingerprintIDs),
		"visits":       len(visitIDs),
		"nodes":        len(nodes),
		"links":        len(links),
		"timeframe":    "last_hour",
	}

	c.JSON(http.StatusOK, gin.H{
		"nodes": nodes,
		"links": links,
		"stats": stats,
	})
}
