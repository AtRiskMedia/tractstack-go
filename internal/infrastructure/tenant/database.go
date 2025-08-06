// Package tenant provides database abstraction for multi-tenant support.
package tenant

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
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
	logger   *logging.ChanneledLogger
}

func NewDatabase(cfg *Config, logger *logging.ChanneledLogger) (*Database, error) {
	start := time.Now()
	logger.Database().Debug("Creating new database connection", "tenantID", cfg.TenantID, "tursoEnabled", cfg.TursoEnabled)

	poolKey := getPoolKey(cfg)

	poolMutex.RLock()
	if pooledConn, exists := connectionPools[poolKey]; exists {
		poolMutex.RUnlock()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := pooledConn.PingContext(ctx); err == nil {
			logger.Database().Info("Reusing existing pooled database connection",
				"tenantID", cfg.TenantID,
				"poolKey", poolKey,
				"useTurso", cfg.TursoDatabase != "",
				"duration", time.Since(start))

			duration := time.Since(start)
			if duration > config.SlowQueryThreshold {
				logger.LogSlowQuery("DATABASE_CONNECTION_REUSE", duration, cfg.TenantID)
			}

			return &Database{
				Conn:     pooledConn,
				TenantID: cfg.TenantID,
				UseTurso: cfg.TursoDatabase != "",
				isPooled: true,
				logger:   logger,
			}, nil
		}

		poolMutex.Lock()
		logger.Database().Warn("Existing pooled connection failed ping, removing from pool", "poolKey", poolKey, "tenantID", cfg.TenantID)
		pooledConn.Close()
		delete(connectionPools, poolKey)
		poolMutex.Unlock()
	} else {
		poolMutex.RUnlock()
	}

	var conn *sql.DB
	var err error
	var useTurso bool

	if cfg.TursoEnabled && cfg.TursoDatabase != "" && cfg.TursoToken != "" {
		logger.Database().Debug("Attempting Turso connection", "tenantID", cfg.TenantID, "database", cfg.TursoDatabase)

		connStr := fmt.Sprintf("%s?authToken=%s", cfg.TursoDatabase, cfg.TursoToken)
		conn, err = sql.Open("libsql", connStr)
		if err != nil {
			logger.Database().Error("Turso connection failed", "error", err.Error(), "tenantID", cfg.TenantID)
			return nil, fmt.Errorf("turso connection failed for tenant %s: %w", cfg.TenantID, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := conn.PingContext(ctx); err != nil {
			logger.Database().Error("Turso ping failed", "error", err.Error(), "tenantID", cfg.TenantID)
			conn.Close()
			return nil, fmt.Errorf("turso ping failed for tenant %s: %w", cfg.TenantID, err)
		}
		logger.Database().Info("Turso connection established", "tenantID", cfg.TenantID, "database", cfg.TursoDatabase)
		useTurso = true
	} else {
		logger.Database().Debug("Using SQLite fallback", "tenantID", cfg.TenantID, "path", cfg.SQLitePath)

		dbDir := filepath.Dir(cfg.SQLitePath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			logger.Database().Error("Failed to create database directory", "error", err.Error(), "dir", dbDir, "tenantID", cfg.TenantID)
			return nil, fmt.Errorf("failed to create database directory for tenant %s: %w", cfg.TenantID, err)
		}

		conn, err = sql.Open("sqlite3", cfg.SQLitePath)
		if err != nil {
			logger.Database().Error("SQLite fallback connection failed", "error", err.Error(), "tenantID", cfg.TenantID, "path", cfg.SQLitePath)
			return nil, fmt.Errorf("sqlite connection failed for tenant %s: %w", cfg.TenantID, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := conn.PingContext(ctx); err != nil {
			logger.Database().Error("SQLite fallback ping failed", "error", err.Error(), "tenantID", cfg.TenantID)
			conn.Close()
			return nil, fmt.Errorf("sqlite ping failed for tenant %s: %w", cfg.TenantID, err)
		}
		logger.Database().Info("SQLite fallback connection established", "tenantID", cfg.TenantID, "path", cfg.SQLitePath)
		useTurso = false
	}

	logger.Database().Debug("Configuring database connection pool",
		"tenantID", cfg.TenantID,
		"maxOpenConns", config.DBMaxOpenConns,
		"maxIdleConns", config.DBMaxIdleConns,
		"connMaxLifetime", config.DBConnMaxLifetimeMinutes,
		"connMaxIdleTime", config.DBConnMaxIdleMinutes)

	conn.SetMaxOpenConns(config.DBMaxOpenConns)
	conn.SetMaxIdleConns(config.DBMaxIdleConns)
	conn.SetConnMaxLifetime(time.Duration(config.DBConnMaxLifetimeMinutes) * time.Minute)
	conn.SetConnMaxIdleTime(time.Duration(config.DBConnMaxIdleMinutes) * time.Minute)

	poolMutex.Lock()
	connectionPools[poolKey] = conn
	poolMutex.Unlock()

	logger.Database().Info("Database connection added to pool", "poolKey", poolKey, "tenantID", cfg.TenantID)

	logger.Database().Info("Database connection created successfully",
		"tenantID", cfg.TenantID,
		"useTurso", useTurso,
		"pooled", true,
		"duration", time.Since(start))

	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		logger.LogSlowQuery("DATABASE_CONNECTION_CREATE", duration, cfg.TenantID)
	}

	return &Database{
		Conn:     conn,
		TenantID: cfg.TenantID,
		UseTurso: useTurso,
		isPooled: true,
		logger:   logger,
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
		db.logger.Database().Debug("Skipping close for pooled connection", "tenantID", db.TenantID)
		return nil
	}
	if db.Conn != nil {
		db.logger.Database().Info("Closing non-pooled database connection", "tenantID", db.TenantID)
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
	stats["totalPools"] = len(connectionPools)

	for poolKey := range connectionPools {
		if conn := connectionPools[poolKey]; conn != nil {
			dbStats := conn.Stats()
			stats[poolKey+"_openConnections"] = dbStats.OpenConnections
			stats[poolKey+"_inUse"] = dbStats.InUse
			stats[poolKey+"_idle"] = dbStats.Idle
		}
	}

	return stats
}

func CleanupPools(logger *logging.ChanneledLogger) {
	start := time.Now()
	logger.Database().Debug("Starting database pool cleanup")

	poolMutex.Lock()
	defer poolMutex.Unlock()

	removedCount := 0
	for poolKey, conn := range connectionPools {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := conn.PingContext(ctx); err != nil {
			logger.Database().Warn("Removing stale connection from pool", "poolKey", poolKey, "error", err.Error())
			conn.Close()
			delete(connectionPools, poolKey)
			removedCount++
		}
		cancel()
	}

	logger.Database().Info("Database pool cleanup completed",
		"removedConnections", removedCount,
		"activeConnections", len(connectionPools))

	duration := time.Since(start)
	if duration > config.SlowQueryThreshold {
		logger.LogSlowQuery("DATABASE_POOL_CLEANUP", duration, "system")
	}
}
