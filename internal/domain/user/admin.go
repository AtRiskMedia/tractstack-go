package user

import "time"

// Admin is a Storykeep staff account (main site admin, table admin, or editor).
type Admin struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Email              string     `json:"email"`
	DisplayName        string     `json:"displayName"`
	Role               string     `json:"role"` // admin | editor
	IsMain             bool       `json:"isMain"`
	PasswordHash       string     `json:"-"`
	MustChangePassword bool       `json:"mustChangePassword"`
	TokenVersion       int        `json:"tokenVersion"`
	CreatedBy          *string    `json:"createdBy,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	Changed            *time.Time `json:"changed,omitempty"`
	LastLoginAt        *time.Time `json:"lastLoginAt,omitempty"`
}

// AdminToken is a one-time password-reset token.
type AdminToken struct {
	ID        string     `json:"id"`
	AdminID   string     `json:"adminId"`
	Purpose   string     `json:"purpose"` // reset
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expiresAt"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// AdminRepository persists Admin rows.
type AdminRepository interface {
	FindByID(id string) (*Admin, error)
	FindByUsername(username string) (*Admin, error)
	FindByEmail(email string) (*Admin, error)
	FindMain() (*Admin, error)
	List() ([]*Admin, error)
	Store(admin *Admin) error
	Update(admin *Admin) error
	Delete(id string) error
	TouchLastLogin(id string) error
}

// AdminTokenRepository persists password-reset tokens.
type AdminTokenRepository interface {
	Store(token *AdminToken) error
	FindValidByHash(tokenHash string) (*AdminToken, error)
	MarkUsed(id string) error
	InvalidateFor(adminID, purpose string) error
	PruneExpired() error
	DeleteForAdmin(adminID string) error
}
