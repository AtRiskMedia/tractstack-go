// Package tenant provides database abstraction for multi-tenant support.
package tenant

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	defaults "github.com/AtRiskMedia/tractstack-go/config"
	_ "github.com/mattn/go-sqlite3"                      // SQLite driver
	_ "github.com/tursodatabase/libsql-client-go/libsql" // Turso driver
)

// Connection pool for reusing database connections per tenant
var (
	connectionPools = make(map[string]*sql.DB)
	poolMutex       = &sync.RWMutex{}
	poolStats       = make(map[string]int)
)

// Database wraps database connection with tenant context
type Database struct {
	Conn     *sql.DB
	TenantID string
	UseTurso bool
	isPooled bool // tracks if this connection is from pool
}

// NewDatabase creates or reuses a database connection for the specified tenant
func NewDatabase(config *Config) (*Database, error) {
	poolKey := getPoolKey(config)

	// Phase 1: Read-only check
	poolMutex.RLock()
	pooledConn, exists := connectionPools[poolKey]
	poolMutex.RUnlock()

	if exists {
		// Test the pooled connection
		if err := pooledConn.Ping(); err == nil {
			// Connection is good, return it
			return &Database{
				Conn:     pooledConn,
				TenantID: config.TenantID,
				UseTurso: config.TursoDatabase != "",
				isPooled: true,
			}, nil
		}

		// Phase 2: Stale connection removal (requires write lock)
		// Connection is bad, remove it from the pool
		poolMutex.Lock()
		// Double-check that the connection is still the same one we initially tested
		if currentConn, stillExists := connectionPools[poolKey]; stillExists && currentConn == pooledConn {
			delete(connectionPools, poolKey)
			currentConn.Close() // Ensure the bad connection is properly closed
		}
		poolMutex.Unlock()
	}

	// Create new connection
	var conn *sql.DB
	var err error
	var useTurso bool

	// Try Turso first if enabled AND credentials are available
	if config.TursoEnabled && config.TursoDatabase != "" && config.TursoToken != "" {
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

	// Configure connection for production use with environment variables
	conn.SetMaxOpenConns(defaults.DBMaxOpenConns)
	conn.SetMaxIdleConns(defaults.DBMaxIdleConns)
	conn.SetConnMaxLifetime(time.Duration(defaults.DBConnMaxLifetimeMinutes) * time.Minute)
	conn.SetConnMaxIdleTime(time.Duration(defaults.DBConnMaxIdleMinutes) * time.Minute)

	// Add to pool
	poolMutex.Lock()
	connectionPools[poolKey] = conn
	poolMutex.Unlock()

	return &Database{
		Conn:     conn,
		TenantID: config.TenantID,
		UseTurso: useTurso,
		isPooled: true,
	}, nil
}

// getPoolKey creates a unique key for the connection pool
func getPoolKey(config *Config) string {
	if config.TursoDatabase != "" {
		return fmt.Sprintf("turso:%s", config.TenantID)
	}
	return fmt.Sprintf("sqlite:%s", config.SQLitePath)
}

// Close closes the database connection (only if not pooled)
func (db *Database) Close() error {
	// Don't close pooled connections
	if db.isPooled {
		return nil
	}

	if db.Conn != nil {
		return db.Conn.Close()
	}
	return nil
}

// GetConnectionInfo returns a string describing the database connection
func (db *Database) GetConnectionInfo() string {
	poolStatus := ""
	if db.isPooled {
		poolStatus = " (pooled)"
	}

	if db.UseTurso {
		return fmt.Sprintf("Turso (tenant: %s)%s", db.TenantID, poolStatus)
	}
	return fmt.Sprintf("SQLite (tenant: %s)%s", db.TenantID, poolStatus)
}

// GetPoolStats returns current pool statistics
func GetPoolStats() map[string]int {
	poolMutex.RLock()
	defer poolMutex.RUnlock()

	stats := make(map[string]int)
	stats["total"] = len(connectionPools)

	active := 0
	for _, conn := range connectionPools {
		if conn.Ping() == nil {
			active++
		}
	}
	stats["active"] = active

	return stats
}

// CleanupStaleConnections removes dead and aged connections from the pool
// This function is called by the cache cleanup routine
func CleanupStaleConnections() {
	poolMutex.Lock()
	defer poolMutex.Unlock()

	staleKeys := make([]string, 0)

	for key, conn := range connectionPools {
		shouldRemove := false
		reason := ""

		// Check if connection is dead
		if err := conn.Ping(); err != nil {
			shouldRemove = true
			reason = "dead"
		} else {
			// Get connection stats to check age
			stats := conn.Stats()

			// Estimate connection age based on open connections
			if stats.OpenConnections > 0 {
				// Close connections that have been in pool too long
				if stats.Idle > 3 && stats.OpenConnections > 10 {
					shouldRemove = true
					reason = "aged"
				}
			}
		}

		if shouldRemove {
			conn.Close()
			staleKeys = append(staleKeys, key)
			if reason == "dead" {
				fmt.Printf("Database pool cleanup: removed dead connection %s\n", key)
			} else {
				fmt.Printf("Database pool cleanup: removed aged connection %s\n", key)
			}
		}
	}

	// Remove stale connections from pool
	for _, key := range staleKeys {
		delete(connectionPools, key)
	}

	if len(staleKeys) > 0 {
		fmt.Printf("Database pool cleanup: removed %d total connections\n", len(staleKeys))
	}
}

// GetConnectionPoolInfo returns detailed information about all pooled connections
func GetConnectionPoolInfo() map[string]map[string]interface{} {
	poolMutex.RLock()
	defer poolMutex.RUnlock()

	info := make(map[string]map[string]interface{})

	for key, conn := range connectionPools {
		stats := conn.Stats()
		isHealthy := conn.Ping() == nil

		info[key] = map[string]interface{}{
			"healthy":      isHealthy,
			"maxOpen":      stats.MaxOpenConnections,
			"open":         stats.OpenConnections,
			"inUse":        stats.InUse,
			"idle":         stats.Idle,
			"waitCount":    stats.WaitCount,
			"waitDuration": stats.WaitDuration.String(),
		}
	}

	return info
}
