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

// =============================================================================
// STAGE 3: Multi-Tenant Storyfragment-Scoped SSE Broadcasting Architecture
// =============================================================================

// SubscriptionRegistry tracks storyfragment subscriptions within a tenant
type SubscriptionRegistry struct {
	SessionToStoryfragment  map[string]string   // sessionId -> storyfragmentId
	StoryfragmentToSessions map[string][]string // storyfragmentId -> []sessionId
	mu                      sync.RWMutex
}

// NewSubscriptionRegistry creates a new subscription registry
func NewSubscriptionRegistry() SubscriptionRegistry {
	return SubscriptionRegistry{
		SessionToStoryfragment:  make(map[string]string),
		StoryfragmentToSessions: make(map[string][]string),
	}
}

// SSEBroadcaster provides tenant-isolated, storyfragment-scoped broadcasting
type SSEBroadcaster struct {
	tenantSessions map[string]map[string][]chan string // tenantId -> sessionId -> []channels
	tenantRegistry map[string]SubscriptionRegistry     // tenantId -> registry
	mu             sync.Mutex
}

var Broadcaster = &SSEBroadcaster{
	tenantSessions: make(map[string]map[string][]chan string),
	tenantRegistry: make(map[string]SubscriptionRegistry),
}

// AddClientWithSession registers a new SSE client with tenant and session isolation
func (b *SSEBroadcaster) AddClientWithSession(tenantID, sessionID string) chan string {
	ch := make(chan string, 10)

	b.mu.Lock()
	defer b.mu.Unlock()

	// Initialize tenant sessions if not exists
	if b.tenantSessions[tenantID] == nil {
		b.tenantSessions[tenantID] = make(map[string][]chan string)
	}

	// Initialize tenant registry if not exists
	if _, exists := b.tenantRegistry[tenantID]; !exists {
		b.tenantRegistry[tenantID] = NewSubscriptionRegistry()
	}

	// Add channel to tenant-specific session
	if b.tenantSessions[tenantID][sessionID] == nil {
		b.tenantSessions[tenantID][sessionID] = make([]chan string, 0)
	}
	b.tenantSessions[tenantID][sessionID] = append(b.tenantSessions[tenantID][sessionID], ch)

	return ch
}

// RemoveClientWithSession removes SSE client with tenant and session context
func (b *SSEBroadcaster) RemoveClientWithSession(ch chan string, tenantID, sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Remove from tenant session clients
	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		if sessionClients, exists := tenantSessions[sessionID]; exists {
			for i, client := range sessionClients {
				if client == ch {
					// Remove channel from slice
					tenantSessions[sessionID] = append(sessionClients[:i], sessionClients[i+1:]...)
					break
				}
			}

			// Clean up empty session
			if len(tenantSessions[sessionID]) == 0 {
				delete(tenantSessions, sessionID)
			}
		}

		// Clean up empty tenant
		if len(tenantSessions) == 0 {
			delete(b.tenantSessions, tenantID)
		}
	}
}

// GetSessionConnectionCount returns connection count for specific tenant session
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

// GetActiveConnectionCount returns total connection count for tenant
func (b *SSEBroadcaster) GetActiveConnectionCount(tenantID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	count := 0
	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		for _, sessionClients := range tenantSessions {
			count += len(sessionClients)
		}
	}
	return count
}

// RegisterStoryfragmentSubscription registers session's interest in a storyfragment within tenant
func (b *SSEBroadcaster) RegisterStoryfragmentSubscription(tenantID, sessionID, storyfragmentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Ensure tenant registry exists
	if _, exists := b.tenantRegistry[tenantID]; !exists {
		b.tenantRegistry[tenantID] = NewSubscriptionRegistry()
	}

	registry := b.tenantRegistry[tenantID]
	registry.mu.Lock()
	defer registry.mu.Unlock()

	// Update session -> storyfragment mapping
	registry.SessionToStoryfragment[sessionID] = storyfragmentID

	// Update storyfragment -> sessions mapping
	if registry.StoryfragmentToSessions[storyfragmentID] == nil {
		registry.StoryfragmentToSessions[storyfragmentID] = make([]string, 0)
	}

	// Add session if not already present
	sessions := registry.StoryfragmentToSessions[storyfragmentID]
	found := false
	for _, existingSession := range sessions {
		if existingSession == sessionID {
			found = true
			break
		}
	}

	if !found {
		registry.StoryfragmentToSessions[storyfragmentID] = append(sessions, sessionID)
	}

	// Update registry back to map
	b.tenantRegistry[tenantID] = registry
}

// UnregisterStoryfragmentSubscription removes session's storyfragment interest within tenant
func (b *SSEBroadcaster) UnregisterStoryfragmentSubscription(tenantID, sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if registry, exists := b.tenantRegistry[tenantID]; exists {
		registry.mu.Lock()
		defer registry.mu.Unlock()

		// Get current storyfragment for session
		if storyfragmentID, exists := registry.SessionToStoryfragment[sessionID]; exists {
			// Remove from session -> storyfragment mapping
			delete(registry.SessionToStoryfragment, sessionID)

			// Remove from storyfragment -> sessions mapping
			if sessions, exists := registry.StoryfragmentToSessions[storyfragmentID]; exists {
				for i, existingSession := range sessions {
					if existingSession == sessionID {
						registry.StoryfragmentToSessions[storyfragmentID] = append(sessions[:i], sessions[i+1:]...)
						break
					}
				}

				// Clean up empty storyfragment
				if len(registry.StoryfragmentToSessions[storyfragmentID]) == 0 {
					delete(registry.StoryfragmentToSessions, storyfragmentID)
				}
			}
		}

		// Update registry back to map
		b.tenantRegistry[tenantID] = registry
	}
}

// BroadcastToAffectedPanes sends targeted updates to sessions viewing specific storyfragment within tenant
func (b *SSEBroadcaster) BroadcastToAffectedPanes(tenantID, storyfragmentID string, paneIDs []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Get sessions viewing this storyfragment within tenant
	var targetSessions []string
	if registry, exists := b.tenantRegistry[tenantID]; exists {
		registry.mu.RLock()
		if sessions, exists := registry.StoryfragmentToSessions[storyfragmentID]; exists {
			targetSessions = append(targetSessions, sessions...)
		}
		registry.mu.RUnlock()
	}

	if len(targetSessions) == 0 {
		return // No sessions viewing this storyfragment
	}

	// Prepare broadcast message
	message := fmt.Sprintf("event: panes_updated\ndata: {\"storyfragmentId\":\"%s\",\"affectedPanes\":%v}\n\n",
		storyfragmentID, paneIDs)

	// Send to all sessions viewing this storyfragment within tenant
	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		var deadChannels []chan string

		for _, sessionID := range targetSessions {
			if sessionClients, exists := tenantSessions[sessionID]; exists {
				for _, ch := range sessionClients {
					select {
					case ch <- message:
						// Success - channel is alive
					default:
						// Channel is blocked/closed - mark for removal
						deadChannels = append(deadChannels, ch)
					}
				}
			}
		}

		// Clean up dead channels
		b.cleanupDeadChannels(tenantID, deadChannels)
	}
}

// CleanupDeadChannels removes and closes dead channels within tenant context
func (b *SSEBroadcaster) CleanupDeadChannels(tenantID string, deadChannels []chan string) {
	b.cleanupDeadChannels(tenantID, deadChannels)
}

// cleanupDeadChannels removes and closes dead channels within tenant context
func (b *SSEBroadcaster) cleanupDeadChannels(tenantID string, deadChannels []chan string) {
	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		for sessionID, sessionClients := range tenantSessions {
			filteredClients := make([]chan string, 0, len(sessionClients))

			for _, ch := range sessionClients {
				isDead := false
				for _, deadCh := range deadChannels {
					if ch == deadCh {
						isDead = true
						// Close dead channel safely
						select {
						case <-ch:
						default:
							close(ch)
						}
						break
					}
				}

				if !isDead {
					filteredClients = append(filteredClients, ch)
				}
			}

			tenantSessions[sessionID] = filteredClients

			// Clean up empty session
			if len(filteredClients) == 0 {
				delete(tenantSessions, sessionID)
			}
		}

		// Clean up empty tenant
		if len(tenantSessions) == 0 {
			delete(b.tenantSessions, tenantID)
		}
	}
}

// HasViewingSessions checks if any sessions are viewing storyfragment within tenant
func (b *SSEBroadcaster) HasViewingSessions(tenantID, storyfragmentID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if registry, exists := b.tenantRegistry[tenantID]; exists {
		registry.mu.RLock()
		defer registry.mu.RUnlock()

		sessions := registry.StoryfragmentToSessions[storyfragmentID]
		return len(sessions) > 0
	}
	return false
}

// GetActiveTenantIDs returns list of tenants with active SSE connections
func (b *SSEBroadcaster) GetActiveTenantIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	var tenantIDs []string
	for tenantID, tenantSessions := range b.tenantSessions {
		if len(tenantSessions) > 0 {
			tenantIDs = append(tenantIDs, tenantID)
		}
	}
	return tenantIDs
}

// GetDeadChannelsForTenant returns dead channels for a specific tenant
func (b *SSEBroadcaster) GetDeadChannelsForTenant(tenantID string) []chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	var deadChannels []chan string

	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		for _, sessionClients := range tenantSessions {
			for _, ch := range sessionClients {
				select {
				case <-ch:
					// Channel is closed/dead
					deadChannels = append(deadChannels, ch)
				default:
					// Channel is still active
				}
			}
		}
	}

	return deadChannels
}

// =============================================================================
// END STAGE 3 SSE Architecture
// =============================================================================

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
	FingerprintID string              `json:"fingerprintId"`
	HeldBeliefs   map[string][]string `json:"heldBeliefs"`
	HeldBadges    map[string]string   `json:"heldBadges"`
	LastActivity  time.Time           `json:"lastActivity"`
}

func (f *FingerprintState) UpdateActivity() {
	if f != nil {
		f.LastActivity = time.Now()
	}
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
