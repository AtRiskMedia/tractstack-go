package api

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	defaults "github.com/AtRiskMedia/tractstack-go/config"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
	"github.com/gin-gonic/gin"
)

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

var (
	activeSSEConnections int64
	maxSSEConnections    = int64(defaults.MaxSessionsPerClient)
)

func NewVisitService(ctx *tenant.Context, _ any) *VisitService {
	return &VisitService{
		ctx: ctx,
	}
}

func VisitHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		log.Printf("DEBUG: VisitHandler - getTenantContext failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		log.Printf("DEBUG: VisitHandler - failed to get raw data: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read request"})
		return
	}

	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var req models.VisitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("DEBUG: VisitHandler - JSON binding failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	visitService := NewVisitService(ctx, nil)

	var finalFpID, finalVisitID string
	var hasProfile bool
	var profile *models.Profile

	if req.SessionID == nil {
		log.Printf("DEBUG: VisitHandler - session ID required but missing")
		c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required"})
		return
	}

	if existingSession, exists := cache.GetGlobalManager().GetSession(ctx.TenantID, *req.SessionID); exists {
		finalFpID = existingSession.FingerprintID
		if existingSession.LeadID != nil {
			if lead, err := GetLeadByID(*existingSession.LeadID, ctx); err == nil && lead != nil {
				profile = &models.Profile{
					Fingerprint:    finalFpID,
					LeadID:         lead.ID,
					Firstname:      lead.FirstName,
					Email:          lead.Email,
					ContactPersona: lead.ContactPersona,
					ShortBio:       lead.ShortBio,
				}
				hasProfile = true
			}
		}
	} else if req.EncryptedEmail != nil && req.EncryptedCode != nil {
		decryptedEmail, err := utils.Decrypt(*req.EncryptedEmail, ctx.Config.AESKey)
		if err != nil {
			log.Printf("DEBUG: VisitHandler - failed to decrypt email: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to decrypt email"})
			return
		}
		decryptedCode, err := utils.Decrypt(*req.EncryptedCode, ctx.Config.AESKey)
		if err != nil {
			log.Printf("DEBUG: VisitHandler - failed to decrypt code: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to decrypt code"})
			return
		}

		lead, err := ValidateLeadCredentials(decryptedEmail, decryptedCode, ctx)
		if err != nil || lead == nil {
			log.Printf("DEBUG: VisitHandler - lead validation failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		if existingFpID := visitService.FindFingerprintByLeadID(lead.ID); existingFpID != nil {
			finalFpID = *existingFpID
		} else {
			finalFpID = utils.GenerateULID()
		}

		if err := visitService.CreateFingerprint(finalFpID, &lead.ID); err != nil {
			if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				log.Printf("DEBUG: VisitHandler - failed to create fingerprint: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create fingerprint"})
				return
			}
		}

		profile = &models.Profile{
			Fingerprint:    finalFpID,
			LeadID:         lead.ID,
			Firstname:      lead.FirstName,
			Email:          lead.Email,
			ContactPersona: lead.ContactPersona,
			ShortBio:       lead.ShortBio,
		}
		hasProfile = true
	} else {
		finalFpID = utils.GenerateULID()
		if err := visitService.CreateFingerprint(finalFpID, nil); err != nil {
			// The only acceptable error here is a UNIQUE constraint violation, which means
			// a concurrent request created the fingerprint between ULID generation and here.
			// Any other error is a real problem.
			if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				log.Printf("DEBUG: VisitHandler - failed to create anonymous fingerprint: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create fingerprint"})
				return
			}
		}
		hasProfile = false
	}

	finalVisitID, err = visitService.HandleVisitCreation(finalFpID, hasProfile)
	if err != nil {
		log.Printf("DEBUG: VisitHandler - failed to create visit: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create visit"})
		return
	}

	var consentValue string
	if req.Consent != nil {
		consentValue = *req.Consent
	} else {
		consentValue = "unknown"
	}

	if cache.GetGlobalManager() != nil {
		visitState := &models.VisitState{
			VisitID:       finalVisitID,
			FingerprintID: finalFpID,
			StartTime:     time.Now().UTC(),
			LastActivity:  time.Now().UTC(),
			CurrentPage:   "",
		}
		cache.GetGlobalManager().SetVisitState(ctx.TenantID, visitState)

		sessionData := &models.SessionData{
			SessionID:     *req.SessionID,
			FingerprintID: finalFpID,
			VisitID:       finalVisitID,
			LastActivity:  time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
		}
		if hasProfile && profile != nil {
			sessionData.LeadID = &profile.LeadID
		}
		cache.GetGlobalManager().SetSession(ctx.TenantID, sessionData)
	} else {
		log.Printf("ERROR: Global cache manager is nil")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cache not available"})
		return
	}

	var fingerprintState *models.FingerprintState
	if existingFpState, exists := cache.GetGlobalManager().GetFingerprintState(ctx.TenantID, finalFpID); exists {
		fingerprintState = existingFpState
		fingerprintState.LastActivity = time.Now().UTC()
	} else {
		fingerprintState = &models.FingerprintState{
			FingerprintID: finalFpID,
			HeldBeliefs:   make(map[string][]string),
			HeldBadges:    make(map[string]string),
			LastActivity:  time.Now().UTC(),
		}
	}
	cache.GetGlobalManager().SetFingerprintState(ctx.TenantID, fingerprintState)

	response := gin.H{
		"fingerprint": finalFpID,
		"visitId":     finalVisitID,
		"hasProfile":  hasProfile,
		"consent":     consentValue,
	}

	if hasProfile && profile != nil {
		token, err := utils.GenerateProfileToken(profile, ctx.Config.JWTSecret, ctx.Config.AESKey)
		if err != nil {
			log.Printf("DEBUG: VisitHandler - failed to generate profile token: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate profile token"})
			return
		}
		response["token"] = token
		response["profile"] = profile
	}

	c.JSON(http.StatusOK, response)
}

func SseHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sessionID := c.Query("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID required for SSE connection"})
		return
	}

	currentConnections := atomic.LoadInt64(&activeSSEConnections)
	if currentConnections >= maxSSEConnections {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "SSE connection limit reached. Please try again later.",
		})
		return
	}

	maxSessionConnections := defaults.MaxSessionConnections
	sessionConnectionCount := models.Broadcaster.GetSessionConnectionCount(ctx.TenantID, sessionID)
	if sessionConnectionCount >= maxSessionConnections {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":              fmt.Sprintf("Too many SSE connections for session (max %d). Close some tabs and try again.", maxSessionConnections),
			"sessionId":          sessionID,
			"currentConnections": sessionConnectionCount,
		})
		return
	}

	atomic.AddInt64(&activeSSEConnections, 1)
	sseCtx, cancel := context.WithTimeout(c.Request.Context(),
		time.Duration(defaults.SSEConnectionTimeoutMinutes)*time.Minute)

	conn := &safeSSEConnection{
		ch: models.Broadcaster.AddClientWithSession(ctx.TenantID, sessionID),
	}

	defer func() {
		atomic.AddInt64(&activeSSEConnections, -1)
		cancel()
		models.Broadcaster.RemoveClientWithSession(conn.ch, ctx.TenantID, sessionID)

		if conn.SafeClose() {
			// This log is fine as it only appears once when the connection is truly closed.
			log.Printf("SSE connection handler terminated for session %s in tenant %s", sessionID, ctx.TenantID)
		}

		if r := recover(); r != nil {
			log.Printf("SSE cleanup panic recovered for session %s in tenant %s: %v", sessionID, ctx.TenantID, r)
		}
	}()

	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := w.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ready\",\"sessionId\":\"%s\",\"tenantId\":\"%s\",\"connectionCount\":%d}\n\n",
		sessionID, ctx.TenantID, models.Broadcaster.GetSessionConnectionCount(ctx.TenantID, sessionID))
	flusher.Flush()

	heartbeat := time.NewTicker(time.Duration(defaults.SSEHeartbeatIntervalSeconds) * time.Second)
	defer heartbeat.Stop()

	inactivityTimeout := time.NewTimer(time.Duration(defaults.SSEInactivityTimeoutMinutes) * time.Minute)
	defer inactivityTimeout.Stop()

	for {
		select {
		case msg, ok := <-conn.ch:
			if !ok {
				return // Channel closed, connection is finished.
			}

			if !inactivityTimeout.Stop() {
				select {
				case <-inactivityTimeout.C:
				default:
				}
			}
			inactivityTimeout.Reset(time.Duration(defaults.SSEInactivityTimeoutMinutes) * time.Minute)

			_, err := fmt.Fprint(w, msg)
			if err != nil {
				if !isClientDisconnectError(err) {
					// If it's not a broken pipe, it's an unexpected error that we should log.
					log.Printf("Unexpected SSE write error for session %s: %v", sessionID, err)
				}
				// In either case, the connection is dead, so we exit.
				return
			}
			flusher.Flush()

		case <-heartbeat.C:
			if !inactivityTimeout.Stop() {
				select {
				case <-inactivityTimeout.C:
				default:
				}
			}
			inactivityTimeout.Reset(time.Duration(defaults.SSEInactivityTimeoutMinutes) * time.Minute)

			// We construct the message before writing to avoid partial writes.
			heartbeatMsg := fmt.Sprintf("event: heartbeat\ndata: {\"timestamp\":%d,\"sessionId\":\"%s\",\"tenantId\":\"%s\"}\n\n", time.Now().UTC().Unix(), sessionID, ctx.TenantID)
			_, err := fmt.Fprint(w, heartbeatMsg)
			if err != nil {
				if !isClientDisconnectError(err) {
					log.Printf("Unexpected SSE heartbeat write error for session %s: %v", sessionID, err)
				}
				return
			}
			flusher.Flush()

		case <-inactivityTimeout.C:
			log.Printf("SSE connection closed due to inactivity for session %s", sessionID)
			fmt.Fprintf(w, "event: timeout\ndata: {\"reason\":\"inactivity\",\"sessionId\":\"%s\"}\n\n", sessionID)
			flusher.Flush()
			return

		case <-sseCtx.Done():
			// This case handles both the 30-minute max duration and client disconnects detected by the context.
			// It's a clean way to terminate the connection. No error logging is needed here.
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

	_, err := vs.ctx.Database.Conn.Exec(query, visitID, fingerprintID, campaignID, time.Now().UTC())
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

	_, err := vs.ctx.Database.Conn.Exec(query, fingerprintID, leadID, time.Now().UTC())
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

func (vs *VisitService) HandleVisitCreation(fingerprintID string, hasProfile bool) (string, error) {
	if latestVisit, err := vs.GetLatestVisitByFingerprint(fingerprintID); err == nil && latestVisit != nil {
		if time.Since(latestVisit.CreatedAt) < 2*time.Hour {
			return latestVisit.ID, nil
		}
	}

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

func (vs *VisitService) UpdateFingerprintLeadID(fingerprintID string, leadID *string) error {
	query := `UPDATE fingerprints SET lead_id = ? WHERE id = ?`

	_, err := vs.ctx.Database.Conn.Exec(query, leadID, fingerprintID)
	if err != nil {
		return fmt.Errorf("failed to update fingerprint lead ID: %w", err)
	}

	return nil
}
