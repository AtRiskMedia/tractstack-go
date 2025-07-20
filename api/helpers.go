// Package api provides shared helper functions
package api

import (
	"database/sql"
	"fmt"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// TestTursoConnection tests the Turso database connection
func TestTursoConnection(databaseURL, authToken string) error {
	// Create connection string
	connStr := fmt.Sprintf("%s?authToken=%s", databaseURL, authToken)

	// Attempt to open connection
	db, err := sql.Open("libsql", connStr)
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	// Test with a simple query
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("connection test query failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected query result: %d", result)
	}

	return nil
}
