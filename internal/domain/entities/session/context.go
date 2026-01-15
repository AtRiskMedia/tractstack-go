// Package session provides domain entities for session and belief state management.
// It defines structures for tracking user sessions, fingerprint states, and
// belief contexts within the personalization system.
package session

import "time"

// BeliefContext represents cached session belief state for a storyfragment
type BeliefContext struct {
	SessionID       string                  `json:"sessionId"`
	StoryfragmentID string                  `json:"storyfragmentId"`
	UserBeliefs     map[string][]string     `json:"userBeliefs"`
	BeliefStates    map[string]*BeliefState `json:"beliefStates"`
	LastEvaluation  time.Time               `json:"lastEvaluation"`
}

// Data represents the core session information
type Data struct {
	SessionID     string            `json:"sessionId"`
	FingerprintID string            `json:"fingerprintId"`
	TenantID      string            `json:"tenantId"`
	CreatedAt     time.Time         `json:"createdAt"`
	LastAccessed  time.Time         `json:"lastAccessed"`
	Properties    map[string]string `json:"properties"` // Additional session metadata
}

// FingerprintState represents user belief state tied to a fingerprint
type FingerprintState struct {
	FingerprintID string                  `json:"fingerprintId"`
	HeldBeliefs   map[string][]string     `json:"heldBeliefs"`
	BeliefStates  map[string]*BeliefState `json:"beliefStates"`
	LastUpdated   time.Time               `json:"lastUpdated"`
}

// BeliefState represents a structured belief with metadata
type BeliefState struct {
	BeliefKey    string    `json:"beliefKey"`
	BeliefValue  any       `json:"beliefValue"`
	IsHeld       bool      `json:"isHeld"`
	Source       string    `json:"source"`     // "widget", "api", "import"
	Confidence   float64   `json:"confidence"` // 0.0-1.0
	LastModified time.Time `json:"lastModified"`
}

// NewBeliefContext creates a new session belief context
func NewBeliefContext(sessionID, storyfragmentID string) *BeliefContext {
	return &BeliefContext{
		SessionID:       sessionID,
		StoryfragmentID: storyfragmentID,
		UserBeliefs:     make(map[string][]string),
		BeliefStates:    make(map[string]*BeliefState),
		LastEvaluation:  time.Now(),
	}
}

// NewData creates a new session data record
func NewData(sessionID, fingerprintID, tenantID string) *Data {
	return &Data{
		SessionID:     sessionID,
		FingerprintID: fingerprintID,
		TenantID:      tenantID,
		CreatedAt:     time.Now(),
		LastAccessed:  time.Now(),
		Properties:    make(map[string]string),
	}
}

// NewFingerprintState creates a new fingerprint state record
func NewFingerprintState(fingerprintID string) *FingerprintState {
	return &FingerprintState{
		FingerprintID: fingerprintID,
		HeldBeliefs:   make(map[string][]string),
		BeliefStates:  make(map[string]*BeliefState),
		LastUpdated:   time.Now(),
	}
}

// UpdateUserBeliefs updates the belief map for this session context
func (bc *BeliefContext) UpdateUserBeliefs(beliefs map[string][]string) {
	bc.UserBeliefs = beliefs
	bc.LastEvaluation = time.Now()
}

// AddBeliefState adds a structured belief state
func (bc *BeliefContext) AddBeliefState(key string, state *BeliefState) {
	bc.BeliefStates[key] = state
	bc.LastEvaluation = time.Now()
}

// GetBeliefState retrieves a belief state by key
func (bc *BeliefContext) GetBeliefState(key string) (*BeliefState, bool) {
	state, exists := bc.BeliefStates[key]
	return state, exists
}

// HasBelief checks if a belief key exists in user beliefs
func (bc *BeliefContext) HasBelief(key string) bool {
	_, exists := bc.UserBeliefs[key]
	return exists
}

// GetBeliefValues retrieves all values for a belief key
func (bc *BeliefContext) GetBeliefValues(key string) []string {
	return bc.UserBeliefs[key]
}

// UpdateLastAccessed updates session access time
func (d *Data) UpdateLastAccessed() {
	d.LastAccessed = time.Now()
}

// AddProperty adds a metadata property to the session
func (d *Data) AddProperty(key, value string) {
	d.Properties[key] = value
}

// GetProperty retrieves a metadata property from the session
func (d *Data) GetProperty(key string) (string, bool) {
	value, exists := d.Properties[key]
	return value, exists
}

// UpdateBeliefState updates or adds a belief state to fingerprint
func (fs *FingerprintState) UpdateBeliefState(key string, state *BeliefState) {
	fs.BeliefStates[key] = state
	fs.LastUpdated = time.Now()
}

// UpdateHeldBeliefs updates the held beliefs map
func (fs *FingerprintState) UpdateHeldBeliefs(beliefs map[string][]string) {
	fs.HeldBeliefs = beliefs
	fs.LastUpdated = time.Now()
}

// HasHeldBelief checks if a belief is held by this fingerprint
func (fs *FingerprintState) HasHeldBelief(key string) bool {
	_, exists := fs.HeldBeliefs[key]
	return exists
}
