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

// SQLFingerprintRepository is the SQL-based implementation of the FingerprintRepository.
type SQLFingerprintRepository struct {
	db     *database.DB
	logger *logging.ChanneledLogger
}

// NewSQLFingerprintRepository creates a new instance of the repository.
func NewSQLFingerprintRepository(db *database.DB, logger *logging.ChanneledLogger) *SQLFingerprintRepository {
	return &SQLFingerprintRepository{
		db:     db,
		logger: logger,
	}
}

// FindByID retrieves a Fingerprint by its unique identifier.
func (r *SQLFingerprintRepository) FindByID(id string) (*user.Fingerprint, error) {
	const query = `
		SELECT id, lead_id, created_at
		FROM fingerprints 
		WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading fingerprint by ID", "id", id)

	row := r.db.QueryRow(query, id)
	fingerprint, err := r.scanFingerprint(row)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.Database().Debug("Fingerprint not found by ID", "id", id)
			return nil, nil
		}
		r.logger.Database().Error("Failed to load fingerprint by ID", "error", err.Error(), "id", id)
		return nil, err
	}

	if fingerprint != nil {
		r.logger.Database().Info("Fingerprint loaded by ID", "id", id, "leadId", fingerprint.LeadID, "duration", time.Since(start))
	}
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		const query = `SELECT id, lead_id, created_at FROM fingerprints WHERE id = ?`
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return fingerprint, nil
}

// FindByLeadID retrieves a Fingerprint associated with a specific Lead.
func (r *SQLFingerprintRepository) FindByLeadID(leadID string) (*user.Fingerprint, error) {
	const query = `
		SELECT id, lead_id, created_at
		FROM fingerprints 
		WHERE lead_id = ?
		LIMIT 1`

	start := time.Now()
	r.logger.Database().Debug("Loading fingerprint by lead ID", "leadId", leadID)

	row := r.db.QueryRow(query, leadID)
	fingerprint, err := r.scanFingerprint(row)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.Database().Debug("Fingerprint not found by lead ID", "leadId", leadID)
			return nil, nil
		}
		r.logger.Database().Error("Failed to load fingerprint by lead ID", "error", err.Error(), "leadId", leadID)
		return nil, err
	}

	if fingerprint != nil {
		r.logger.Database().Info("Fingerprint loaded by lead ID", "leadId", leadID, "fingerprintId", fingerprint.ID, "duration", time.Since(start))
	}
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		const query = `SELECT id, lead_id, created_at FROM fingerprints WHERE lead_id = ? LIMIT 1`
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return fingerprint, nil
}

// Create saves a new Fingerprint to the database.
func (r *SQLFingerprintRepository) Create(fingerprint *user.Fingerprint) error {
	const query = `
		INSERT INTO fingerprints (id, lead_id, created_at)
		VALUES (?, ?, ?)`

	start := time.Now()
	r.logger.Database().Debug("Executing fingerprint insert", "id", fingerprint.ID, "leadId", fingerprint.LeadID)

	_, err := r.db.Exec(
		query,
		fingerprint.ID,
		fingerprint.LeadID,
		fingerprint.CreatedAt,
	)
	if err != nil {
		r.logger.Database().Error("Fingerprint insert failed", "error", err.Error(), "id", fingerprint.ID, "leadId", fingerprint.LeadID)
		return err
	}

	r.logger.Database().Info("Fingerprint insert completed", "id", fingerprint.ID, "leadId", fingerprint.LeadID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		const query = `INSERT INTO fingerprints (id, lead_id, created_at) VALUES (?, ?, ?)`
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return nil
}

// LinkToLead associates a Fingerprint with a Lead by updating the lead_id.
func (r *SQLFingerprintRepository) LinkToLead(fingerprintID, leadID string) error {
	const query = `
		UPDATE fingerprints 
		SET lead_id = ?
		WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Executing fingerprint link to lead", "fingerprintId", fingerprintID, "leadId", leadID)

	_, err := r.db.Exec(query, leadID, fingerprintID)
	if err != nil {
		r.logger.Database().Error("Fingerprint link to lead failed", "error", err.Error(), "fingerprintId", fingerprintID, "leadId", leadID)
		return err
	}

	r.logger.Database().Info("Fingerprint link to lead completed", "fingerprintId", fingerprintID, "leadId", leadID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		const query = `UPDATE fingerprints SET lead_id = ? WHERE id = ?`
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return nil
}

// Exists checks if a Fingerprint with the given ID exists.
func (r *SQLFingerprintRepository) Exists(fingerprintID string) (bool, error) {
	const query = `
		SELECT 1 FROM fingerprints 
		WHERE id = ? 
		LIMIT 1`

	start := time.Now()
	r.logger.Database().Debug("Checking fingerprint existence", "fingerprintId", fingerprintID)

	var exists int
	err := r.db.QueryRow(query, fingerprintID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.Database().Debug("Fingerprint does not exist", "fingerprintId", fingerprintID, "duration", time.Since(start))
			return false, nil
		}
		r.logger.Database().Error("Failed to check fingerprint existence", "error", err.Error(), "fingerprintId", fingerprintID)
		return false, err
	}

	r.logger.Database().Info("Fingerprint existence confirmed", "fingerprintId", fingerprintID, "exists", true, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		const query = `SELECT 1 FROM fingerprints WHERE id = ? LIMIT 1`
		r.logger.LogSlowQuery(query, duration, "system")
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
