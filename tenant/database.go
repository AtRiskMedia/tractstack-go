// Package tenant provides database abstraction for multi-tenant support.
package tenant

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"                      // SQLite driver
	_ "github.com/tursodatabase/libsql-client-go/libsql" // Turso driver
)

// Database wraps database connection with tenant context
type Database struct {
	Conn     *sql.DB
	TenantID string
	UseTurso bool
}

// NewDatabase creates a database connection for the specified tenant
func NewDatabase(config *Config) (*Database, error) {
	var conn *sql.DB
	var err error
	var useTurso bool

	// Try Turso first if credentials are available
	if config.TursoDatabase != "" && config.TursoToken != "" {
		connStr := config.TursoDatabase + "?authToken=" + config.TursoToken
		conn, err = sql.Open("libsql", connStr)
		if err == nil {
			if pingErr := conn.Ping(); pingErr == nil {
				useTurso = true
			} else {
				conn.Close()
				conn = nil
			}
		}
	}

	// Fallback to SQLite if Turso failed or not configured
	if conn == nil {
		// Ensure directory exists
		dbDir := filepath.Dir(config.SQLitePath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}

		conn, err = sql.Open("sqlite3", config.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open SQLite database: %w", err)
		}

		if err := conn.Ping(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("SQLite database ping failed: %w", err)
		}
		useTurso = false
	}

	return &Database{
		Conn:     conn,
		TenantID: config.TenantID,
		UseTurso: useTurso,
	}, nil
}

// Close closes the database connection
func (db *Database) Close() error {
	if db.Conn != nil {
		return db.Conn.Close()
	}
	return nil
}

// GetConnectionInfo returns a string describing the database connection
func (db *Database) GetConnectionInfo() string {
	if db.UseTurso {
		return fmt.Sprintf("Turso (tenant: %s)", db.TenantID)
	}
	return fmt.Sprintf("SQLite (tenant: %s)", db.TenantID)
}
