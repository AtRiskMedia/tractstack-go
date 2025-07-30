// Package user provides the concrete SQL-based implementations of
// the user domain repositories (Lead, Fingerprint, Visit).
package user

import (
	"database/sql"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

// SQLVisitRepository is the SQL-based implementation of the VisitRepository.
type SQLVisitRepository struct {
	db *database.DB
}

// NewSQLVisitRepository creates a new instance of the repository.
func NewSQLVisitRepository(db *database.DB) *SQLVisitRepository {
	return &SQLVisitRepository{db: db}
}

// FindByID retrieves a Visit by its unique identifier.
func (r *SQLVisitRepository) FindByID(id string) (*user.Visit, error) {
	const query = `
		SELECT id, fingerprint_id, campaign_id, created_at
		FROM visits 
		WHERE id = ?`

	row := r.db.QueryRow(query, id)
	return r.scanVisit(row)
}

// FindByFingerprintID retrieves all Visits associated with a specific Fingerprint.
func (r *SQLVisitRepository) FindByFingerprintID(fingerprintID string) ([]*user.Visit, error) {
	const query = `
		SELECT id, fingerprint_id, campaign_id, created_at
		FROM visits 
		WHERE fingerprint_id = ?
		ORDER BY created_at DESC`

	rows, err := r.db.Query(query, fingerprintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var visits []*user.Visit
	for rows.Next() {
		visit, err := r.scanVisitFromRows(rows)
		if err != nil {
			return nil, err
		}
		visits = append(visits, visit)
	}

	return visits, rows.Err()
}

// GetLatestByFingerprintID retrieves the most recent Visit for a Fingerprint.
func (r *SQLVisitRepository) GetLatestByFingerprintID(fingerprintID string) (*user.Visit, error) {
	const query = `
		SELECT id, fingerprint_id, campaign_id, created_at
		FROM visits 
		WHERE fingerprint_id = ?
		ORDER BY created_at DESC
		LIMIT 1`

	row := r.db.QueryRow(query, fingerprintID)
	return r.scanVisit(row)
}

// Create saves a new Visit to the database.
func (r *SQLVisitRepository) Create(visit *user.Visit) error {
	const query = `
		INSERT INTO visits (id, fingerprint_id, campaign_id, created_at)
		VALUES (?, ?, ?, ?)`

	_, err := r.db.Exec(
		query,
		visit.ID,
		visit.FingerprintID,
		visit.CampaignID,
		visit.CreatedAt,
	)
	return err
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
