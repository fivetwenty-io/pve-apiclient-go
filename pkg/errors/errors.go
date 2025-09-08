package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fivetwenty-io/pve-apiclient-go/internal/constants"
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
	return e.HTTPCode == constants.HTTPStatusNotFound || e.Code == constants.HTTPStatusNotFound
}

// IsUnauthorized returns true if the error indicates unauthorized access.
func (e *APIError) IsUnauthorized() bool {
	return e.HTTPCode == constants.HTTPStatusUnauthorized || e.Code == constants.HTTPStatusUnauthorized
}

// IsForbidden returns true if the error indicates forbidden access.
func (e *APIError) IsForbidden() bool {
	return e.HTTPCode == constants.HTTPStatusForbidden || e.Code == constants.HTTPStatusForbidden
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

	return "permission denied: " + e.APIError.Error()
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

	return "parameter error: " + e.APIError.Error()
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
		msg += " for realm " + e.Realm
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
	msg := "SSL error for " + e.Host
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
	err := json.Unmarshal(body, &apiErr)
	if err != nil {
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
	case constants.HTTPStatusUnauthorized:
		// Check if it's a TFA requirement
		var tfaErr TFARequiredError
		if json.Unmarshal(body, &tfaErr) == nil && tfaErr.Ticket != "" {
			return &tfaErr
		}

		return &AuthenticationError{APIError: apiErr}
	case constants.HTTPStatusForbidden:
		return &PermissionError{APIError: apiErr}
	case constants.HTTPStatusBadRequest:
		return &ParameterError{APIError: apiErr}
	default:
		return &apiErr
	}
}

// IsAPIError checks if an error is an APIError or one of its subtypes.
func IsAPIError(err error) bool {
	var (
		apiErr   *APIError
		permErr  *PermissionError
		paramErr *ParameterError
		authErr  *AuthenticationError
	)

	return errors.As(err, &apiErr) ||
		errors.As(err, &permErr) ||
		errors.As(err, &paramErr) ||
		errors.As(err, &authErr)
}

// IsConnectionError checks if an error is a ConnectionError.
func IsConnectionError(err error) bool {
	var connErr *ConnectionError

	return errors.As(err, &connErr)
}

// IsSSLError checks if an error is an SSLError.
func IsSSLError(err error) bool {
	var sslErr *SSLError

	return errors.As(err, &sslErr)
}

// IsTimeoutError checks if an error is a TimeoutError.
func IsTimeoutError(err error) bool {
	var timeoutErr *TimeoutError

	return errors.As(err, &timeoutErr)
}

// IsTFARequired checks if an error indicates TFA is required.
func IsTFARequired(err error) bool {
	var tfaErr *TFARequiredError

	return errors.As(err, &tfaErr)
}
