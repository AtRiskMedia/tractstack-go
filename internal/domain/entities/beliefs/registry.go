// Package beliefs provides domain entities for the belief registry system.
// It defines structures for managing belief requirements, pane visibility rules,
// and widget belief mappings within the epistemic hypermedia engine.
package beliefs

import "time"

// BeliefRegistry represents the complete belief system for a storyfragment
type BeliefRegistry struct {
	StoryfragmentID    string                     `json:"storyfragmentId"`
	PaneBeliefPayloads map[string]*PaneBeliefData `json:"paneBeliefPayloads"`
	PaneWidgetBeliefs  map[string][]string        `json:"paneWidgetBeliefs"`
	LastUpdated        time.Time                  `json:"lastUpdated"`
}

// PaneBeliefData contains visibility rules for a specific pane
type PaneBeliefData struct {
	PaneID       string                 `json:"paneId"`
	BeliefMode   string                 `json:"beliefMode"`   // "neutral", "held", "withheld", "match-across"
	RequiredData map[string]interface{} `json:"requiredData"` // Flexible belief requirements
}

// BeliefRequirement represents a specific belief condition
type BeliefRequirement struct {
	BeliefKey   string      `json:"beliefKey"`
	BeliefValue interface{} `json:"beliefValue"`
	Operator    string      `json:"operator"` // "equals", "contains", "exists", "not_exists"
}

// VisibilityState represents computed visibility for a pane
type VisibilityState struct {
	PaneID    string    `json:"paneId"`
	IsVisible bool      `json:"isVisible"`
	Reason    string    `json:"reason"` // Why visible/hidden
	AppliedAt time.Time `json:"appliedAt"`
}

// NewBeliefRegistry creates a new belief registry for a storyfragment
func NewBeliefRegistry(storyfragmentID string) *BeliefRegistry {
	return &BeliefRegistry{
		StoryfragmentID:    storyfragmentID,
		PaneBeliefPayloads: make(map[string]*PaneBeliefData),
		PaneWidgetBeliefs:  make(map[string][]string),
		LastUpdated:        time.Now(),
	}
}

// AddPaneBeliefData adds belief requirements for a pane
func (br *BeliefRegistry) AddPaneBeliefData(paneID string, beliefData *PaneBeliefData) {
	br.PaneBeliefPayloads[paneID] = beliefData
	br.LastUpdated = time.Now()
}

// AddPaneWidgetBeliefs maps widget belief dependencies for a pane
func (br *BeliefRegistry) AddPaneWidgetBeliefs(paneID string, beliefs []string) {
	br.PaneWidgetBeliefs[paneID] = beliefs
	br.LastUpdated = time.Now()
}

// GetPaneBeliefData retrieves belief requirements for a pane
func (br *BeliefRegistry) GetPaneBeliefData(paneID string) (*PaneBeliefData, bool) {
	data, exists := br.PaneBeliefPayloads[paneID]
	return data, exists
}

// GetPaneWidgetBeliefs retrieves widget belief dependencies for a pane
func (br *BeliefRegistry) GetPaneWidgetBeliefs(paneID string) ([]string, bool) {
	beliefs, exists := br.PaneWidgetBeliefs[paneID]
	return beliefs, exists
}

// HasWidgetBeliefs checks if a pane has any widget belief dependencies
func (br *BeliefRegistry) HasWidgetBeliefs(paneID string) bool {
	beliefs, exists := br.PaneWidgetBeliefs[paneID]
	return exists && len(beliefs) > 0
}
