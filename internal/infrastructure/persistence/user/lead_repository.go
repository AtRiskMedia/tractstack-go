// Package user provides the concrete SQL-based implementations of
// the user domain repositories (Lead, Fingerprint, Visit).
package user

import (
	"database/sql"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

// SQLLeadRepository is the SQL-based implementation of the LeadRepository.
type SQLLeadRepository struct {
	db *database.DB
}

// NewSQLLeadRepository creates a new instance of the repository.
func NewSQLLeadRepository(db *database.DB) *SQLLeadRepository {
	return &SQLLeadRepository{db: db}
}

// FindByID retrieves a Lead by their unique identifier.
func (r *SQLLeadRepository) FindByID(id string) (*user.Lead, error) {
	const query = `
		SELECT id, first_name, email, password_hash, contact_persona, 
		       short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE id = ?`

	row := r.db.QueryRow(query, id)
	return r.scanLead(row)
}

// FindByEmail retrieves a Lead by their email address.
func (r *SQLLeadRepository) FindByEmail(email string) (*user.Lead, error) {
	const query = `
		SELECT id, first_name, email, password_hash, contact_persona, 
		       short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE email = ?`

	row := r.db.QueryRow(query, email)
	return r.scanLead(row)
}

// Store saves a new Lead to the database.
func (r *SQLLeadRepository) Store(lead *user.Lead) error {
	const query = `
		INSERT INTO leads (id, first_name, email, password_hash, contact_persona, 
		                   short_bio, encrypted_code, encrypted_email, created_at, changed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.Exec(
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
	return err
}

// Update modifies an existing Lead in the database.
func (r *SQLLeadRepository) Update(lead *user.Lead) error {
	const query = `
		UPDATE leads 
		SET first_name = ?, email = ?, password_hash = ?, contact_persona = ?,
		    short_bio = ?, encrypted_code = ?, encrypted_email = ?, changed = ?
		WHERE id = ?`

	_, err := r.db.Exec(
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
	return err
}

// ValidateCredentials checks email/password combination and returns the Lead if valid.
func (r *SQLLeadRepository) ValidateCredentials(email, password string) (*user.Lead, error) {
	lead, err := r.FindByEmail(email)
	if err != nil {
		return nil, err
	}
	if lead == nil {
		return nil, nil // Lead not found
	}

	// Password validation would be done at service layer
	// Repository just returns the lead for credential checking
	return lead, nil
}

// scanLead is a helper function to scan a sql.Row into a Lead struct.
func (r *SQLLeadRepository) scanLead(row *sql.Row) (*user.Lead, error) {
	var lead user.Lead
	var shortBio, encryptedCode, encryptedEmail sql.NullString
	var changed sql.NullTime
	var createdAtStr, changedStr string

	err := row.Scan(
		&lead.ID,
		&lead.FirstName,
		&lead.Email,
		&lead.PasswordHash,
		&lead.ContactPersona,
		&shortBio,
		&encryptedCode,
		&encryptedEmail,
		&createdAtStr,
		&changedStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, err
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

	// Parse timestamps
	lead.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		// Try alternative timestamp format if RFC3339 fails
		lead.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			return nil, err
		}
	}

	if changed.Valid {
		lead.Changed = changed.Time
	}

	return &lead, nil
}
