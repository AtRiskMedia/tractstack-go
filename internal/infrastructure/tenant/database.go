// Package tenant provides database abstraction for multi-tenant support.
package tenant

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/pkg/config"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

var (
	connectionPools = make(map[string]*sql.DB)
	poolMutex       = &sync.RWMutex{}
	poolStats       = make(map[string]int)
)

type Database struct {
	Conn     *sql.DB
	TenantID string
	UseTurso bool
	isPooled bool
}

func NewDatabase(cfg *Config) (*Database, error) {
	poolKey := getPoolKey(cfg)

	poolMutex.Lock()
	defer poolMutex.Unlock()

	if pooledConn, exists := connectionPools[poolKey]; exists {
		if err := pooledConn.Ping(); err == nil {
			return &Database{
				Conn:     pooledConn,
				TenantID: cfg.TenantID,
				UseTurso: cfg.TursoDatabase != "",
				isPooled: true,
			}, nil
		}
		pooledConn.Close()
		delete(connectionPools, poolKey)
	}

	var conn *sql.DB
	var err error
	var useTurso bool

	if cfg.TursoEnabled && cfg.TursoDatabase != "" && cfg.TursoToken != "" {
		connStr := cfg.TursoDatabase + "?authToken=" + cfg.TursoToken
		conn, err = sql.Open("libsql", connStr)
		if err != nil || conn.Ping() != nil {
			return nil, fmt.Errorf("tenant %s degraded: turso connection failed", cfg.TenantID)
		}
		useTurso = true
	} else {
		// SQLite3 if Turso is not enabled on this tenant
		conn, err = sql.Open("sqlite3", cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("sqlite connection failed: %w", err)
		}
		useTurso = false
	}

	if conn == nil {
		dbDir := filepath.Dir(cfg.SQLitePath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}

		conn, err = sql.Open("sqlite3", cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open SQLite database: %w", err)
		}

		if err := conn.Ping(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("SQLite database ping failed: %w", err)
		}
		useTurso = false
	}

	conn.SetMaxOpenConns(config.DBMaxOpenConns)
	conn.SetMaxIdleConns(config.DBMaxIdleConns)
	conn.SetConnMaxLifetime(time.Duration(config.DBConnMaxLifetimeMinutes) * time.Minute)
	conn.SetConnMaxIdleTime(time.Duration(config.DBConnMaxIdleMinutes) * time.Minute)

	connectionPools[poolKey] = conn

	return &Database{
		Conn:     conn,
		TenantID: cfg.TenantID,
		UseTurso: useTurso,
		isPooled: true,
	}, nil
}

func getPoolKey(config *Config) string {
	if config.TursoDatabase != "" {
		return fmt.Sprintf("turso:%s", config.TenantID)
	}
	return fmt.Sprintf("sqlite:%s", config.SQLitePath)
}

func (db *Database) Close() error {
	if db.isPooled {
		return nil
	}
	if db.Conn != nil {
		return db.Conn.Close()
	}
	return nil
}

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

func CleanupStaleConnections() {
	poolMutex.Lock()
	defer poolMutex.Unlock()

	staleKeys := make([]string, 0)
	for key, conn := range connectionPools {
		shouldRemove := false
		reason := ""

		if err := conn.Ping(); err != nil {
			shouldRemove = true
			reason = "dead"
		} else {
			stats := conn.Stats()
			if stats.OpenConnections > 0 {
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

	for _, key := range staleKeys {
		delete(connectionPools, key)
	}
	if len(staleKeys) > 0 {
		fmt.Printf("Database pool cleanup: removed %d total connections\n", len(staleKeys))
	}
}

func GetConnectionPoolInfo() map[string]map[string]any {
	poolMutex.RLock()
	defer poolMutex.RUnlock()

	info := make(map[string]map[string]any)
	for key, conn := range connectionPools {
		stats := conn.Stats()
		isHealthy := conn.Ping() == nil
		info[key] = map[string]any{
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
