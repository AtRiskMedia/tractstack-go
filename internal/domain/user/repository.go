// Package user defines the interfaces for accessing lead, fingerprint, and visit entities.
// These repositories abstract the data persistence details, ensuring the core
// application is clean and decoupled from the database.
// Note: Sessions are handled by the cache layer, not persistence.
package user

import "time"

// Lead represents an authenticated user in the system.
type Lead struct {
	ID             string    `json:"id"`
	FirstName      string    `json:"firstName"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"` // Never serialize password hash
	ContactPersona string    `json:"contactPersona"`
	ShortBio       string    `json:"shortBio"`
	EncryptedCode  string    `json:"-"` // Never serialize encrypted code
	EncryptedEmail string    `json:"-"` // Never serialize encrypted email
	CreatedAt      time.Time `json:"createdAt"`
	Changed        time.Time `json:"changed"`
}

// Fingerprint represents a unique visitor tracking identifier.
// Can optionally be linked to a Lead for authenticated users.
type Fingerprint struct {
	ID        string    `json:"id"`
	LeadID    *string   `json:"leadId,omitempty"` // Optional foreign key to leads
	CreatedAt time.Time `json:"createdAt"`
}

// Visit represents a session/visit tied to a specific fingerprint.
type Visit struct {
	ID            string    `json:"id"`
	FingerprintID string    `json:"fingerprintId"`        // Required foreign key to fingerprints
	CampaignID    *string   `json:"campaignId,omitempty"` // Optional foreign key to campaigns
	CreatedAt     time.Time `json:"createdAt"`
}

// Profile represents a view of Lead data for frontend consumption.
// This is a derived entity, not persisted directly.
type Profile struct {
	Fingerprint    string `json:"Fingerprint"`
	LeadID         string `json:"LeadID"`
	Firstname      string `json:"Firstname"`
	Email          string `json:"Email"`
	ContactPersona string `json:"ContactPersona"`
	ShortBio       string `json:"ShortBio"`
}

// LeadRepository defines the operations for persisting Lead entities.
type LeadRepository interface {
	FindByID(id string) (*Lead, error)
	FindByEmail(email string) (*Lead, error)
	Store(lead *Lead) error
	Update(lead *Lead) error
	ValidateCredentials(email, password string) (*Lead, error)
}

// FingerprintRepository defines the operations for persisting Fingerprint entities.
type FingerprintRepository interface {
	FindByID(id string) (*Fingerprint, error)
	FindByLeadID(leadID string) (*Fingerprint, error)
	Create(fingerprint *Fingerprint) error
	LinkToLead(fingerprintID, leadID string) error
	Exists(fingerprintID string) (bool, error)
}

// VisitRepository defines the operations for persisting Visit entities.
type VisitRepository interface {
	FindByID(id string) (*Visit, error)
	FindByFingerprintID(fingerprintID string) ([]*Visit, error)
	Create(visit *Visit) error
	GetLatestByFingerprintID(fingerprintID string) (*Visit, error)
}
