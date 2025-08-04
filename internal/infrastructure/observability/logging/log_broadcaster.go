// Package logging provides the log broadcaster for real-time log streaming.
package logging

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// LogEntry represents a single log entry to be sent to the client.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Channel   string `json:"channel"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	TenantID  string `json:"tenantId,omitempty"`
}

// Client represents a single connected client (a browser tab) listening for logs.
type Client struct {
	id      string         // Unique ID for the client connection.
	Channel chan []byte    // Channel to send log messages to this client.
	filters AppliedFilters // Filters for channel and level.
}

// AppliedFilters defines the filtering criteria for a client.
type AppliedFilters struct {
	Channel Channel    // e.g., "database", "auth"
	Level   slog.Level // e.g., slog.LevelInfo
}

// LogBroadcaster manages clients and broadcasts log messages.
type LogBroadcaster struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	mu         sync.RWMutex
	logger     *slog.Logger // Use a standard slog logger for broadcaster's internal logging.
	stop       chan struct{}
}

var (
	broadcaster *LogBroadcaster
	once        sync.Once
)

// GetBroadcaster initializes and returns the singleton LogBroadcaster instance.
func GetBroadcaster() *LogBroadcaster {
	once.Do(func() {
		broadcaster = &LogBroadcaster{
			clients:    make(map[*Client]bool),
			register:   make(chan *Client),
			unregister: make(chan *Client),
			broadcast:  make(chan []byte, 1000), // Buffered channel for logs.
			logger:     slog.Default().With("component", "LogBroadcaster"),
			stop:       make(chan struct{}),
		}
		go broadcaster.run()
	})
	return broadcaster
}

// run is the central loop that manages the broadcaster's state and operations.
func (b *LogBroadcaster) run() {
	b.logger.Info("Log broadcaster is running.")
	for {
		select {
		case <-b.stop:
			b.logger.Info("Log broadcaster is shutting down.")
			return
		case client := <-b.register:
			b.mu.Lock()
			b.clients[client] = true
			b.logger.Info("Client registered", "id", client.id, "filters", client.filters)
			b.mu.Unlock()
		case client := <-b.unregister:
			b.mu.Lock()
			if _, ok := b.clients[client]; ok {
				delete(b.clients, client)
				close(client.Channel)
				b.logger.Info("Client unregistered", "id", client.id)
			}
			b.mu.Unlock()
		case message := <-b.broadcast:
			b.distribute(message)
		}
	}
}

// distribute sends a log message to all clients whose filters match.
func (b *LogBroadcaster) distribute(message []byte) {
	var entry LogEntry
	if err := json.Unmarshal(message, &entry); err != nil {
		b.logger.Error("Failed to unmarshal log entry for distribution", "error", err)
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for client := range b.clients {
		// Check if the log entry matches the client's filters.
		// "all" channel matches every log.
		channelMatch := client.filters.Channel == "all" || client.filters.Channel == Channel(entry.Channel)
		levelMatch := entry.Level >= client.filters.Level.String()

		if channelMatch && levelMatch {
			select {
			case client.Channel <- message:
			default:
				// If the client's channel is full, they are likely disconnected or slow.
				// We can choose to drop the message or unregister the client.
				// For simplicity, we'll just drop it.
			}
		}
	}
}

// SubmitLog is the public method used by the logger to send a log entry to the broadcaster.
func (b *LogBroadcaster) SubmitLog(entry LogEntry) {
	message, err := json.Marshal(entry)
	if err != nil {
		b.logger.Error("Failed to marshal log entry for broadcast", "error", err)
		return
	}

	// Send to the broadcast channel without blocking.
	select {
	case b.broadcast <- message:
	default:
		// If the broadcast channel is full, this means the system is under very high logging load.
		// We drop the log to prevent blocking the application.
		fmt.Println("Log broadcaster channel full. Log message dropped.")
	}
}

// NewClient creates a new client for the broadcaster.
func (b *LogBroadcaster) NewClient(filters AppliedFilters) *Client {
	return &Client{
		id:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Channel: make(chan []byte, 100), // Buffer for each client.
		filters: filters,
	}
}

// Shutdown gracefully stops the broadcaster.
func (b *LogBroadcaster) Shutdown() {
	close(b.stop)
}

// RegisterClient is the public method for adding a new client.
func (b *LogBroadcaster) RegisterClient(client *Client) {
	b.register <- client
}

// UnregisterClient is the public method for removing a client.
func (b *LogBroadcaster) UnregisterClient(client *Client) {
	b.unregister <- client
}
