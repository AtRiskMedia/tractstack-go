// Package api provides HTTP handlers and database connectivity for the application's API.
package api

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/tursodatabase/libsql-client-go/libsql" // Register libsql driver
)

// DB holds the database connection.
type DB struct {
	Conn *sql.DB
}

// NewDB initializes a new Turso database connection.
func NewDB() (*DB, error) {
	dbURL := os.Getenv("TURSO_DATABASE_URL")
	authToken := os.Getenv("TURSO_AUTH_TOKEN")
	log.Printf("Attempting to connect to Turso: URL=%s, Token=%s", dbURL, maskToken(authToken))
	if dbURL == "" || authToken == "" {
		return nil, fmt.Errorf("missing TURSO_DATABASE_URL or TURSO_AUTH_TOKEN")
	}

	connStr := dbURL + "?authToken=" + authToken
	conn, err := sql.Open("libsql", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	log.Println("Successfully connected to Turso database")
	return &DB{Conn: conn}, nil
}

// maskToken hides most of the token for logging.
func maskToken(token string) string {
	if len(token) < 8 {
		return "****"
	}
	return token[:4] + "****" + token[len(token)-4:]
}
