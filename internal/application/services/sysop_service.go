// Package services provides sysop dashboard operations
package services

import (
	"fmt"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/manager"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// SysOpService handles sysop dashboard operations following DI pattern
type SysOpService struct {
	cacheManager      *manager.Manager
	tenantManager     *tenant.Manager
	contentMapService *ContentMapService
	logger            *logging.ChanneledLogger
	perfTracker       *performance.Tracker
}

// NewSysOpService creates a new sysop service with injected dependencies
func NewSysOpService(
	cacheManager *manager.Manager,
	tenantManager *tenant.Manager,
	contentMapService *ContentMapService,
	logger *logging.ChanneledLogger,
	perfTracker *performance.Tracker,
) *SysOpService {
	return &SysOpService{
		cacheManager:      cacheManager,
		tenantManager:     tenantManager,
		contentMapService: contentMapService,
		logger:            logger,
		perfTracker:       perfTracker,
	}
}

// GraphNode represents a node in the activity graph with enhanced metadata
type GraphNode struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Label           string `json:"label"`
	Size            int    `json:"size"`
	LeadName        string `json:"leadName,omitempty"`
	LastActivity    string `json:"lastActivity,omitempty"`    // "X minutes ago"
	BeliefCount     int    `json:"beliefCount,omitempty"`     // Number of held beliefs
	PageTitle       string `json:"pageTitle,omitempty"`       // Actual page title from content map
	IsAuthenticated bool   `json:"isAuthenticated,omitempty"` // Whether fingerprint has a lead
	SessionCount    int    `json:"sessionCount,omitempty"`    // Number of sessions for this fingerprint
	ActivityLevel   string `json:"activityLevel,omitempty"`   // "active", "recent", "dormant"
}

// GraphLink represents a link between nodes in the activity graph
type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// ActivityGraphResponse represents the complete activity graph response
type ActivityGraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
	Stats GraphStats  `json:"stats"`
}

// GraphStats provides summary statistics for the activity graph
type GraphStats struct {
	Sessions           int    `json:"sessions"`
	Fingerprints       int    `json:"fingerprints"`
	Nodes              int    `json:"nodes"`
	Links              int    `json:"links"`
	AuthenticatedUsers int    `json:"authenticatedUsers"`
	TotalBeliefs       int    `json:"totalBeliefs"`
	Timeframe          string `json:"timeframe"`
}

// GetActivityGraph returns enhanced real-time session/fingerprint/belief graph data
func (s *SysOpService) GetActivityGraph(tenantID string) (*ActivityGraphResponse, error) {
	start := time.Now()
	marker := s.perfTracker.StartOperation("get_activity_graph", tenantID)
	defer marker.Complete()

	s.logger.System().Debug("Starting activity graph generation", "tenantId", tenantID)

	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

	// Get all session and fingerprint IDs from cache
	sessionIDs := s.cacheManager.GetAllSessionIDs(tenantID)
	fingerprintIDs := s.cacheManager.GetAllFingerprintIDs(tenantID)

	s.logger.System().Debug("Retrieved cache IDs",
		"tenantId", tenantID,
		"sessions", len(sessionIDs),
		"fingerprints", len(fingerprintIDs))

	nodes := []GraphNode{}
	links := []GraphLink{}
	nodeSet := make(map[string]bool) // Track unique nodes
	stats := GraphStats{
		Timeframe: "last_hour",
	}

	// Track fingerprint session counts for enhanced metadata
	fingerprintSessionCounts := make(map[string]int)

	// Process sessions -> fingerprints (last hour only)
	for _, sessionID := range sessionIDs {
		sessionData, exists := s.cacheManager.GetSession(tenantID, sessionID)
		if !exists {
			continue
		}

		// Skip if session activity is older than 1 hour
		if sessionData.LastActivity.Before(oneHourAgo) {
			continue
		}

		stats.Sessions++

		// Count sessions per fingerprint
		fingerprintSessionCounts[sessionData.FingerprintID]++

		// Add session node with enhanced metadata
		timeAgo := s.formatTimeAgo(sessionData.LastActivity)
		sessionLabel := fmt.Sprintf("Session\n%s\n%s", sessionID, timeAgo)

		if !nodeSet[sessionID] {
			sessionNode := GraphNode{
				ID:            sessionID,
				Type:          "session",
				Label:         sessionLabel,
				Size:          10,
				LastActivity:  timeAgo,
				ActivityLevel: s.getActivityLevel(sessionData.LastActivity),
			}

			if sessionData.LeadID != nil {
				sessionNode.IsAuthenticated = true
			}

			nodes = append(nodes, sessionNode)
			nodeSet[sessionID] = true
		}

		// Add fingerprint node with enhanced metadata
		fingerprintID := sessionData.FingerprintID
		fingerprintLabel := fingerprintID
		if len(fingerprintID) > 8 {
			fingerprintLabel = fingerprintID[:8] + "..."
		}

		if !nodeSet[fingerprintID] {
			fingerprintNode := GraphNode{
				ID:              fingerprintID,
				Type:            "fingerprint",
				Label:           fingerprintLabel,
				Size:            12,
				IsAuthenticated: sessionData.LeadID != nil,
			}

			// Get fingerprint state for belief count and activity
			if fpState, exists := s.cacheManager.GetFingerprintState(tenantID, fingerprintID); exists {
				beliefCount := 0
				for _, beliefValues := range fpState.HeldBeliefs {
					beliefCount += len(beliefValues)
				}
				fingerprintNode.BeliefCount = beliefCount
				fingerprintNode.LastActivity = s.formatTimeAgo(fpState.LastActivity)
				fingerprintNode.ActivityLevel = s.getActivityLevel(fpState.LastActivity)
			}

			nodes = append(nodes, fingerprintNode)
			nodeSet[fingerprintID] = true
			stats.Fingerprints++
		}

		// Link session to fingerprint
		links = append(links, GraphLink{
			Source: sessionID,
			Target: fingerprintID,
			Type:   "session_fingerprint",
		})

		// Add lead node if this fingerprint has a lead
		if sessionData.LeadID != nil && *sessionData.LeadID != "" {
			leadID := *sessionData.LeadID

			// Try to get lead name from database
			leadName := "Unknown Lead"
			if tenantCtx, err := s.tenantManager.NewContextFromID(tenantID); err == nil {
				if lead, err := tenantCtx.LeadRepo().FindByID(leadID); err == nil && lead != nil {
					leadName = lead.FirstName
					if lead.Email != "" {
						leadName += " (" + lead.Email + ")"
					}
				}
				tenantCtx.Close()
			}

			leadLabel := leadName
			if len(leadLabel) > 20 {
				leadLabel = leadLabel[:17] + "..."
			}

			if !nodeSet[leadID] {
				leadNode := GraphNode{
					ID:       leadID,
					Type:     "lead",
					Label:    leadLabel,
					Size:     12,
					LeadName: leadName,
				}
				nodes = append(nodes, leadNode)
				nodeSet[leadID] = true
				stats.AuthenticatedUsers++
			}

			// Link fingerprint to lead
			links = append(links, GraphLink{
				Source: fingerprintID,
				Target: leadID,
				Type:   "fingerprint_lead",
			})
		}
	}

	// Update fingerprint nodes with session counts
	for i := range nodes {
		if nodes[i].Type == "fingerprint" {
			nodes[i].SessionCount = fingerprintSessionCounts[nodes[i].ID]
		}
	}

	// Process fingerprints -> beliefs (last hour only)
	for _, fingerprintID := range fingerprintIDs {
		fpState, exists := s.cacheManager.GetFingerprintState(tenantID, fingerprintID)
		if !exists {
			continue
		}

		// Skip if fingerprint activity is older than 1 hour
		if fpState.LastActivity.Before(oneHourAgo) {
			continue
		}

		// Process held beliefs
		for beliefKey, beliefValues := range fpState.HeldBeliefs {
			stats.TotalBeliefs += len(beliefValues)

			for _, beliefValue := range beliefValues {
				beliefNodeID := beliefKey + ":" + beliefValue
				beliefLabel := s.formatBeliefLabel(beliefKey, beliefValue)

				if !nodeSet[beliefNodeID] {
					beliefNode := GraphNode{
						ID:    beliefNodeID,
						Type:  "belief",
						Label: beliefLabel,
						Size:  8,
					}
					nodes = append(nodes, beliefNode)
					nodeSet[beliefNodeID] = true
				}

				// Link fingerprint to belief
				links = append(links, GraphLink{
					Source: fingerprintID,
					Target: beliefNodeID,
					Type:   "fingerprint_belief",
				})
			}
		}
	}

	// Final stats
	stats.Nodes = len(nodes)
	stats.Links = len(links)

	result := &ActivityGraphResponse{
		Nodes: nodes,
		Links: links,
		Stats: stats,
	}

	marker.SetSuccess(true)
	s.logger.System().Info("Activity graph generation completed",
		"tenantId", tenantID,
		"nodes", stats.Nodes,
		"links", stats.Links,
		"sessions", stats.Sessions,
		"fingerprints", stats.Fingerprints,
		"authenticatedUsers", stats.AuthenticatedUsers,
		"totalBeliefs", stats.TotalBeliefs,
		"duration", time.Since(start))

	s.logger.Perf().Info("Performance for GetActivityGraph",
		"duration", marker.Duration,
		"tenantId", tenantID,
		"success", true)

	return result, nil
}

// formatBeliefLabel creates a readable belief label
func (s *SysOpService) formatBeliefLabel(beliefKey, beliefValue string) string {
	// Create a more readable belief label
	label := fmt.Sprintf("%s:%s", beliefKey, beliefValue)
	if len(label) > 15 {
		if len(beliefKey) > 8 {
			beliefKey = beliefKey[:8] + "..."
		}
		if len(beliefValue) > 6 {
			beliefValue = beliefValue[:6] + "..."
		}
		label = fmt.Sprintf("%s:%s", beliefKey, beliefValue)
	}
	return label
}

// formatTimeAgo formats a time as "X minutes ago", "X hours ago", or "now"
func (s *SysOpService) formatTimeAgo(t time.Time) string {
	now := time.Now()
	duration := now.Sub(t)

	if duration < time.Minute {
		return "now"
	} else if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", minutes)
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// getActivityLevel determines activity level based on last activity time
func (s *SysOpService) getActivityLevel(lastActivity time.Time) string {
	now := time.Now()
	duration := now.Sub(lastActivity)

	if duration < 5*time.Minute {
		return "active"
	} else if duration < 30*time.Minute {
		return "recent"
	} else {
		return "dormant"
	}
}
