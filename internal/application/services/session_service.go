// Package services provides application-level orchestration services
package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"golang.org/x/crypto/bcrypt"
)

// Database semaphore to limit concurrent database operations
var dbSemaphore = make(chan struct{}, 100) // Allow 100 concurrent DB operations

// SessionService handles session management, fingerprinting, and user authentication
type SessionService struct {
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewSessionService creates a new session service
func NewSessionService(logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *SessionService {
	return &SessionService{
		logger:      logger,
		perfTracker: perfTracker,
	}
}

// SessionResult holds the result of session operations
type SessionResult struct {
	FingerprintID string        `json:"fingerprint"`
	VisitID       string        `json:"visitId"`
	HasProfile    bool          `json:"hasProfile"`
	Profile       *user.Profile `json:"profile,omitempty"`
	Token         string        `json:"token,omitempty"`
	Consent       string        `json:"consent"`
	Success       bool          `json:"success"`
	Error         string        `json:"error,omitempty"`
}

// VisitRequest represents the structure for visit creation requests
type VisitRequest struct {
	SessionID      *string `json:"sessionId,omitempty"`
	EncryptedEmail *string `json:"encryptedEmail,omitempty"`
	EncryptedCode  *string `json:"encryptedCode,omitempty"`
	Consent        *string `json:"consent,omitempty"`
}

// SessionResponse holds session handling response data
type SessionResponse struct {
	Fingerprint string `json:"fingerprint"`
	VisitID     string `json:"visitId"`
}

// VisitRowData represents visit data from database
type VisitRowData struct {
	ID            string
	FingerprintID string
	CampaignID    *string
	CreatedAt     time.Time
}

// ProcessVisitRequest handles the complete visit creation workflow
func (s *SessionService) ProcessVisitRequest(req *VisitRequest, tenantCtx *tenant.Context) *SessionResult {
	if req.SessionID == nil {
		return &SessionResult{
			Success: false,
			Error:   "session ID required",
		}
	}

	var finalFpID, finalVisitID string
	var hasProfile bool
	var profile *user.Profile

	if existingSession, exists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, *req.SessionID); exists {
		finalFpID = existingSession.FingerprintID
		finalVisitID = existingSession.VisitID

		if existingSession.LeadID != nil {
			if lead, err := s.GetLeadByID(*existingSession.LeadID, tenantCtx); err == nil && lead != nil {
				profile = &user.Profile{
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
		result := s.processEncryptedAuthentication(*req.EncryptedEmail, *req.EncryptedCode, tenantCtx)
		if !result.Success {
			return result
		}
		finalFpID = result.FingerprintID
		profile = result.Profile
		hasProfile = true
	} else {
		finalFpID = security.GenerateULID()
		if err := s.CreateFingerprint(finalFpID, nil, tenantCtx); err != nil {
			if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return &SessionResult{
					Success: false,
					Error:   "failed to create fingerprint",
				}
			}
		}
		hasProfile = false
	}

	if finalVisitID == "" {
		var err error
		finalVisitID, err = s.HandleVisitCreation(finalFpID, hasProfile, tenantCtx)
		if err != nil {
			return &SessionResult{
				Success: false,
				Error:   "failed to create visit",
			}
		}
	}

	var consentValue string
	if req.Consent != nil {
		consentValue = *req.Consent
	} else {
		consentValue = "unknown"
	}

	s.updateCacheStates(tenantCtx, *req.SessionID, finalFpID, finalVisitID)

	var token string
	if hasProfile && profile != nil {
		generatedToken, err := security.GenerateProfileToken(profile, tenantCtx.Config.JWTSecret, tenantCtx.Config.AESKey)
		if err != nil {
			log.Printf("Failed to generate profile token: %v", err)
		} else {
			token = generatedToken
		}
	}

	return &SessionResult{
		FingerprintID: finalFpID,
		VisitID:       finalVisitID,
		HasProfile:    hasProfile,
		Profile:       profile,
		Token:         token,
		Consent:       consentValue,
		Success:       true,
	}
}

// processEncryptedAuthentication handles authentication with encrypted credentials
func (s *SessionService) processEncryptedAuthentication(encryptedEmail, encryptedCode string, tenantCtx *tenant.Context) *SessionResult {
	decryptedEmail, err := security.Decrypt(encryptedEmail, tenantCtx.Config.AESKey)
	if err != nil {
		return &SessionResult{
			Success: false,
			Error:   "failed to decrypt email",
		}
	}

	decryptedCode, err := security.Decrypt(encryptedCode, tenantCtx.Config.AESKey)
	if err != nil {
		return &SessionResult{
			Success: false,
			Error:   "failed to decrypt code",
		}
	}

	lead, err := s.ValidateLeadCredentials(decryptedEmail, decryptedCode, tenantCtx)
	if err != nil || lead == nil {
		return &SessionResult{
			Success: false,
			Error:   "invalid credentials",
		}
	}

	var finalFpID string
	if existingFpID := s.FindFingerprintByLeadID(lead.ID, tenantCtx); existingFpID != nil {
		finalFpID = *existingFpID
	} else {
		finalFpID = security.GenerateULID()
	}

	finalVisitID, err := s.HandleVisitCreation(finalFpID, true, tenantCtx)
	if err != nil {
		return &SessionResult{
			Success: false,
			Error:   "failed to create visit",
		}
	}

	if err := s.CreateFingerprint(finalFpID, &lead.ID, tenantCtx); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return &SessionResult{
				Success: false,
				Error:   "failed to create fingerprint",
			}
		}
	}

	profile := &user.Profile{
		Fingerprint:    finalFpID,
		LeadID:         lead.ID,
		Firstname:      lead.FirstName,
		Email:          lead.Email,
		ContactPersona: lead.ContactPersona,
		ShortBio:       lead.ShortBio,
	}

	return &SessionResult{
		FingerprintID: finalFpID,
		VisitID:       finalVisitID,
		Profile:       profile,
		Success:       true,
	}
}

func (s *SessionService) HandleVisitCreation(fingerprintID string, hasProfile bool, tenantCtx *tenant.Context) (string, error) {
	dbSemaphore <- struct{}{}
	defer func() { <-dbSemaphore }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if latestVisit, err := s.GetLatestVisitByFingerprint(fingerprintID, tenantCtx); err == nil && latestVisit != nil {
		if time.Since(latestVisit.CreatedAt) < 2*time.Hour {
			return latestVisit.ID, nil
		}
	}

	visitID := security.GenerateULID()
	query := `INSERT INTO visits (id, fingerprint_id, created_at) VALUES (?, ?, ?)`
	_, err := tenantCtx.Database.Conn.ExecContext(ctx, query, visitID, fingerprintID, time.Now().UTC())
	if err != nil {
		return "", fmt.Errorf("failed to create visit: %w", err)
	}

	return visitID, nil
}

func (s *SessionService) GetLatestVisitByFingerprint(fingerprintID string, tenantCtx *tenant.Context) (*VisitRowData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT id, fingerprint_id, campaign_id, created_at
              FROM visits
              WHERE fingerprint_id = ?
              ORDER BY created_at DESC
              LIMIT 1`

	row := tenantCtx.Database.Conn.QueryRowContext(ctx, query, fingerprintID)

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

// CreateFingerprint creates a new fingerprint record
func (s *SessionService) CreateFingerprint(fingerprintID string, leadID *string, tenantCtx *tenant.Context) error {
	dbSemaphore <- struct{}{}
	defer func() { <-dbSemaphore }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `INSERT INTO fingerprints (id, lead_id, created_at) VALUES (?, ?, ?)`
	_, err := tenantCtx.Database.Conn.ExecContext(ctx, query, fingerprintID, leadID, time.Now().UTC())
	return err
}

// FindFingerprintByLeadID finds an existing fingerprint for a lead
func (s *SessionService) FindFingerprintByLeadID(leadID string, tenantCtx *tenant.Context) *string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var fingerprintID string
	query := `SELECT id FROM fingerprints WHERE lead_id = ? LIMIT 1`
	err := tenantCtx.Database.Conn.QueryRowContext(ctx, query, leadID).Scan(&fingerprintID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		log.Printf("Error finding fingerprint by lead ID: %v", err)
		return nil
	}
	return &fingerprintID
}

// GetLeadByFingerprint retrieves a lead associated with a fingerprint
func (s *SessionService) GetLeadByFingerprint(fingerprintID string, tenantCtx *tenant.Context) (*user.Lead, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT l.id, l.first_name, l.email, l.password_hash, l.contact_persona, l.short_bio, l.encrypted_code, l.encrypted_email, l.created_at, l.changed
		FROM leads l
		JOIN fingerprints f ON l.id = f.lead_id
		WHERE f.id = ?
		LIMIT 1
	`

	var lead user.Lead
	var shortBio, encryptedCode, encryptedEmail sql.NullString
	var changed sql.NullTime

	err := tenantCtx.Database.Conn.QueryRowContext(ctx, query, fingerprintID).Scan(
		&lead.ID,
		&lead.FirstName,
		&lead.Email,
		&lead.PasswordHash,
		&lead.ContactPersona,
		&shortBio,
		&encryptedCode,
		&encryptedEmail,
		&lead.CreatedAt,
		&changed,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lead by fingerprint: %w", err)
	}

	if shortBio.Valid {
		lead.ShortBio = shortBio.String
	}
	if encryptedCode.Valid {
		lead.EncryptedCode = encryptedCode.String
	}
	if encryptedEmail.Valid {
		lead.EncryptedEmail = encryptedEmail.String
	}
	if changed.Valid {
		lead.Changed = changed.Time
	}

	return &lead, nil
}

// GetLeadByID retrieves a lead by ID
func (s *SessionService) GetLeadByID(leadID string, tenantCtx *tenant.Context) (*user.Lead, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT id, first_name, email, password_hash, contact_persona, short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE id = ?
		LIMIT 1
	`

	var lead user.Lead
	var shortBio, encryptedCode, encryptedEmail sql.NullString
	var changed sql.NullTime

	err := tenantCtx.Database.Conn.QueryRowContext(ctx, query, leadID).Scan(
		&lead.ID,
		&lead.FirstName,
		&lead.Email,
		&lead.PasswordHash,
		&lead.ContactPersona,
		&shortBio,
		&encryptedCode,
		&encryptedEmail,
		&lead.CreatedAt,
		&changed,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("lead not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lead by ID: %w", err)
	}

	if shortBio.Valid {
		lead.ShortBio = shortBio.String
	}
	if encryptedCode.Valid {
		lead.EncryptedCode = encryptedCode.String
	}
	if encryptedEmail.Valid {
		lead.EncryptedEmail = encryptedEmail.String
	}
	if changed.Valid {
		lead.Changed = changed.Time
	}

	return &lead, nil
}

// ValidateLeadCredentials validates email and password credentials
func (s *SessionService) ValidateLeadCredentials(email, password string, tenantCtx *tenant.Context) (*user.Lead, error) {
	lead, err := s.GetLeadByEmail(email, tenantCtx)
	if err != nil || lead == nil {
		return nil, fmt.Errorf("lead not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(lead.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	return lead, nil
}

// GetLeadByEmail retrieves a lead by email
func (s *SessionService) GetLeadByEmail(email string, tenantCtx *tenant.Context) (*user.Lead, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT id, first_name, email, password_hash, contact_persona, short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE email = ?
		LIMIT 1
	`

	var lead user.Lead
	var shortBio, encryptedCode, encryptedEmail sql.NullString
	var changed sql.NullTime

	err := tenantCtx.Database.Conn.QueryRowContext(ctx, query, email).Scan(
		&lead.ID,
		&lead.FirstName,
		&lead.Email,
		&lead.PasswordHash,
		&lead.ContactPersona,
		&shortBio,
		&encryptedCode,
		&encryptedEmail,
		&lead.CreatedAt,
		&changed,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lead by email: %w", err)
	}

	if shortBio.Valid {
		lead.ShortBio = shortBio.String
	}
	if encryptedCode.Valid {
		lead.EncryptedCode = encryptedCode.String
	}
	if encryptedEmail.Valid {
		lead.EncryptedEmail = encryptedEmail.String
	}
	if changed.Valid {
		lead.Changed = changed.Time
	}

	return &lead, nil
}

// HandleProfileSession handles session creation/update for profile operations
func (s *SessionService) HandleProfileSession(tenantCtx *tenant.Context, profile *user.Profile, sessionID string) (*SessionResponse, error) {
	var fingerprintID string

	if existingFpID := s.FindFingerprintByLeadID(profile.LeadID, tenantCtx); existingFpID != nil {
		fingerprintID = *existingFpID
	} else {
		if existingSession, exists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, sessionID); exists {
			fingerprintID = existingSession.FingerprintID
		} else {
			fingerprintID = security.GenerateULID()
		}
	}

	if err := s.CreateFingerprint(fingerprintID, &profile.LeadID, tenantCtx); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("failed to create fingerprint: %w", err)
		}
	}

	var visitID string
	if existingSession, exists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, sessionID); exists {
		visitID = existingSession.VisitID
	} else {
		var err error
		visitID, err = s.HandleVisitCreation(fingerprintID, true, tenantCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to create visit: %w", err)
		}
	}

	sessionData := &types.SessionData{
		SessionID:     sessionID,
		FingerprintID: fingerprintID,
		VisitID:       visitID,
		LeadID:        &profile.LeadID,
		LastActivity:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		IsExpired:     false,
	}

	tenantCtx.CacheManager.SetSession(tenantCtx.TenantID, sessionData)

	fingerprintState := &types.FingerprintState{
		FingerprintID: fingerprintID,
		LeadID:        &profile.LeadID,
		HeldBeliefs:   make(map[string][]string),
		HeldBadges:    make(map[string]string),
		LastActivity:  time.Now().UTC(),
	}
	tenantCtx.CacheManager.SetFingerprintState(tenantCtx.TenantID, fingerprintState)

	visitState := &types.VisitState{
		VisitID:       visitID,
		FingerprintID: fingerprintID,
		StartTime:     time.Now().UTC(),
		CurrentPage:   "/",
		CreatedAt:     time.Now().UTC(),
		LastActivity:  time.Now().UTC(),
	}
	tenantCtx.CacheManager.SetVisitState(tenantCtx.TenantID, visitState)

	return &SessionResponse{
		Fingerprint: fingerprintID,
		VisitID:     visitID,
	}, nil
}

// updateCacheStates updates all cache states with new session data
func (s *SessionService) updateCacheStates(tenantCtx *tenant.Context, sessionID, fingerprintID, visitID string) {
	cacheManager := tenantCtx.CacheManager

	sessionData := &types.SessionData{
		SessionID:     sessionID,
		FingerprintID: fingerprintID,
		VisitID:       visitID,
		LeadID:        nil,
		LastActivity:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		IsExpired:     false,
	}

	if lead, err := s.GetLeadByFingerprint(fingerprintID, tenantCtx); err == nil && lead != nil {
		sessionData.LeadID = &lead.ID
	}

	cacheManager.SetSession(tenantCtx.TenantID, sessionData)

	var fingerprintState *types.FingerprintState
	if existingFpState, exists := cacheManager.GetFingerprintState(tenantCtx.TenantID, fingerprintID); exists {
		fingerprintState = existingFpState
		fingerprintState.LastActivity = time.Now().UTC()
	} else {
		fingerprintState = &types.FingerprintState{
			FingerprintID: fingerprintID,
			HeldBeliefs:   make(map[string][]string),
			HeldBadges:    make(map[string]string),
			LastActivity:  time.Now().UTC(),
		}
	}
	cacheManager.SetFingerprintState(tenantCtx.TenantID, fingerprintState)

	visitState := &types.VisitState{
		VisitID:       visitID,
		FingerprintID: fingerprintID,
		StartTime:     time.Now().UTC(),
		CurrentPage:   "/",
		CreatedAt:     time.Now().UTC(),
		LastActivity:  time.Now().UTC(),
	}
	cacheManager.SetVisitState(tenantCtx.TenantID, visitState)
}
