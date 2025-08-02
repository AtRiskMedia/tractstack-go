// Package database provides database helper functions
package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/pkg/config"
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

// TestTursoConnectionWithLogger tests the Turso database connection with logging
func TestTursoConnectionWithLogger(databaseURL, authToken string, logger *logging.ChanneledLogger) error {
	start := time.Now()
	logger.Database().Debug("Testing Turso database connection", "databaseURL", databaseURL)

	// Create connection string
	connStr := fmt.Sprintf("%s?authToken=%s", databaseURL, authToken)

	// Attempt to open connection
	db, err := sql.Open("libsql", connStr)
	if err != nil {
		logger.Database().Error("Failed to open Turso connection", "error", err.Error(), "databaseURL", databaseURL)
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	// Test with a simple query
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		logger.Database().Error("Turso connection test query failed", "error", err.Error(), "databaseURL", databaseURL)
		return fmt.Errorf("connection test query failed: %w", err)
	}

	if result != 1 {
		logger.Database().Error("Unexpected Turso query result", "result", result, "expected", 1, "databaseURL", databaseURL)
		return fmt.Errorf("unexpected query result: %d", result)
	}

	logger.Database().Info("Turso connection test successful", "databaseURL", databaseURL, "duration", time.Since(start))
	return nil
}

// GetSlowQueryThreshold returns the configured slow query threshold
// Default is 100ms, configurable via environment variable
func GetSlowQueryThreshold() time.Duration {
	// Check if we have a configured value in config package
	// If not, return default
	return config.SlowQueryThreshold
}

// CheckAndLogSlowQuery checks if a query duration exceeds threshold
// and logs it using the slow query channel if it does
func CheckAndLogSlowQuery(logger *logging.ChanneledLogger, query string, duration time.Duration, tenantID string) {
	threshold := GetSlowQueryThreshold() // Starts at 500ms

	// Apply a 5x multiplier for known, long-running bulk operations
	if strings.HasPrefix(query, "BULK_") {
		threshold *= 3
	}

	if duration > threshold {
		logger.LogSlowQuery(query, duration, tenantID)
	}
}
