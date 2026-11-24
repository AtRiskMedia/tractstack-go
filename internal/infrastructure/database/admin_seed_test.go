package database

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func setupAdminsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS admins (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL COLLATE NOCASE UNIQUE,
		email TEXT NOT NULL COLLATE NOCASE UNIQUE,
		display_name TEXT,
		role TEXT NOT NULL CHECK (role IN ('admin','editor')),
		is_main INTEGER NOT NULL DEFAULT 0 CHECK (is_main IN (0,1)),
		password_hash TEXT NOT NULL,
		must_change_password INTEGER NOT NULL DEFAULT 0,
		token_version INTEGER NOT NULL DEFAULT 0,
		created_by TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		changed TIMESTAMP,
		last_login_at TIMESTAMP,
		CHECK (is_main = 0 OR role = 'admin')
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_admins_one_main ON admins(is_main) WHERE is_main = 1`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEnsureMainAdmin_SeedsDistractor(t *testing.T) {
	db := setupAdminsDB(t)
	defer db.Close()

	if err := EnsureMainAdmin(db, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	var username, email, hash string
	var isMain int
	err := db.QueryRow(`SELECT username, email, password_hash, is_main FROM admins WHERE is_main = 1`).
		Scan(&username, &email, &hash, &isMain)
	if err != nil {
		t.Fatal(err)
	}
	if username != "admin" || email != "admin@example.com" || isMain != 1 {
		t.Fatalf("unexpected row: %s %s %d", username, email, isMain)
	}
	if hash == "" {
		t.Fatal("expected non-empty distractor hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("password")); err == nil {
		t.Fatal("distractor should not match arbitrary password")
	}
}

func TestEnsureMainAdmin_Idempotent(t *testing.T) {
	db := setupAdminsDB(t)
	defer db.Close()

	if err := EnsureMainAdmin(db, "a@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureMainAdmin(db, "b@example.com"); err != nil {
		t.Fatal(err)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM admins WHERE is_main = 1`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 main row, got %d", count)
	}
	var email string
	_ = db.QueryRow(`SELECT email FROM admins WHERE is_main = 1`).Scan(&email)
	if email != "a@example.com" {
		t.Fatalf("email should stay first seed, got %s", email)
	}
}

func TestEnsureMainAdmin_RequiresEmail(t *testing.T) {
	db := setupAdminsDB(t)
	defer db.Close()
	if err := EnsureMainAdmin(db, "  "); err == nil {
		t.Fatal("expected error for empty email")
	}
}
