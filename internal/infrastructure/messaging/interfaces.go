// Package messaging defines interfaces for real-time communication.
package messaging

// Broadcaster defines the interface for managing SSE client connections and broadcasting messages.
type Broadcaster interface {
	AddClientWithSession(tenantID, sessionID string) chan string
	RemoveClientWithSession(ch chan string, tenantID, sessionID string)
	GetSessionConnectionCount(tenantID, sessionID string) int
	BroadcastToSpecificSession(tenantID, sessionID, storyfragmentID string, paneIDs []string, scrollTarget *string)
	HasViewingSessions(tenantID, storyfragmentID string) bool
}
