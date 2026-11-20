package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"golang.org/x/crypto/bcrypt"
)

// EnsureMainAdmin inserts the main admin identity row if one does not exist.
// password_hash is a random bcrypt distractor — never used for authentication.
// The real main password lives only in env.json ADMIN_PASSWORD_HASH.
func EnsureMainAdmin(db *sql.DB, adminEmail string) error {
	email := strings.ToLower(strings.TrimSpace(adminEmail))
	if email == "" {
		return fmt.Errorf("ADMIN_EMAIL is required to seed the main admin row")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM admins WHERE is_main = 1`).Scan(&count); err != nil {
		return fmt.Errorf("failed to check main admin: %w", err)
	}
	if count > 0 {
		return nil
	}

	distractor, err := randomDistractorHash()
	if err != nil {
		return err
	}

	id := security.GenerateULID()
	_, err = db.Exec(`
		INSERT INTO admins (
			id, username, email, display_name, role, is_main,
			password_hash, must_change_password, token_version, created_by
		)
		SELECT ?, 'admin', ?, '', 'admin', 1, ?, 0, 0, NULL
		WHERE NOT EXISTS (SELECT 1 FROM admins WHERE is_main = 1)
	`, id, email, distractor)
	if err != nil {
		return fmt.Errorf("failed to seed main admin: %w", err)
	}
	return nil
}

func randomDistractorHash() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate distractor entropy: %w", err)
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(buf)), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash distractor: %w", err)
	}
	return string(hashed), nil
}
