// Package services provides application-level orchestration services
package services

import (
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

	// Check for existing session first
	if existingSession, exists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, *req.SessionID); exists {
		finalFpID = existingSession.FingerprintID
		// FIXED: Check if session has lead info (without LeadID field)
		// We'll need to look up the lead by fingerprint instead
		if lead, err := s.GetLeadByFingerprint(finalFpID, tenantCtx); err == nil && lead != nil {
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
	} else if req.EncryptedEmail != nil && req.EncryptedCode != nil {
		// Handle authentication with encrypted credentials
		result := s.processEncryptedAuthentication(*req.EncryptedEmail, *req.EncryptedCode, tenantCtx)
		if !result.Success {
			return result
		}
		finalFpID = result.FingerprintID
		profile = result.Profile
		hasProfile = true
	} else {
		// Create anonymous fingerprint
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

	// Create visit
	var err error
	finalVisitID, err = s.HandleVisitCreation(finalFpID, hasProfile, tenantCtx)
	if err != nil {
		return &SessionResult{
			Success: false,
			Error:   "failed to create visit",
		}
	}

	// Determine consent value
	var consentValue string
	if req.Consent != nil {
		consentValue = *req.Consent
	} else {
		consentValue = "unknown"
	}

	// Update cache states
	s.updateCacheStates(tenantCtx, *req.SessionID, finalFpID, finalVisitID)

	// Generate token if profile exists
	var token string
	if hasProfile && profile != nil {
		token, err = security.GenerateProfileToken(profile, tenantCtx.Config.JWTSecret, tenantCtx.Config.AESKey)
		if err != nil {
			log.Printf("Failed to generate profile token: %v", err)
			// Don't fail the request, just log the error
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
	// Decrypt credentials
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

	// Validate credentials
	lead, err := s.ValidateLeadCredentials(decryptedEmail, decryptedCode, tenantCtx)
	if err != nil || lead == nil {
		return &SessionResult{
			Success: false,
			Error:   "invalid credentials",
		}
	}

	// Check for existing fingerprint
	var finalFpID string
	if existingFpID := s.FindFingerprintByLeadID(lead.ID, tenantCtx); existingFpID != nil {
		finalFpID = *existingFpID
	} else {
		finalFpID = security.GenerateULID()
	}

	// Create or link fingerprint
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
		Profile:       profile,
		Success:       true,
	}
}

// HandleVisitCreation creates a new visit for the given fingerprint
func (s *SessionService) HandleVisitCreation(fingerprintID string, hasProfile bool, tenantCtx *tenant.Context) (string, error) {
	visitID := security.GenerateULID()

	query := `INSERT INTO visits (id, fingerprint_id, created_at) VALUES (?, ?, ?)`
	_, err := tenantCtx.Database.Conn.Exec(query, visitID, fingerprintID, time.Now().UTC())
	if err != nil {
		return "", fmt.Errorf("failed to create visit: %w", err)
	}

	return visitID, nil
}

// CreateFingerprint creates a new fingerprint record
func (s *SessionService) CreateFingerprint(fingerprintID string, leadID *string, tenantCtx *tenant.Context) error {
	query := `INSERT INTO fingerprints (id, lead_id, created_at) VALUES (?, ?, ?)`
	_, err := tenantCtx.Database.Conn.Exec(query, fingerprintID, leadID, time.Now().UTC())
	return err
}

// FindFingerprintByLeadID finds an existing fingerprint for a lead
func (s *SessionService) FindFingerprintByLeadID(leadID string, tenantCtx *tenant.Context) *string {
	var fingerprintID string
	query := `SELECT id FROM fingerprints WHERE lead_id = ? LIMIT 1`
	err := tenantCtx.Database.Conn.QueryRow(query, leadID).Scan(&fingerprintID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		log.Printf("Error finding fingerprint by lead ID: %v", err)
		return nil
	}
	return &fingerprintID
}

func (s *SessionService) GetLeadByFingerprint(fingerprintID string, tenantCtx *tenant.Context) (*user.Lead, error) {
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

	err := tenantCtx.Database.Conn.QueryRow(query, fingerprintID).Scan(
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

	// Handle nullable fields
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
	query := `
		SELECT id, first_name, email, password_hash, contact_persona, short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE id = ?
		LIMIT 1
	`

	var lead user.Lead
	var shortBio, encryptedCode, encryptedEmail sql.NullString
	var changed sql.NullTime

	err := tenantCtx.Database.Conn.QueryRow(query, leadID).Scan(
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
		return nil, fmt.Errorf("failed to get lead by ID: %w", err)
	}

	// Handle nullable fields
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

// ValidateLeadCredentials validates email and password against stored lead
func (s *SessionService) ValidateLeadCredentials(email, password string, tenantCtx *tenant.Context) (*user.Lead, error) {
	lead, err := s.GetLeadByEmail(email, tenantCtx)
	if err != nil {
		return nil, err
	}
	if lead == nil {
		return nil, nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(lead.PasswordHash), []byte(password)); err != nil {
		return nil, nil // Invalid password
	}

	return lead, nil
}

// GetLeadByEmail retrieves a lead by email address
func (s *SessionService) GetLeadByEmail(email string, tenantCtx *tenant.Context) (*user.Lead, error) {
	query := `
		SELECT id, first_name, email, password_hash, contact_persona, short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE email = ?
		LIMIT 1
	`

	var lead user.Lead
	var shortBio, encryptedCode, encryptedEmail sql.NullString
	var changed sql.NullTime

	err := tenantCtx.Database.Conn.QueryRow(query, email).Scan(
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

	// Handle nullable fields
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

// updateCacheStates updates visit state, session data, and fingerprint state in cache
func (s *SessionService) updateCacheStates(tenantCtx *tenant.Context, sessionID, fingerprintID, visitID string) {
	cacheManager := tenantCtx.CacheManager

	// FIXED: Update visit state using correct types
	visitState := &types.VisitState{
		VisitID:       visitID,
		FingerprintID: fingerprintID,
		LastActivity:  time.Now().UTC(),
	}
	cacheManager.SetVisitState(tenantCtx.TenantID, visitState)

	sessionData := &types.SessionData{
		SessionID:     sessionID,
		FingerprintID: fingerprintID,
		VisitID:       visitID,
		LastActivity:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}
	// Note: The new SessionData type doesn't have LeadID field
	// Lead association is handled through fingerprint linking
	cacheManager.SetSession(tenantCtx.TenantID, sessionData)

	// Update fingerprint state
	var fingerprintState *types.FingerprintState
	if existingFpState, exists := cacheManager.GetFingerprintState(tenantCtx.TenantID, fingerprintID); exists {
		fingerprintState = existingFpState
		fingerprintState.LastActivity = time.Now().UTC()
	} else {
		fingerprintState = &types.FingerprintState{
			FingerprintID: fingerprintID,
			HeldBeliefs:   make(map[string][]string),
			// HeldBadges:    make(map[string]string),
			LastActivity: time.Now().UTC(),
		}
	}
	cacheManager.SetFingerprintState(tenantCtx.TenantID, fingerprintState)
}

// SessionResponse holds session handling response data
type SessionResponse struct {
	Fingerprint string `json:"fingerprint"`
	VisitID     string `json:"visitId"`
}

// HandleProfileSession handles session creation/update for profile operations
func (s *SessionService) HandleProfileSession(tenantCtx *tenant.Context, profile *user.Profile, sessionID string) (*SessionResponse, error) {
	var fingerprintID string

	// Check if this lead already has a fingerprint
	if existingFpID := s.FindFingerprintByLeadID(profile.LeadID, tenantCtx); existingFpID != nil {
		fingerprintID = *existingFpID
	} else {
		// Check if session has an existing fingerprint
		if existingSession, exists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, sessionID); exists {
			fingerprintID = existingSession.FingerprintID
		} else {
			// Create new fingerprint
			fingerprintID = security.GenerateULID()
		}
	}

	// Ensure fingerprint exists in database
	if err := s.CreateFingerprint(fingerprintID, &profile.LeadID, tenantCtx); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("failed to create fingerprint: %w", err)
		}
	}

	// Create visit
	visitID, err := s.HandleVisitCreation(fingerprintID, true, tenantCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create visit: %w", err)
	}

	// FIXED: Update session data without LeadID field
	sessionData := &types.SessionData{
		SessionID:     sessionID,
		FingerprintID: fingerprintID,
		VisitID:       visitID,
		LastActivity:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	// Store session in cache
	tenantCtx.CacheManager.SetSession(tenantCtx.TenantID, sessionData)

	return &SessionResponse{
		Fingerprint: fingerprintID,
		VisitID:     visitID,
	}, nil
}
