package services

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	domainUser "github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/database"
	persistenceUser "github.com/AtRiskMedia/tractstack-go/internal/infrastructure/persistence/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/security"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func setupAuthTenant(t *testing.T, envPassword string) (*tenant.Context, *sql.DB) {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`CREATE TABLE admins (
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
	_, err = raw.Exec(`CREATE TABLE admin_tokens (
		id TEXT PRIMARY KEY,
		admin_id TEXT NOT NULL,
		purpose TEXT NOT NULL CHECK (purpose IN ('reset')),
		token_hash TEXT NOT NULL UNIQUE,
		expires_at TIMESTAMP NOT NULL,
		used_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatal(err)
	}

	envHash, err := bcrypt.GenerateFromPassword([]byte(envPassword), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	distractor, err := bcrypt.GenerateFromPassword([]byte("distractor-secret-never-used"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}

	mainID := security.GenerateULID()
	_, err = raw.Exec(`INSERT INTO admins (id, username, email, display_name, role, is_main, password_hash, must_change_password, token_version)
		VALUES (?, 'admin', 'main@example.com', 'Main', 'admin', 1, ?, 0, 0)`, mainID, string(distractor))
	if err != nil {
		t.Fatal(err)
	}

	editorHash, err := bcrypt.GenerateFromPassword([]byte("editorpass1"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	editorID := security.GenerateULID()
	_, err = raw.Exec(`INSERT INTO admins (id, username, email, display_name, role, is_main, password_hash, must_change_password, token_version)
		VALUES (?, 'editor1', 'editor@example.com', 'Ed', 'editor', 0, ?, 1, 0)`, editorID, string(editorHash))
	if err != nil {
		t.Fatal(err)
	}

	ctx := &tenant.Context{
		TenantID: "default",
		Config: &tenant.Config{
			TenantID:          "default",
			AdminPasswordHash: string(envHash),
			JWTSecret:         "test-jwt-secret-at-least-32-bytes!!",
		},
		Database: &tenant.Database{Conn: raw},
		Status:   "active",
	}
	return ctx, raw
}

func TestAuthenticateAdmin_MainUsesEnvNotRowHash(t *testing.T) {
	ctx, db := setupAuthTenant(t, "env-password-ok")
	defer db.Close()
	auth := &AuthService{}

	// Correct env password succeeds
	ok := auth.AuthenticateAdminUsername("admin", "env-password-ok", ctx)
	if !ok.Success || ok.Principal == nil || !ok.Principal.IsMain {
		t.Fatalf("expected main login via env hash, got %+v", ok)
	}

	// Distractor password must fail even though it matches row.password_hash
	bad := auth.AuthenticateAdminUsername("admin", "distractor-secret-never-used", ctx)
	if bad.Success {
		t.Fatal("main must never authenticate against row distractor hash")
	}

	// Wrong env password fails
	wrong := auth.AuthenticateAdminUsername("admin", "wrong-password", ctx)
	if wrong.Success {
		t.Fatal("wrong env password should fail")
	}
}

func TestAuthenticateAdmin_TableUserUsesRowHash(t *testing.T) {
	ctx, db := setupAuthTenant(t, "env-password-ok")
	defer db.Close()
	auth := &AuthService{}

	ok := auth.AuthenticateAdminUsername("editor1", "editorpass1", ctx)
	if !ok.Success || ok.Principal == nil || ok.Principal.Role != "editor" {
		t.Fatalf("expected editor login, got %+v", ok)
	}
	if !ok.Principal.MustChangePassword {
		t.Fatal("expected mcp claim from row")
	}
	if ok.Principal.IsMain {
		t.Fatal("editor must not be main")
	}

	bad := auth.AuthenticateAdminUsername("editor1", "env-password-ok", ctx)
	if bad.Success {
		t.Fatal("editor must not use env hash")
	}
}

func TestAuthenticateAdmin_MainTVChangesWithEnvHash(t *testing.T) {
	ctx, db := setupAuthTenant(t, "env-password-ok")
	defer db.Close()
	auth := &AuthService{}

	first := auth.AuthenticateAdminUsername("admin", "env-password-ok", ctx)
	if !first.Success {
		t.Fatal(first.Error)
	}
	tv1 := first.Principal.TokenVersion

	newHash, _ := bcrypt.GenerateFromPassword([]byte("new-env-password"), bcrypt.DefaultCost)
	ctx.Config.AdminPasswordHash = string(newHash)
	second := auth.AuthenticateAdminUsername("admin", "new-env-password", ctx)
	if !second.Success {
		t.Fatal(second.Error)
	}
	if second.Principal.TokenVersion == tv1 {
		t.Fatal("tv must change when ADMIN_PASSWORD_HASH changes")
	}

	// Old token should fail tv check
	p, valid := auth.ValidateTokenWithRolesPrincipal(first.Token, ctx, []string{"admin"})
	if valid {
		t.Fatalf("old token should be revoked after env hash change, got %+v", p)
	}
}

func TestAdminAccount_ReservedUsernameRejected(t *testing.T) {
	ctx, db := setupAuthTenant(t, "env-password-ok")
	defer db.Close()
	svc := NewAdminAccountService(nil, &AuthService{})
	main := &AdminPrincipal{Sub: "main-id", Role: "admin", IsMain: true}

	_, err := svc.Create(main, CreateAdminInput{
		Username: "Admin",
		Email:    "other@example.com",
		Role:     "editor",
		Password: "password1",
	}, ctx)
	if err == nil || err.Error() != "username is reserved" {
		t.Fatalf("expected reserved username error, got %v", err)
	}
}

func TestAdminAccount_CapabilityMatrix(t *testing.T) {
	ctx, db := setupAuthTenant(t, "env-password-ok")
	defer db.Close()
	svc := NewAdminAccountService(nil, &AuthService{})

	createdAdminID := security.GenerateULID()
	hash, _ := bcrypt.GenerateFromPassword([]byte("adminpass1"), bcrypt.DefaultCost)
	_, err := db.Exec(`INSERT INTO admins (id, username, email, display_name, role, is_main, password_hash, must_change_password, token_version)
		VALUES (?, 'siteadmin', 'siteadmin@example.com', 'SA', 'admin', 0, ?, 0, 0)`, createdAdminID, string(hash))
	if err != nil {
		t.Fatal(err)
	}

	nonMain := &AdminPrincipal{Sub: createdAdminID, Role: "admin", IsMain: false}
	_, err = svc.Create(nonMain, CreateAdminInput{
		Username: "newadmin",
		Email:    "newadmin@example.com",
		Role:     "admin",
		Password: "password1",
	}, ctx)
	if err == nil {
		t.Fatal("non-main admin must not create admins")
	}

	ed, err := svc.Create(nonMain, CreateAdminInput{
		Username: "neweditor",
		Email:    "neweditor@example.com",
		Role:     "editor",
		Password: "password1",
	}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ed.Role != "editor" {
		t.Fatal("expected editor")
	}

	editor := &AdminPrincipal{Sub: ed.ID, Role: "editor", IsMain: false}
	_, err = svc.Create(editor, CreateAdminInput{
		Username: "x",
		Email:    "x@example.com",
		Role:     "editor",
		Password: "password1",
	}, ctx)
	if err == nil {
		t.Fatal("editor must not create accounts")
	}
}

func TestRequestPasswordReset_NoResendNoOp(t *testing.T) {
	ctx, db := setupAuthTenant(t, "env-password-ok")
	defer db.Close()
	auth := &AuthService{}
	auth.RequestPasswordReset("editor@example.com", ctx, false)
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM admin_tokens`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected no tokens without resend, got %d", count)
	}
}

func TestConsumePasswordReset_EditorSingleUse(t *testing.T) {
	ctx, db := setupAuthTenant(t, "env-password-ok")
	defer db.Close()
	auth := &AuthService{}

	repo := persistenceUser.NewSQLAdminRepository(&database.DB{DB: db}, nil)
	editor, err := repo.FindByUsername("editor1")
	if err != nil || editor == nil {
		t.Fatal(err)
	}

	raw := "reset-token-raw-value-32chars!!"
	sum := sha256.Sum256([]byte(raw))
	tokenHash := hex.EncodeToString(sum[:])
	tokenRepo := persistenceUser.NewSQLAdminTokenRepository(&database.DB{DB: db}, nil)
	tok := &domainUser.AdminToken{
		ID:        security.GenerateULID(),
		AdminID:   editor.ID,
		Purpose:   "reset",
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		CreatedAt: time.Now().UTC(),
	}
	if err := tokenRepo.Store(tok); err != nil {
		t.Fatal(err)
	}

	if err := auth.ConsumePasswordReset(raw, "neweditorpass", ctx); err != nil {
		t.Fatal(err)
	}
	// No session issued — just verify login works with new password
	ok := auth.AuthenticateAdminUsername("editor1", "neweditorpass", ctx)
	if !ok.Success {
		t.Fatal("reset password should work for login")
	}
	if ok.Principal.MustChangePassword {
		t.Fatal("mcp should clear after reset")
	}
	// Token single-use
	if err := auth.ConsumePasswordReset(raw, "anotherpass1", ctx); err == nil {
		t.Fatal("token must be single-use")
	}
}

func TestPrincipalFromAdmin_McpAndMainTV(t *testing.T) {
	admin := &domainUser.Admin{
		ID: "1", Username: "admin", Role: "admin", IsMain: true,
		MustChangePassword: false, TokenVersion: 99,
	}
	p := principalFromAdmin(admin, "env-hash")
	if p.TokenVersion != mainPasswordTV("env-hash") {
		t.Fatal("main tv must come from env hash, not row token_version")
	}
	ed := &domainUser.Admin{
		ID: "2", Username: "ed", Role: "editor", IsMain: false,
		MustChangePassword: true, TokenVersion: 3,
	}
	pe := principalFromAdmin(ed, "env-hash")
	if pe.TokenVersion != "3" || !pe.MustChangePassword {
		t.Fatalf("unexpected editor principal %+v", pe)
	}
}
