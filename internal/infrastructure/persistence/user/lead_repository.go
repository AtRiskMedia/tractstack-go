// Package user provides the concrete SQL-based implementations of
// the user domain repositories (Lead, Fingerprint, Visit).
package user

import (
	"database/sql"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
)

// SQLLeadRepository is the SQL-based implementation of the LeadRepository.
type SQLLeadRepository struct {
	db     *database.DB
	logger *logging.ChanneledLogger
}

// NewSQLLeadRepository creates a new instance of the repository.
func NewSQLLeadRepository(db *database.DB, logger *logging.ChanneledLogger) *SQLLeadRepository {
	return &SQLLeadRepository{
		db:     db,
		logger: logger,
	}
}

// FindByID retrieves a Lead by their unique identifier.
func (r *SQLLeadRepository) FindByID(id string) (*user.Lead, error) {
	const query = `
		SELECT id, first_name, email, password_hash, contact_persona, 
		       short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading lead by ID", "id", id)

	row := r.db.QueryRow(query, id)
	lead, err := r.scanLead(row)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.Database().Debug("Lead not found by ID", "id", id)
			return nil, nil
		}
		r.logger.Database().Error("Failed to load lead by ID", "error", err.Error(), "id", id)
		return nil, err
	}

	if lead != nil {
		r.logger.Database().Info("Lead loaded by ID", "id", id, "duration", time.Since(start))
	}
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		const query = `SELECT id, first_name, email, password_hash, contact_persona, 
		               short_bio, encrypted_code, encrypted_email, created_at, changed
		               FROM leads WHERE id = ?`
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return lead, nil
}

// FindByEmail retrieves a Lead by their email address.
func (r *SQLLeadRepository) FindByEmail(email string) (*user.Lead, error) {
	const query = `
		SELECT id, first_name, email, password_hash, contact_persona, 
		       short_bio, encrypted_code, encrypted_email, created_at, changed
		FROM leads 
		WHERE email = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading lead by email", "email", email)

	row := r.db.QueryRow(query, email)
	lead, err := r.scanLead(row)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.Database().Debug("Lead not found by email", "email", email)
			return nil, nil
		}
		r.logger.Database().Error("Failed to load lead by email", "error", err.Error(), "email", email)
		return nil, err
	}

	if lead != nil {
		r.logger.Database().Info("Lead loaded by email", "email", email, "leadId", lead.ID, "duration", time.Since(start))
	}
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		const query = `SELECT id, first_name, email, password_hash, contact_persona, 
		               short_bio, encrypted_code, encrypted_email, created_at, changed
		               FROM leads WHERE email = ?`
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return lead, nil
}

// Store saves a new Lead to the database.
func (r *SQLLeadRepository) Store(lead *user.Lead) error {
	const query = `
		INSERT INTO leads (id, first_name, email, password_hash, contact_persona, 
		                   short_bio, encrypted_code, encrypted_email, created_at, changed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	start := time.Now()
	r.logger.Database().Debug("Executing lead insert", "id", lead.ID, "email", lead.Email)

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
	if err != nil {
		r.logger.Database().Error("Lead insert failed", "error", err.Error(), "id", lead.ID, "email", lead.Email)
		return err
	}

	r.logger.Database().Info("Lead insert completed", "id", lead.ID, "email", lead.Email, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		const query = `INSERT INTO leads (id, first_name, email, password_hash, contact_persona, 
		               short_bio, encrypted_code, encrypted_email, created_at, changed)
		               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return nil
}

// Update modifies an existing Lead in the database.
func (r *SQLLeadRepository) Update(lead *user.Lead) error {
	const query = `
		UPDATE leads 
		SET first_name = ?, email = ?, password_hash = ?, contact_persona = ?,
		    short_bio = ?, encrypted_code = ?, encrypted_email = ?, changed = ?
		WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing lead update", "id", lead.ID, "email", lead.Email)

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
	if err != nil {
		r.logger.Database().Error("Lead update failed", "error", err.Error(), "id", lead.ID, "email", lead.Email)
		return err
	}

	r.logger.Database().Info("Lead update completed", "id", lead.ID, "email", lead.Email, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		const query = `UPDATE leads 
		               SET first_name = ?, email = ?, password_hash = ?, contact_persona = ?,
		                   short_bio = ?, encrypted_code = ?, encrypted_email = ?, changed = ?
		               WHERE id = ?`
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return nil
}

// ValidateCredentials checks email/password combination and returns the Lead if valid.
func (r *SQLLeadRepository) ValidateCredentials(email, password string) (*user.Lead, error) {
	start := time.Now()
	r.logger.Database().Debug("Validating lead credentials", "email", email)

	lead, err := r.FindByEmail(email)
	if err != nil {
		r.logger.Database().Error("Failed to validate credentials - email lookup failed", "error", err.Error(), "email", email)
		return nil, err
	}
	if lead == nil {
		r.logger.Database().Debug("Credential validation failed - lead not found", "email", email, "duration", time.Since(start))
		return nil, nil // Lead not found
	}

	// Password validation would be done at service layer
	// Repository just returns the lead for credential checking
	r.logger.Database().Info("Credential validation completed", "email", email, "leadId", lead.ID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery("CREDENTIAL_VALIDATION", duration, "system")
	}
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
