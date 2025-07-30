// Package analytics defines the interfaces for accessing analytics data.
package analytics

import "time"

// ActionEvent represents a user action on a piece of content.
type ActionEvent struct {
	ObjectID      string
	ObjectType    string
	Verb          string
	FingerprintID string
	Duration      int    `json:"duration"`
	VisitID       string `json:"visitId"`
	CreatedAt     time.Time
}

// BeliefEvent represents a user's expressed belief or identity.
type BeliefEvent struct {
	BeliefID      string
	FingerprintID string
	Verb          string
	Object        *string // For identifyAs events
	UpdatedAt     time.Time
}

// EventRepository defines the contract for storing and retrieving analytics events.
type EventRepository interface {
	// StoreActionEvent saves a user action event to the persistence layer.
	StoreActionEvent(event *ActionEvent) error

	// FindActionEventsInRange retrieves all action events within a given time range.
	FindActionEventsInRange(start, end time.Time) ([]*ActionEvent, error)

	// FindBeliefEventsInRange retrieves all belief events within a given time range.
	FindBeliefEventsInRange(start, end time.Time) ([]*BeliefEvent, error)
}
