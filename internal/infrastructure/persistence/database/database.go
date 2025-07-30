// Package database provides the core functionality for creating and managing
// database connections in a clean, isolated manner.
package database

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// DB represents a wrapper around the standard SQL database connection.
type DB struct {
	*sql.DB
}

// NewConnection establishes a new database connection for the specified driver.
func NewConnection(driverName, dataSourceName string) (*DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return &DB{db}, nil
}
