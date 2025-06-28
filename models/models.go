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
	HasConsent    bool      `json:"hasConsent"` // NEW: Track user consent for extended sessions
	LastActivity  time.Time `json:"lastActivity"`
	CreatedAt     time.Time `json:"createdAt"`
}

// IsExpired checks if session has exceeded TTL based on consent level
func (s *SessionData) IsExpired() bool {
	if s.HasConsent {
		// Extended TTL for users who have given consent (30 days)
		return time.Since(s.LastActivity) > 30*24*time.Hour
	}
	// Standard TTL for non-consented users (2 hours)
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
	clients []chan string
	mu      sync.Mutex
}

func (b *SSEBroadcaster) AddClient() chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string)
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
	for _, ch := range b.clients {
		ch <- message
	}
}

var Broadcaster = &SSEBroadcaster{}

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

func (f *FingerprintState) UpdateActivity() {
	if f != nil {
		f.LastActivity = time.Now()
	}
}
