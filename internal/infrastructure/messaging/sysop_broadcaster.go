package messaging

import (
	"encoding/json"
	"log"
	"math"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/gorilla/websocket"
)

// SysOpClient represents a single connected sysop dashboard client.
type SysOpClient struct {
	Conn     *websocket.Conn
	TenantID string
	Send     chan []byte
}

// SessionState represents the detailed state of a single user session for visualization.
type SessionState struct {
	IsLead       bool      `json:"isLead"`
	HasBeliefs   bool      `json:"hasBeliefs"`
	LastActivity time.Time `json:"lastActivity"`
}

// SessionStatePayload is the complete data structure sent to the frontend on each tick.
type SessionStatePayload struct {
	SessionStates    []SessionState `json:"sessionStates"`
	DisplayMode      string         `json:"displayMode"` // "1:1" or "PROPORTIONAL"
	TotalCount       int            `json:"totalCount"`
	LeadCount        int            `json:"leadCount"`
	ActiveCount      int            `json:"activeCount"`
	DormantCount     int            `json:"dormantCount"`
	WithBeliefsCount int            `json:"withBeliefsCount"`
}

// sessionStats holds the raw counts for proportional calculation.
type sessionStats struct{ Total, Lead, Active, Dormant, WithBeliefs int }

// SysOpBroadcaster manages all connected sysop clients and broadcasts data.
type SysOpBroadcaster struct {
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
	tenantIDs := make([]string, 0, len(b.tenantClients))
	for tenantID := range b.tenantClients {
		tenantIDs = append(tenantIDs, tenantID)
	}
	b.mu.RUnlock()

	for _, tenantID := range tenantIDs {
		fullStateList := b.getSessionStatesForTenant(tenantID)
		payload := b.preparePayload(fullStateList)

		message, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Error marshaling session state for tenant %s: %v", tenantID, err)
			continue
		}

		b.mu.RLock()
		if clients, ok := b.tenantClients[tenantID]; ok {
			for client := range clients {
				select {
				case client.Send <- message:
				default:
				}
			}
		}
		b.mu.RUnlock()
	}
}

// getSessionStatesForTenant is the core logic for calculating the state of each session.
func (b *SysOpBroadcaster) getSessionStatesForTenant(tenantID string) []SessionState {
	ctx, err := b.tenantManager.NewContextFromID(tenantID)
	if err != nil {
		log.Printf("SysOp broadcaster could not create context for tenant %s: %v", tenantID, err)
		return []SessionState{}
	}
	defer ctx.Close()

	userCache, err := ctx.CacheManager.GetTenantUserStateCache(tenantID)
	if err != nil {
		log.Printf("SysOp broadcaster could not get user state cache for tenant %s: %v", tenantID, err)
		return []SessionState{}
	}

	userCache.Mu.RLock()
	defer userCache.Mu.RUnlock()

	states := make([]SessionState, 0, len(userCache.SessionStates))
	for _, session := range userCache.SessionStates {
		hasBeliefs := false
		if fpState, exists := userCache.FingerprintStates[session.FingerprintID]; exists {
			hasBeliefs = len(fpState.HeldBeliefs) > 0
		}
		states = append(states, SessionState{
			IsLead:       session.LeadID != nil && *session.LeadID != "",
			HasBeliefs:   hasBeliefs,
			LastActivity: session.LastActivity,
		})
	}
	return states
}

// preparePayload handles the logic for proportional scaling.
func (b *SysOpBroadcaster) preparePayload(fullStateList []SessionState) SessionStatePayload {
	stats := b.calculateStats(fullStateList)
	var displayStates []SessionState
	displayMode := "1:1"

	if stats.Total > 200 {
		displayMode = "PROPORTIONAL"
		displayStates = b.calculateProportionalStates(fullStateList, 200)
	} else {
		displayStates = fullStateList
	}

	return SessionStatePayload{
		SessionStates:    displayStates,
		DisplayMode:      displayMode,
		TotalCount:       stats.Total,
		LeadCount:        stats.Lead,
		ActiveCount:      stats.Active,
		DormantCount:     stats.Dormant,
		WithBeliefsCount: stats.WithBeliefs,
	}
}

// calculateStats calculates aggregate statistics from the full session list.
func (b *SysOpBroadcaster) calculateStats(fullStateList []SessionState) (stats sessionStats) {
	stats.Total = len(fullStateList)
	now := time.Now()
	for _, s := range fullStateList {
		if s.HasBeliefs {
			stats.WithBeliefs++
		}
		if s.IsLead {
			stats.Lead++
		}
		if now.Sub(s.LastActivity).Minutes() <= 45 {
			stats.Active++
		} else {
			stats.Dormant++
		}
	}
	return stats
}

// calculateProportionalStates creates a representative sample of states for large session counts.
func (b *SysOpBroadcaster) calculateProportionalStates(fullStateList []SessionState, displayCount int) []SessionState {
	total := len(fullStateList)
	if total == 0 {
		return []SessionState{}
	}

	// Detailed category counts for accurate proportions
	counts := make(map[string]int)
	now := time.Now()
	for _, s := range fullStateList {
		minutesSince := now.Sub(s.LastActivity).Minutes()
		isActive := minutesSince <= 45

		var category string
		if s.IsLead {
			if isActive {
				category = "leadActive"
			} else {
				category = "leadDormant"
			}
		} else {
			if isActive && s.HasBeliefs {
				category = "anonActiveBeliefs"
			}
			if isActive && !s.HasBeliefs {
				category = "anonActiveNoBeliefs"
			}
			if !isActive {
				category = "anonDormant"
			} // Dormant is always one category now
		}
		counts[category]++
	}

	proportionalStates := make([]SessionState, 0, displayCount)

	// Helper to append a slice of states
	appendStates := func(states []SessionState) {
		proportionalStates = append(proportionalStates, states...)
	}

	// This function modernizes the loop by creating a pre-sized slice and filling it.
	repeatState := func(num int, state SessionState) []SessionState {
		if num <= 0 {
			return nil
		}
		slice := make([]SessionState, num)
		for i := range slice {
			slice[i] = state
		}
		return slice
	}

	// Calculate and append states for each category
	appendStates(repeatState(
		int(math.Round((float64(counts["leadActive"])/float64(total))*float64(displayCount))),
		SessionState{IsLead: true, HasBeliefs: true, LastActivity: now},
	))
	appendStates(repeatState(
		int(math.Round((float64(counts["anonActiveBeliefs"])/float64(total))*float64(displayCount))),
		SessionState{IsLead: false, HasBeliefs: true, LastActivity: now},
	))
	appendStates(repeatState(
		int(math.Round((float64(counts["anonActiveNoBeliefs"])/float64(total))*float64(displayCount))),
		SessionState{IsLead: false, HasBeliefs: false, LastActivity: now},
	))

	// For dormant, use an old timestamp to trigger the correct styles
	oldTime := now.Add(-60 * time.Minute)
	appendStates(repeatState(
		int(math.Round((float64(counts["leadDormant"])/float64(total))*float64(displayCount))),
		SessionState{IsLead: true, HasBeliefs: true, LastActivity: oldTime},
	))
	appendStates(repeatState(
		int(math.Round((float64(counts["anonDormant"])/float64(total))*float64(displayCount))),
		SessionState{IsLead: false, HasBeliefs: false, LastActivity: oldTime},
	))

	// Adjust for rounding errors to ensure exactly `displayCount` items
	for len(proportionalStates) < displayCount {
		proportionalStates = append(proportionalStates, SessionState{IsLead: false, HasBeliefs: false, LastActivity: oldTime})
	}

	return proportionalStates[:displayCount]
}
