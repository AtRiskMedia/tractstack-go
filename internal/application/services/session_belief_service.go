package services

import (
	"encoding/json"
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/session"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/caching/types"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// SessionBeliefService handles session belief resolution and management
type SessionBeliefService struct{}

// NewSessionBeliefService creates a new session belief service
func NewSessionBeliefService() *SessionBeliefService {
	return &SessionBeliefService{}
}

// GetSessionBeliefContext retrieves cached session belief context
func (s *SessionBeliefService) GetSessionBeliefContext(
	tenantCtx *tenant.Context,
	sessionID, storyfragmentID string,
) (*session.SessionBeliefContext, error) {
	if sessionID == "" || storyfragmentID == "" {
		return nil, nil
	}

	cacheManager := tenantCtx.CacheManager

	sessionBeliefCtx, exists := cacheManager.GetSessionBeliefContext(tenantCtx.TenantID, sessionID, storyfragmentID)
	if !exists {
		return nil, nil
	}

	// Convert from types to our domain entity
	domainCtx := &session.SessionBeliefContext{
		SessionID:       sessionBeliefCtx.SessionID,
		StoryfragmentID: sessionBeliefCtx.StoryfragmentID,
		UserBeliefs:     sessionBeliefCtx.UserBeliefs,
		BeliefStates:    make(map[string]*session.BeliefState),
		LastEvaluation:  sessionBeliefCtx.LastEvaluation,
	}

	return domainCtx, nil
}

// GetUserBeliefs resolves user beliefs via session→fingerprint→beliefs chain
// This implements the core getUserBeliefsFromContext logic from legacy
func (s *SessionBeliefService) GetUserBeliefs(
	tenantCtx *tenant.Context,
	sessionID string,
) (map[string][]string, error) {
	if sessionID == "" {
		return make(map[string][]string), nil
	}

	cacheManager := tenantCtx.CacheManager

	// Step 1: Get session data to find fingerprint ID
	sessionData, exists := cacheManager.GetSession(tenantCtx.TenantID, sessionID)
	if !exists {
		return make(map[string][]string), nil
	}

	// Step 2: Get fingerprint state to access user beliefs
	fingerprintState, exists := cacheManager.GetFingerprintState(tenantCtx.TenantID, sessionData.FingerprintID)
	if !exists || fingerprintState.HeldBeliefs == nil {
		return make(map[string][]string), nil
	}

	// Step 3: Return the held beliefs map
	return fingerprintState.HeldBeliefs, nil
}

// CreateSessionBeliefContext creates a new session belief context
// This implements createSessionBeliefContext from legacy
func (s *SessionBeliefService) CreateSessionBeliefContext(
	tenantCtx *tenant.Context,
	sessionID, storyfragmentID string,
) (*session.SessionBeliefContext, error) {
	// Get user beliefs via session→fingerprint chain
	userBeliefs, err := s.GetUserBeliefs(tenantCtx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user beliefs: %w", err)
	}

	// Parse belief states
	beliefStates, err := s.ParseBeliefStates(userBeliefs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse belief states: %w", err)
	}

	// Create domain context
	ctx := session.NewSessionBeliefContext(sessionID, storyfragmentID)
	ctx.UpdateUserBeliefs(userBeliefs)

	// Add structured belief states
	for key, state := range beliefStates {
		ctx.AddBeliefState(key, state)
	}

	// Convert to types for caching
	typesCtx := &types.SessionBeliefContext{
		TenantID:        tenantCtx.TenantID,
		SessionID:       sessionID,
		StoryfragmentID: storyfragmentID,
		UserBeliefs:     userBeliefs,
		LastEvaluation:  ctx.LastEvaluation,
	}

	// Cache the context
	cacheManager := tenantCtx.CacheManager
	cacheManager.SetSessionBeliefContext(tenantCtx.TenantID, typesCtx)

	return ctx, nil
}

// ParseBeliefStates converts belief JSON strings to structured data
func (s *SessionBeliefService) ParseBeliefStates(
	userBeliefs map[string][]string,
) (map[string]*session.BeliefState, error) {
	beliefStates := make(map[string]*session.BeliefState)

	for beliefKey, beliefValues := range userBeliefs {
		if len(beliefValues) == 0 {
			continue
		}

		// Try to parse the first value as JSON for structured beliefs
		var parsedValue interface{}
		if err := json.Unmarshal([]byte(beliefValues[0]), &parsedValue); err != nil {
			// If JSON parsing fails, use the raw string value
			parsedValue = beliefValues[0]
		}

		beliefState := &session.BeliefState{
			BeliefKey:   beliefKey,
			BeliefValue: parsedValue,
			IsHeld:      true,
			Source:      "session",
			Confidence:  1.0,
		}

		beliefStates[beliefKey] = beliefState
	}

	return beliefStates, nil
}

// HasBeliefIntersection checks if user has any of the required beliefs
// This implements the core cache bypass logic: widget beliefs × user beliefs intersection
func (s *SessionBeliefService) HasBeliefIntersection(
	userBeliefs map[string][]string,
	requiredBeliefs []string,
) bool {
	for _, required := range requiredBeliefs {
		if _, exists := userBeliefs[required]; exists {
			return true // Found intersection - bypass cache needed
		}
	}
	return false // No intersection - use cache
}

// GetBeliefIntersection returns which beliefs intersect between user and required
func (s *SessionBeliefService) GetBeliefIntersection(
	userBeliefs map[string][]string,
	requiredBeliefs []string,
) []string {
	var intersection []string

	for _, required := range requiredBeliefs {
		if _, exists := userBeliefs[required]; exists {
			intersection = append(intersection, required)
		}
	}

	return intersection
}

// ShouldCreateSessionContext determines if session context should be created
// This implements shouldCreateSessionContext logic from legacy
func (s *SessionBeliefService) ShouldCreateSessionContext(
	tenantCtx *tenant.Context,
	sessionID, storyfragmentID string,
) bool {
	if sessionID == "" || storyfragmentID == "" {
		return false
	}

	// Get belief registry to check for pane requirements
	cacheManager := tenantCtx.CacheManager
	registry, found := cacheManager.GetStoryfragmentBeliefRegistry(tenantCtx.TenantID, storyfragmentID)
	if !found {
		return false // No registry = no requirements
	}

	// If ANY pane has belief requirements, we need to evaluate visibility
	// This ensures that panes get hidden when user has empty beliefs
	for _, paneBeliefs := range registry.PaneBeliefPayloads {
		if len(paneBeliefs.HeldBeliefs) > 0 || len(paneBeliefs.WithheldBeliefs) > 0 || len(paneBeliefs.MatchAcross) > 0 {
			return true // At least one pane has requirements = need evaluation
		}
	}

	return false // No panes have requirements
}
