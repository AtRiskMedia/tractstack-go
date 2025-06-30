package api

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
	"github.com/gin-gonic/gin"
)

type VisitRowData struct {
	ID            string    `json:"id"`
	FingerprintID string    `json:"fingerprint_id"`
	CampaignID    *string   `json:"campaign_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type FingerprintRowData struct {
	ID        string    `json:"id"`
	LeadID    *string   `json:"lead_id"`
	CreatedAt time.Time `json:"created_at"`
}

type VisitService struct {
	ctx *tenant.Context
}

// getEnvInt reads environment variable with fallback
func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// Global SSE connection tracking
var (
	activeSSEConnections int64
	maxSSEConnections    = int64(getEnvInt("MAX_SESSIONS_PER_CLIENT", 10000))
)

func NewVisitService(ctx *tenant.Context, _ any) *VisitService {
	return &VisitService{
		ctx: ctx,
	}
}

func VisitHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Activate tenant if needed
	if ctx.Status == "inactive" {
		if err := tenant.ActivateTenant(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tenant activation failed: %v", err)})
			return
		}
	}

	// Log the raw request for debugging
	body, _ := c.GetRawData()

	// Reset the body for binding
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// Parse form data
	var req models.VisitRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	// Initialize response variables
	var finalFpID, finalVisitID string
	var leadID *string
	var hasProfile bool
	var profile *models.Profile
	consentValue := "0"

	// Process consent
	if req.Consent != nil && *req.Consent == "1" {
		consentValue = "1"
	}

	// Check for encrypted credentials (profile restoration)
	if req.EncryptedEmail != nil && req.EncryptedCode != nil &&
		*req.EncryptedEmail != "" && *req.EncryptedCode != "" {
		// Use the existing validation function from profile_handlers.go
		profile = ValidateEncryptedCredentials(*req.EncryptedEmail, *req.EncryptedCode, ctx)
		if profile != nil {
			hasProfile = true
			leadID = &profile.LeadID
		}
	}

	// Extract session ID for session handling
	sessionID := generateULID()
	if req.SessionID != nil && *req.SessionID != "" {
		sessionID = *req.SessionID
	}

	// Check for existing session in cache
	sessionData, sessionExists := cache.GetGlobalManager().GetSession(ctx.TenantID, sessionID)

	if sessionExists {
		// Use existing session data
		finalFpID = sessionData.FingerprintID
		finalVisitID = sessionData.VisitID

		// Update session activity
		sessionData.UpdateActivity()
		cache.GetGlobalManager().SetSession(ctx.TenantID, sessionData)

	} else {
		// Create new session with backend-generated IDs
		visitService := NewVisitService(ctx, nil)

		if hasProfile {
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
				c.JSON(http.StatusInternalServerError, gin.H{"error": "fingerprint creation failed"})
				return
			}

			globalCache := cache.GetGlobalManager()
			if globalCache != nil {
				globalCache.SetKnownFingerprint(ctx.TenantID, finalFpID, leadID != nil)
			} else {
				log.Printf("ERROR: Global cache manager is nil")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "cache initialization failed"})
				return
			}

		}

		// Handle visit creation/reuse
		finalVisitID, err = visitService.HandleVisitCreation(finalFpID, hasProfile)
		if err != nil {
			log.Printf("ERROR: Visit creation error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "session handling failed"})
			return
		}

		// Create new session data
		sessionData = &models.SessionData{
			SessionID:     sessionID,
			FingerprintID: finalFpID,
			VisitID:       finalVisitID,
			LeadID:        leadID,
			LastActivity:  time.Now(),
			CreatedAt:     time.Now(),
		}

		// Store session in cache
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
	globalCache := cache.GetGlobalManager()
	if globalCache != nil {
		if globalCache.CacheManager != nil {
			globalCache.SetVisitState(ctx.TenantID, visitState)
		} else {
			log.Printf("ERROR: CacheManager is nil")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "cache manager internal error"})
			return
		}
	} else {
		log.Printf("ERROR: Global cache manager is nil at crash point")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cache not available"})
		return
	}

	// Update fingerprint state cache
	fingerprintState := &models.FingerprintState{
		FingerprintID: finalFpID,
		HeldBeliefs:   make(map[string]models.BeliefValue),
		HeldBadges:    make(map[string]string),
		LastActivity:  time.Now(),
	}
	cache.GetGlobalManager().SetFingerprintState(ctx.TenantID, fingerprintState)
	// Build response
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

// SseHandler provides Production-ready SSE handler with proper resource management
// and per-session connection limits
func SseHandler(c *gin.Context) {
	// Get session ID from header or extract from tenant context
	sessionID := c.GetHeader("X-TractStack-Session-ID")
	if sessionID == "" {
		// Try to extract from Authorization or other headers if needed
		sessionID = c.GetHeader("Authorization") // Adjust based on your auth flow
	}
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID required for SSE connection"})
		return
	}

	// Check global connection limit first
	currentConnections := atomic.LoadInt64(&activeSSEConnections)
	if currentConnections >= maxSSEConnections {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "SSE connection limit reached. Please try again later.",
		})
		return
	}

	// Check per-session connection limit
	const maxSessionConnections = 3 // Allow up to 3 tabs per session
	sessionConnectionCount := models.Broadcaster.GetSessionConnectionCount(sessionID)
	if sessionConnectionCount >= maxSessionConnections {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":              fmt.Sprintf("Too many SSE connections for session (max %d). Close some tabs and try again.", maxSessionConnections),
			"sessionId":          sessionID,
			"currentConnections": sessionConnectionCount,
		})
		return
	}

	// Increment global connection counter
	atomic.AddInt64(&activeSSEConnections, 1)

	// Create timeout context (30 minutes max - after this the connection is force-closed)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)

	// Register client with broadcaster (now tracks session ID)
	ch := models.Broadcaster.AddClientWithSession(sessionID)

	// FIXED: Comprehensive cleanup with proper order
	defer func() {
		// 1. Decrement connection counter first
		atomic.AddInt64(&activeSSEConnections, -1)

		// 2. Cancel context to stop any ongoing operations
		cancel()

		// 3. Remove from broadcaster before closing channel
		models.Broadcaster.RemoveClientWithSession(ch, sessionID)

		// 4. Safely close the channel (non-blocking)
		select {
		case <-ch:
			// Channel already closed or has data
		default:
			// Channel is empty, safe to close
			close(ch)
		}

		// 5. Recover from any panics during cleanup
		if r := recover(); r != nil {
			log.Printf("SSE cleanup panic recovered for session %s: %v", sessionID, r)
		}
	}()

	// Set SSE headers
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Verify flusher support
	flusher, ok := w.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	// Send initial connection confirmation with session info
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ready\",\"sessionId\":\"%s\",\"connectionCount\":%d}\n\n",
		sessionID, models.Broadcaster.GetSessionConnectionCount(sessionID))
	flusher.Flush()

	// Heartbeat ticker to detect dead connections
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	// Connection timeout for inactive clients
	inactivityTimeout := time.NewTimer(5 * time.Minute)
	defer inactivityTimeout.Stop()

	log.Printf("SSE connection established for session %s. Active connections: %d (session: %d)",
		sessionID, atomic.LoadInt64(&activeSSEConnections), models.Broadcaster.GetSessionConnectionCount(sessionID))

	for {
		select {
		case msg := <-ch:
			// Reset inactivity timer on message
			if !inactivityTimeout.Stop() {
				select {
				case <-inactivityTimeout.C:
				default:
				}
			}
			inactivityTimeout.Reset(5 * time.Minute)

			// Send message to client
			_, err := fmt.Fprint(w, msg)
			if err != nil {
				log.Printf("SSE write error for session %s: %v", sessionID, err)
				return
			}
			flusher.Flush()

		case <-heartbeat.C:
			// Send heartbeat to detect broken connections
			_, err := fmt.Fprint(w, "event: heartbeat\ndata: {\"timestamp\":")
			if err != nil {
				log.Printf("SSE heartbeat failed for session %s: %v", sessionID, err)
				return
			}
			_, err = fmt.Fprintf(w, "%d,\"sessionId\":\"%s\"}\n\n", time.Now().Unix(), sessionID)
			if err != nil {
				log.Printf("SSE heartbeat failed for session %s: %v", sessionID, err)
				return
			}
			flusher.Flush()

		case <-inactivityTimeout.C:
			// Close inactive connections after 5 minutes of no activity
			log.Printf("SSE connection closed due to inactivity for session %s", sessionID)
			fmt.Fprintf(w, "event: timeout\ndata: {\"reason\":\"inactivity\",\"sessionId\":\"%s\"}\n\n", sessionID)
			flusher.Flush()
			return

		case <-ctx.Done():
			// Handle context cancellation (client disconnect, timeout, server shutdown)
			switch ctx.Err() {
			case context.DeadlineExceeded:
				// After 30 minutes, force close and tell client to reconnect
				log.Printf("SSE connection closed due to 30-minute timeout for session %s - client should reconnect", sessionID)
				fmt.Fprintf(w, "event: timeout\ndata: {\"reason\":\"max_duration\",\"action\":\"reconnect\",\"sessionId\":\"%s\"}\n\n", sessionID)
			case context.Canceled:
				log.Printf("SSE connection closed by client for session %s", sessionID)
			default:
				log.Printf("SSE connection closed for session %s: %v", sessionID, ctx.Err())
			}
			flusher.Flush()
			return
		}
	}
}

func (vs *VisitService) GetLatestVisitByFingerprint(fingerprintID string) (*VisitRowData, error) {
	query := `SELECT id, fingerprint_id, campaign_id, created_at 
              FROM visits 
              WHERE fingerprint_id = ? 
              ORDER BY created_at DESC 
              LIMIT 1`

	row := vs.ctx.Database.Conn.QueryRow(query, fingerprintID)

	var visit VisitRowData
	var campaignID sql.NullString

	err := row.Scan(&visit.ID, &visit.FingerprintID, &campaignID, &visit.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan visit: %w", err)
	}

	if campaignID.Valid {
		visit.CampaignID = &campaignID.String
	}

	return &visit, nil
}

func (vs *VisitService) CreateVisit(visitID, fingerprintID string, campaignID *string) error {
	query := `INSERT INTO visits (id, fingerprint_id, campaign_id, created_at) 
              VALUES (?, ?, ?, ?)`

	_, err := vs.ctx.Database.Conn.Exec(query, visitID, fingerprintID, campaignID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create visit: %w", err)
	}

	return nil
}

func (vs *VisitService) FingerprintExists(fingerprintID string) (bool, error) {
	query := `SELECT 1 FROM fingerprints WHERE id = ? LIMIT 1`

	var exists int
	err := vs.ctx.Database.Conn.QueryRow(query, fingerprintID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check fingerprint existence: %w", err)
	}

	return true, nil
}

func (vs *VisitService) CreateFingerprint(fingerprintID string, leadID *string) error {
	query := `INSERT INTO fingerprints (id, lead_id, created_at) 
              VALUES (?, ?, ?)`

	_, err := vs.ctx.Database.Conn.Exec(query, fingerprintID, leadID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create fingerprint: %w", err)
	}

	return nil
}

func (vs *VisitService) GetFingerprintLeadID(fingerprintID string) (*string, error) {
	query := `SELECT lead_id FROM fingerprints WHERE id = ? LIMIT 1`
	var leadID sql.NullString
	err := vs.ctx.Database.Conn.QueryRow(query, fingerprintID).Scan(&leadID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get fingerprint lead_id: %w", err)
	}
	if leadID.Valid {
		return &leadID.String, nil
	}
	return nil, nil
}

// FindFingerprintByLeadID finds existing fingerprint for a lead
func (vs *VisitService) FindFingerprintByLeadID(leadID string) *string {
	query := `SELECT id FROM fingerprints WHERE lead_id = ? LIMIT 1`

	var fingerprintID string
	err := vs.ctx.Database.Conn.QueryRow(query, leadID).Scan(&fingerprintID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return nil
	}

	return &fingerprintID
}

// HandleVisitCreation creates or reuses visits based on recency
func (vs *VisitService) HandleVisitCreation(fingerprintID string, hasProfile bool) (string, error) {
	// Check for recent visit (< 2 hours)
	if latestVisit, err := vs.GetLatestVisitByFingerprint(fingerprintID); err == nil && latestVisit != nil {
		if time.Since(latestVisit.CreatedAt) < 2*time.Hour {
			// Reuse existing recent visit
			return latestVisit.ID, nil
		}
	}

	// Create new visit (this will automatically invalidate previous ones)
	visitID := utils.GenerateULID()
	if err := vs.CreateVisit(visitID, fingerprintID, nil); err != nil {
		return "", fmt.Errorf("failed to create visit: %w", err)
	}

	return visitID, nil
}

func (vs *VisitService) IsVisitExpired(visit *VisitRowData) bool {
	if visit == nil {
		return true
	}
	return time.Since(visit.CreatedAt) > 2*time.Hour
}

func (vs *VisitService) HandleVisitSession(requestFpID, requestVisitID *string, hasProfile bool) (string, string, *string, error) {
	var fpID, visitID string

	// Use provided fingerprint or generate new one
	if requestFpID != nil && *requestFpID != "" {
		fpID = *requestFpID
	} else {
		fpID = utils.GenerateULID()
	}

	// Check if fingerprint exists in database
	fpExists, err := vs.FingerprintExists(fpID)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to check fingerprint: %w", err)
	}

	// Create fingerprint if it doesn't exist
	if !fpExists {
		var leadID *string
		// leadID would be set if this is a known user

		if err := vs.CreateFingerprint(fpID, leadID); err != nil {
			return "", "", nil, fmt.Errorf("failed to create fingerprint: %w", err)
		}

		// Update cache with known fingerprint status
		cache.GetGlobalManager().SetKnownFingerprint(vs.ctx.TenantID, fpID, leadID != nil)
	}

	shouldCreateNewVisit := true

	// Check if we should reuse existing visit
	if requestVisitID != nil && *requestVisitID != "" {
		latestVisit, err := vs.GetLatestVisitByFingerprint(fpID)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to get latest visit: %w", err)
		}

		// Reuse visit if it matches requested visit ID and hasn't expired
		if latestVisit != nil && latestVisit.ID == *requestVisitID && !vs.IsVisitExpired(latestVisit) {
			visitID = *requestVisitID
			shouldCreateNewVisit = false
		}
	}

	// Create new visit if needed (this will invalidate previous ones)
	if shouldCreateNewVisit {
		visitID = utils.GenerateULID()
		if err := vs.CreateVisit(visitID, fpID, nil); err != nil {
			return "", "", nil, fmt.Errorf("failed to create visit: %w", err)
		}

		// Update cache with new visit state
		visitState := &models.VisitState{
			VisitID:       visitID,
			FingerprintID: fpID,
			StartTime:     time.Now(),
			LastActivity:  time.Now(),
			CurrentPage:   "/",
		}
		cache.GetGlobalManager().SetVisitState(vs.ctx.TenantID, visitState)
	}

	// Get lead_id for profile restoration
	leadID, err := vs.GetFingerprintLeadID(fpID)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to check fingerprint lead: %w", err)
	}

	return fpID, visitID, leadID, nil
}
