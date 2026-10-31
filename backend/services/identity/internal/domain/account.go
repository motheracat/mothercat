package domain

import (
	"errors"
	"time"
)

var ErrInvalidCredentials = errors.New("invalid_credentials")

// Account is the authenticated Identity service principal used by the API and
// session validation workflow. It deliberately keeps the internal account ID
// separate from the business staff identifier.
type Account struct {
	ID          string   `json:"id"`
	StaffID     string   `json:"staff_id"`
	DisplayName string   `json:"display_name"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

// Session represents a server-issued user session.
type Session struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	RevokedAt time.Time `json:"revoked_at,omitempty"`
}
