// Package events provides event types
package events

// Event defines the structure for belief and interaction events.
type Event struct {
	ID     string
	Type   string
	Verb   string
	Object string
}
