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

// Global SSE connection tracking
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
	// log.Printf("DEBUG: VisitHandler - got tenant context for tenant: %s", ctx.TenantID)

	// Log the raw request for debugging
	body, err := c.GetRawData()
	if err != nil {
		log.Printf("DEBUG: VisitHandler - failed to get raw data: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read request"})
		return
	}

	// Reset the body for binding
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// Parse form data
	var req models.VisitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("DEBUG: VisitHandler - JSON binding failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}
	// log.Printf("DEBUG: VisitHandler - JSON binding success")

	visitService := NewVisitService(ctx, nil)

	// Determine if we're handling profile/auth or anonymous visit
	var finalFpID, finalVisitID string
	var hasProfile bool
	var profile *models.Profile

	// log.Printf("DEBUG: VisitHandler - checking request type (encrypted email: %v, session ID: %v)",
	//	req.EncryptedEmail != nil, req.SessionID != nil)

	if req.EncryptedEmail != nil && req.EncryptedCode != nil {
		// log.Printf("DEBUG: VisitHandler - handling encrypted email/code scenario")

		// Handle encrypted email/code scenario
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

		// Verify lead exists
		lead, err := ValidateLeadCredentials(decryptedEmail, decryptedCode, ctx)
		if err != nil || lead == nil {
			log.Printf("DEBUG: VisitHandler - lead validation failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		// log.Printf("DEBUG: VisitHandler - lead validation successful for lead %s", lead.ID)

		// PRIORITY 1: Always use existing fingerprint for this lead
		if existingFpID := visitService.FindFingerprintByLeadID(lead.ID); existingFpID != nil {
			finalFpID = *existingFpID
			// log.Printf("DEBUG: VisitHandler - PRIORITY 1: Using existing lead fingerprint %s for lead %s", finalFpID, lead.ID)
		} else {
			// PRIORITY 2: No existing lead fingerprint, check if session has one
			if req.SessionID != nil {
				if existingSession, exists := cache.GetGlobalManager().GetSession(ctx.TenantID, *req.SessionID); exists {
					finalFpID = existingSession.FingerprintID
					// log.Printf("DEBUG: VisitHandler - PRIORITY 2: Using session fingerprint %s for lead %s", finalFpID, lead.ID)

					// Link this fingerprint to the lead
					if err := visitService.UpdateFingerprintLeadID(finalFpID, &lead.ID); err != nil {
						log.Printf("ERROR: VisitHandler - Failed to link fingerprint to lead: %v", err)
						//} else {
						//	log.Printf("DEBUG: VisitHandler - Successfully linked fingerprint %s to lead %s", finalFpID, lead.ID)
					}
				} else {
					// PRIORITY 3: Generate new fingerprint
					finalFpID = utils.GenerateULID()
					// log.Printf("DEBUG: VisitHandler - PRIORITY 3: Generated new fingerprint %s for lead %s", finalFpID, lead.ID)
				}
			} else {
				// PRIORITY 3: Generate new fingerprint
				finalFpID = utils.GenerateULID()
				// log.Printf("DEBUG: VisitHandler - PRIORITY 3: Generated new fingerprint %s for lead %s (no session)", finalFpID, lead.ID)
			}

			// Create fingerprint if it doesn't exist
			if err := visitService.CreateFingerprint(finalFpID, &lead.ID); err != nil {
				log.Printf("DEBUG: VisitHandler - failed to create fingerprint: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create fingerprint"})
				return
			}
		}

		// Create profile
		profile = &models.Profile{
			Fingerprint:    finalFpID,
			LeadID:         lead.ID,
			Firstname:      lead.FirstName,
			Email:          lead.Email,
			ContactPersona: lead.ContactPersona,
			ShortBio:       lead.ShortBio,
		}

		hasProfile = true
		// log.Printf("DEBUG: VisitHandler - created profile for authenticated user with fingerprint %s", finalFpID)

	} else {
		// log.Printf("DEBUG: VisitHandler - handling anonymous visit")
		// Anonymous visit - generate ULID fingerprint (NOT session ID)
		if req.SessionID == nil {
			log.Printf("DEBUG: VisitHandler - session ID required but missing")
			c.JSON(http.StatusBadRequest, gin.H{"error": "session ID required for anonymous visits"})
			return
		}

		finalFpID = utils.GenerateULID()
		// log.Printf("DEBUG: VisitHandler - generated fingerprint %s for anonymous visit", finalFpID)

		// Check if fingerprint exists, create if not
		exists, err := visitService.FingerprintExists(finalFpID)
		if err != nil {
			// log.Printf("DEBUG: VisitHandler - failed to check fingerprint existence: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check fingerprint"})
			return
		}

		if !exists {
			if err := visitService.CreateFingerprint(finalFpID, nil); err != nil {
				// Check if it's a UNIQUE constraint error (race condition)
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					// log.Printf("DEBUG: VisitHandler - fingerprint already created by concurrent request, continuing")
				} else {
					log.Printf("DEBUG: VisitHandler - failed to create fingerprint: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create fingerprint"})
					return
				}
			}
		}

		hasProfile = false
	}

	// log.Printf("DEBUG: VisitHandler - final fingerprint: %s", finalFpID)

	// Handle visit creation
	finalVisitID, err = visitService.HandleVisitCreation(finalFpID, hasProfile)
	if err != nil {
		log.Printf("DEBUG: VisitHandler - failed to create visit: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create visit"})
		return
	}
	// log.Printf("DEBUG: VisitHandler - visit creation successful, ID: %s", finalVisitID)

	// Determine consent value
	var consentValue string
	if req.Consent != nil {
		consentValue = *req.Consent
	} else {
		consentValue = "unknown"
	}

	// Cache user state
	if cache.GetGlobalManager() != nil {
		// Create visit state
		visitState := &models.VisitState{
			VisitID:       finalVisitID,
			FingerprintID: finalFpID,
			StartTime:     time.Now(),
			LastActivity:  time.Now(),
			CurrentPage:   "",
		}
		cache.GetGlobalManager().SetVisitState(ctx.TenantID, visitState)

		// Create session state
		sessionData := &models.SessionData{
			SessionID:     *req.SessionID, // Use actual session ID
			FingerprintID: finalFpID,      // Use proper fingerprint ID
			VisitID:       finalVisitID,
			LastActivity:  time.Now(),
			CreatedAt:     time.Now(),
		}
		if hasProfile && profile != nil {
			sessionData.LeadID = &profile.LeadID
		}
		cache.GetGlobalManager().SetSession(ctx.TenantID, sessionData)
		// log.Printf("DEBUG: VisitHandler - cached session data: session=%s, fingerprint=%s", *req.SessionID, finalFpID)
	} else {
		log.Printf("ERROR: Global cache manager is nil")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cache not available"})
		return
	}

	// Update fingerprint state cache
	fingerprintState := &models.FingerprintState{
		FingerprintID: finalFpID,
		HeldBeliefs:   make(map[string][]string),
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
			log.Printf("DEBUG: VisitHandler - failed to generate profile token: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate profile token"})
			return
		}
		response["token"] = token
		response["profile"] = profile
	}

	// log.Printf("DEBUG: VisitHandler - sending successful response with fingerprint %s", finalFpID)
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

	storyfragmentID := c.Query("storyfragment")

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
	ch := models.Broadcaster.AddClientWithSession(ctx.TenantID, sessionID)

	if storyfragmentID != "" {
		models.Broadcaster.RegisterStoryfragmentSubscription(ctx.TenantID, sessionID, storyfragmentID)
		// log.Printf("SSE: Registered session %s with storyfragment %s in tenant %s",
		//	sessionID, storyfragmentID, ctx.TenantID)
	}

	defer func() {
		atomic.AddInt64(&activeSSEConnections, -1)
		cancel()
		models.Broadcaster.RemoveClientWithSession(ch, ctx.TenantID, sessionID)
		models.Broadcaster.UnregisterStoryfragmentSubscription(ctx.TenantID, sessionID)

		select {
		case <-ch:
		default:
			close(ch)
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

	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ready\",\"sessionId\":\"%s\",\"tenantId\":\"%s\",\"storyfragmentId\":\"%s\",\"connectionCount\":%d}\n\n",
		sessionID, ctx.TenantID, storyfragmentID, models.Broadcaster.GetSessionConnectionCount(ctx.TenantID, sessionID))
	flusher.Flush()

	heartbeat := time.NewTicker(time.Duration(defaults.SSEHeartbeatIntervalSeconds) * time.Second)
	defer heartbeat.Stop()

	inactivityTimeout := time.NewTimer(time.Duration(defaults.SSEInactivityTimeoutMinutes) * time.Minute)
	defer inactivityTimeout.Stop()

	// log.Printf("SSE connection established for session %s in tenant %s (storyfragment: %s). Active connections: %d (session: %d)",
	//	sessionID, ctx.TenantID, storyfragmentID, atomic.LoadInt64(&activeSSEConnections),
	//	models.Broadcaster.GetSessionConnectionCount(ctx.TenantID, sessionID))

	for {
		select {
		case msg := <-ch:
			if !inactivityTimeout.Stop() {
				select {
				case <-inactivityTimeout.C:
				default:
				}
			}
			inactivityTimeout.Reset(time.Duration(defaults.SSEInactivityTimeoutMinutes) * time.Minute)

			_, err := fmt.Fprint(w, msg)
			if err != nil {
				log.Printf("SSE write error for session %s in tenant %s: %v", sessionID, ctx.TenantID, err)
				return
			}
			flusher.Flush()

		case <-heartbeat.C:
			_, err := fmt.Fprint(w, "event: heartbeat\ndata: {\"timestamp\":")
			if err != nil {
				log.Printf("SSE heartbeat failed for session %s in tenant %s: %v", sessionID, ctx.TenantID, err)
				return
			}
			_, err = fmt.Fprintf(w, "%d,\"sessionId\":\"%s\",\"tenantId\":\"%s\"}\n\n",
				time.Now().Unix(), sessionID, ctx.TenantID)
			if err != nil {
				log.Printf("SSE heartbeat failed for session %s in tenant %s: %v", sessionID, ctx.TenantID, err)
				return
			}
			flusher.Flush()

		case <-inactivityTimeout.C:
			log.Printf("SSE connection closed due to inactivity for session %s in tenant %s", sessionID, ctx.TenantID)
			fmt.Fprintf(w, "event: timeout\ndata: {\"reason\":\"inactivity\",\"sessionId\":\"%s\",\"tenantId\":\"%s\"}\n\n",
				sessionID, ctx.TenantID)
			flusher.Flush()
			return

		case <-sseCtx.Done():
			switch sseCtx.Err() {
			case context.DeadlineExceeded:
				log.Printf("SSE connection closed due to 30-minute timeout for session %s in tenant %s - client should reconnect",
					sessionID, ctx.TenantID)
				fmt.Fprintf(w, "event: timeout\ndata: {\"reason\":\"max_duration\",\"action\":\"reconnect\",\"sessionId\":\"%s\",\"tenantId\":\"%s\"}\n\n",
					sessionID, ctx.TenantID)
			// case context.Canceled:
			//	log.Printf("SSE connection closed by client for session %s in tenant %s", sessionID, ctx.TenantID)
			default:
				// log.Printf("SSE connection closed for session %s in tenant %s: %v", sessionID, ctx.TenantID, sseCtx.Err())
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

func (vs *VisitService) UpdateFingerprintLeadID(fingerprintID string, leadID *string) error {
	query := `UPDATE fingerprints SET lead_id = ? WHERE id = ?`

	_, err := vs.ctx.Database.Conn.Exec(query, leadID, fingerprintID)
	if err != nil {
		return fmt.Errorf("failed to update fingerprint lead ID: %w", err)
	}

	return nil
}
