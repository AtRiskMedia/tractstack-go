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

	// StoreBeliefEvent saves a user belief event to the persistence layer.
	StoreBeliefEvent(event *BeliefEvent) error

	// FindActionEventsInRange retrieves all action events within a given time range, filtered by verb.
	FindActionEventsInRange(start, end time.Time, verbFilter []string) ([]*ActionEvent, error)

	// FindBeliefEventsInRange retrieves all belief events within a given time range, filtered by value.
	FindBeliefEventsInRange(start, end time.Time, valueFilter []string) ([]*BeliefEvent, error)

	// CountEventsInRange returns the total event count for a time range.
	CountEventsInRange(startTime, endTime time.Time) (int, error)

	// LoadFingerprintBeliefs reconstructs the belief state for a fingerprint from the heldbeliefs table.
	LoadFingerprintBeliefs(fingerprintID string) (map[string][]string, error)
}
