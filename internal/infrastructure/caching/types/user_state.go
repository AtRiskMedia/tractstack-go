// Package types defines user state and session data structures.
package types

import (
	"sync"
	"time"
)

// TenantUserStateCache holds user state data for a single tenant
type TenantUserStateCache struct {
	// Persistent user state by fingerprint
	FingerprintStates             map[string]*FingerprintState            // fingerprintId -> state
	VisitStates                   map[string]*VisitState                  // visitId -> state
	KnownFingerprints             map[string]bool                         // fingerprintId -> isKnown
	StoryfragmentBeliefRegistries map[string]*StoryfragmentBeliefRegistry `json:"storyfragmentBeliefRegistries"`
	SessionStates                 map[string]*SessionData                 // sessionId -> session data
	SessionBeliefContexts         map[string]*SessionBeliefContext        // "sessionId:storyfragmentId" -> context
	FingerprintToSessions         map[string][]string

	// Cache metadata
	LastLoaded time.Time
	Mu         sync.RWMutex // Exported for access
}

// FingerprintState represents a user fingerprint's persistent state
type FingerprintState struct {
	FingerprintID string              `json:"fingerprintId"`
	LeadID        *string             `json:"leadId,omitempty"`
	HeldBeliefs   map[string][]string `json:"heldBeliefs"`
	HeldBadges    map[string]string   `json:"badges"`
	LastActivity  time.Time           `json:"lastActivity"`
}

// VisitState represents the state of a single visit session.
// Each visit tracks analytics and user journey data for a specific fingerprint.
type VisitState struct {
	VisitID       string    `json:"visitId"`
	FingerprintID string    `json:"fingerprintId"`
	CampaignID    *string   `json:"campaignId,omitempty"`
	Referrer      *Referrer `json:"referrer,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	LastActivity  time.Time `json:"lastActivity"`
	StartTime     time.Time `json:"startTime"`
}

// SessionData represents ephemeral session state and serves as the coordination hub.
// Sessions link frontend/backend interactions and own references to both fingerprint and visit.
type SessionData struct {
	SessionID     string    `json:"sessionId"`
	FingerprintID string    `json:"fingerprintId"`
	VisitID       string    `json:"visitId"`
	LeadID        *string   `json:"leadId,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	LastActivity  time.Time `json:"lastActivity"`
	ExpiresAt     time.Time `json:"expiresAt"`
	IsExpired     bool      `json:"isExpired"`
}

// SessionBeliefContext represents belief evaluation context for a session+storyfragment.
// This enables personalized content filtering based on user's held beliefs.
type SessionBeliefContext struct {
	TenantID        string              `json:"tenantId"`
	SessionID       string              `json:"sessionId"`
	StoryfragmentID string              `json:"storyfragmentId"`
	UserBeliefs     map[string][]string `json:"userBeliefs"`
	LastEvaluation  time.Time           `json:"lastEvaluation"`
}

// Referrer contains tracking information for visit attribution
type Referrer struct {
	HTTPReferrer *string `json:"httpReferrer,omitempty"`
	UTMSource    *string `json:"utmSource,omitempty"`
	UTMMedium    *string `json:"utmMedium,omitempty"`
	UTMCampaign  *string `json:"utmCampaign,omitempty"`
	UTMTerm      *string `json:"utmTerm,omitempty"`
	UTMContent   *string `json:"utmContent,omitempty"`
}
