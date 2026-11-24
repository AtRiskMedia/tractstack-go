package user

import (
	"database/sql"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

// SQLAdminTokenRepository persists admin password-reset tokens.
type SQLAdminTokenRepository struct {
	db     *database.DB
	logger *logging.ChanneledLogger
}

// NewSQLAdminTokenRepository creates a SQLAdminTokenRepository.
func NewSQLAdminTokenRepository(db *database.DB, logger *logging.ChanneledLogger) *SQLAdminTokenRepository {
	return &SQLAdminTokenRepository{db: db, logger: logger}
}

func (r *SQLAdminTokenRepository) Store(token *user.AdminToken) error {
	_, err := r.db.Exec(`
		INSERT INTO admin_tokens (id, admin_id, purpose, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		token.ID,
		token.AdminID,
		token.Purpose,
		token.TokenHash,
		token.ExpiresAt.UTC().Format(time.RFC3339),
		token.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (r *SQLAdminTokenRepository) FindValidByHash(tokenHash string) (*user.AdminToken, error) {
	row := r.db.QueryRow(`
		SELECT id, admin_id, purpose, token_hash, expires_at, used_at, created_at
		FROM admin_tokens
		WHERE token_hash = ? AND used_at IS NULL
	`, tokenHash)

	var tok user.AdminToken
	var expiresStr, createdStr string
	var usedStr sql.NullString
	err := row.Scan(&tok.ID, &tok.AdminID, &tok.Purpose, &tok.TokenHash, &expiresStr, &usedStr, &createdStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	tok.ExpiresAt, err = parseTimestamp(expiresStr)
	if err != nil {
		return nil, err
	}
	tok.CreatedAt, err = parseTimestamp(createdStr)
	if err != nil {
		return nil, err
	}
	if usedStr.Valid && usedStr.String != "" {
		t, err := parseTimestamp(usedStr.String)
		if err == nil {
			tok.UsedAt = &t
		}
	}
	if time.Now().UTC().After(tok.ExpiresAt) {
		return nil, nil
	}
	return &tok, nil
}

func (r *SQLAdminTokenRepository) MarkUsed(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`UPDATE admin_tokens SET used_at = ? WHERE id = ?`, now, id)
	return err
}

func (r *SQLAdminTokenRepository) InvalidateFor(adminID, purpose string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`
		UPDATE admin_tokens SET used_at = ?
		WHERE admin_id = ? AND purpose = ? AND used_at IS NULL
	`, now, adminID, purpose)
	return err
}

func (r *SQLAdminTokenRepository) PruneExpired() error {
	_, err := r.db.Exec(`
		DELETE FROM admin_tokens
		WHERE used_at IS NOT NULL OR expires_at < ?
	`, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (r *SQLAdminTokenRepository) DeleteForAdmin(adminID string) error {
	_, err := r.db.Exec(`DELETE FROM admin_tokens WHERE admin_id = ?`, adminID)
	return err
}
