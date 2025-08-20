package errors

import (
	"encoding/json"
	"fmt"
	"strings"
)

// APIError represents a general API error from PVE.
type APIError struct {
	Message  string            `json:"message"`
	Code     int               `json:"code"`
	Errors   map[string]string `json:"errors,omitempty"`
	File     string            `json:"file,omitempty"`
	Line     int               `json:"line,omitempty"`
	HTTPCode int               `json:"-"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		var errStrs []string
		for field, msg := range e.Errors {
			errStrs = append(errStrs, fmt.Sprintf("%s: %s", field, msg))
		}
		return fmt.Sprintf("%s (code: %d, errors: %s)", e.Message, e.Code, strings.Join(errStrs, ", "))
	}
	return fmt.Sprintf("%s (code: %d)", e.Message, e.Code)
}

// IsNotFound returns true if the error indicates a resource was not found.
func (e *APIError) IsNotFound() bool {
	return e.HTTPCode == 404 || e.Code == 404
}

// IsUnauthorized returns true if the error indicates unauthorized access.
func (e *APIError) IsUnauthorized() bool {
	return e.HTTPCode == 401 || e.Code == 401
}

// IsForbidden returns true if the error indicates forbidden access.
func (e *APIError) IsForbidden() bool {
	return e.HTTPCode == 403 || e.Code == 403
}

// PermissionError represents a permission-related error.
type PermissionError struct {
	APIError
	What string `json:"what"` // What resource/action was denied
}

// Error implements the error interface.
func (e *PermissionError) Error() string {
	if e.What != "" {
		return fmt.Sprintf("permission denied for %s: %s", e.What, e.APIError.Error())
	}
	return fmt.Sprintf("permission denied: %s", e.APIError.Error())
}

// ParameterError represents a parameter validation error.
type ParameterError struct {
	APIError
	Usage string `json:"usage"` // Expected parameter usage
}

// Error implements the error interface.
func (e *ParameterError) Error() string {
	if e.Usage != "" {
		return fmt.Sprintf("parameter error: %s (usage: %s)", e.APIError.Error(), e.Usage)
	}
	return fmt.Sprintf("parameter error: %s", e.APIError.Error())
}

// AuthenticationError represents an authentication failure.
type AuthenticationError struct {
	APIError
	Realm string `json:"realm,omitempty"` // Authentication realm
	TFA   bool   `json:"tfa,omitempty"`   // Whether TFA is required
}

// Error implements the error interface.
func (e *AuthenticationError) Error() string {
	msg := "authentication failed"
	if e.Realm != "" {
		msg += fmt.Sprintf(" for realm %s", e.Realm)
	}
	if e.TFA {
		msg += " (TFA required)"
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// TFARequiredError indicates that two-factor authentication is required.
type TFARequiredError struct {
	Ticket    string   `json:"ticket"`    // Partial ticket for TFA
	Challenge string   `json:"challenge"` // TFA challenge (if any)
	Types     []string `json:"types"`     // Available TFA types
}

// Error implements the error interface.
func (e *TFARequiredError) Error() string {
	return fmt.Sprintf("two-factor authentication required (available types: %s)", strings.Join(e.Types, ", "))
}

// ConnectionError represents a connection-related error.
type ConnectionError struct {
	Host    string
	Port    int
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *ConnectionError) Error() string {
	msg := fmt.Sprintf("connection to %s:%d failed", e.Host, e.Port)
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.Cause != nil {
		msg += fmt.Sprintf(" (caused by: %v)", e.Cause)
	}
	return msg
}

// Unwrap returns the underlying error.
func (e *ConnectionError) Unwrap() error {
	return e.Cause
}

// SSLError represents an SSL/TLS related error.
type SSLError struct {
	Host        string
	Fingerprint string
	Message     string
	Cause       error
}

// Error implements the error interface.
func (e *SSLError) Error() string {
	msg := fmt.Sprintf("SSL error for %s", e.Host)
	if e.Fingerprint != "" {
		msg += fmt.Sprintf(" (fingerprint: %s)", e.Fingerprint)
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.Cause != nil {
		msg += fmt.Sprintf(" (caused by: %v)", e.Cause)
	}
	return msg
}

// Unwrap returns the underlying error.
func (e *SSLError) Unwrap() error {
	return e.Cause
}

// TimeoutError represents a request timeout.
type TimeoutError struct {
	Operation string
	Duration  string
}

// Error implements the error interface.
func (e *TimeoutError) Error() string {
	return fmt.Sprintf("operation %s timed out after %s", e.Operation, e.Duration)
}

// ParseAPIError attempts to parse an error response into an appropriate error type.
func ParseAPIError(statusCode int, body []byte) error {
	var apiErr APIError

	// Try to parse JSON error response
	if err := json.Unmarshal(body, &apiErr); err != nil {
		// If JSON parsing fails, create a generic error
		return &APIError{
			Message:  string(body),
			Code:     statusCode,
			HTTPCode: statusCode,
		}
	}

	apiErr.HTTPCode = statusCode

	// Determine specific error type based on status code and content
	switch statusCode {
	case 401:
		// Check if it's a TFA requirement
		var tfaErr TFARequiredError
		if json.Unmarshal(body, &tfaErr) == nil && tfaErr.Ticket != "" {
			return &tfaErr
		}
		return &AuthenticationError{APIError: apiErr}
	case 403:
		return &PermissionError{APIError: apiErr}
	case 400:
		return &ParameterError{APIError: apiErr}
	default:
		return &apiErr
	}
}

// IsAPIError checks if an error is an APIError or one of its subtypes.
func IsAPIError(err error) bool {
	switch err.(type) {
	case *APIError, *PermissionError, *ParameterError, *AuthenticationError:
		return true
	default:
		return false
	}
}

// IsConnectionError checks if an error is a ConnectionError.
func IsConnectionError(err error) bool {
	_, ok := err.(*ConnectionError)
	return ok
}

// IsSSLError checks if an error is an SSLError.
func IsSSLError(err error) bool {
	_, ok := err.(*SSLError)
	return ok
}

// IsTimeoutError checks if an error is a TimeoutError.
func IsTimeoutError(err error) bool {
	_, ok := err.(*TimeoutError)
	return ok
}

// IsTFARequired checks if an error indicates TFA is required.
func IsTFARequired(err error) bool {
	_, ok := err.(*TFARequiredError)
	return ok
}
