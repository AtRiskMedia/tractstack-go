package models

import (
	"fmt"
	"sync"
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
	EncryptedEmail *string `json:"encryptedEmail,omitempty"`
	EncryptedCode  *string `json:"encryptedCode,omitempty"`
	Fingerprint    *string `json:"fingerprint,omitempty"`
	VisitID        *string `json:"visitId,omitempty"`
	Consent        *string `json:"consent,omitempty"`
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
