// Package widgets provides domain entities for widget state management and personalization.
// It defines structures for pre-resolved widget contexts that eliminate the need
// for templates to make direct cache calls during rendering.
package widgets

import (
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/session"
)

// WidgetContext represents pre-resolved widget state for template rendering
type WidgetContext struct {
	SessionID         string                  `json:"sessionId"`
	StoryfragmentID   string                  `json:"storyfragmentId"`
	UserBeliefs       map[string][]string     `json:"userBeliefs"`
	WidgetStates      map[string]*WidgetState `json:"widgetStates"`
	PaneWidgetMap     map[string][]string     `json:"paneWidgetMap"` // paneID → widget belief keys
	PersonalizationOn bool                    `json:"personalizationOn"`
	LastUpdated       time.Time               `json:"lastUpdated"`
}

// WidgetState represents the resolved state for a specific widget
type WidgetState struct {
	WidgetID        string                 `json:"widgetId"`
	WidgetType      string                 `json:"widgetType"` // "belief", "toggle", "identifyAs", etc.
	BeliefKey       string                 `json:"beliefKey"`
	CurrentValue    interface{}            `json:"currentValue"`
	IsPersonalized  bool                   `json:"isPersonalized"`
	VisibilityState string                 `json:"visibilityState"` // "visible", "hidden", "default"
	Properties      map[string]interface{} `json:"properties"`      // Widget-specific props
	LastModified    time.Time              `json:"lastModified"`
}

// WidgetBeliefMapping represents widget→belief relationships for a pane
type WidgetBeliefMapping struct {
	PaneID        string            `json:"paneId"`
	WidgetBeliefs map[string]string `json:"widgetBeliefs"` // widgetID → beliefKey
	LastScanned   time.Time         `json:"lastScanned"`
}

// NewWidgetContext creates a new widget context
func NewWidgetContext(sessionID, storyfragmentID string) *WidgetContext {
	return &WidgetContext{
		SessionID:       sessionID,
		StoryfragmentID: storyfragmentID,
		UserBeliefs:     make(map[string][]string),
		WidgetStates:    make(map[string]*WidgetState),
		PaneWidgetMap:   make(map[string][]string),
		LastUpdated:     time.Now(),
	}
}

// NewWidgetState creates a new widget state
func NewWidgetState(widgetID, widgetType, beliefKey string) *WidgetState {
	return &WidgetState{
		WidgetID:        widgetID,
		WidgetType:      widgetType,
		BeliefKey:       beliefKey,
		IsPersonalized:  false,
		VisibilityState: "default",
		Properties:      make(map[string]interface{}),
		LastModified:    time.Now(),
	}
}

// UpdateFromSessionContext updates widget context from session belief context
func (wc *WidgetContext) UpdateFromSessionContext(sbc *session.SessionBeliefContext) {
	if sbc != nil {
		wc.UserBeliefs = sbc.UserBeliefs
		wc.PersonalizationOn = len(sbc.UserBeliefs) > 0

		// Update widget states based on user beliefs
		for _, widgetState := range wc.WidgetStates {
			if beliefValues, exists := sbc.UserBeliefs[widgetState.BeliefKey]; exists {
				widgetState.CurrentValue = beliefValues
				widgetState.IsPersonalized = true
				widgetState.VisibilityState = "visible"
				widgetState.LastModified = time.Now()
			} else {
				widgetState.IsPersonalized = false
				widgetState.VisibilityState = "default"
			}
		}
	}
	wc.LastUpdated = time.Now()
}

// AddWidgetState adds a widget state to the context
func (wc *WidgetContext) AddWidgetState(widgetState *WidgetState) {
	wc.WidgetStates[widgetState.WidgetID] = widgetState
	wc.LastUpdated = time.Now()
}

// GetWidgetState retrieves a widget state by ID
func (wc *WidgetContext) GetWidgetState(widgetID string) (*WidgetState, bool) {
	state, exists := wc.WidgetStates[widgetID]
	return state, exists
}

// AddPaneWidgetMapping adds widget belief mapping for a pane
func (wc *WidgetContext) AddPaneWidgetMapping(paneID string, beliefKeys []string) {
	wc.PaneWidgetMap[paneID] = beliefKeys
	wc.LastUpdated = time.Now()
}

// GetPaneWidgetBeliefs retrieves widget belief keys for a pane
func (wc *WidgetContext) GetPaneWidgetBeliefs(paneID string) []string {
	return wc.PaneWidgetMap[paneID]
}

// HasPersonalizedWidgets checks if any widgets are personalized
func (wc *WidgetContext) HasPersonalizedWidgets() bool {
	for _, widgetState := range wc.WidgetStates {
		if widgetState.IsPersonalized {
			return true
		}
	}
	return false
}

// GetBeliefValue retrieves belief value for template rendering
func (wc *WidgetContext) GetBeliefValue(beliefKey string) interface{} {
	if values, exists := wc.UserBeliefs[beliefKey]; exists && len(values) > 0 {
		return values[0] // Return first value for simple widgets
	}
	return nil
}

// GetBeliefValues retrieves all belief values for a key
func (wc *WidgetContext) GetBeliefValues(beliefKey string) []string {
	if values, exists := wc.UserBeliefs[beliefKey]; exists {
		return values
	}
	return []string{}
}

// HasBelief checks if a belief exists
func (wc *WidgetContext) HasBelief(beliefKey string) bool {
	_, exists := wc.UserBeliefs[beliefKey]
	return exists
}

// IsPersonalizationEnabled checks if personalization is active
func (wc *WidgetContext) IsPersonalizationEnabled() bool {
	return wc.PersonalizationOn && len(wc.UserBeliefs) > 0
}

// SetProperty sets a property on a widget state
func (ws *WidgetState) SetProperty(key string, value interface{}) {
	ws.Properties[key] = value
	ws.LastModified = time.Now()
}

// GetProperty retrieves a property from widget state
func (ws *WidgetState) GetProperty(key string) (interface{}, bool) {
	value, exists := ws.Properties[key]
	return value, exists
}

// UpdateCurrentValue updates the current value of the widget
func (ws *WidgetState) UpdateCurrentValue(value interface{}) {
	ws.CurrentValue = value
	ws.IsPersonalized = true
	ws.LastModified = time.Now()
}

// IsVisible checks if the widget should be visible
func (ws *WidgetState) IsVisible() bool {
	return ws.VisibilityState == "visible" || ws.VisibilityState == "default"
}

// SetVisibility updates the visibility state
func (ws *WidgetState) SetVisibility(state string) {
	ws.VisibilityState = state
	ws.LastModified = time.Now()
}
