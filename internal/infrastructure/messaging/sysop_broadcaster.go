package messaging

import (
	"encoding/json"
	"log"
	"math"
	"sort"
	"strings"
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
	ticker := time.NewTicker(20 * time.Second) // Slowed down as requested
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

	// Switch to proportional mode if session count is high
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

// REFACTORED: This function now calculates proportions based on the 5-tier activity decay.
func (b *SysOpBroadcaster) calculateProportionalStates(fullStateList []SessionState, displayCount int) []SessionState {
	total := len(fullStateList)
	if total == 0 {
		return []SessionState{}
	}

	now := time.Now()
	// Representative timestamps for each activity tier to trigger the correct CSS on the frontend.
	timeTiers := map[string]time.Time{
		"ultra":   now,
		"bright":  now.Add(-10 * time.Minute),
		"medium":  now.Add(-20 * time.Minute),
		"light":   now.Add(-40 * time.Minute),
		"dormant": now.Add(-60 * time.Minute),
	}

	// 1. Group sessions into detailed buckets based on type and activity tier.
	counts := make(map[string]int)
	for _, s := range fullStateList {
		minutesSince := now.Sub(s.LastActivity).Minutes()

		var tier string
		if minutesSince < 1 {
			tier = "ultra"
		} else if minutesSince <= 15 {
			tier = "bright"
		} else if minutesSince <= 30 {
			tier = "medium"
		} else if minutesSince <= 45 {
			tier = "light"
		} else {
			tier = "dormant"
		}

		var categoryPrefix string
		if s.IsLead {
			categoryPrefix = "lead"
		} else if s.HasBeliefs {
			categoryPrefix = "anonBeliefs"
		} else {
			categoryPrefix = "anon"
		}
		counts[categoryPrefix+"_"+tier]++
	}

	// 2. Build the final list of 200 states based on the calculated proportions.
	proportionalStates := make([]SessionState, 0, displayCount)
	categoryOrder := []string{ // Define order for consistent display
		"lead_ultra", "lead_bright", "lead_medium", "lead_light", "lead_dormant",
		"anonBeliefs_ultra", "anonBeliefs_bright", "anonBeliefs_medium", "anonBeliefs_light", "anonBeliefs_dormant",
		"anon_ultra", "anon_bright", "anon_medium", "anon_light", "anon_dormant",
	}

	// Helper to create multiple copies of a state
	repeatState := func(num int, state SessionState) {
		for i := 0; i < num; i++ {
			proportionalStates = append(proportionalStates, state)
		}
	}

	for _, category := range categoryOrder {
		categoryCount := counts[category]
		if categoryCount == 0 {
			continue
		}

		// Determine the representative SessionState template for this category
		var template SessionState
		switch {
		case strings.HasPrefix(category, "lead"):
			template.IsLead = true
			template.HasBeliefs = true // Leads inherently have belief data
		case strings.HasPrefix(category, "anonBeliefs"):
			template.HasBeliefs = true
		default: // "anon"
			// IsLead and HasBeliefs are already false
		}

		tier := strings.Split(category, "_")[1]
		template.LastActivity = timeTiers[tier]

		// Calculate how many blocks this category gets and add them to the list
		numBlocks := int(math.Round((float64(categoryCount) / float64(total)) * float64(displayCount)))
		if numBlocks > 0 {
			repeatState(numBlocks, template)
		}
	}

	// 3. Shuffle and adjust for rounding errors to ensure a clean visual mix and exact count.
	sort.SliceStable(proportionalStates, func(i, j int) bool {
		// A simple sort to group types, which looks better than pure random
		if proportionalStates[i].IsLead != proportionalStates[j].IsLead {
			return proportionalStates[i].IsLead
		}
		return proportionalStates[i].HasBeliefs
	})

	if len(proportionalStates) > displayCount {
		return proportionalStates[:displayCount]
	}
	for len(proportionalStates) < displayCount {
		// Pad with the most common "anonymous dormant" state if we're short due to rounding
		proportionalStates = append(proportionalStates, SessionState{LastActivity: timeTiers["dormant"]})
	}

	return proportionalStates
}
