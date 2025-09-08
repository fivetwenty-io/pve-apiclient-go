package errors_test

import (
	"errors"
	"testing"

	pveerr "github.com/fivetwenty-io/pve-apiclient-go/pkg/errors"
)

func TestAPIError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      pveerr.APIError
		expected string
	}{
		{
			name: "simple message",
			err: pveerr.APIError{
				Message: "Something went wrong",
				Code:    500,
			},
			expected: "Something went wrong (code: 500)",
		},
		{
			name: "with errors map",
			err: pveerr.APIError{
				Message: "Validation failed",
				Code:    400,
				Errors: map[string]string{
					"username": "required",
					"password": "too short",
				},
			},
			expected: "Validation failed (code: 400, errors:",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := testCase.err.Error()
			if testCase.err.Errors != nil {
				// For errors with map, just check it contains the base message
				if !contains(result, "Validation failed (code: 400, errors:") {
					t.Errorf("Error() = %v, want containing %v", result, testCase.expected)
				}
			} else if result != testCase.expected {
				t.Errorf("Error() = %v, want %v", result, testCase.expected)
			}
		})
	}
}

func TestAPIError_StatusChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        pveerr.APIError
		isNotFound bool
		isUnauth   bool
		isForbid   bool
	}{
		{
			name:       "404 not found",
			err:        pveerr.APIError{HTTPCode: 404},
			isNotFound: true,
			isUnauth:   false,
			isForbid:   false,
		},
		{
			name:       "401 unauthorized",
			err:        pveerr.APIError{HTTPCode: 401},
			isNotFound: false,
			isUnauth:   true,
			isForbid:   false,
		},
		{
			name:       "403 forbidden",
			err:        pveerr.APIError{HTTPCode: 403},
			isNotFound: false,
			isUnauth:   false,
			isForbid:   true,
		},
		{
			name:       "500 server error",
			err:        pveerr.APIError{HTTPCode: 500},
			isNotFound: false,
			isUnauth:   false,
			isForbid:   false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if result := testCase.err.IsNotFound(); result != testCase.isNotFound {
				t.Errorf("IsNotFound() = %v, want %v", result, testCase.isNotFound)
			}

			if result := testCase.err.IsUnauthorized(); result != testCase.isUnauth {
				t.Errorf("IsUnauthorized() = %v, want %v", result, testCase.isUnauth)
			}

			if result := testCase.err.IsForbidden(); result != testCase.isForbid {
				t.Errorf("IsForbidden() = %v, want %v", result, testCase.isForbid)
			}
		})
	}
}

func TestPermissionError_Error(t *testing.T) {
	t.Parallel()

	err := pveerr.PermissionError{
		APIError: pveerr.APIError{
			Message: "access denied",
			Code:    403,
		},
		What: "/vms/100",
	}

	expected := "permission denied for /vms/100: access denied (code: 403)"

	result := err.Error()
	if result != expected {
		t.Errorf("Error() = %v, want %v", result, expected)
	}

	// Test without What field
	err.What = ""
	expected = "permission denied: access denied (code: 403)"

	result = err.Error()
	if result != expected {
		t.Errorf("Error() = %v, want %v", result, expected)
	}
}

func TestParameterError_Error(t *testing.T) {
	t.Parallel()

	err := pveerr.ParameterError{
		APIError: pveerr.APIError{
			Message: "invalid value",
			Code:    400,
		},
		Usage: "must be between 1 and 100",
	}

	expected := "parameter error: invalid value (code: 400) (usage: must be between 1 and 100)"

	result := err.Error()
	if result != expected {
		t.Errorf("Error() = %v, want %v", result, expected)
	}

	// Test without Usage field
	err.Usage = ""
	expected = "parameter error: invalid value (code: 400)"

	result = err.Error()
	if result != expected {
		t.Errorf("Error() = %v, want %v", result, expected)
	}
}

func TestAuthenticationError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      pveerr.AuthenticationError
		expected string
	}{
		{
			name:     "simple auth error",
			err:      pveerr.AuthenticationError{},
			expected: "authentication failed",
		},
		{
			name: "with realm",
			err: pveerr.AuthenticationError{
				Realm: "pam",
			},
			expected: "authentication failed for realm pam",
		},
		{
			name: "with TFA",
			err: pveerr.AuthenticationError{
				TFA: true,
			},
			expected: "authentication failed (TFA required)",
		},
		{
			name: "with realm and TFA",
			err: pveerr.AuthenticationError{
				Realm: "pve",
				TFA:   true,
			},
			expected: "authentication failed for realm pve (TFA required)",
		},
		{
			name: "with message",
			err: pveerr.AuthenticationError{
				APIError: pveerr.APIError{
					Message: "invalid credentials",
				},
			},
			expected: "authentication failed: invalid credentials",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := testCase.err.Error()
			if result != testCase.expected {
				t.Errorf("Error() = %v, want %v", result, testCase.expected)
			}
		})
	}
}

func TestTFARequiredError_Error(t *testing.T) {
	t.Parallel()

	err := pveerr.TFARequiredError{
		Types: []string{"totp", "yubico"},
	}

	expected := "two-factor authentication required (available types: totp, yubico)"

	result := err.Error()
	if result != expected {
		t.Errorf("Error() = %v, want %v", result, expected)
	}
}

func TestConnectionError(t *testing.T) {
	t.Parallel()

	t.Run("Error message", func(t *testing.T) {
		t.Parallel()

		err := pveerr.ConnectionError{
			Host:    "pve.example.com",
			Port:    8006,
			Message: "connection refused",
		}

		expected := "connection to pve.example.com:8006 failed: connection refused"

		result := err.Error()
		if result != expected {
			t.Errorf("Error() = %v, want %v", result, expected)
		}
	})

	t.Run("With cause", func(t *testing.T) {
		t.Parallel()

		cause := &pveerr.APIError{Message: "timeout"}
		err := pveerr.ConnectionError{
			Host:  "pve.example.com",
			Port:  8006,
			Cause: cause,
		}

		if !errors.Is(err.Unwrap(), cause) {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), cause)
		}
	})
}

func TestSSLError(t *testing.T) {
	t.Parallel()

	t.Run("Error message", func(t *testing.T) {
		t.Parallel()

		err := pveerr.SSLError{
			Host:        "pve.example.com",
			Fingerprint: "AA:BB:CC:DD",
			Message:     "certificate verification failed",
		}

		expected := "SSL error for pve.example.com (fingerprint: AA:BB:CC:DD): certificate verification failed"

		result := err.Error()
		if result != expected {
			t.Errorf("Error() = %v, want %v", result, expected)
		}
	})

	t.Run("With cause", func(t *testing.T) {
		t.Parallel()

		cause := &pveerr.APIError{Message: "expired"}
		err := pveerr.SSLError{
			Host:  "pve.example.com",
			Cause: cause,
		}

		if !errors.Is(err.Unwrap(), cause) {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), cause)
		}
	})
}

func TestTimeoutError_Error(t *testing.T) {
	t.Parallel()

	err := pveerr.TimeoutError{
		Operation: "login",
		Duration:  "30s",
	}

	expected := "operation login timed out after 30s"

	result := err.Error()
	if result != expected {
		t.Errorf("Error() = %v, want %v", result, expected)
	}
}

func TestParseAPIError(t *testing.T) {
	t.Parallel()

	tests := getParseAPIErrorTestCases()
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runParseAPIErrorTest(t, testCase)
		})
	}
}

type parseAPIErrorTestCase struct {
	name       string
	statusCode int
	body       []byte
	wantType   string
}

func getParseAPIErrorTestCases() []parseAPIErrorTestCase {
	return []parseAPIErrorTestCase{
		{
			name:       "401 authentication error",
			statusCode: 401,
			body:       []byte(`{"message": "unauthorized"}`),
			wantType:   "*pveerr.AuthenticationError",
		},
		{
			name:       "403 permission error",
			statusCode: 403,
			body:       []byte(`{"message": "forbidden"}`),
			wantType:   "*pveerr.PermissionError",
		},
		{
			name:       "400 parameter error",
			statusCode: 400,
			body:       []byte(`{"message": "bad request"}`),
			wantType:   "*pveerr.ParameterError",
		},
		{
			name:       "500 generic error",
			statusCode: 500,
			body:       []byte(`{"message": "internal server error"}`),
			wantType:   "*pveerr.APIError",
		},
		{
			name:       "TFA required",
			statusCode: 401,
			body:       []byte(`{"ticket": "partial", "types": ["totp"]}`),
			wantType:   "*pveerr.TFARequiredError",
		},
		{
			name:       "invalid JSON",
			statusCode: 500,
			body:       []byte(`not json`),
			wantType:   "*pveerr.APIError",
		},
	}
}

func runParseAPIErrorTest(t *testing.T, testCase parseAPIErrorTestCase) {
	t.Helper()

	err := pveerr.ParseAPIError(testCase.statusCode, testCase.body)
	if err == nil {
		t.Errorf("ParseAPIError() returned nil error")

		return
	}

	// Check error type
	gotType := typeOf(err)
	if gotType != testCase.wantType {
		t.Errorf("ParseAPIError() returned type %v, want %v", gotType, testCase.wantType)
	}
}

func TestIsErrorType(t *testing.T) {
	t.Parallel()

	apiErr := &pveerr.APIError{Message: "test"}
	permErr := &pveerr.PermissionError{APIError: pveerr.APIError{Message: "test"}}
	authErr := &pveerr.AuthenticationError{APIError: pveerr.APIError{Message: "test"}}
	connErr := &pveerr.ConnectionError{Message: "test"}
	sslErr := &pveerr.SSLError{Message: "test"}
	timeoutErr := &pveerr.TimeoutError{Operation: "test"}
	tfaErr := &pveerr.TFARequiredError{Types: []string{"totp"}}

	tests := []struct {
		name     string
		err      error
		checkFn  func(error) bool
		expected bool
	}{
		{"IsAPIError with pveerr.APIError", apiErr, pveerr.IsAPIError, true},
		{"IsAPIError with pveerr.PermissionError", permErr, pveerr.IsAPIError, true},
		{"IsAPIError with ConnectionError", connErr, pveerr.IsAPIError, false},
		{"IsConnectionError with ConnectionError", connErr, pveerr.IsConnectionError, true},
		{"IsConnectionError with pveerr.APIError", apiErr, pveerr.IsConnectionError, false},
		{"IsSSLError with SSLError", sslErr, pveerr.IsSSLError, true},
		{"IsSSLError with pveerr.APIError", apiErr, pveerr.IsSSLError, false},
		{"IsTimeoutError with TimeoutError", timeoutErr, pveerr.IsTimeoutError, true},
		{"IsTimeoutError with pveerr.APIError", apiErr, pveerr.IsTimeoutError, false},
		{"IsTFARequired with TFARequiredError", tfaErr, pveerr.IsTFARequired, true},
		{"IsTFARequired with AuthenticationError", authErr, pveerr.IsTFARequired, false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := testCase.checkFn(testCase.err)
			if result != testCase.expected {
				t.Errorf("%s = %v, want %v", testCase.name, result, testCase.expected)
			}
		})
	}
}

// Helper functions.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func typeOf(v interface{}) string {
	if v == nil {
		return "nil"
	}

	switch v.(type) {
	case *pveerr.APIError:
		return "*pveerr.APIError"
	case *pveerr.PermissionError:
		return "*pveerr.PermissionError"
	case *pveerr.ParameterError:
		return "*pveerr.ParameterError"
	case *pveerr.AuthenticationError:
		return "*pveerr.AuthenticationError"
	case *pveerr.TFARequiredError:
		return "*pveerr.TFARequiredError"
	default:
		return "unknown"
	}
}
