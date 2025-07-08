// Package events provides analytics event processing for panes and story fragments
package events

import (
	"fmt"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/AtRiskMedia/tractstack-go/utils"
)

// AnalyticsProcessor handles pure analytics events (Pane and StoryFragment events)
type AnalyticsProcessor struct {
	tenantID     string
	sessionID    string
	ctx          *tenant.Context
	cacheManager *cache.Manager
}

// NewAnalyticsProcessor creates a new analytics processor
func NewAnalyticsProcessor(tenantID, sessionID string, ctx *tenant.Context, cacheManager *cache.Manager) *AnalyticsProcessor {
	return &AnalyticsProcessor{
		tenantID:     tenantID,
		sessionID:    sessionID,
		ctx:          ctx,
		cacheManager: cacheManager,
	}
}

// ProcessAnalyticsEvent processes analytics events (Pane and StoryFragment)
func (ap *AnalyticsProcessor) ProcessAnalyticsEvent(event models.Event) error {
	// Get session data to find fingerprint and visit
	sessionData, exists := ap.cacheManager.GetSession(ap.tenantID, ap.sessionID)
	if !exists {
		return fmt.Errorf("session not found: %s", ap.sessionID)
	}

	// Process based on event type
	switch event.Type {
	case "Pane":
		return ap.processPaneEvent(event, sessionData)
	case "StoryFragment":
		return ap.processStoryFragmentEvent(event, sessionData)
	default:
		return fmt.Errorf("unsupported analytics event type: %s", event.Type)
	}
}

// processPaneEvent handles pane-specific analytics events
func (ap *AnalyticsProcessor) processPaneEvent(event models.Event, sessionData *models.SessionData) error {
	// Pane events use the pane ID directly (no slug resolution needed)
	objectID := event.ID

	// Convert duration from float64 to integer milliseconds for database
	var durationMs *int
	if event.Duration != nil {
		ms := int(*event.Duration * 1000) // Convert seconds to milliseconds
		durationMs = &ms
	}

	// Insert into actions table
	return ap.insertAction(objectID, event.Type, event.Verb, durationMs, sessionData)
}

// processStoryFragmentEvent handles story fragment analytics events
func (ap *AnalyticsProcessor) processStoryFragmentEvent(event models.Event, sessionData *models.SessionData) error {
	// StoryFragment events use the fragment ID directly
	objectID := event.ID

	// StoryFragment events typically don't have duration
	return ap.insertAction(objectID, event.Type, event.Verb, nil, sessionData)
}

// insertAction inserts an analytics record into the actions table
func (ap *AnalyticsProcessor) insertAction(objectID, objectType, verb string, durationMs *int, sessionData *models.SessionData) error {
	// Build query with optional duration field
	var query string
	var args []interface{}

	if durationMs != nil {
		query = `INSERT INTO actions 
			(id, object_id, object_type, verb, duration, visit_id, fingerprint_id, created_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		args = []interface{}{
			utils.GenerateULID(),
			objectID,
			objectType,
			verb,
			*durationMs,
			sessionData.VisitID,
			sessionData.FingerprintID,
			time.Now().UTC(),
		}
	} else {
		query = `INSERT INTO actions 
			(id, object_id, object_type, verb, visit_id, fingerprint_id, created_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?)`
		args = []interface{}{
			utils.GenerateULID(),
			objectID,
			objectType,
			verb,
			sessionData.VisitID,
			sessionData.FingerprintID,
			time.Now().UTC(),
		}
	}

	// Execute the insert
	_, err := ap.ctx.Database.Conn.Exec(query, args...)
	if err != nil {
		log.Printf("Failed to insert %s analytics event: %v", objectType, err)
		return fmt.Errorf("failed to insert %s action: %w", objectType, err)
	}

	log.Printf("Analytics: %s %s event recorded for session %s", objectType, verb, ap.sessionID)
	return nil
}
