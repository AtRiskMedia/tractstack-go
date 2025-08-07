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

// SQLVisitRepository is the SQL-based implementation of the VisitRepository.
type SQLVisitRepository struct {
	db     *database.DB
	logger *logging.ChanneledLogger
}

// NewSQLVisitRepository creates a new instance of the repository.
func NewSQLVisitRepository(db *database.DB, logger *logging.ChanneledLogger) *SQLVisitRepository {
	return &SQLVisitRepository{
		db:     db,
		logger: logger,
	}
}

// FindByID retrieves a Visit by its unique identifier.
func (r *SQLVisitRepository) FindByID(id string) (*user.Visit, error) {
	const query = `
		SELECT id, fingerprint_id, campaign_id, created_at
		FROM visits 
		WHERE id = ?`

	start := time.Now()
	r.logger.Database().Debug("Loading visit by ID", "id", id)

	row := r.db.QueryRow(query, id)
	visit, err := r.scanVisit(row)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.Database().Debug("Visit not found by ID", "id", id)
			return nil, nil
		}
		r.logger.Database().Error("Failed to load visit by ID", "error", err.Error(), "id", id)
		return nil, err
	}

	if visit != nil {
		r.logger.Database().Info("Visit loaded by ID", "id", id, "fingerprintId", visit.FingerprintID, "duration", time.Since(start))
	}
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return visit, nil
}

// FindByFingerprintID retrieves all Visits associated with a specific Fingerprint.
func (r *SQLVisitRepository) FindByFingerprintID(fingerprintID string) ([]*user.Visit, error) {
	const query = `
		SELECT id, fingerprint_id, campaign_id, created_at
		FROM visits 
		WHERE fingerprint_id = ?
		ORDER BY created_at DESC`

	start := time.Now()
	r.logger.Database().Debug("Loading visits by fingerprint ID", "fingerprintId", fingerprintID)

	rows, err := r.db.Query(query, fingerprintID)
	if err != nil {
		r.logger.Database().Error("Failed to query visits by fingerprint ID", "error", err.Error(), "fingerprintId", fingerprintID)
		return nil, err
	}
	defer rows.Close()

	var visits []*user.Visit
	for rows.Next() {
		visit, err := r.scanVisitFromRows(rows)
		if err != nil {
			r.logger.Database().Error("Failed to scan visit row", "error", err.Error(), "fingerprintId", fingerprintID)
			return nil, err
		}
		visits = append(visits, visit)
	}

	if err := rows.Err(); err != nil {
		r.logger.Database().Error("Row iteration error for visits", "error", err.Error(), "fingerprintId", fingerprintID)
		return nil, err
	}

	r.logger.Database().Info("Visits loaded by fingerprint ID", "fingerprintId", fingerprintID, "count", len(visits), "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return visits, nil
}

// GetLatestByFingerprintID retrieves the most recent Visit for a Fingerprint.
func (r *SQLVisitRepository) GetLatestByFingerprintID(fingerprintID string) (*user.Visit, error) {
	const query = `
		SELECT id, fingerprint_id, campaign_id, created_at
		FROM visits 
		WHERE fingerprint_id = ?
		ORDER BY created_at DESC
		LIMIT 1`

	start := time.Now()
	r.logger.Database().Debug("Loading latest visit by fingerprint ID", "fingerprintId", fingerprintID)

	row := r.db.QueryRow(query, fingerprintID)
	visit, err := r.scanVisit(row)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.Database().Debug("No visits found for fingerprint ID", "fingerprintId", fingerprintID)
			return nil, nil
		}
		r.logger.Database().Error("Failed to load latest visit by fingerprint ID", "error", err.Error(), "fingerprintId", fingerprintID)
		return nil, err
	}

	if visit != nil {
		r.logger.Database().Info("Latest visit loaded by fingerprint ID", "fingerprintId", fingerprintID, "visitId", visit.ID, "duration", time.Since(start))
	}
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return visit, nil
}

// Create saves a new Visit to the database.
func (r *SQLVisitRepository) Create(visit *user.Visit) error {
	const query = `
		INSERT INTO visits (id, fingerprint_id, campaign_id, created_at)
		VALUES (?, ?, ?, ?)`

	start := time.Now()
	r.logger.Database().Debug("Executing visit insert", "id", visit.ID, "fingerprintId", visit.FingerprintID, "campaignId", visit.CampaignID)

	_, err := r.db.Exec(
		query,
		visit.ID,
		visit.FingerprintID,
		visit.CampaignID,
		visit.CreatedAt,
	)
	if err != nil {
		r.logger.Database().Error("Visit insert failed", "error", err.Error(), "id", visit.ID, "fingerprintId", visit.FingerprintID)
		return err
	}

	r.logger.Database().Info("Visit insert completed", "id", visit.ID, "fingerprintId", visit.FingerprintID, "campaignId", visit.CampaignID, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		r.logger.LogSlowQuery(query, duration, "system")
	}
	return nil
}

// scanVisit is a helper function to scan a sql.Row into a Visit struct.
func (r *SQLVisitRepository) scanVisit(row *sql.Row) (*user.Visit, error) {
	var visit user.Visit
	var campaignID sql.NullString
	var createdAtStr string

	err := row.Scan(
		&visit.ID,
		&visit.FingerprintID,
		&campaignID,
		&createdAtStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, err
	}

	// Handle nullable campaign_id
	if campaignID.Valid {
		visit.CampaignID = &campaignID.String
	}

	// Parse timestamp
	visit.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		// Try alternative timestamp format if RFC3339 fails
		visit.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			return nil, err
		}
	}

	return &visit, nil
}

// scanVisitFromRows is a helper function to scan from sql.Rows into a Visit struct.
func (r *SQLVisitRepository) scanVisitFromRows(rows *sql.Rows) (*user.Visit, error) {
	var visit user.Visit
	var campaignID sql.NullString
	var createdAtStr string

	err := rows.Scan(
		&visit.ID,
		&visit.FingerprintID,
		&campaignID,
		&createdAtStr,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable campaign_id
	if campaignID.Valid {
		visit.CampaignID = &campaignID.String
	}

	// Parse timestamp
	visit.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		// Try alternative timestamp format if RFC3339 fails
		visit.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			return nil, err
		}
	}

	return &visit, nil
}
