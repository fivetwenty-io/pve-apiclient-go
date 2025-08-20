package auth

import (
	"time"
)

// Authenticator defines the interface for authentication mechanisms.
type Authenticator interface {
	// Authenticate performs the authentication process.
	Authenticate() error

	// IsAuthenticated checks if the current session is authenticated.
	IsAuthenticated() bool

	// GetHeaders returns the authentication headers to be added to requests.
	GetHeaders() map[string]string

	// Refresh refreshes the authentication if necessary.
	Refresh() error

	// Logout performs logout operations.
	Logout() error
}

// Credentials represents authentication credentials.
type Credentials struct {
	Username string
	Password string
	Realm    string // e.g., "pam", "pve", "ldap"
	OTP      string // One-time password for TFA
}

// Token represents an API token.
type Token struct {
	ID     string // Full token ID (e.g., "user@realm!tokenid")
	Secret string // Token secret
}

// Ticket represents a PVE authentication ticket.
type Ticket struct {
	Value      string    // The ticket value
	CSRFToken  string    // CSRF prevention token
	Username   string    // Authenticated username
	ValidUntil time.Time // Ticket expiration time
}

// IsValid checks if the ticket is still valid.
func (t *Ticket) IsValid() bool {
	return t.Value != "" && time.Now().Before(t.ValidUntil)
}

// GetHeaders returns the headers for ticket-based authentication.
func (t *Ticket) GetHeaders() map[string]string {
	headers := make(map[string]string)
	if t.Value != "" {
		headers["Cookie"] = "PVEAuthCookie=" + t.Value
	}
	if t.CSRFToken != "" {
		headers["CSRFPreventionToken"] = t.CSRFToken
	}
	return headers
}

// TFAChallenge represents a two-factor authentication challenge.
type TFAChallenge struct {
	Ticket    string   // Partial ticket for TFA
	Challenge string   // Challenge data (e.g., for U2F)
	Types     []string // Available TFA types (totp, yubico, recovery, etc.)
	Prompt    string   // User prompt message
}

// TFAResponse represents a response to a TFA challenge.
type TFAResponse struct {
	Response string // The TFA response (e.g., TOTP code)
	Type     string // Type of TFA response
}

// AuthResult represents the result of an authentication attempt.
type AuthResult struct {
	Success      bool
	Ticket       *Ticket
	TFAChallenge *TFAChallenge
	Error        error
}

// AuthProvider defines the interface for authentication providers.
type AuthProvider interface {
	// Login performs the login operation.
	Login(credentials *Credentials) (*AuthResult, error)

	// LoginWithToken performs token-based authentication.
	LoginWithToken(token *Token) (*AuthResult, error)

	// CompleteTFA completes the two-factor authentication process.
	CompleteTFA(challenge *TFAChallenge, response *TFAResponse) (*AuthResult, error)

	// Logout performs the logout operation.
	Logout(ticket *Ticket) error
}

// SessionManager manages authentication sessions.
type SessionManager interface {
	// GetCurrentSession returns the current session.
	GetCurrentSession() *Ticket

	// SetSession sets the current session.
	SetSession(ticket *Ticket)

	// ClearSession clears the current session.
	ClearSession()

	// IsSessionValid checks if the current session is valid.
	IsSessionValid() bool

	// RefreshSession attempts to refresh the session.
	RefreshSession() error
}
