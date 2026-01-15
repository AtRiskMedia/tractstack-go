// Package logging provides structured logging channels for TractStack operations
// with multi-tenant support and performance correlation capabilities.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Channel represents a logical logging channel for different system components
type Channel string

const (
	// ChannelSystem represents general system operations.
	ChannelSystem Channel = "system"
	// ChannelStartup represents application startup and initialization events.
	ChannelStartup Channel = "startup"
	// ChannelShutdown represents application shutdown events.
	ChannelShutdown Channel = "shutdown"

	// ChannelAuth represents authentication and authorization events.
	ChannelAuth Channel = "auth"
	// ChannelContent represents the logging channel for content management operations.
	ChannelContent Channel = "content"
	// ChannelAnalytics represents the logging channel for analytics and event processing operations.
	ChannelAnalytics Channel = "analytics"
	// ChannelCache represents the logging channel for internal caching operations and management.
	ChannelCache Channel = "cache"

	// ChannelDatabase represents database operations and queries.
	ChannelDatabase Channel = "database"
	// ChannelTenant represents the logging channel for tenant-specific configuration and lifecycle events.
	ChannelTenant Channel = "tenant"
	// ChannelSSE represents the logging channel for Server-Sent Events (SSE) streaming and connection management.
	ChannelSSE Channel = "sse"

	// ChannelPerf represents performance monitoring and metrics.
	ChannelPerf Channel = "performance"
	// ChannelSlowQuery represents the logging channel specifically for identifying and recording database queries that exceed the performance threshold.
	ChannelSlowQuery Channel = "slow-query"
	// ChannelMemory represents the logging channel for memory usage, allocation, and garbage collection events.
	ChannelMemory Channel = "memory"
	// ChannelAlert represents the logging channel for critical system alerts and notifications.
	ChannelAlert Channel = "alert"

	// ChannelDebug represents development and debugging information.
	ChannelDebug Channel = "debug"
	// ChannelTrace represents the logging channel for high-verbosity execution tracing and spans.
	ChannelTrace Channel = "trace"
)

// LogLevel represents the severity level of log messages
type LogLevel string

// LevelTrace represents the most detailed tracing information level.
const (
	LevelTrace LogLevel = "TRACE"
	LevelDebug LogLevel = "DEBUG"
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
	LevelFatal LogLevel = "FATAL"
)

// ChanneledLogger provides structured logging with multiple channels
type ChanneledLogger struct {
	channels map[Channel]*slog.Logger
	config   *LoggerConfig
	baseDir  string
	configMu sync.RWMutex
}

// LoggerConfig contains configuration options for the channeled logger
type LoggerConfig struct {
	// Output configuration
	OutputToFile    bool   `json:"outputToFile"`    // Whether to write logs to files
	OutputToConsole bool   `json:"outputToConsole"` // Whether to write logs to console
	LogDirectory    string `json:"logDirectory"`    // Directory for log files
	FileRotation    bool   `json:"fileRotation"`    // Whether to rotate log files

	// Formatting configuration
	JSONFormat      bool   `json:"jsonFormat"`      // Use JSON format for structured logging
	IncludeSource   bool   `json:"includeSource"`   // Include source file and line in logs
	TimestampFormat string `json:"timestampFormat"` // Timestamp format for logs

	// Level configuration per channel
	DefaultLevel  slog.Level             `json:"defaultLevel"`  // Default log level
	ChannelLevels map[Channel]slog.Level `json:"channelLevels"` // Per-channel log levels

	// Performance integration
	EnablePerformanceCorrelation bool `json:"enablePerformanceCorrelation"` // Correlate with performance markers
	IncludeMemoryStats           bool `json:"includeMemoryStats"`           // Include memory stats in logs
	IncludeTenantContext         bool `json:"includeTenantContext"`         // Include tenant context in logs
}

// DefaultLoggerConfig returns a sensible default configuration
func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{
		OutputToFile:                 true,
		OutputToConsole:              true,
		LogDirectory:                 "logs",
		FileRotation:                 true,
		JSONFormat:                   true,
		IncludeSource:                true,
		TimestampFormat:              time.RFC3339,
		DefaultLevel:                 slog.LevelInfo,
		ChannelLevels:                make(map[Channel]slog.Level), // Start with empty map to respect DefaultLevel
		EnablePerformanceCorrelation: true,
		IncludeMemoryStats:           false,
		IncludeTenantContext:         true,
	}
}

// NewChanneledLogger creates a new channeled logger with the given configuration
func NewChanneledLogger(config *LoggerConfig) (*ChanneledLogger, error) {
	if config == nil {
		config = DefaultLoggerConfig()
	}

	logger := &ChanneledLogger{
		channels: make(map[Channel]*slog.Logger),
		config:   config,
		baseDir:  config.LogDirectory,
	}

	// Create log directory if file output is enabled
	if config.OutputToFile {
		if err := os.MkdirAll(config.LogDirectory, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	// Initialize all channels
	channels := []Channel{
		ChannelSystem, ChannelStartup, ChannelShutdown,
		ChannelAuth, ChannelContent, ChannelAnalytics, ChannelCache,
		ChannelDatabase, ChannelTenant, ChannelSSE,
		ChannelPerf, ChannelSlowQuery, ChannelMemory, ChannelAlert,
		ChannelDebug, ChannelTrace,
	}

	for _, channel := range channels {
		channelLogger, err := logger.createChannelLogger(channel)
		if err != nil {
			return nil, fmt.Errorf("failed to create logger for channel %s: %w", channel, err)
		}
		logger.channels[channel] = channelLogger
	}

	return logger, nil
}

// createChannelLogger creates a slog.Logger for a specific channel
func (cl *ChanneledLogger) createChannelLogger(channel Channel) (*slog.Logger, error) {
	cl.configMu.RLock()
	defer cl.configMu.RUnlock()

	// Determine log level for this channel - respect DefaultLevel unless explicitly overridden
	level := cl.config.DefaultLevel
	if channelLevel, exists := cl.config.ChannelLevels[channel]; exists {
		level = channelLevel
	}

	var writers []io.Writer

	// Add console output if enabled
	if cl.config.OutputToConsole {
		writers = append(writers, os.Stdout)
	}

	// Add file output if enabled
	if cl.config.OutputToFile {
		filename := fmt.Sprintf("%s.log", string(channel))
		filepath := filepath.Join(cl.config.LogDirectory, filename)

		file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", filepath, err)
		}

		writers = append(writers, file)
	}

	writers = append(writers, NewSSEWriter())

	// Create multi-writer if we have multiple outputs
	var writer io.Writer
	switch len(writers) {
	case 0:
		// Fallback to stdout
		writer = os.Stdout
	case 1:
		writer = writers[0]
	default:
		writer = io.MultiWriter(writers...)
	}

	// Configure handler options
	handlerOpts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cl.config.IncludeSource,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			return a
		},
	}

	// Create handler based on format preference
	var handler slog.Handler
	if cl.config.JSONFormat {
		handler = slog.NewJSONHandler(writer, handlerOpts)
	} else {
		handler = slog.NewTextHandler(writer, handlerOpts)
	}

	// Create logger with the base 'channel' attribute.
	logger := slog.New(handler).With(slog.String("channel", string(channel)))

	return logger, nil
}

// System returns the logger channel for system operations.
func (cl *ChanneledLogger) System() *slog.Logger { return cl.channels[ChannelSystem] }

// Startup returns the logger channel for startup events.
func (cl *ChanneledLogger) Startup() *slog.Logger { return cl.channels[ChannelStartup] }

// Shutdown returns the logger channel for shutdown events.
func (cl *ChanneledLogger) Shutdown() *slog.Logger { return cl.channels[ChannelShutdown] }

// Auth returns the logger channel for authentication events.
func (cl *ChanneledLogger) Auth() *slog.Logger { return cl.channels[ChannelAuth] }

// Content returns the logger channel for content management operations.
func (cl *ChanneledLogger) Content() *slog.Logger { return cl.channels[ChannelContent] }

// Analytics returns the logger channel for analytics processing.
func (cl *ChanneledLogger) Analytics() *slog.Logger { return cl.channels[ChannelAnalytics] }

// Cache returns the logger channel for cache operations.
func (cl *ChanneledLogger) Cache() *slog.Logger { return cl.channels[ChannelCache] }

// Database returns the logger channel for database operations.
func (cl *ChanneledLogger) Database() *slog.Logger { return cl.channels[ChannelDatabase] }

// Tenant returns the logger channel for tenant-specific operations.
func (cl *ChanneledLogger) Tenant() *slog.Logger { return cl.channels[ChannelTenant] }

// SSE returns the logger channel for Server-Sent Events.
func (cl *ChanneledLogger) SSE() *slog.Logger { return cl.channels[ChannelSSE] }

// Perf returns the logger channel for performance metrics.
func (cl *ChanneledLogger) Perf() *slog.Logger { return cl.channels[ChannelPerf] }

// SlowQuery returns the logger channel specifically for slow database queries.
func (cl *ChanneledLogger) SlowQuery() *slog.Logger { return cl.channels[ChannelSlowQuery] }

// Memory returns the logger channel for memory usage stats.
func (cl *ChanneledLogger) Memory() *slog.Logger { return cl.channels[ChannelMemory] }

// Alert returns the logger channel for system alerts.
func (cl *ChanneledLogger) Alert() *slog.Logger { return cl.channels[ChannelAlert] }

// Debug returns the logger channel for debugging information.
func (cl *ChanneledLogger) Debug() *slog.Logger { return cl.channels[ChannelDebug] }

// Trace returns the logger channel for detailed tracing.
func (cl *ChanneledLogger) Trace() *slog.Logger { return cl.channels[ChannelTrace] }

// GetChannel returns a logger for a specific channel
func (cl *ChanneledLogger) GetChannel(channel Channel) *slog.Logger {
	if logger, exists := cl.channels[channel]; exists {
		return logger
	}
	// Fallback to system channel
	return cl.channels[ChannelSystem]
}

// WithTenant returns a logger with tenant context
func (cl *ChanneledLogger) WithTenant(channel Channel, tenantID string) *slog.Logger {
	logger := cl.GetChannel(channel)
	return logger.With(slog.String("tenantId", tenantID))
}

// WithOperation returns a logger with operation context
func (cl *ChanneledLogger) WithOperation(channel Channel, operation string) *slog.Logger {
	logger := cl.GetChannel(channel)
	return logger.With(slog.String("operation", operation))
}

// WithTenantAndOperation returns a logger with both tenant and operation context
func (cl *ChanneledLogger) WithTenantAndOperation(channel Channel, tenantID, operation string) *slog.Logger {
	logger := cl.GetChannel(channel)
	return logger.With(
		slog.String("tenantId", tenantID),
		slog.String("operation", operation),
	)
}

// LogPerformanceMarker logs a performance marker with appropriate context
func (cl *ChanneledLogger) LogPerformanceMarker(marker any) {
	// This would integrate with the performance package
	// For now, we'll use a generic approach
	cl.Perf().Info("Performance marker recorded",
		slog.Any("marker", marker),
		slog.String("timestamp", time.Now().Format(time.RFC3339)),
	)
}

// LogSlowQuery logs a slow database query
func (cl *ChanneledLogger) LogSlowQuery(query string, duration time.Duration, tenantID string) {
	cl.SlowQuery().Warn("Slow query detected",
		slog.String("query", cl.sanitizeQuery(query)),
		slog.Duration("duration", duration),
		slog.String("tenantId", tenantID),
		slog.String("timestamp", time.Now().Format(time.RFC3339)),
	)
}

// LogCacheOperation logs cache operations with performance context
func (cl *ChanneledLogger) LogCacheOperation(operation, key string, hit bool, duration time.Duration, tenantID string) {
	logger := cl.Cache().With(
		slog.String("operation", operation),
		slog.String("key", key),
		slog.Bool("hit", hit),
		slog.Duration("duration", duration),
		slog.String("tenantId", tenantID),
	)

	if hit {
		logger.Debug("Cache hit")
	} else {
		logger.Debug("Cache miss")
	}
}

// LogAuthOperation logs authentication operations with security context
func (cl *ChanneledLogger) LogAuthOperation(operation, tenantID, userID string, success bool, metadata map[string]any) {
	logger := cl.Auth().With(
		slog.String("operation", operation),
		slog.String("tenantId", tenantID),
		slog.String("userId", cl.sanitizeUserID(userID)),
		slog.Bool("success", success),
	)

	// Add metadata if provided
	for key, value := range metadata {
		logger = logger.With(slog.Any(key, value))
	}

	if success {
		logger.Info("Authentication operation completed")
	} else {
		logger.Warn("Authentication operation failed")
	}
}

// LogError logs an error with appropriate context and channel
func (cl *ChanneledLogger) LogError(channel Channel, operation string, err error, tenantID string, metadata map[string]any) {
	logger := cl.GetChannel(channel).With(
		slog.String("operation", operation),
		slog.String("tenantId", tenantID),
		slog.String("error", err.Error()),
	)

	// Add metadata if provided
	for key, value := range metadata {
		logger = logger.With(slog.Any(key, value))
	}

	logger.Error("Operation failed")
}

// LogTenantOperation logs tenant-specific operations
func (cl *ChanneledLogger) LogTenantOperation(operation, tenantID string, success bool, duration time.Duration, metadata map[string]any) {
	logger := cl.Tenant().With(
		slog.String("operation", operation),
		slog.String("tenantId", tenantID),
		slog.Bool("success", success),
		slog.Duration("duration", duration),
	)

	// Add metadata if provided
	for key, value := range metadata {
		logger = logger.With(slog.Any(key, value))
	}

	if success {
		logger.Info("Tenant operation completed")
	} else {
		logger.Error("Tenant operation failed")
	}
}

// LogStartupPhase logs application startup phases
func (cl *ChanneledLogger) LogStartupPhase(phase string, duration time.Duration, success bool, metadata map[string]any) {
	logger := cl.Startup().With(
		slog.String("phase", phase),
		slog.Duration("duration", duration),
		slog.Bool("success", success),
	)

	// Add metadata if provided
	for key, value := range metadata {
		logger = logger.With(slog.Any(key, value))
	}

	if success {
		logger.Info("Startup phase completed")
	} else {
		logger.Error("Startup phase failed")
	}
}

// LogSSEEvent logs server-sent events operations
func (cl *ChanneledLogger) LogSSEEvent(event, tenantID, sessionID string, clientCount int) {
	cl.SSE().Info("SSE event broadcasted",
		slog.String("event", event),
		slog.String("tenantId", tenantID),
		slog.String("sessionId", cl.sanitizeSessionID(sessionID)),
		slog.Int("clientCount", clientCount),
	)
}

// sanitizeQuery removes sensitive information from SQL queries for logging
func (cl *ChanneledLogger) sanitizeQuery(query string) string {
	// Remove potential sensitive data from queries
	// This is a simple implementation - in production you might want more sophisticated sanitization
	query = strings.ReplaceAll(query, "\n", " ")
	query = strings.ReplaceAll(query, "\t", " ")

	// Truncate very long queries
	if len(query) > 500 {
		query = query[:500] + "..."
	}

	return query
}

// sanitizeUserID partially masks user IDs for privacy
func (cl *ChanneledLogger) sanitizeUserID(userID string) string {
	if len(userID) <= 4 {
		return "****"
	}
	return userID[:2] + "****" + userID[len(userID)-2:]
}

// sanitizeSessionID partially masks session IDs for privacy
func (cl *ChanneledLogger) sanitizeSessionID(sessionID string) string {
	if len(sessionID) <= 8 {
		return "********"
	}
	return sessionID[:4] + "****" + sessionID[len(sessionID)-4:]
}

// Close closes all file handles and cleans up resources
func (cl *ChanneledLogger) Close() error {
	// In a more sophisticated implementation, you would close file handles here
	// For now, we'll just log that the logger is being closed
	cl.System().Info("Channeled logger shutting down")
	return nil
}

// GetConfig returns the current logger configuration
func (cl *ChanneledLogger) GetConfig() *LoggerConfig {
	return cl.config
}

// SetChannelLevel dynamically sets the log level for a specific channel
func (cl *ChanneledLogger) SetChannelLevel(channel Channel, level slog.Level) error {
	cl.configMu.Lock()
	defer cl.configMu.Unlock()

	if _, exists := cl.channels[channel]; !exists {
		return fmt.Errorf("channel %s does not exist", channel)
	}

	// Update the configuration map
	cl.config.ChannelLevels[channel] = level

	// Recreate the specific logger for this channel with the new level
	newLogger, err := cl.createChannelLogger(channel)
	if err != nil {
		// Log the error but don't halt the application
		cl.System().Error("Failed to recreate logger for channel on level change", "channel", channel, "error", err)
		return fmt.Errorf("failed to recreate logger for channel %s: %w", channel, err)
	}

	// Atomically replace the old logger with the new one
	cl.channels[channel] = newLogger

	cl.System().Info("Channel log level updated dynamically",
		slog.String("channel", string(channel)),
		slog.String("level", level.String()),
	)

	return nil
}

// GetChannelLevels returns the current log levels for all channels.
func (cl *ChanneledLogger) GetChannelLevels() map[string]string {
	cl.configMu.RLock()
	defer cl.configMu.RUnlock()

	levels := make(map[string]string)
	for channel := range cl.channels {
		if level, ok := cl.config.ChannelLevels[channel]; ok {
			levels[string(channel)] = level.String()
		} else {
			levels[string(channel)] = cl.config.DefaultLevel.String()
		}
	}
	return levels
}

// GetChannelStats returns statistics about log messages per channel
func (cl *ChanneledLogger) GetChannelStats() map[string]any {
	// In a production implementation, you would track message counts, sizes, etc.
	// For now, return basic information
	stats := make(map[string]any)

	for channel := range cl.channels {
		stats[string(channel)] = map[string]any{
			"level":  cl.config.ChannelLevels[channel].String(),
			"active": true,
		}
	}

	return stats
}
