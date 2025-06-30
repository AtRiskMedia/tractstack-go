// Package models defines data structures for the application's core entities and API payloads.
package models

import (
	"fmt"
	"sync"
	"time"
)

type Profile struct {
	Fingerprint    string
	LeadID         string
	Firstname      string
	Email          string
	ContactPersona string
	ShortBio       string
}

type VisitRequest struct {
	SessionID      *string `json:"sessionId,omitempty"`
	EncryptedEmail *string `json:"encryptedEmail,omitempty"`
	EncryptedCode  *string `json:"encryptedCode,omitempty"`
	Consent        *string `json:"consent,omitempty"`
}

type SessionData struct {
	SessionID     string    `json:"sessionId"`
	FingerprintID string    `json:"fingerprintId"`
	VisitID       string    `json:"visitId"`
	LeadID        *string   `json:"leadId,omitempty"`
	LastActivity  time.Time `json:"lastActivity"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (s *SessionData) IsExpired() bool {
	return time.Since(s.LastActivity) > 2*time.Hour
}

func (s *SessionData) UpdateActivity() {
	s.LastActivity = time.Now()
}

type Event struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Verb   string `json:"verb"`
	Object string `json:"object"`
}

type LoginRequest struct {
	Password string `json:"password"`
	TenantID string `json:"tenantId"`
}

type SSEBroadcaster struct {
	clients        []chan string
	sessionClients map[string][]chan string // sessionId -> []channels
	mu             sync.Mutex
}

var Broadcaster = &SSEBroadcaster{
	sessionClients: make(map[string][]chan string),
}

type VisitState struct {
	VisitID       string    `json:"visitId"`
	FingerprintID string    `json:"fingerprintId"`
	StartTime     time.Time `json:"startTime"`
	LastActivity  time.Time `json:"lastActivity"`
	CurrentPage   string    `json:"currentPage"`
}

func (v *VisitState) IsVisitActive() bool {
	if v == nil {
		return false
	}
	return time.Since(v.StartTime) <= 2*time.Hour
}

func (v *VisitState) UpdateActivity() {
	if v != nil {
		v.LastActivity = time.Now()
	}
}

type FingerprintState struct {
	FingerprintID string                 `json:"fingerprintId"`
	HeldBeliefs   map[string]BeliefValue `json:"heldBeliefs"`
	HeldBadges    map[string]string      `json:"heldBadges"`
	LastActivity  time.Time              `json:"lastActivity"`
}

func (f *FingerprintState) UpdateActivity() {
	if f != nil {
		f.LastActivity = time.Now()
	}
}

type BeliefValue struct {
	Verb   string  `json:"verb"`   // BELIEVES_YES, BELIEVES_NO, IDENTIFY_AS
	Object *string `json:"object"` // only used when verb=IDENTIFY_AS
}

// Lead represents a user profile in the database
type Lead struct {
	ID             string    `json:"id"`
	FirstName      string    `json:"firstName"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"` // Never serialize password hash
	ContactPersona string    `json:"contactPersona"`
	ShortBio       string    `json:"shortBio"`
	EncryptedCode  string    `json:"-"` // Never serialize encrypted code
	EncryptedEmail string    `json:"-"` // Never serialize encrypted email
	CreatedAt      time.Time `json:"createdAt"`
	Changed        time.Time `json:"changed"`
}

func (b *SSEBroadcaster) AddClient() chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, 10)
	b.clients = append(b.clients, ch)
	return ch
}

func (b *SSEBroadcaster) RemoveClient(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, client := range b.clients {
		if client == ch {
			b.clients = append(b.clients[:i], b.clients[i+1:]...)
			close(ch)
			break
		}
	}
}

func (b *SSEBroadcaster) Broadcast(eventType, data string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	message := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)

	// Defensive broadcasting - collect dead channels to remove
	var deadChannels []int
	for i, ch := range b.clients {
		select {
		case ch <- message:
			// Success - channel is alive
		default:
			// Channel is blocked/closed - mark for removal
			deadChannels = append(deadChannels, i)
		}
	}

	// Remove dead channels (in reverse order to maintain indices)
	for i := len(deadChannels) - 1; i >= 0; i-- {
		idx := deadChannels[i]
		// Only close if channel is not already closed
		select {
		case <-b.clients[idx]:
		default:
			close(b.clients[idx])
		}
		b.clients = append(b.clients[:idx], b.clients[idx+1:]...)
	}
}

func (b *SSEBroadcaster) AddClientWithSession(sessionID string) chan string {
	ch := make(chan string, 10)
	b.mu.Lock()
	b.clients = append(b.clients, ch)

	// Track by session
	if b.sessionClients[sessionID] == nil {
		b.sessionClients[sessionID] = make([]chan string, 0)
	}
	b.sessionClients[sessionID] = append(b.sessionClients[sessionID], ch)
	b.mu.Unlock()
	return ch
}

func (b *SSEBroadcaster) RemoveClientWithSession(ch chan string, sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Remove from main clients list
	for i, client := range b.clients {
		if client == ch {
			b.clients = append(b.clients[:i], b.clients[i+1:]...)
			break
		}
	}

	// Remove from session clients list
	if sessionClients, exists := b.sessionClients[sessionID]; exists {
		for i, client := range sessionClients {
			if client == ch {
				b.sessionClients[sessionID] = append(sessionClients[:i], sessionClients[i+1:]...)
				break
			}
		}
		// Clean up empty session entries
		if len(b.sessionClients[sessionID]) == 0 {
			delete(b.sessionClients, sessionID)
		}
	}
}

func (b *SSEBroadcaster) GetSessionConnectionCount(sessionID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	if sessionClients, exists := b.sessionClients[sessionID]; exists {
		return len(sessionClients)
	}
	return 0
}

func (b *SSEBroadcaster) GetActiveConnectionCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.clients)
}

func (b *SSEBroadcaster) CleanupDeadChannels() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	cleanedCount := 0

	// Clean up main clients list
	activeClients := make([]chan string, 0, len(b.clients))
	for _, client := range b.clients {
		select {
		case <-client:
			// Channel is closed, don't keep it
			cleanedCount++
		default:
			// Channel is still active
			activeClients = append(activeClients, client)
		}
	}
	b.clients = activeClients

	// Clean up session clients
	for sessionID, sessionClients := range b.sessionClients {
		activeSessionClients := make([]chan string, 0, len(sessionClients))
		for _, client := range sessionClients {
			select {
			case <-client:
				// Channel is closed, don't keep it
			default:
				// Channel is still active
				activeSessionClients = append(activeSessionClients, client)
			}
		}

		if len(activeSessionClients) == 0 {
			delete(b.sessionClients, sessionID)
		} else {
			b.sessionClients[sessionID] = activeSessionClients
		}
	}

	return cleanedCount
}
