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

type SessionService struct {
	beliefBroadcaster *BeliefBroadcastService
	logger            *logging.ChanneledLogger
	perfTracker       *performance.Tracker
}

func NewSessionService(beliefBroadcaster *BeliefBroadcastService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *SessionService {
	return &SessionService{
		beliefBroadcaster: beliefBroadcaster,
		logger:            logger,
		perfTracker:       perfTracker,
	}
}

type SessionResult struct {
	FingerprintID string        `json:"fingerprint"`
	VisitID       string        `json:"visitId"`
	SessionID     string        `json:"sessionId"`
	HasProfile    bool          `json:"hasProfile"`
	Profile       *user.Profile `json:"profile,omitempty"`
	Token         string        `json:"token,omitempty"`
	Consent       string        `json:"consent"`
	Restored      bool          `json:"restored"`
	AffectedPanes []string      `json:"affectedPanes"`
	Success       bool          `json:"success"`
	Error         string        `json:"error,omitempty"`
}

type VisitRequest struct {
	SessionID           *string `json:"sessionId,omitempty"`
	StoryfragmentID     *string `json:"storyfragmentId,omitempty"`
	EncryptedEmail      *string `json:"encryptedEmail,omitempty"`
	EncryptedCode       *string `json:"encryptedCode,omitempty"`
	TractStackSessionID *string `json:"tractstack_session_id,omitempty"`
	Consent             *string `json:"consent,omitempty"`
}

type SessionResponse struct {
	Fingerprint string `json:"fingerprint"`
	VisitID     string `json:"visitId"`
}

type VisitRowData struct {
	ID            string
	FingerprintID string
	CampaignID    *string
	CreatedAt     time.Time
}

func (s *SessionService) ProcessVisitRequest(req *VisitRequest, storyfragmentID string, tenantCtx *tenant.Context) *SessionResult {
	if req.SessionID == nil {
		return &SessionResult{Success: false, Error: "session ID required"}
	}
	sessionID := *req.SessionID

	var consentValue string
	if req.Consent != nil {
		consentValue = *req.Consent
	} else {
		consentValue = "unknown"
	}

	// Priority 1: Profile unlock (encrypted credentials provided)
	if req.EncryptedEmail != nil && req.EncryptedCode != nil {
		return s.processProfileUnlock(sessionID, storyfragmentID, *req.EncryptedEmail, *req.EncryptedCode, consentValue, tenantCtx)
	}

	// Priority 2: Cross-tab session cloning (different session ID provided)
	if req.TractStackSessionID != nil {
		return s.processSessionCloning(sessionID, storyfragmentID, *req.TractStackSessionID, consentValue, tenantCtx)
	}

	// Priority 3: Existing session - check for same-session restoration
	if existingSession, exists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, sessionID); exists {
		return s.processExistingSession(existingSession, sessionID, storyfragmentID, consentValue, tenantCtx)
	}

	// Priority 4: New session warming
	return s.processSessionWarming(sessionID, consentValue, tenantCtx)
}

func (s *SessionService) processExistingSession(session *types.SessionData, sessionID, storyfragmentID, consent string, tenantCtx *tenant.Context) *SessionResult {
	profile, hasProfile := s.getProfileFromSession(session, tenantCtx)

	var token string
	if profile != nil {
		token, _ = security.GenerateProfileToken(profile, tenantCtx.Config.JWTSecret, tenantCtx.Config.AESKey)
	}

	// For existing sessions, check if user has beliefs that affect this storyfragment
	beforeBeliefs := make(map[string][]string) // Page always renders empty initially
	afterBeliefs := make(map[string][]string)  // User's actual beliefs

	if fpState, exists := tenantCtx.CacheManager.GetFingerprintState(tenantCtx.TenantID, session.FingerprintID); exists {
		afterBeliefs = fpState.HeldBeliefs
	}

	affectedPanes := s.beliefBroadcaster.CalculateBeliefDiff(tenantCtx.TenantID, storyfragmentID, beforeBeliefs, afterBeliefs)
	restored := len(affectedPanes) > 0

	s.logger.Auth().Debug("Restoration calculation result",
		"sessionId", sessionID,
		"storyfragmentId", storyfragmentID,
		"affectedPanes", affectedPanes,
		"restored", restored)

	return &SessionResult{
		Success:       true,
		SessionID:     sessionID,
		FingerprintID: session.FingerprintID,
		VisitID:       session.VisitID,
		HasProfile:    hasProfile,
		Profile:       profile,
		Token:         token,
		Consent:       consent,
		Restored:      restored,
		AffectedPanes: affectedPanes,
	}
}

func (s *SessionService) getProfileFromSession(session *types.SessionData, tenantCtx *tenant.Context) (*user.Profile, bool) {
	if session.LeadID != nil {
		if lead, err := s.GetLeadByID(*session.LeadID, tenantCtx); err == nil && lead != nil {
			profile := &user.Profile{
				Fingerprint:    session.FingerprintID,
				LeadID:         lead.ID,
				Firstname:      lead.FirstName,
				Email:          lead.Email,
				ContactPersona: lead.ContactPersona,
				ShortBio:       lead.ShortBio,
			}
			return profile, true
		}
	}
	return nil, false
}

func (s *SessionService) processSessionWarming(sessionID, consent string, tenantCtx *tenant.Context) *SessionResult {
	fingerprintID := security.GenerateULID()
	if err := s.CreateFingerprint(fingerprintID, nil, tenantCtx); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return &SessionResult{Success: false, Error: "failed to create fingerprint"}
		}
	}

	visitID, err := s.HandleVisitCreation(fingerprintID, false, tenantCtx)
	if err != nil {
		return &SessionResult{Success: false, Error: "failed to create visit"}
	}

	s.updateCacheStates(tenantCtx, sessionID, fingerprintID, visitID, nil)

	return &SessionResult{
		Success:       true,
		SessionID:     sessionID,
		FingerprintID: fingerprintID,
		VisitID:       visitID,
		HasProfile:    false,
		Consent:       consent,
		Restored:      false,
	}
}

func (s *SessionService) processProfileUnlock(sessionID, storyfragmentID, encryptedEmail, encryptedCode, consent string, tenantCtx *tenant.Context) *SessionResult {
	decryptedEmail, err := security.Decrypt(encryptedEmail, tenantCtx.Config.AESKey)
	if err != nil {
		return &SessionResult{Success: false, Error: "failed to decrypt email"}
	}
	decryptedCode, err := security.Decrypt(encryptedCode, tenantCtx.Config.AESKey)
	if err != nil {
		return &SessionResult{Success: false, Error: "failed to decrypt code"}
	}
	lead, err := s.ValidateLeadCredentials(decryptedEmail, decryptedCode, tenantCtx)
	if err != nil || lead == nil {
		return &SessionResult{Success: false, Error: "invalid credentials"}
	}

	fingerprintID := s.FindFingerprintByLeadID(lead.ID, tenantCtx)
	s.logger.Auth().Debug("Profile unlock fingerprint lookup",
		"sessionId", sessionID,
		"leadId", lead.ID,
		"foundFingerprintId", fingerprintID,
		"fingerprintExists", fingerprintID != nil)

	if fingerprintID == nil {
		newFpID := security.GenerateULID()
		if err := s.CreateFingerprint(newFpID, &lead.ID, tenantCtx); err != nil {
			return &SessionResult{Success: false, Error: "failed to create fingerprint for existing lead"}
		}
		fingerprintID = &newFpID
	}

	beforeBeliefs := make(map[string][]string)
	afterBeliefs := make(map[string][]string)
	if fpState, exists := tenantCtx.CacheManager.GetFingerprintState(tenantCtx.TenantID, *fingerprintID); exists {
		afterBeliefs = fpState.HeldBeliefs
	}
	s.logger.Auth().Debug("Profile unlock cache state check",
		"fingerprintId", *fingerprintID,
		"heldBeliefsCount", len(afterBeliefs))

	affectedPanes := s.beliefBroadcaster.CalculateBeliefDiff(tenantCtx.TenantID, storyfragmentID, beforeBeliefs, afterBeliefs)

	visitID, err := s.HandleVisitCreation(*fingerprintID, true, tenantCtx)
	if err != nil {
		return &SessionResult{Success: false, Error: "failed to create visit for profile"}
	}

	s.updateCacheStates(tenantCtx, sessionID, *fingerprintID, visitID, &lead.ID)

	profile := &user.Profile{
		Fingerprint:    *fingerprintID,
		LeadID:         lead.ID,
		Firstname:      lead.FirstName,
		Email:          lead.Email,
		ContactPersona: lead.ContactPersona,
		ShortBio:       lead.ShortBio,
	}
	token, _ := security.GenerateProfileToken(profile, tenantCtx.Config.JWTSecret, tenantCtx.Config.AESKey)

	return &SessionResult{
		Success:       true,
		SessionID:     sessionID,
		FingerprintID: *fingerprintID,
		VisitID:       visitID,
		HasProfile:    true,
		Profile:       profile,
		Token:         token,
		Consent:       consent,
		Restored:      len(affectedPanes) > 0,
		AffectedPanes: affectedPanes,
	}
}

func (s *SessionService) processSessionCloning(newSessionID, storyfragmentID, oldSessionID, consent string, tenantCtx *tenant.Context) *SessionResult {
	oldSession, exists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, oldSessionID)
	if !exists {
		return s.processSessionWarming(newSessionID, consent, tenantCtx)
	}

	fingerprintID := oldSession.FingerprintID
	leadID := oldSession.LeadID

	beforeBeliefs := make(map[string][]string)
	afterBeliefs := make(map[string][]string)
	if fpState, fpExists := tenantCtx.CacheManager.GetFingerprintState(tenantCtx.TenantID, fingerprintID); fpExists {
		afterBeliefs = fpState.HeldBeliefs
	}

	affectedPanes := s.beliefBroadcaster.CalculateBeliefDiff(tenantCtx.TenantID, storyfragmentID, beforeBeliefs, afterBeliefs)

	visitID, err := s.HandleVisitCreation(fingerprintID, leadID != nil, tenantCtx)
	if err != nil {
		return &SessionResult{Success: false, Error: "failed to create visit for cloned session"}
	}

	s.updateCacheStates(tenantCtx, newSessionID, fingerprintID, visitID, leadID)

	profile, hasProfile := s.getProfileFromSession(oldSession, tenantCtx)
	var token string
	if profile != nil {
		token, _ = security.GenerateProfileToken(profile, tenantCtx.Config.JWTSecret, tenantCtx.Config.AESKey)
	}

	return &SessionResult{
		Success:       true,
		SessionID:     newSessionID,
		FingerprintID: fingerprintID,
		VisitID:       visitID,
		HasProfile:    hasProfile,
		Profile:       profile,
		Token:         token,
		Consent:       consent,
		Restored:      len(affectedPanes) > 0,
		AffectedPanes: affectedPanes,
	}
}

func (s *SessionService) HandleVisitCreation(fingerprintID string, hasProfile bool, tenantCtx *tenant.Context) (string, error) {
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

func (s *SessionService) CreateFingerprint(fingerprintID string, leadID *string, tenantCtx *tenant.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `INSERT INTO fingerprints (id, lead_id, created_at) VALUES (?, ?, ?)`
	_, err := tenantCtx.Database.Conn.ExecContext(ctx, query, fingerprintID, leadID, time.Now().UTC())
	return err
}

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

func (s *SessionService) ValidateLeadCredentials(email, password string, tenantCtx *tenant.Context) (*user.Lead, error) {
	s.logger.Auth().Info("Validating lead credentials", "email", email)

	lead, err := s.GetLeadByEmail(email, tenantCtx)
	if err != nil || lead == nil {
		s.logger.Auth().Error("Lead lookup failed", "email", email, "error", err)
		return nil, fmt.Errorf("lead not found")
	}

	s.logger.Auth().Info("Lead found, checking password", "email", email, "leadId", lead.ID)

	if err := bcrypt.CompareHashAndPassword([]byte(lead.PasswordHash), []byte(password)); err != nil {
		s.logger.Auth().Error("Password validation failed", "email", email, "leadId", lead.ID, "error", err)
		return nil, fmt.Errorf("invalid password")
	}

	s.logger.Auth().Info("Lead credentials validated successfully", "email", email, "leadId", lead.ID)
	return lead, nil
}

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

func (s *SessionService) HandleProfileSession(tenantCtx *tenant.Context, profile *user.Profile, sessionID string) (*SessionResponse, error) {
	s.logger.Auth().Debug("HandleProfileSession ENTRY",
		"sessionId", sessionID,
		"leadId", profile.LeadID,
		"tenantId", tenantCtx.TenantID)

	var fingerprintID string

	if existingFpID := s.FindFingerprintByLeadID(profile.LeadID, tenantCtx); existingFpID != nil {
		fingerprintID = *existingFpID
		s.logger.Auth().Debug("HandleProfileSession EXISTING_FINGERPRINT_FOUND",
			"sessionId", sessionID,
			"leadId", profile.LeadID,
			"existingFingerprintId", fingerprintID)
	} else {
		if existingSession, exists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, sessionID); exists {
			fingerprintID = existingSession.FingerprintID
			s.logger.Auth().Debug("HandleProfileSession USING_SESSION_FINGERPRINT",
				"sessionId", sessionID,
				"leadId", profile.LeadID,
				"sessionFingerprintId", fingerprintID)
		} else {
			fingerprintID = security.GenerateULID()
			s.logger.Auth().Debug("HandleProfileSession GENERATED_NEW_FINGERPRINT",
				"sessionId", sessionID,
				"leadId", profile.LeadID,
				"newFingerprintId", fingerprintID)
		}
	}

	if err := s.CreateFingerprint(fingerprintID, &profile.LeadID, tenantCtx); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			s.logger.Auth().Debug("HandleProfileSession CREATE_FINGERPRINT_FAILED",
				"sessionId", sessionID,
				"leadId", profile.LeadID,
				"fingerprintId", fingerprintID,
				"error", err.Error())
			return nil, fmt.Errorf("failed to create fingerprint: %w", err)
		}
		s.logger.Auth().Debug("HandleProfileSession FINGERPRINT_ALREADY_EXISTS",
			"sessionId", sessionID,
			"leadId", profile.LeadID,
			"fingerprintId", fingerprintID)
	}

	var visitID string
	if existingSession, exists := tenantCtx.CacheManager.GetSession(tenantCtx.TenantID, sessionID); exists {
		visitID = existingSession.VisitID
		s.logger.Auth().Debug("HandleProfileSession USING_EXISTING_VISIT",
			"sessionId", sessionID,
			"leadId", profile.LeadID,
			"fingerprintId", fingerprintID,
			"existingVisitId", visitID)
	} else {
		var err error
		visitID, err = s.HandleVisitCreation(fingerprintID, true, tenantCtx)
		if err != nil {
			s.logger.Auth().Debug("HandleProfileSession VISIT_CREATION_FAILED",
				"sessionId", sessionID,
				"leadId", profile.LeadID,
				"fingerprintId", fingerprintID,
				"error", err.Error())
			return nil, fmt.Errorf("failed to create visit: %w", err)
		}
		s.logger.Auth().Debug("HandleProfileSession CREATED_NEW_VISIT",
			"sessionId", sessionID,
			"leadId", profile.LeadID,
			"fingerprintId", fingerprintID,
			"newVisitId", visitID)
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
	s.logger.Auth().Debug("HandleProfileSession SESSION_CACHE_UPDATED",
		"sessionId", sessionID,
		"leadId", profile.LeadID,
		"fingerprintId", fingerprintID,
		"visitId", visitID)

	var fingerprintState *types.FingerprintState
	if existingFpState, exists := tenantCtx.CacheManager.GetFingerprintState(tenantCtx.TenantID, fingerprintID); exists {
		fingerprintState = existingFpState
		fingerprintState.LastActivity = time.Now().UTC()
		s.logger.Auth().Debug("HandleProfileSession PRESERVED_EXISTING_BELIEFS",
			"fingerprintId", fingerprintID,
			"existingBeliefCount", len(existingFpState.HeldBeliefs))
	} else {
		// Cache miss - load beliefs from database if this is an existing user
		beliefs := make(map[string][]string)
		if profile.LeadID != "" {
			loadedBeliefs, err := tenantCtx.EventRepo().LoadFingerprintBeliefs(fingerprintID)
			if err != nil {
				s.logger.Auth().Debug("HandleProfileSession FAILED_TO_LOAD_DB_BELIEFS",
					"error", err.Error(),
					"fingerprintId", fingerprintID,
					"leadId", profile.LeadID)
			} else {
				beliefs = loadedBeliefs
				s.logger.Auth().Debug("HandleProfileSession LOADED_DB_BELIEFS",
					"fingerprintId", fingerprintID,
					"beliefCount", len(beliefs))
			}
		}

		fingerprintState = &types.FingerprintState{
			FingerprintID: fingerprintID,
			LeadID:        &profile.LeadID,
			HeldBeliefs:   beliefs,
			HeldBadges:    make(map[string]string),
			LastActivity:  time.Now().UTC(),
		}
	}

	tenantCtx.CacheManager.SetFingerprintState(tenantCtx.TenantID, fingerprintState)
	s.logger.Auth().Debug("HandleProfileSession FINGERPRINT_CACHE_UPDATED",
		"sessionId", sessionID,
		"leadId", profile.LeadID,
		"fingerprintId", fingerprintID,
		"heldBeliefsCount", len(fingerprintState.HeldBeliefs))

	visitState := &types.VisitState{
		VisitID:       visitID,
		FingerprintID: fingerprintID,
		StartTime:     time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		LastActivity:  time.Now().UTC(),
	}
	tenantCtx.CacheManager.SetVisitState(tenantCtx.TenantID, visitState)
	s.logger.Auth().Debug("HandleProfileSession VISIT_CACHE_UPDATED",
		"sessionId", sessionID,
		"leadId", profile.LeadID,
		"fingerprintId", fingerprintID,
		"visitId", visitID)

	s.logger.Auth().Debug("HandleProfileSession COMPLETE_SUCCESS",
		"sessionId", sessionID,
		"leadId", profile.LeadID,
		"fingerprintId", fingerprintID,
		"visitId", visitID)

	return &SessionResponse{
		Fingerprint: fingerprintID,
		VisitID:     visitID,
	}, nil
}

func (s *SessionService) updateCacheStates(tenantCtx *tenant.Context, sessionID, fingerprintID, visitID string, leadID *string) {
	cacheManager := tenantCtx.CacheManager

	sessionData := &types.SessionData{
		SessionID:     sessionID,
		FingerprintID: fingerprintID,
		VisitID:       visitID,
		LeadID:        leadID,
		LastActivity:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		IsExpired:     false,
	}

	cacheManager.SetSession(tenantCtx.TenantID, sessionData)

	var fingerprintState *types.FingerprintState
	if existingFpState, exists := cacheManager.GetFingerprintState(tenantCtx.TenantID, fingerprintID); exists {
		fingerprintState = existingFpState
		fingerprintState.LastActivity = time.Now().UTC()
	} else {
		// Cache miss - load beliefs from database if this is an existing user
		beliefs := make(map[string][]string)
		if leadID != nil {
			loadedBeliefs, err := tenantCtx.EventRepo().LoadFingerprintBeliefs(fingerprintID)
			if err != nil {
				s.logger.Auth().Error("Failed to load fingerprint beliefs from database",
					"error", err.Error(),
					"fingerprintId", fingerprintID,
					"leadId", *leadID)
			} else {
				beliefs = loadedBeliefs
				s.logger.Auth().Debug("Loaded fingerprint beliefs from database",
					"fingerprintId", fingerprintID,
					"beliefCount", len(beliefs))
			}
		}

		fingerprintState = &types.FingerprintState{
			FingerprintID: fingerprintID,
			LeadID:        leadID,
			HeldBeliefs:   beliefs,
			HeldBadges:    make(map[string]string),
			LastActivity:  time.Now().UTC(),
		}
	}
	cacheManager.SetFingerprintState(tenantCtx.TenantID, fingerprintState)

	visitState := &types.VisitState{
		VisitID:       visitID,
		FingerprintID: fingerprintID,
		StartTime:     time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		LastActivity:  time.Now().UTC(),
	}
	cacheManager.SetVisitState(tenantCtx.TenantID, visitState)
}
