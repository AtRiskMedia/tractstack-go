// Package logging provides the custom io.Writer for SSE log streaming.
package logging

import (
	"encoding/json"
	"log/slog"
	"time"
)

// SSEWriter is a custom io.Writer that intercepts log messages
// and forwards them to the LogBroadcaster.
type SSEWriter struct {
	broadcaster *LogBroadcaster
}

// NewSSEWriter creates a new writer that sends log data to the broadcaster.
func NewSSEWriter() *SSEWriter {
	return &SSEWriter{
		broadcaster: GetBroadcaster(), // Get the singleton broadcaster instance.
	}
}

// Write is the method that satisfies the io.Writer interface.
// It receives log messages (as JSON bytes), parses them, and submits them
// to the broadcaster for distribution.
func (w *SSEWriter) Write(p []byte) (n int, err error) {
	var rawLog map[string]any
	if err := json.Unmarshal(p, &rawLog); err != nil {
		// This can happen if a non-JSON log is somehow written.
		// We'll log it as a simple message.
		go w.broadcaster.SubmitLog(LogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Level:     slog.LevelError.String(),
			Channel:   string(ChannelSystem),
			Message:   "sse_writer: failed to parse incoming log message",
		})
		return len(p), nil
	}

	// Extract fields from the structured log to create a clean LogEntry.
	// This ensures we only send the necessary data to the frontend.
	entry := LogEntry{
		Timestamp: w.getString(rawLog, "time"),
		Level:     w.getString(rawLog, "level"),
		Channel:   w.getString(rawLog, "channel"),
		Message:   w.getString(rawLog, "msg"),
		TenantID:  w.getString(rawLog, "tenantId"),
	}

	// Submit the structured log entry to the broadcaster.
	// We run this in a goroutine to avoid blocking the logging call.
	go w.broadcaster.SubmitLog(entry)

	return len(p), nil
}

// getString is a helper to safely extract a string value from the log map.
func (w *SSEWriter) getString(data map[string]any, key string) string {
	if val, ok := data[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return ""
}
