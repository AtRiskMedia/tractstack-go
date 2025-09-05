// Package logging provides structured logging channels for TractStack operations
// with multi-tenant support and performance correlation capabilities.
package logging

import (
	"context"
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
	// System channels
	ChannelSystem   Channel = "system"   // General system operations
	ChannelStartup  Channel = "startup"  // Application startup and initialization
	ChannelShutdown Channel = "shutdown" // Application shutdown and cleanup

	// Business logic channels
	ChannelAuth      Channel = "auth"      // Authentication and authorization
	ChannelContent   Channel = "content"   // Content management operations
	ChannelAnalytics Channel = "analytics" // Analytics processing and queries
	ChannelCache     Channel = "cache"     // Cache operations and management

	// Infrastructure channels
	ChannelDatabase Channel = "database" // Database operations and queries
	ChannelTenant   Channel = "tenant"   // Multi-tenant operations
	ChannelSSE      Channel = "sse"      // Server-sent events and real-time

	// Performance and monitoring channels
	ChannelPerf      Channel = "performance" // Performance monitoring and metrics
	ChannelSlowQuery Channel = "slow-query"  // Slow database queries
	ChannelMemory    Channel = "memory"      // Memory usage and garbage collection
	ChannelAlert     Channel = "alert"       // Performance alerts and warnings

	// Development and debugging channels
	ChannelDebug Channel = "debug" // Debug information
	ChannelTrace Channel = "trace" // Detailed tracing information
)

// LogLevel represents the severity level of log messages
type LogLevel string

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
		if err := os.MkdirAll(config.LogDirectory, 0755); err != nil {
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

		file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", filepath, err)
		}

		writers = append(writers, file)
	}

	// Add our custom SSE writer to the list of outputs.
	// Now, every log message will also be sent to the broadcaster.
	writers = append(writers, NewSSEWriter())

	// Create multi-writer if we have multiple outputs
	var writer io.Writer
	if len(writers) == 1 {
		writer = writers[0]
	} else if len(writers) > 1 {
		writer = io.MultiWriter(writers...)
	} else {
		// Fallback to stdout
		writer = os.Stdout
	}

	// Configure handler options
	handlerOpts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cl.config.IncludeSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
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

func (cl *ChanneledLogger) System() *slog.Logger    { return cl.channels[ChannelSystem] }
func (cl *ChanneledLogger) Startup() *slog.Logger   { return cl.channels[ChannelStartup] }
func (cl *ChanneledLogger) Shutdown() *slog.Logger  { return cl.channels[ChannelShutdown] }
func (cl *ChanneledLogger) Auth() *slog.Logger      { return cl.channels[ChannelAuth] }
func (cl *ChanneledLogger) Content() *slog.Logger   { return cl.channels[ChannelContent] }
func (cl *ChanneledLogger) Analytics() *slog.Logger { return cl.channels[ChannelAnalytics] }
func (cl *ChanneledLogger) Cache() *slog.Logger     { return cl.channels[ChannelCache] }
func (cl *ChanneledLogger) Database() *slog.Logger  { return cl.channels[ChannelDatabase] }
func (cl *ChanneledLogger) Tenant() *slog.Logger    { return cl.channels[ChannelTenant] }
func (cl *ChanneledLogger) SSE() *slog.Logger       { return cl.channels[ChannelSSE] }
func (cl *ChanneledLogger) Perf() *slog.Logger      { return cl.channels[ChannelPerf] }
func (cl *ChanneledLogger) SlowQuery() *slog.Logger { return cl.channels[ChannelSlowQuery] }
func (cl *ChanneledLogger) Memory() *slog.Logger    { return cl.channels[ChannelMemory] }
func (cl *ChanneledLogger) Alert() *slog.Logger     { return cl.channels[ChannelAlert] }
func (cl *ChanneledLogger) Debug() *slog.Logger     { return cl.channels[ChannelDebug] }
func (cl *ChanneledLogger) Trace() *slog.Logger     { return cl.channels[ChannelTrace] }

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

// WithContext returns a logger with context from the provided context.Context
func (cl *ChanneledLogger) WithContext(channel Channel, ctx context.Context) *slog.Logger {
	logger := cl.GetChannel(channel)

	// Extract common context values
	if tenantID := ctx.Value("tenantId"); tenantID != nil {
		if tenantStr, ok := tenantID.(string); ok {
			logger = logger.With(slog.String("tenantId", tenantStr))
		}
	}

	if operation := ctx.Value("operation"); operation != nil {
		if opStr, ok := operation.(string); ok {
			logger = logger.With(slog.String("operation", opStr))
		}
	}

	if requestID := ctx.Value("requestId"); requestID != nil {
		if reqStr, ok := requestID.(string); ok {
			logger = logger.With(slog.String("requestId", reqStr))
		}
	}

	return logger
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
