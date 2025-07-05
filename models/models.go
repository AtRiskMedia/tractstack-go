// Package models defines data structures for the application's core entities and API payloads.
package models

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type VisitState struct {
	VisitID       string    `json:"visitId"`
	FingerprintID string    `json:"fingerprintId"`
	StartTime     time.Time `json:"startTime"`
	LastActivity  time.Time `json:"lastActivity"`
	CurrentPage   string    `json:"currentPage"`
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

type Profile struct {
	Fingerprint    string
	LeadID         string
	Firstname      string
	Email          string
	ContactPersona string
	ShortBio       string
}

type SSEBroadcaster struct {
	tenantSessions map[string]map[string][]chan string // tenantId -> sessionId -> []channels
	tenantRegistry map[string]*SubscriptionRegistry    // tenantId -> registry (NOW POINTER)
	mu             sync.Mutex
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
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Verb     string   `json:"verb"`
	Object   string   `json:"object"`
	Duration *float64 `json:"duration,omitempty"`
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
func NewSubscriptionRegistry() *SubscriptionRegistry {
	return &SubscriptionRegistry{
		SessionToStoryfragment:  make(map[string]string),
		StoryfragmentToSessions: make(map[string][]string),
	}
}

var Broadcaster = &SSEBroadcaster{
	tenantSessions: make(map[string]map[string][]chan string),
	tenantRegistry: make(map[string]*SubscriptionRegistry), // NOW POINTER MAP
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

func (b *SSEBroadcaster) RegisterStoryfragmentSubscription(tenantID, sessionID, storyfragmentID string) {
	// No b.mu.Lock() here - this was the deadlock cause

	// Get/create registry with minimal locking
	b.mu.Lock()
	if _, exists := b.tenantRegistry[tenantID]; !exists {
		b.tenantRegistry[tenantID] = NewSubscriptionRegistry()
	}
	registry := b.tenantRegistry[tenantID]
	b.mu.Unlock()

	// Work with registry under its own lock only
	registry.mu.Lock()
	defer registry.mu.Unlock()

	// Update mappings
	registry.SessionToStoryfragment[sessionID] = storyfragmentID
	if registry.StoryfragmentToSessions[storyfragmentID] == nil {
		registry.StoryfragmentToSessions[storyfragmentID] = make([]string, 0)
	}

	// Add session if not present
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

	// Save back with minimal locking
	b.mu.Lock()
	b.tenantRegistry[tenantID] = registry
	b.mu.Unlock()
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
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔊 BROADCAST PANIC: %v", r)
		}
	}()

	log.Printf("🔊 BROADCAST: Starting broadcast to tenant %s, storyfragment %s, panes: %v", tenantID, storyfragmentID, paneIDs)

	// Get sessions viewing this storyfragment within tenant - FIX COPYLOCKS + ADD ERROR HANDLING
	var targetSessions []string

	log.Printf("🔊 BROADCAST: Checking tenant registry for tenant %s", tenantID)

	if registryPtr, exists := b.tenantRegistry[tenantID]; exists {
		log.Printf("🔊 BROADCAST: Found tenant registry, attempting to lock")

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("🔊 BROADCAST: Panic in registry access: %v", r)
				}
			}()

			registryPtr.mu.RLock()
			defer registryPtr.mu.RUnlock()

			log.Printf("🔊 BROADCAST: Registry locked, checking storyfragment %s", storyfragmentID)

			if sessions, exists := registryPtr.StoryfragmentToSessions[storyfragmentID]; exists {
				targetSessions = append(targetSessions, sessions...)
				log.Printf("🔊 BROADCAST: Found %d sessions viewing storyfragment %s: %v", len(sessions), storyfragmentID, sessions)
			} else {
				log.Printf("🔊 BROADCAST: No sessions found for storyfragment %s", storyfragmentID)
				log.Printf("🔊 BROADCAST: Available storyfragments: %v", func() []string {
					var keys []string
					for k := range registryPtr.StoryfragmentToSessions {
						keys = append(keys, k)
					}
					return keys
				}())
			}
		}()
	} else {
		log.Printf("🔊 BROADCAST: No tenant registry found for tenant %s", tenantID)
		log.Printf("🔊 BROADCAST: Available tenants: %v", func() []string {
			var keys []string
			for k := range b.tenantRegistry {
				keys = append(keys, k)
			}
			return keys
		}())
	}

	if len(targetSessions) == 0 {
		log.Printf("🔊 BROADCAST: No target sessions found - aborting broadcast")
		return // No sessions viewing this storyfragment
	}

	// Prepare broadcast message
	paneIDsJSON, _ := json.Marshal(paneIDs)
	message := fmt.Sprintf("event: panes_updated\ndata: {\"storyfragmentId\":\"%s\",\"affectedPanes\":%s}\n\n", storyfragmentID, paneIDsJSON)

	log.Printf("🔊 BROADCAST: Prepared message: %s", strings.ReplaceAll(message, "\n", "\\n"))

	// Send to all sessions viewing this storyfragment within tenant
	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		log.Printf("🔊 BROADCAST: Found tenant sessions for %s", tenantID)

		var deadChannels []chan string
		sentCount := 0
		failedCount := 0

		for _, sessionID := range targetSessions {
			log.Printf("🔊 BROADCAST: Processing session %s", sessionID)

			if sessionClients, exists := tenantSessions[sessionID]; exists {
				log.Printf("🔊 BROADCAST: Found %d clients for session %s", len(sessionClients), sessionID)

				for i, ch := range sessionClients {
					log.Printf("🔊 BROADCAST: Sending to client %d for session %s", i, sessionID)

					select {
					case ch <- message:
						// Success - channel is alive
						sentCount++
						log.Printf("🔊 BROADCAST: ✅ Successfully sent to client %d for session %s", i, sessionID)
					default:
						// Channel is blocked/closed - mark for removal
						deadChannels = append(deadChannels, ch)
						failedCount++
						log.Printf("🔊 BROADCAST: ❌ Failed to send to client %d for session %s (dead channel)", i, sessionID)
					}
				}
			} else {
				log.Printf("🔊 BROADCAST: ❌ No clients found for session %s", sessionID)
			}
		}

		log.Printf("🔊 BROADCAST: Broadcast complete - sent: %d, failed: %d, dead channels: %d", sentCount, failedCount, len(deadChannels))

		// Clean up dead channels
		if len(deadChannels) > 0 {
			log.Printf("🔊 BROADCAST: Cleaning up %d dead channels", len(deadChannels))
			for _, deadCh := range deadChannels {
				for sessionID, sessionClients := range tenantSessions {
					for i, ch := range sessionClients {
						if ch == deadCh {
							// Remove this channel from the session
							tenantSessions[sessionID] = append(sessionClients[:i], sessionClients[i+1:]...)
							close(ch)
							log.Printf("🔊 BROADCAST: Removed dead channel from session %s", sessionID)
							break
						}
					}
				}
			}
		}
	} else {
		log.Printf("🔊 BROADCAST: ❌ No tenant sessions found for tenant %s", tenantID)
	}

	log.Printf("🔊 BROADCAST: Broadcast operation completed")
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

// BroadcastToSpecificSession sends updates to a specific session within tenant
func (b *SSEBroadcaster) BroadcastToSpecificSession(tenantID, sessionID, storyfragmentID string, paneIDs []string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔊 SESSION BROADCAST PANIC: %v", r)
		}
	}()

	log.Printf("🔊 SESSION BROADCAST: Starting broadcast to tenant %s, session %s, storyfragment %s, panes: %v",
		tenantID, sessionID, storyfragmentID, paneIDs)

	// Prepare broadcast message
	paneIDsJSON, _ := json.Marshal(paneIDs)
	message := fmt.Sprintf("event: panes_updated\ndata: {\"storyfragmentId\":\"%s\",\"affectedPanes\":%s}\n\n", storyfragmentID, paneIDsJSON)

	log.Printf("🔊 SESSION BROADCAST: Prepared message: %s", strings.ReplaceAll(message, "\n", "\\n"))

	b.mu.Lock()
	defer b.mu.Unlock()

	// Send to specific session only
	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		log.Printf("🔊 SESSION BROADCAST: Found tenant sessions for %s", tenantID)

		if sessionClients, exists := tenantSessions[sessionID]; exists {
			log.Printf("🔊 SESSION BROADCAST: Found %d clients for session %s", len(sessionClients), sessionID)

			var deadChannels []chan string
			sentCount := 0
			failedCount := 0

			for i, ch := range sessionClients {
				log.Printf("🔊 SESSION BROADCAST: Sending to client %d for session %s", i, sessionID)

				select {
				case ch <- message:
					// Success - channel is alive
					sentCount++
					log.Printf("🔊 SESSION BROADCAST: ✅ Successfully sent to client %d for session %s", i, sessionID)
				default:
					// Channel is blocked/closed - mark for removal
					deadChannels = append(deadChannels, ch)
					failedCount++
					log.Printf("🔊 SESSION BROADCAST: ❌ Failed to send to client %d for session %s (dead channel)", i, sessionID)
				}
			}

			log.Printf("🔊 SESSION BROADCAST: Broadcast complete - sent: %d, failed: %d, dead channels: %d", sentCount, failedCount, len(deadChannels))

			// Clean up dead channels for this session
			if len(deadChannels) > 0 {
				log.Printf("🔊 SESSION BROADCAST: Cleaning up %d dead channels for session %s", len(deadChannels), sessionID)
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
		} else {
			log.Printf("🔊 SESSION BROADCAST: ❌ No clients found for session %s", sessionID)
		}

		// Clean up empty tenant
		if len(tenantSessions) == 0 {
			delete(b.tenantSessions, tenantID)
		}
	} else {
		log.Printf("🔊 SESSION BROADCAST: ❌ No tenant sessions found for tenant %s", tenantID)
	}

	log.Printf("🔊 SESSION BROADCAST: Session broadcast operation completed")
}
