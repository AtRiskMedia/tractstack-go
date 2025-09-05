// Package database provides the core functionality for creating and managing
// database connections in a clean, isolated manner.
package database

import (
	"database/sql"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
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

// NewConnectionWithLogger establishes a new database connection for the specified driver with logging.
func NewConnectionWithLogger(driverName, dataSourceName string, logger *logging.ChanneledLogger) (*DB, error) {
	start := time.Now()
	logger.Database().Debug("Creating new database connection", "driverName", driverName)

	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		logger.Database().Error("Failed to open database connection", "error", err.Error(), "driverName", driverName)
		return nil, err
	}

	if err = db.Ping(); err != nil {
		logger.Database().Error("Database ping failed", "error", err.Error(), "driverName", driverName)
		return nil, err
	}

	logger.Database().Info("Database connection established", "driverName", driverName, "duration", time.Since(start))
	duration := time.Since(start)
	if duration > GetSlowQueryThreshold() {
		logger.LogSlowQuery("DATABASE_CONNECTION", duration, "system")
	}

	return &DB{db}, nil
}
