package messaging

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/gorilla/websocket"
)

// SysOpClient represents a single connected sysop dashboard client.
// Its fields are exported to be accessible from the handler for managing pumps.
type SysOpClient struct {
	Conn     *websocket.Conn
	TenantID string
	Send     chan []byte
}

// SessionStatePayload is the data structure sent to the frontend on each tick.
type SessionStatePayload struct {
	SessionStates []string `json:"sessionStates"`
}

// SysOpBroadcaster manages all connected sysop clients and broadcasts data.
type SysOpBroadcaster struct {
	// A map where the key is a tenantID and the value is a map of connected clients for that tenant.
	tenantClients map[string]map[*SysOpClient]bool
	register      chan *SysOpClient
	unregister    chan *SysOpClient
	cacheManager  *manager.Manager
	tenantManager *tenant.Manager
	mu            sync.RWMutex
}

// NewSysOpBroadcaster creates a new broadcaster instance.
func NewSysOpBroadcaster(tm *tenant.Manager, cm *manager.Manager) *SysOpBroadcaster {
	return &SysOpBroadcaster{
		tenantClients: make(map[string]map[*SysOpClient]bool),
		register:      make(chan *SysOpClient),
		unregister:    make(chan *SysOpClient),
		cacheManager:  cm,
		tenantManager: tm,
	}
}

// Run starts the broadcaster's main loop. This should be run as a goroutine.
func (b *SysOpBroadcaster) Run() {
	// Ticker for broadcasting session data every 5 seconds.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-b.register:
			b.mu.Lock()
			if _, ok := b.tenantClients[client.TenantID]; !ok {
				b.tenantClients[client.TenantID] = make(map[*SysOpClient]bool)
			}
			b.tenantClients[client.TenantID][client] = true
			log.Printf("SysOp client registered for tenant: %s", client.TenantID)
			b.mu.Unlock()

		case client := <-b.unregister:
			b.mu.Lock()
			if clients, ok := b.tenantClients[client.TenantID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.Send)
					if len(clients) == 0 {
						delete(b.tenantClients, client.TenantID)
					}
				}
			}
			log.Printf("SysOp client unregistered for tenant: %s", client.TenantID)
			b.mu.Unlock()

		case <-ticker.C:
			b.broadcastSessionMaps()
		}
	}
}

// Register queues a client for registration.
func (b *SysOpBroadcaster) Register(client *SysOpClient) {
	b.register <- client
}

// Unregister queues a client for unregistration.
func (b *SysOpBroadcaster) Unregister(client *SysOpClient) {
	b.unregister <- client
}

// broadcastSessionMaps gathers and sends the session state for all tenants with active clients.
func (b *SysOpBroadcaster) broadcastSessionMaps() {
	b.mu.RLock()
	// Create a copy of the keys to avoid holding the lock during the entire loop
	tenantIDs := make([]string, 0, len(b.tenantClients))
	for tenantID := range b.tenantClients {
		tenantIDs = append(tenantIDs, tenantID)
	}
	b.mu.RUnlock()

	for _, tenantID := range tenantIDs {
		// Get the session states for the current tenant.
		states := b.getSessionStatesForTenant(tenantID)
		payload := SessionStatePayload{SessionStates: states}
		message, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Error marshaling session state for tenant %s: %v", tenantID, err)
			continue
		}

		// Send the message to all clients watching this tenant.
		b.mu.RLock()
		if clients, ok := b.tenantClients[tenantID]; ok {
			for client := range clients {
				select {
				case client.Send <- message:
				default:
					// If the send buffer is full, assume the client is slow or disconnected.
					// The ReadPump will eventually detect the closed connection and unregister.
				}
			}
		}
		b.mu.RUnlock()
	}
}

// getSessionStatesForTenant is the core logic for calculating the state of each session.
func (b *SysOpBroadcaster) getSessionStatesForTenant(tenantID string) []string {
	// CORRECTED: Use the TenantManager to get a proper, initialized context.
	ctx, err := b.tenantManager.NewContextFromID(tenantID)
	if err != nil {
		log.Printf("SysOp broadcaster could not create context for tenant %s: %v", tenantID, err)
		return []string{}
	}
	// CRITICAL: Defer closing the context to prevent resource leaks.
	defer ctx.Close()

	// Now, access the cache through the guaranteed-to-be-initialized context.
	userCache, err := ctx.CacheManager.GetTenantUserStateCache(tenantID)
	if err != nil {
		log.Printf("SysOp broadcaster could not get user state cache for tenant %s: %v", tenantID, err)
		return []string{}
	}

	userCache.Mu.RLock()
	defer userCache.Mu.RUnlock()

	states := make([]string, 0, len(userCache.SessionStates))

	for _, session := range userCache.SessionStates {
		// 1. Check for Known Lead
		if session.LeadID != nil && *session.LeadID != "" {
			states = append(states, "lead")
			continue
		}

		// 2. Check for Dormancy vs. Activity
		inactiveDuration := time.Since(session.LastActivity)

		// Dormant/Fading (older than 30 minutes)
		if inactiveDuration > (30 * time.Minute) {
			if inactiveDuration > (2 * time.Hour) {
				states = append(states, "dormant_deep") // Nearing cleanup
			} else if inactiveDuration > (1 * time.Hour) {
				states = append(states, "dormant_medium")
			} else {
				states = append(states, "dormant_light")
			}
		} else { // Active/Pulsing
			if inactiveDuration < (2 * time.Minute) {
				states = append(states, "active_bright")
			} else if inactiveDuration < (10 * time.Minute) {
				states = append(states, "active_medium")
			} else {
				states = append(states, "active_light")
			}
		}
	}
	return states
}
