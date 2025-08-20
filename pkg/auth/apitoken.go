package auth

import (
	"fmt"
	"strings"
)

// APITokenAuthenticator provides API token-based authentication for PVE.
type APITokenAuthenticator struct {
	token *Token
}

// NewAPITokenAuthenticator creates a new API token authenticator.
func NewAPITokenAuthenticator(token *Token) *APITokenAuthenticator {
	return &APITokenAuthenticator{
		token: token,
	}
}

// NewAPITokenAuthenticatorFromString creates a new API token authenticator from a string.
// The token string should be in the format: "user@realm!tokenid=secret"
func NewAPITokenAuthenticatorFromString(tokenString string) (*APITokenAuthenticator, error) {
	token, err := ParseAPIToken(tokenString)
	if err != nil {
		return nil, err
	}
	return NewAPITokenAuthenticator(token), nil
}

// Authenticate performs the authentication process.
// For API tokens, this is a no-op as tokens don't require login.
func (ata *APITokenAuthenticator) Authenticate() error {
	if ata.token == nil || ata.token.ID == "" || ata.token.Secret == "" {
		return fmt.Errorf("invalid API token")
	}
	return nil
}

// IsAuthenticated checks if the authenticator has a valid token.
func (ata *APITokenAuthenticator) IsAuthenticated() bool {
	return ata.token != nil && ata.token.ID != "" && ata.token.Secret != ""
}

// GetHeaders returns the authentication headers for API token auth.
func (ata *APITokenAuthenticator) GetHeaders() map[string]string {
	if !ata.IsAuthenticated() {
		return nil
	}

	return map[string]string{
		"Authorization": fmt.Sprintf("PVEAPIToken=%s=%s", ata.token.ID, ata.token.Secret),
	}
}

// Refresh is a no-op for API tokens as they don't expire.
func (ata *APITokenAuthenticator) Refresh() error {
	return nil
}

// Logout is a no-op for API tokens.
func (ata *APITokenAuthenticator) Logout() error {
	return nil
}

// GetToken returns the API token.
func (ata *APITokenAuthenticator) GetToken() *Token {
	return ata.token
}

// SetToken sets the API token.
func (ata *APITokenAuthenticator) SetToken(token *Token) {
	ata.token = token
}

// ParseAPIToken parses an API token string into a Token struct.
// Expected format: "user@realm!tokenid=secret"
func ParseAPIToken(tokenString string) (*Token, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("empty token string")
	}

	// Split on '=' to separate ID and secret
	parts := strings.SplitN(tokenString, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid token format: expected 'user@realm!tokenid=secret'")
	}

	id := parts[0]
	secret := parts[1]

	// Validate token ID format (should contain @ and !)
	if !strings.Contains(id, "@") || !strings.Contains(id, "!") {
		return nil, fmt.Errorf("invalid token ID format: expected 'user@realm!tokenid'")
	}

	// Validate secret is not empty
	if secret == "" {
		return nil, fmt.Errorf("token secret cannot be empty")
	}

	return &Token{
		ID:     id,
		Secret: secret,
	}, nil
}

// FormatAPIToken formats a Token into a string.
func FormatAPIToken(token *Token) string {
	if token == nil {
		return ""
	}
	return fmt.Sprintf("%s=%s", token.ID, token.Secret)
}

// ValidateTokenID validates the format of a token ID.
// Valid format: user@realm!tokenid
func ValidateTokenID(id string) error {
	if id == "" {
		return fmt.Errorf("token ID cannot be empty")
	}

	// Check for @ sign (user@realm)
	atIndex := strings.Index(id, "@")
	if atIndex < 1 {
		return fmt.Errorf("token ID must contain user@realm")
	}

	// Check for ! sign (realm!tokenid)
	exclamIndex := strings.Index(id, "!")
	if exclamIndex <= atIndex {
		return fmt.Errorf("token ID must contain !tokenid after realm")
	}

	// Extract parts
	user := id[:atIndex]
	realm := id[atIndex+1 : exclamIndex]
	tokenName := id[exclamIndex+1:]

	// Validate parts
	if user == "" {
		return fmt.Errorf("user part cannot be empty")
	}
	if realm == "" {
		return fmt.Errorf("realm part cannot be empty")
	}
	if tokenName == "" {
		return fmt.Errorf("token name part cannot be empty")
	}

	return nil
}
