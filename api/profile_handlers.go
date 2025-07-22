package api

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type ProfileRequest struct {
	SessionID      *string `json:"sessionId,omitempty"`
	Firstname      string  `json:"firstname"`
	Email          string  `json:"email"`
	Codeword       string  `json:"codeword"`
	ContactPersona string  `json:"contactPersona"`
	ShortBio       string  `json:"shortBio"`
	EncryptedEmail *string `json:"encryptedEmail,omitempty"`
	EncryptedCode  *string `json:"encryptedCode,omitempty"`
	IsUpdate       bool    `json:"isUpdate"` // true for update, false for create
}

type ProfileResponse struct {
	Success        bool            `json:"success"`
	Profile        *models.Profile `json:"profile,omitempty"`
	Token          string          `json:"token,omitempty"`
	EncryptedEmail string          `json:"encryptedEmail,omitempty"`
	EncryptedCode  string          `json:"encryptedCode,omitempty"`
	Error          string          `json:"error,omitempty"`
	Fingerprint    string          `json:"fingerprint,omitempty"`
	VisitID        string          `json:"visitId,omitempty"`
	HasProfile     bool            `json:"hasProfile"`
	Consent        string          `json:"consent"`
}

// ProfileHandler handles both create and update profile operations
func ProfileHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req ProfileRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ProfileResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	// Validate session ID
	if req.SessionID == nil || *req.SessionID == "" {
		c.JSON(http.StatusBadRequest, ProfileResponse{
			Success: false,
			Error:   "Session ID required",
		})
		return
	}

	// Handle encrypted credentials for fast pass
	if req.EncryptedEmail != nil && req.EncryptedCode != nil {
		handleEncryptedCredentials(c, ctx, &req)
		return
	}

	// Handle regular email/codeword authentication
	if req.Email == "" || req.Codeword == "" {
		c.JSON(http.StatusBadRequest, ProfileResponse{
			Success: false,
			Error:   "Email and codeword required",
		})
		return
	}

	if req.IsUpdate {
		// For unlock/update: only email, codeword, sessionId required
		handleUpdateProfile(c, ctx, &req)
	} else {
		// For create: all fields required
		if req.Firstname == "" || req.ContactPersona == "" {
			c.JSON(http.StatusBadRequest, ProfileResponse{
				Success: false,
				Error:   "Firstname and ContactPersona required for profile creation",
			})
			return
		}
		handleCreateProfile(c, ctx, &req)
	}
}

func handleCreateProfile(c *gin.Context, ctx *tenant.Context, req *ProfileRequest) {
	// Check if email already exists
	existingLead, err := GetLeadByEmail(req.Email, ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Database error checking existing email",
		})
		return
	}

	if existingLead != nil {
		c.JSON(http.StatusConflict, ProfileResponse{
			Success: false,
			Error:   "Email already registered",
		})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Codeword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Password hashing failed",
		})
		return
	}

	// Generate encrypted credentials
	encryptedEmail, err := utils.Encrypt(req.Email, ctx.Config.AESKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Encryption failed",
		})
		return
	}

	encryptedCode, err := utils.Encrypt(req.Codeword, ctx.Config.AESKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Encryption failed",
		})
		return
	}

	// Create new lead
	leadID := utils.GenerateULID()
	lead := &models.Lead{
		ID:             leadID,
		FirstName:      req.Firstname,
		Email:          req.Email,
		PasswordHash:   string(hashedPassword),
		ContactPersona: req.ContactPersona,
		ShortBio:       req.ShortBio,
		EncryptedCode:  encryptedCode,
		EncryptedEmail: encryptedEmail,
		CreatedAt:      time.Now().UTC(),
		Changed:        time.Now().UTC(),
	}

	if err := CreateLead(lead, ctx); err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Failed to create profile",
		})
		return
	}

	// Generate profile and session response
	profile := &models.Profile{
		LeadID:         leadID,
		Firstname:      req.Firstname,
		Email:          req.Email,
		ContactPersona: req.ContactPersona,
		ShortBio:       req.ShortBio,
	}

	// Handle session and fingerprint creation
	sessionResponse := handleProfileSession(c, ctx, profile, *req.SessionID)
	if sessionResponse == nil {
		return // Error already handled in function
	}

	// Generate JWT token
	token, err := utils.GenerateProfileToken(profile, ctx.Config.JWTSecret, ctx.Config.AESKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Token generation failed",
		})
		return
	}

	c.JSON(http.StatusOK, ProfileResponse{
		Success:        true,
		Profile:        profile,
		Token:          token,
		EncryptedEmail: encryptedEmail,
		EncryptedCode:  encryptedCode,
		Fingerprint:    sessionResponse.Fingerprint,
		VisitID:        sessionResponse.VisitID,
		HasProfile:     true,
		Consent:        "1",
	})
}

func handleUpdateProfile(c *gin.Context, ctx *tenant.Context, req *ProfileRequest) {
	// Get existing lead by email
	existingLead, err := GetLeadByEmail(req.Email, ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Database error",
		})
		return
	}

	if existingLead == nil {
		c.JSON(http.StatusNotFound, ProfileResponse{
			Success: false,
			Error:   "Profile not found",
		})
		return
	}

	// For unlock operations (no firstname/contactPersona provided), just authenticate and return existing data
	if req.Firstname == "" && req.ContactPersona == "" {
		// This is an unlock operation - return existing profile data
		profile := &models.Profile{
			LeadID:         existingLead.ID,
			Firstname:      existingLead.FirstName,
			Email:          existingLead.Email,
			ContactPersona: existingLead.ContactPersona,
			ShortBio:       existingLead.ShortBio,
		}

		// Handle session and fingerprint
		sessionResponse := handleProfileSession(c, ctx, profile, *req.SessionID)
		if sessionResponse == nil {
			return // Error already handled in function
		}

		// Generate JWT token
		token, err := utils.GenerateProfileToken(profile, ctx.Config.JWTSecret, ctx.Config.AESKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ProfileResponse{
				Success: false,
				Error:   "Token generation failed",
			})
			return
		}

		c.JSON(http.StatusOK, ProfileResponse{
			Success:        true,
			Profile:        profile,
			Token:          token,
			EncryptedEmail: existingLead.EncryptedEmail,
			EncryptedCode:  existingLead.EncryptedCode,
			Fingerprint:    sessionResponse.Fingerprint,
			VisitID:        sessionResponse.VisitID,
			HasProfile:     true,
			Consent:        "1",
		})
		return
	}

	// This is an actual profile update - proceed with updating data
	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(existingLead.PasswordHash), []byte(req.Codeword)); err != nil {
		c.JSON(http.StatusUnauthorized, ProfileResponse{
			Success: false,
			Error:   "Invalid credentials",
		})
		return
	}

	// Hash new password if provided
	hashedPassword := existingLead.PasswordHash
	if req.Codeword != "" {
		newHash, err := bcrypt.GenerateFromPassword([]byte(req.Codeword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ProfileResponse{
				Success: false,
				Error:   "Password hashing failed",
			})
			return
		}
		hashedPassword = string(newHash)
	}

	// Generate new encrypted credentials
	encryptedEmail, err := utils.Encrypt(req.Email, ctx.Config.AESKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Encryption failed",
		})
		return
	}

	encryptedCode, err := utils.Encrypt(req.Codeword, ctx.Config.AESKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Encryption failed",
		})
		return
	}

	// Update lead
	updatedLead := &models.Lead{
		ID:             existingLead.ID,
		FirstName:      req.Firstname,
		Email:          req.Email,
		PasswordHash:   hashedPassword,
		ContactPersona: req.ContactPersona,
		ShortBio:       req.ShortBio,
		EncryptedCode:  encryptedCode,
		EncryptedEmail: encryptedEmail,
		CreatedAt:      existingLead.CreatedAt,
		Changed:        time.Now().UTC(),
	}

	if err := UpdateLead(updatedLead, ctx); err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Failed to update profile",
		})
		return
	}

	// Generate profile response
	profile := &models.Profile{
		LeadID:         existingLead.ID,
		Firstname:      req.Firstname,
		Email:          req.Email,
		ContactPersona: req.ContactPersona,
		ShortBio:       req.ShortBio,
	}

	// Handle session and fingerprint
	sessionResponse := handleProfileSession(c, ctx, profile, *req.SessionID)
	if sessionResponse == nil {
		return // Error already handled in function
	}

	// Generate JWT token
	token, err := utils.GenerateProfileToken(profile, ctx.Config.JWTSecret, ctx.Config.AESKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Token generation failed",
		})
		return
	}

	c.JSON(http.StatusOK, ProfileResponse{
		Success:        true,
		Profile:        profile,
		Token:          token,
		EncryptedEmail: encryptedEmail,
		EncryptedCode:  encryptedCode,
		Fingerprint:    sessionResponse.Fingerprint,
		VisitID:        sessionResponse.VisitID,
		HasProfile:     true,
		Consent:        "1",
	})
}

type SessionResponse struct {
	Fingerprint string
	VisitID     string
}

// Database operations

func CreateLead(lead *models.Lead, ctx *tenant.Context) error {
	query := `
		INSERT INTO leads (id, first_name, email, password_hash, contact_persona, short_bio, encrypted_code, encrypted_email, created_at, changed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := ctx.Database.Conn.Exec(
		query,
		lead.ID,
		lead.FirstName,
		lead.Email,
		lead.PasswordHash,
		lead.ContactPersona,
		lead.ShortBio,
		lead.EncryptedCode,
		lead.EncryptedEmail,
		lead.CreatedAt,
		lead.Changed,
	)
	if err != nil {
		return fmt.Errorf("failed to create lead: %w", err)
	}

	return nil
}

func UpdateLead(lead *models.Lead, ctx *tenant.Context) error {
	query := `
		UPDATE leads 
		SET first_name = ?, email = ?, password_hash = ?, contact_persona = ?, short_bio = ?, encrypted_code = ?, encrypted_email = ?, changed = ?
		WHERE id = ?
	`

	_, err := ctx.Database.Conn.Exec(
		query,
		lead.FirstName,
		lead.Email,
		lead.PasswordHash,
		lead.ContactPersona,
		lead.ShortBio,
		lead.EncryptedCode,
		lead.EncryptedEmail,
		lead.Changed,
		lead.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update lead: %w", err)
	}

	return nil
}

func GetLeadByEmail(email string, ctx *tenant.Context) (*models.Lead, error) {
	query := `
		SELECT id, first_name, email, password_hash, contact_persona, short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE email = ?
		LIMIT 1
	`

	var lead models.Lead
	var shortBio, encryptedCode, encryptedEmail sql.NullString
	var changed sql.NullTime

	err := ctx.Database.Conn.QueryRow(query, email).Scan(
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

// HandleEncryptedCredentials handles fast pass authentication using encrypted credentials
func handleEncryptedCredentials(c *gin.Context, ctx *tenant.Context, req *ProfileRequest) {
	// Validate encrypted credentials
	profile := ValidateEncryptedCredentials(*req.EncryptedEmail, *req.EncryptedCode, ctx)
	if profile == nil {
		c.JSON(http.StatusUnauthorized, ProfileResponse{
			Success: false,
			Error:   "Invalid encrypted credentials",
		})
		return
	}

	// Handle session and fingerprint
	sessionResponse := handleProfileSession(c, ctx, profile, *req.SessionID)
	if sessionResponse == nil {
		return // Error already handled in function
	}

	// Generate JWT token
	token, err := utils.GenerateProfileToken(profile, ctx.Config.JWTSecret, ctx.Config.AESKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Token generation failed",
		})
		return
	}

	c.JSON(http.StatusOK, ProfileResponse{
		Success:        true,
		Profile:        profile,
		Token:          token,
		EncryptedEmail: *req.EncryptedEmail,
		EncryptedCode:  *req.EncryptedCode,
		Fingerprint:    sessionResponse.Fingerprint,
		VisitID:        sessionResponse.VisitID,
		HasProfile:     true,
		Consent:        "1",
	})
}

// ValidateEncryptedCredentials validates profile credentials using encrypted data
func ValidateEncryptedCredentials(encryptedEmail, encryptedCode string, ctx *tenant.Context) *models.Profile {
	// Decrypt credentials
	decryptedEmail, err := utils.Decrypt(encryptedEmail, ctx.Config.AESKey)
	if err != nil {
		return nil
	}

	decryptedCode, err := utils.Decrypt(encryptedCode, ctx.Config.AESKey)
	if err != nil {
		return nil
	}

	// Validate against database
	lead, err := ValidateLeadCredentials(decryptedEmail, decryptedCode, ctx)
	if err != nil || lead == nil {
		return nil
	}

	// Convert lead to profile
	return &models.Profile{
		LeadID:         lead.ID,
		Firstname:      lead.FirstName,
		Email:          lead.Email,
		ContactPersona: lead.ContactPersona,
		ShortBio:       lead.ShortBio,
	}
}

func ValidateLeadCredentials(email, password string, ctx *tenant.Context) (*models.Lead, error) {
	lead, err := GetLeadByEmail(email, ctx)
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

func GetLeadByID(leadID string, ctx *tenant.Context) (*models.Lead, error) {
	query := `
		SELECT id, first_name, email, password_hash, contact_persona, short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE id = ?
		LIMIT 1
	`

	var lead models.Lead
	var shortBio, encryptedCode, encryptedEmail sql.NullString
	var changed sql.NullTime

	err := ctx.Database.Conn.QueryRow(query, leadID).Scan(
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

func handleProfileSession(c *gin.Context, ctx *tenant.Context, profile *models.Profile, sessionID string) *SessionResponse {
	// Create visit service to handle fingerprint and visit creation
	visitService := NewVisitService(ctx, nil)

	var fingerprintID string

	// PRIORITY 1: Always check if this lead already has a fingerprint (for profile unlocks)
	if existingFpID := visitService.FindFingerprintByLeadID(profile.LeadID); existingFpID != nil {
		fingerprintID = *existingFpID
	} else {
		// PRIORITY 2: No existing lead fingerprint, check if session has one (for new profile creation)
		if existingSession, exists := cache.GetGlobalManager().GetSession(ctx.TenantID, sessionID); exists {
			fingerprintID = existingSession.FingerprintID
			// Link this fingerprint to the new lead
			if err := visitService.UpdateFingerprintLeadID(fingerprintID, &profile.LeadID); err != nil {
				log.Printf("ERROR: handleProfileSession - Failed to link fingerprint %s to lead %s: %v", fingerprintID, profile.LeadID, err)
				c.JSON(http.StatusInternalServerError, ProfileResponse{
					Success: false,
					Error:   "Failed to link session to profile",
				})
				return nil
				//} else {
				//	log.Printf("DEBUG: handleProfileSession - Successfully linked fingerprint %s to lead %s", fingerprintID, profile.LeadID)
			}
		} else {
			// PRIORITY 3: No existing fingerprint anywhere, generate new one
			fingerprintID = utils.GenerateULID()
		}
	}

	// Ensure fingerprint exists in database
	if exists, err := visitService.FingerprintExists(fingerprintID); err == nil && !exists {
		if err := visitService.CreateFingerprint(fingerprintID, &profile.LeadID); err != nil {
			// Check if it's a UNIQUE constraint error (race condition)
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				// log.Printf("DEBUG: handleProfileSession - fingerprint %s already created by concurrent request, continuing", fingerprintID)
			} else {
				log.Printf("ERROR: handleProfileSession - Failed to create fingerprint %s: %v", fingerprintID, err)
				c.JSON(http.StatusInternalServerError, ProfileResponse{
					Success: false,
					Error:   "Fingerprint creation failed",
				})
				return nil
			}
		}
		//log.Printf("DEBUG: handleProfileSession - Successfully created fingerprint %s", fingerprintID)
		//} else if err != nil {
		//	log.Printf("ERROR: handleProfileSession - Error checking fingerprint existence: %v", err)
		//} else {
		//	log.Printf("DEBUG: handleProfileSession - Fingerprint %s already exists", fingerprintID)
	}

	// Handle visit creation/reuse
	// log.Printf("DEBUG: handleProfileSession - Creating visit for fingerprint %s", fingerprintID)
	visitID, err := visitService.HandleVisitCreation(fingerprintID, true)
	if err != nil {
		log.Printf("ERROR: handleProfileSession - Failed to create visit: %v", err)
		c.JSON(http.StatusInternalServerError, ProfileResponse{
			Success: false,
			Error:   "Visit creation failed",
		})
		return nil
	}
	// log.Printf("DEBUG: handleProfileSession - Created visit %s", visitID)

	// Update session data with lead information
	sessionData := &models.SessionData{
		SessionID:     sessionID,
		FingerprintID: fingerprintID,
		VisitID:       visitID,
		LeadID:        &profile.LeadID,
		LastActivity:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	// log.Printf("DEBUG: handleProfileSession - Storing session data: session=%s, fingerprint=%s, visit=%s, lead=%s",
	//	sessionID, fingerprintID, visitID, profile.LeadID)

	// Store session in cache
	cache.GetGlobalManager().SetSession(ctx.TenantID, sessionData)

	// log.Printf("DEBUG: handleProfileSession - Returning fingerprint %s, visit %s", fingerprintID, visitID)
	return &SessionResponse{
		Fingerprint: fingerprintID,
		VisitID:     visitID,
	}
}
