package user

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
)

const adminSelectCols = `id, username, email, display_name, role, is_main, password_hash,
	must_change_password, token_version, created_by, created_at, changed, last_login_at`

// SQLAdminRepository persists Admin rows.
type SQLAdminRepository struct {
	db     *database.DB
	logger *logging.ChanneledLogger
}

// NewSQLAdminRepository creates a SQLAdminRepository.
func NewSQLAdminRepository(db *database.DB, logger *logging.ChanneledLogger) *SQLAdminRepository {
	return &SQLAdminRepository{db: db, logger: logger}
}

func (r *SQLAdminRepository) FindByID(id string) (*user.Admin, error) {
	row := r.db.QueryRow(`SELECT `+adminSelectCols+` FROM admins WHERE id = ?`, id)
	return r.scanAdmin(row)
}

func (r *SQLAdminRepository) FindByUsername(username string) (*user.Admin, error) {
	row := r.db.QueryRow(
		`SELECT `+adminSelectCols+` FROM admins WHERE username = ? COLLATE NOCASE`,
		strings.TrimSpace(username),
	)
	return r.scanAdmin(row)
}

func (r *SQLAdminRepository) FindByEmail(email string) (*user.Admin, error) {
	row := r.db.QueryRow(
		`SELECT `+adminSelectCols+` FROM admins WHERE email = ? COLLATE NOCASE`,
		strings.ToLower(strings.TrimSpace(email)),
	)
	return r.scanAdmin(row)
}

func (r *SQLAdminRepository) FindMain() (*user.Admin, error) {
	row := r.db.QueryRow(`SELECT ` + adminSelectCols + ` FROM admins WHERE is_main = 1 LIMIT 1`)
	return r.scanAdmin(row)
}

func (r *SQLAdminRepository) List() ([]*user.Admin, error) {
	rows, err := r.db.Query(`SELECT ` + adminSelectCols + ` FROM admins ORDER BY is_main DESC, username ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*user.Admin
	for rows.Next() {
		admin, err := r.scanAdminRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, admin)
	}
	return out, rows.Err()
}

func (r *SQLAdminRepository) Store(admin *user.Admin) error {
	var createdBy any
	if admin.CreatedBy != nil {
		createdBy = *admin.CreatedBy
	}
	isMain := 0
	if admin.IsMain {
		isMain = 1
	}
	mcp := 0
	if admin.MustChangePassword {
		mcp = 1
	}
	displayName := admin.DisplayName
	_, err := r.db.Exec(`
		INSERT INTO admins (
			id, username, email, display_name, role, is_main,
			password_hash, must_change_password, token_version, created_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		admin.ID,
		admin.Username,
		strings.ToLower(strings.TrimSpace(admin.Email)),
		displayName,
		admin.Role,
		isMain,
		admin.PasswordHash,
		mcp,
		admin.TokenVersion,
		createdBy,
		admin.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (r *SQLAdminRepository) Update(admin *user.Admin) error {
	mcp := 0
	if admin.MustChangePassword {
		mcp = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`
		UPDATE admins SET
			username = ?, email = ?, display_name = ?, role = ?,
			password_hash = ?, must_change_password = ?, token_version = ?,
			changed = ?
		WHERE id = ?
	`,
		admin.Username,
		strings.ToLower(strings.TrimSpace(admin.Email)),
		admin.DisplayName,
		admin.Role,
		admin.PasswordHash,
		mcp,
		admin.TokenVersion,
		now,
		admin.ID,
	)
	return err
}

func (r *SQLAdminRepository) Delete(id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM admin_tokens WHERE admin_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM admins WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLAdminRepository) TouchLastLogin(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`UPDATE admins SET last_login_at = ? WHERE id = ?`, now, id)
	return err
}

func (r *SQLAdminRepository) scanAdmin(row *sql.Row) (*user.Admin, error) {
	admin, err := scanAdminFields(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return admin, err
}

func (r *SQLAdminRepository) scanAdminRows(rows *sql.Rows) (*user.Admin, error) {
	return scanAdminFields(rows)
}

type scannable interface {
	Scan(dest ...any) error
}

func scanAdminFields(row scannable) (*user.Admin, error) {
	var admin user.Admin
	var displayName, createdBy sql.NullString
	var isMain, mcp int
	var createdAtStr string
	var changedStr, lastLoginStr sql.NullString

	err := row.Scan(
		&admin.ID,
		&admin.Username,
		&admin.Email,
		&displayName,
		&admin.Role,
		&isMain,
		&admin.PasswordHash,
		&mcp,
		&admin.TokenVersion,
		&createdBy,
		&createdAtStr,
		&changedStr,
		&lastLoginStr,
	)
	if err != nil {
		return nil, err
	}

	admin.IsMain = isMain == 1
	admin.MustChangePassword = mcp == 1
	if displayName.Valid {
		admin.DisplayName = displayName.String
	}
	if createdBy.Valid {
		v := createdBy.String
		admin.CreatedBy = &v
	}

	admin.CreatedAt, err = parseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if changedStr.Valid && changedStr.String != "" {
		t, err := parseTimestamp(changedStr.String)
		if err == nil {
			admin.Changed = &t
		}
	}
	if lastLoginStr.Valid && lastLoginStr.String != "" {
		t, err := parseTimestamp(lastLoginStr.String)
		if err == nil {
			admin.LastLoginAt = &t
		}
	}
	return &admin, nil
}

func parseTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", s)
}
