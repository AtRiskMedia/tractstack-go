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

type EpinetNode struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	Promoted bool             `json:"promoted"`
	Steps    []EpinetNodeStep `json:"steps"`
}

type EpinetNodeStep struct {
	GateType   string   `json:"gateType"`
	Title      string   `json:"title"`
	Values     []string `json:"values"`
	ObjectType *string  `json:"objectType,omitempty"`
	ObjectIDs  []string `json:"objectIds,omitempty"`
}

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
	s.LastActivity = time.Now().UTC()
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
// SIMPLIFIED SSE Broadcasting Architecture - Session-Only
// =============================================================================

var Broadcaster = &SSEBroadcaster{
	tenantSessions: make(map[string]map[string][]chan string),
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

// BroadcastToAffectedPanes sends updates to all sessions within tenant (client filters by storyfragmentId)
func (b *SSEBroadcaster) BroadcastToAffectedPanes(tenantID, storyfragmentID string, paneIDs []string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔊 BROADCAST PANIC: %v", r)
		}
	}()

	log.Printf("🔊 BROADCAST: Starting broadcast to tenant %s, storyfragment %s, panes: %v", tenantID, storyfragmentID, paneIDs)

	// Build message payload
	message := fmt.Sprintf("event: panes_updated\ndata: %s\n\n",
		func() string {
			data := map[string]interface{}{
				"storyfragmentId": storyfragmentID,
				"affectedPanes":   paneIDs,
			}
			jsonData, _ := json.Marshal(data)
			return string(jsonData)
		}())

	b.mu.Lock()
	defer b.mu.Unlock()

	var sentCount, failedCount, deadChannelCount int
	var deadChannels []chan string

	// Send to ALL sessions in tenant (client will filter by storyfragmentId)
	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		for sessionID, sessionClients := range tenantSessions {
			log.Printf("🔊 BROADCAST: Sending to session %s (%d channels)", sessionID, len(sessionClients))

			for i, ch := range sessionClients {
				select {
				case ch <- message:
					sentCount++
				case <-time.After(100 * time.Millisecond):
					log.Printf("🔊 BROADCAST: ❌ Timeout sending to session %s channel %d", sessionID, i)
					failedCount++
					deadChannels = append(deadChannels, ch)
				}
			}
		}
	} else {
		log.Printf("🔊 BROADCAST: ❌ No tenant sessions found for tenant %s", tenantID)
	}

	log.Printf("🔊 BROADCAST: Broadcast complete - sent: %d, failed: %d, dead channels: %d", sentCount, failedCount, deadChannelCount)

	// Clean up dead channels
	if len(deadChannels) > 0 {
		b.cleanupDeadChannels(tenantID, deadChannels)
	}
}

// BroadcastToSpecificSession sends updates to a specific session within tenant
func (b *SSEBroadcaster) BroadcastToSpecificSession(tenantID, sessionID, storyfragmentID string, paneIDs []string, scrollTarget *string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔊 SESSION BROADCAST PANIC: %v", r)
		}
	}()

	// Prepare broadcast message
	paneIDsJSON, _ := json.Marshal(paneIDs)
	var message string
	if scrollTarget != nil {
		message = fmt.Sprintf("event: panes_updated\ndata: {\"storyfragmentId\":\"%s\",\"affectedPanes\":%s,\"gotoPaneId\":\"%s\"}\n\n",
			storyfragmentID, paneIDsJSON, *scrollTarget)
	} else {
		message = fmt.Sprintf("event: panes_updated\ndata: {\"storyfragmentId\":\"%s\",\"affectedPanes\":%s}\n\n",
			storyfragmentID, paneIDsJSON)
	}

	log.Printf("🔊 SESSION BROADCAST: Outgoing message: %s", strings.ReplaceAll(message, "\n", "\\n"))

	b.mu.Lock()
	defer b.mu.Unlock()

	// Send to specific session only
	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		if sessionClients, exists := tenantSessions[sessionID]; exists {
			var deadChannels []chan string
			sentCount := 0
			failedCount := 0

			for i, ch := range sessionClients {
				select {
				case ch <- message:
					// Success - channel is alive
					sentCount++
				default:
					// Channel is blocked/closed - mark for removal
					deadChannels = append(deadChannels, ch)
					failedCount++
					log.Printf("🔊 SESSION BROADCAST: ❌ Failed to send to session %s channel %d (dead channel)", sessionID, i)
				}
			}

			log.Printf("🔊 SESSION BROADCAST: Broadcast complete - sent: %d, failed: %d, dead channels: %d", sentCount, failedCount, len(deadChannels))

			// Clean up dead channels immediately
			if len(deadChannels) > 0 {
				for _, deadCh := range deadChannels {
					for j, ch := range sessionClients {
						if ch == deadCh {
							// Remove this channel from the session
							tenantSessions[sessionID] = append(sessionClients[:j], sessionClients[j+1:]...)
							close(ch)
							log.Printf("🔊 SESSION BROADCAST: Removed dead channel from session %s", sessionID)
							break
						}
					}
				}
			}
		} else {
			log.Printf("🔊 SESSION BROADCAST: ❌ Session %s not found in tenant %s", sessionID, tenantID)
		}
	} else {
		log.Printf("🔊 SESSION BROADCAST: ❌ No tenant sessions found for tenant %s", tenantID)
	}

	log.Printf("🔊 SESSION BROADCAST: Broadcast operation completed")
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

// HasViewingSessions checks if any sessions exist for the tenant (simplified - no storyfragment filtering)
func (b *SSEBroadcaster) HasViewingSessions(tenantID, storyfragmentID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if tenantSessions, exists := b.tenantSessions[tenantID]; exists {
		return len(tenantSessions) > 0
	}
	return false
}

func (v *VisitState) IsVisitActive() bool {
	if v == nil {
		return false
	}
	return time.Since(v.StartTime) <= 2*time.Hour
}

func (v *VisitState) UpdateActivity() {
	if v != nil {
		v.LastActivity = time.Now().UTC()
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
		f.LastActivity = time.Now().UTC()
	}
}

// FullContentMapItem matches V1's FullContentMap structure exactly
type FullContentMapItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
	Type  string `json:"type"`
	// Epinet specific
	Promoted *bool `json:"promoted,omitempty"`
	// Menu specific
	Theme *string `json:"theme,omitempty"`
	// Resource specific
	CategorySlug *string `json:"categorySlug,omitempty"`
	// Pane specific
	IsContext *bool `json:"isContext,omitempty"`
	// StoryFragment specific
	ParentID        *string  `json:"parentId,omitempty"`
	ParentTitle     *string  `json:"parentTitle,omitempty"`
	ParentSlug      *string  `json:"parentSlug,omitempty"`
	Panes           []string `json:"panes,omitempty"`
	SocialImagePath *string  `json:"socialImagePath,omitempty"`
	ThumbSrc        *string  `json:"thumbSrc,omitempty"`
	ThumbSrcSet     *string  `json:"thumbSrcSet,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Topics          []string `json:"topics,omitempty"`
	Changed         *string  `json:"changed,omitempty"`
	// Belief specific
	Scale *string `json:"scale,omitempty"`
}
