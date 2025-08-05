// Package messaging provides the concrete implementation of the SSE broadcaster.
package messaging

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
)

// SSEBroadcaster manages tenant-scoped, session-specific SSE connections.
type SSEBroadcaster struct {
	tenantSessions map[string]map[string][]chan string // tenantId -> sessionId -> []channels
	mu             sync.Mutex
	logger         *logging.ChanneledLogger
}

var (
	globalBroadcaster *SSEBroadcaster
	once              sync.Once
)

// NewSSEBroadcaster creates the singleton SSEBroadcaster instance.
func NewSSEBroadcaster(logger *logging.ChanneledLogger) *SSEBroadcaster {
	once.Do(func() {
		globalBroadcaster = &SSEBroadcaster{
			tenantSessions: make(map[string]map[string][]chan string),
			logger:         logger,
		}
	})
	return globalBroadcaster
}

// AddClientWithSession registers a new SSE client with tenant and session isolation.
func (b *SSEBroadcaster) AddClientWithSession(tenantID, sessionID string) chan string {
	ch := make(chan string, 10)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.tenantSessions[tenantID] == nil {
		b.tenantSessions[tenantID] = make(map[string][]chan string)
	}

	if b.tenantSessions[tenantID][sessionID] == nil {
		b.tenantSessions[tenantID][sessionID] = make([]chan string, 0)
	}
	b.tenantSessions[tenantID][sessionID] = append(b.tenantSessions[tenantID][sessionID], ch)

	b.logger.SSE().Debug("SSE client registered", "tenantId", tenantID, "sessionId", sessionID)
	return ch
}

// RemoveClientWithSession removes an SSE client with tenant and session context.
func (b *SSEBroadcaster) RemoveClientWithSession(ch chan string, tenantID, sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		if sessionClients, exists := tenantSessions[sessionID]; exists {
			newClients := make([]chan string, 0, len(sessionClients)-1)
			for _, client := range sessionClients {
				if client != ch {
					newClients = append(newClients, client)
				}
			}
			tenantSessions[sessionID] = newClients

			if len(tenantSessions[sessionID]) == 0 {
				delete(tenantSessions, sessionID)
			}
		}

		if len(tenantSessions) == 0 {
			delete(b.tenantSessions, tenantID)
		}
	}
	b.logger.SSE().Debug("SSE client unregistered", "tenantId", tenantID, "sessionId", sessionID)
}

// GetSessionConnectionCount returns the connection count for a specific tenant session.
func (b *SSEBroadcaster) GetSessionConnectionCount(tenantID, sessionID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		if sessionClients, exists := tenantSessions[sessionID]; exists {
			return len(sessionClients)
		}
	}
	return 0
}

// BroadcastToSpecificSession sends updates to a specific session within a tenant.
func (b *SSEBroadcaster) BroadcastToSpecificSession(tenantID, sessionID, storyfragmentID string, paneIDs []string, scrollTarget *string) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.SSE().Error("Panic recovered in BroadcastToSpecificSession", "error", r, "tenantId", tenantID, "sessionId", sessionID)
		}
	}()

	paneIDsJSON, _ := json.Marshal(paneIDs)
	var message string
	if scrollTarget != nil && *scrollTarget != "" {
		message = fmt.Sprintf("event: panes_updated\ndata: {\"storyfragmentId\":\"%s\",\"affectedPanes\":%s,\"gotoPaneId\":\"%s\"}\n\n",
			storyfragmentID, paneIDsJSON, *scrollTarget)
	} else {
		message = fmt.Sprintf("event: panes_updated\ndata: {\"storyfragmentId\":\"%s\",\"affectedPanes\":%s}\n\n",
			storyfragmentID, paneIDsJSON)
	}

	b.logger.SSE().Debug("Broadcasting to session", "message", strings.ReplaceAll(message, "\n", "\\n"), "tenantId", tenantID, "sessionId", sessionID)

	b.mu.Lock()
	defer b.mu.Unlock()

	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		if sessionClients, exists := tenantSessions[sessionID]; exists {
			for _, ch := range sessionClients {
				select {
				case ch <- message:
				default:
					b.logger.SSE().Warn("SSE channel full, message dropped", "tenantId", tenantID, "sessionId", sessionID)
				}
			}
		}
	}
}

// HasViewingSessions checks if any sessions are viewing a specific storyfragment.
func (b *SSEBroadcaster) HasViewingSessions(tenantID, storyfragmentID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// This is a simplified check. A more robust implementation would track the
	// active storyfragment per session. For now, we check if any session for the tenant exists.
	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		return len(tenantSessions) > 0
	}
	return false
}
