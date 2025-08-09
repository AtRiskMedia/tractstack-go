// Package services provides sysop dashboard operations
package services

import (
	"fmt"
	"strings"
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
	StoryFragments     int    `json:"storyfragments"`
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
		fingerprintSessionCounts[sessionData.FingerprintID]++

		// Add session node
		if !nodeSet[sessionID] {
			nodes = append(nodes, GraphNode{
				ID:   sessionID,
				Type: "session",
				Size: 3,
			})
			nodeSet[sessionID] = true
		}

		// Add fingerprint node and link
		if !nodeSet[sessionData.FingerprintID] {
			fingerprintState, fpExists := s.cacheManager.GetFingerprintState(tenantID, sessionData.FingerprintID)
			isAuthenticated := sessionData.LeadID != nil
			if isAuthenticated {
				stats.AuthenticatedUsers++
			}

			beliefCount := 0
			if fpExists && fingerprintState.HeldBeliefs != nil {
				beliefCount = len(fingerprintState.HeldBeliefs)
				stats.TotalBeliefs += beliefCount
			}

			nodes = append(nodes, GraphNode{
				ID:              sessionData.FingerprintID,
				Type:            "fingerprint",
				Size:            5,
				IsAuthenticated: isAuthenticated,
				BeliefCount:     beliefCount,
			})
			nodeSet[sessionData.FingerprintID] = true
			stats.Fingerprints++
		}

		// Link session to fingerprint
		links = append(links, GraphLink{
			Source: sessionID,
			Target: sessionData.FingerprintID,
			Type:   "session_fingerprint",
		})

		// Add lead node if authenticated
		if sessionData.LeadID != nil {
			leadID := *sessionData.LeadID
			if !nodeSet[leadID] {
				nodes = append(nodes, GraphNode{
					ID:   leadID,
					Type: "lead",
					Size: 6,
				})
				nodeSet[leadID] = true
			}

			// Link fingerprint to lead
			links = append(links, GraphLink{
				Source: sessionData.FingerprintID,
				Target: leadID,
				Type:   "fingerprint_lead",
			})
		}
	}

	// Process fingerprint beliefs
	for _, fingerprintID := range fingerprintIDs {
		fingerprintState, exists := s.cacheManager.GetFingerprintState(tenantID, fingerprintID)
		if !exists || fingerprintState.HeldBeliefs == nil {
			continue
		}

		// Only include fingerprints with recent activity
		if fingerprintState.LastActivity.Before(oneHourAgo) {
			continue
		}

		for beliefSlug := range fingerprintState.HeldBeliefs {
			beliefNodeID := "belief_" + beliefSlug
			if !nodeSet[beliefNodeID] {
				nodes = append(nodes, GraphNode{
					ID:    beliefNodeID,
					Type:  "belief",
					Label: beliefSlug,
					Size:  4,
				})
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

	// NEW: Process StoryFragment activity from hourly epinet bins
	storyFragmentActivity, err := s.extractStoryFragmentActivity(tenantID, oneHourAgo)
	if err != nil {
		s.logger.System().Warn("Failed to extract StoryFragment activity", "tenantId", tenantID, "error", err)
	} else {
		// Get content map for StoryFragment titles
		contentMap, err := s.getContentMap(tenantID)
		if err != nil {
			s.logger.System().Warn("Failed to get content map for StoryFragments", "tenantId", tenantID, "error", err)
			contentMap = make(map[string]struct{ Title, Slug string }) // fallback to empty map
		}

		// Add StoryFragment nodes and links
		for storyfragmentID, fingerprintActivity := range storyFragmentActivity {
			sfNodeID := "storyfragment_" + storyfragmentID

			// Add StoryFragment node if not already added
			if !nodeSet[sfNodeID] {
				title := contentMap[storyfragmentID].Title
				slug := contentMap[storyfragmentID].Slug
				if title == "" {
					title = storyfragmentID // fallback if title not found
				}
				if slug == "" {
					slug = storyfragmentID // fallback if slug not found
				}

				nodes = append(nodes, GraphNode{
					ID:        sfNodeID,
					Type:      "storyfragment",
					Label:     "/" + slug, // Show slug with leading slash
					PageTitle: title,      // Keep full title for tooltip
					Size:      7,          // Larger than beliefs but smaller than leads
				})
				nodeSet[sfNodeID] = true
				stats.StoryFragments++
			}

			// Add links from fingerprints to StoryFragments
			for fingerprintID, verbSet := range fingerprintActivity {
				// Only link if fingerprint is in our active set
				if !nodeSet[fingerprintID] {
					continue
				}

				// Create one link per verb type to avoid duplicates
				hasEntered := verbSet["ENTERED"]
				hasPageviewed := verbSet["PAGEVIEWED"]

				if hasEntered {
					links = append(links, GraphLink{
						Source: fingerprintID,
						Target: sfNodeID,
						Type:   "fingerprint_entered",
					})
				}
				if hasPageviewed {
					links = append(links, GraphLink{
						Source: fingerprintID,
						Target: sfNodeID,
						Type:   "fingerprint_pageviewed",
					})
				}
			}
		}
	}

	// Update final stats
	stats.Nodes = len(nodes)
	stats.Links = len(links)

	s.logger.System().Debug("Activity graph generation completed",
		"tenantId", tenantID,
		"nodes", stats.Nodes,
		"links", stats.Links,
		"storyfragments", stats.StoryFragments,
		"duration", time.Since(start))

	return &ActivityGraphResponse{
		Nodes: nodes,
		Links: links,
		Stats: stats,
	}, nil
}

// extractStoryFragmentActivity extracts StoryFragment activity from hourly epinet bins
func (s *SysOpService) extractStoryFragmentActivity(tenantID string, oneHourAgo time.Time) (map[string]map[string]map[string]bool, error) {
	// Structure: storyfragmentID -> fingerprintID -> verb -> bool
	activity := make(map[string]map[string]map[string]bool)

	// Get recent hour keys (current hour and previous hour)
	now := time.Now().UTC()
	currentHour := now.Format("2006-01-02-15")
	previousHour := now.Add(-1 * time.Hour).Format("2006-01-02-15")
	hourKeys := []string{currentHour, previousHour}

	// Get all epinets for this tenant
	epinets, err := s.getEpinets(tenantID)
	if err != nil {
		return activity, err
	}

	// Process each epinet's hourly bins
	for _, epinetID := range epinets {
		for _, hourKey := range hourKeys {
			bin, exists := s.cacheManager.GetHourlyEpinetBin(tenantID, epinetID, hourKey)
			if !exists {
				continue
			}

			// Process each step in the bin
			for nodeID, stepData := range bin.Data.Steps {
				// Parse nodeID to extract StoryFragment activity
				// Format: "commitmentAction_StoryFragment_VERB_storyfragmentID"
				if !strings.Contains(nodeID, "StoryFragment") {
					continue
				}

				parts := strings.Split(nodeID, "_")
				if len(parts) < 4 {
					continue
				}

				// Extract verb and storyfragmentID
				verb := parts[len(parts)-2]
				storyfragmentID := parts[len(parts)-1]

				// Only process ENTERED and PAGEVIEWED verbs
				if verb != "ENTERED" && verb != "PAGEVIEWED" {
					continue
				}

				// Initialize nested maps if needed
				if activity[storyfragmentID] == nil {
					activity[storyfragmentID] = make(map[string]map[string]bool)
				}

				// Process each visitor (fingerprintID) for this step
				for fingerprintID := range stepData.Visitors {
					// Check if fingerprint has recent activity (within our time window)
					fingerprintState, exists := s.cacheManager.GetFingerprintState(tenantID, fingerprintID)
					if !exists || fingerprintState.LastActivity.Before(oneHourAgo) {
						continue
					}

					if activity[storyfragmentID][fingerprintID] == nil {
						activity[storyfragmentID][fingerprintID] = make(map[string]bool)
					}
					activity[storyfragmentID][fingerprintID][verb] = true
				}
			}
		}
	}

	return activity, nil
}

// getContentMap retrieves StoryFragment titles and slugs from content map using correct cache access
func (s *SysOpService) getContentMap(tenantID string) (map[string]struct{ Title, Slug string }, error) {
	contentMap := make(map[string]struct{ Title, Slug string })

	// Create tenant context to access content map properly
	tenantCtx, err := s.tenantManager.NewContextFromID(tenantID)
	if err != nil {
		return contentMap, fmt.Errorf("failed to create tenant context: %w", err)
	}
	defer tenantCtx.Close()

	// Use content map service to get cached content map
	response, _, err := s.contentMapService.GetContentMap(tenantCtx, "", tenantCtx.CacheManager)
	if err != nil {
		return contentMap, fmt.Errorf("failed to get content map: %w", err)
	}

	// Extract StoryFragment titles and slugs
	for _, item := range response.Data {
		if item.Type == "StoryFragment" {
			contentMap[item.ID] = struct{ Title, Slug string }{
				Title: item.Title,
				Slug:  item.Slug,
			}
		}
	}

	return contentMap, nil
}

// getEpinets retrieves epinet IDs for the tenant
func (s *SysOpService) getEpinets(tenantID string) ([]string, error) {
	// Create tenant context to access epinet repository
	tenantCtx, err := s.tenantManager.NewContextFromID(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant context: %w", err)
	}
	defer tenantCtx.Close()

	epinetRepo := tenantCtx.EpinetRepo()
	epinets, err := epinetRepo.FindAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get epinets: %w", err)
	}

	epinetIDs := make([]string, 0, len(epinets))
	for _, epinet := range epinets {
		if epinet != nil {
			epinetIDs = append(epinetIDs, epinet.ID)
		}
	}

	return epinetIDs, nil
}
