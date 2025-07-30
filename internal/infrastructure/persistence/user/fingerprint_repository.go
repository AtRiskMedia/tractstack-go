// Package user provides the concrete SQL-based implementations of
// the user domain repositories (Lead, Fingerprint, Visit).
package user

import (
	"database/sql"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

// SQLFingerprintRepository is the SQL-based implementation of the FingerprintRepository.
type SQLFingerprintRepository struct {
	db *database.DB
}

// NewSQLFingerprintRepository creates a new instance of the repository.
func NewSQLFingerprintRepository(db *database.DB) *SQLFingerprintRepository {
	return &SQLFingerprintRepository{db: db}
}

// FindByID retrieves a Fingerprint by its unique identifier.
func (r *SQLFingerprintRepository) FindByID(id string) (*user.Fingerprint, error) {
	const query = `
		SELECT id, lead_id, created_at
		FROM fingerprints 
		WHERE id = ?`

	row := r.db.QueryRow(query, id)
	return r.scanFingerprint(row)
}

// FindByLeadID retrieves a Fingerprint associated with a specific Lead.
func (r *SQLFingerprintRepository) FindByLeadID(leadID string) (*user.Fingerprint, error) {
	const query = `
		SELECT id, lead_id, created_at
		FROM fingerprints 
		WHERE lead_id = ?
		LIMIT 1`

	row := r.db.QueryRow(query, leadID)
	return r.scanFingerprint(row)
}

// Create saves a new Fingerprint to the database.
func (r *SQLFingerprintRepository) Create(fingerprint *user.Fingerprint) error {
	const query = `
		INSERT INTO fingerprints (id, lead_id, created_at)
		VALUES (?, ?, ?)`

	_, err := r.db.Exec(
		query,
		fingerprint.ID,
		fingerprint.LeadID,
		fingerprint.CreatedAt,
	)
	return err
}

// LinkToLead associates a Fingerprint with a Lead by updating the lead_id.
func (r *SQLFingerprintRepository) LinkToLead(fingerprintID, leadID string) error {
	const query = `
		UPDATE fingerprints 
		SET lead_id = ?
		WHERE id = ?`

	_, err := r.db.Exec(query, leadID, fingerprintID)
	return err
}

// Exists checks if a Fingerprint with the given ID exists.
func (r *SQLFingerprintRepository) Exists(fingerprintID string) (bool, error) {
	const query = `
		SELECT 1 FROM fingerprints 
		WHERE id = ? 
		LIMIT 1`

	var exists int
	err := r.db.QueryRow(query, fingerprintID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// scanFingerprint is a helper function to scan a sql.Row into a Fingerprint struct.
func (r *SQLFingerprintRepository) scanFingerprint(row *sql.Row) (*user.Fingerprint, error) {
	var fingerprint user.Fingerprint
	var leadID sql.NullString
	var createdAtStr string

	err := row.Scan(
		&fingerprint.ID,
		&leadID,
		&createdAtStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, err
	}

	// Handle nullable lead_id
	if leadID.Valid {
		fingerprint.LeadID = &leadID.String
	}

	// Parse timestamp
	fingerprint.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		// Try alternative timestamp format if RFC3339 fails
		fingerprint.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			return nil, err
		}
	}

	return &fingerprint, nil
}
