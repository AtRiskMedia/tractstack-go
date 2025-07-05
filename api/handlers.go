// Package api provides HTTP handlers and database connectivity for the application's API.
package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/AtRiskMedia/tractstack-go/cache"
	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/AtRiskMedia/tractstack-go/tenant"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// getTenantContext is a helper to extract tenant context from gin context
func getTenantContext(c *gin.Context) (*tenant.Context, error) {
	tenantCtx, exists := c.Get("tenant")
	if !exists {
		return nil, fmt.Errorf("no tenant context")
	}
	return tenantCtx.(*tenant.Context), nil
}

// getTenantManager is a helper to extract tenant manager from gin context
func getTenantManager(c *gin.Context) (*tenant.Manager, error) {
	manager, exists := c.Get("tenantManager")
	if !exists {
		return nil, fmt.Errorf("no tenant manager")
	}
	return manager.(*tenant.Manager), nil
}

// generateULID creates a new ULID
func generateULID() string {
	return ulid.Make().String()
}

// DBStatusHandler checks tenant status - SIMPLIFIED: No activation logic
func DBStatusHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// REMOVED: All activation logic - tenants should be pre-activated during startup
	// If tenant is not active, this indicates a serious problem
	if ctx.Status != "active" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("tenant %s is not active (status: %s) - should have been pre-activated",
				ctx.TenantID, ctx.Status),
		})
		return
	}

	// If we reach here and status is active, tables are guaranteed to exist
	allTablesExist := (ctx.Status == "active")

	c.JSON(http.StatusOK, gin.H{
		"tenantId":       ctx.TenantID,
		"status":         ctx.Status,
		"database":       ctx.Database.GetConnectionInfo(),
		"allTablesExist": allTablesExist,
	})
}

// StateHandler - Phase 1: Accept form data from widgets and convert to events
func StateHandler(c *gin.Context) {
	ctx, err := getTenantContext(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Extract form data from widgets
	beliefID := c.PostForm("beliefId")
	beliefType := c.PostForm("beliefType")
	beliefValue := c.PostForm("beliefValue")
	beliefObject := c.PostForm("beliefObject")

	// Validate required fields
	if beliefID == "" || beliefType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "beliefId and beliefType required"})
		return
	}

	// Extract session context
	sessionID := c.GetHeader("X-TractStack-Session-ID")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID required"})
		return
	}

	// Convert form data to event structure following the plan's conversion logic
	var event models.Event
	if beliefObject != "" {
		// IdentifyAs event - detected by presence of beliefObject
		event = models.Event{
			ID:     beliefID,
			Type:   beliefType,
			Verb:   "IDENTIFY_AS",
			Object: beliefObject,
		}
	} else if beliefValue != "" && beliefValue != "0" {
		// Scale or Toggle event with actual selection - beliefValue contains the verb
		// Exclude "0" which is the default prompt option in belief scales
		event = models.Event{
			ID:     beliefID,
			Type:   beliefType,
			Verb:   beliefValue,
			Object: beliefID, // Use slug as object for belief events
		}
	} else {
		// Belief scale with no selection (default "0" state) or toggle unchecked
		// For belief scales, missing/default beliefValue indicates no selection yet
		// This creates an UNSET event following V1 pattern
		event = models.Event{
			ID:     beliefID,
			Type:   beliefType,
			Verb:   "UNSET",
			Object: beliefID,
		}
	}

	// Log the converted event (Phase 1: log everything, process nothing)
	log.Printf("StateHandler: Processing %s event in tenant %s: %+v",
		event.Type, ctx.TenantID, event)

	// TODO Phase 2: Add belief slug→ID cache lookup
	// TODO Phase 3: Create EventProcessor factory
	// TODO Phase 4: Add database persistence
	// TODO Phase 5: Implement full event type handlers

	// Phase 1 success response - widgets should now work without 400 errors
	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"tenantId": ctx.TenantID,
		"event":    event,
	})
}

// updateFingerprintBelief updates user beliefs in fingerprint state
func updateFingerprintBelief(tenantID, sessionID string, event models.Event) error {
	// Get session data to find fingerprint ID
	sessionData, exists := cache.GetGlobalManager().GetSession(tenantID, sessionID)
	if !exists {
		return fmt.Errorf("session %s not found in tenant %s", sessionID, tenantID)
	}

	// Get current fingerprint state
	fingerprintState, exists := cache.GetGlobalManager().GetFingerprintState(tenantID, sessionData.FingerprintID)
	if !exists {
		// Create new fingerprint state if it doesn't exist
		fingerprintState = &models.FingerprintState{
			FingerprintID: sessionData.FingerprintID,
			HeldBeliefs:   make(map[string][]string),
			HeldBadges:    make(map[string]string),
			LastActivity:  time.Now(),
		}
	}

	// Update beliefs based on event
	beliefSlug := event.Object
	beliefValue := event.Verb

	// Initialize belief array if it doesn't exist
	if fingerprintState.HeldBeliefs[beliefSlug] == nil {
		fingerprintState.HeldBeliefs[beliefSlug] = make([]string, 0)
	}

	// Update belief value (simple append for now - can be enhanced for proper belief logic)
	switch event.ID {
	case "ADD_BELIEF", "UPDATE_BELIEF":
		// Add or update belief value
		found := false
		for _, existing := range fingerprintState.HeldBeliefs[beliefSlug] {
			if existing == beliefValue {
				found = true
				break
			}
		}
		if !found {
			fingerprintState.HeldBeliefs[beliefSlug] = append(fingerprintState.HeldBeliefs[beliefSlug], beliefValue)
		}

	case "REMOVE_BELIEF":
		// Remove belief value
		for i, existing := range fingerprintState.HeldBeliefs[beliefSlug] {
			if existing == beliefValue {
				fingerprintState.HeldBeliefs[beliefSlug] = append(
					fingerprintState.HeldBeliefs[beliefSlug][:i],
					fingerprintState.HeldBeliefs[beliefSlug][i+1:]...)
				break
			}
		}

	default:
		// Default: set belief value
		fingerprintState.HeldBeliefs[beliefSlug] = []string{beliefValue}
	}

	// Update timestamp
	fingerprintState.UpdateActivity()

	// Save updated fingerprint state
	cache.GetGlobalManager().SetFingerprintState(tenantID, fingerprintState)

	log.Printf("Updated belief %s = %s for fingerprint %s in tenant %s",
		beliefSlug, beliefValue, sessionData.FingerprintID, tenantID)

	return nil
}
