package errors

import (
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      APIError
		expected string
	}{
		{
			name: "simple message",
			err: APIError{
				Message: "Something went wrong",
				Code:    500,
			},
			expected: "Something went wrong (code: 500)",
		},
		{
			name: "with errors map",
			err: APIError{
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if tt.err.Errors != nil {
				// For errors with map, just check it contains the base message
				if !contains(result, "Validation failed (code: 400, errors:") {
					t.Errorf("Error() = %v, want containing %v", result, tt.expected)
				}
			} else if result != tt.expected {
				t.Errorf("Error() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAPIError_StatusChecks(t *testing.T) {
	tests := []struct {
		name       string
		err        APIError
		isNotFound bool
		isUnauth   bool
		isForbid   bool
	}{
		{
			name:       "404 not found",
			err:        APIError{HTTPCode: 404},
			isNotFound: true,
			isUnauth:   false,
			isForbid:   false,
		},
		{
			name:       "401 unauthorized",
			err:        APIError{HTTPCode: 401},
			isNotFound: false,
			isUnauth:   true,
			isForbid:   false,
		},
		{
			name:       "403 forbidden",
			err:        APIError{HTTPCode: 403},
			isNotFound: false,
			isUnauth:   false,
			isForbid:   true,
		},
		{
			name:       "500 server error",
			err:        APIError{HTTPCode: 500},
			isNotFound: false,
			isUnauth:   false,
			isForbid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := tt.err.IsNotFound(); result != tt.isNotFound {
				t.Errorf("IsNotFound() = %v, want %v", result, tt.isNotFound)
			}
			if result := tt.err.IsUnauthorized(); result != tt.isUnauth {
				t.Errorf("IsUnauthorized() = %v, want %v", result, tt.isUnauth)
			}
			if result := tt.err.IsForbidden(); result != tt.isForbid {
				t.Errorf("IsForbidden() = %v, want %v", result, tt.isForbid)
			}
		})
	}
}

func TestPermissionError_Error(t *testing.T) {
	err := PermissionError{
		APIError: APIError{
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
	err := ParameterError{
		APIError: APIError{
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
	tests := []struct {
		name     string
		err      AuthenticationError
		expected string
	}{
		{
			name:     "simple auth error",
			err:      AuthenticationError{},
			expected: "authentication failed",
		},
		{
			name: "with realm",
			err: AuthenticationError{
				Realm: "pam",
			},
			expected: "authentication failed for realm pam",
		},
		{
			name: "with TFA",
			err: AuthenticationError{
				TFA: true,
			},
			expected: "authentication failed (TFA required)",
		},
		{
			name: "with realm and TFA",
			err: AuthenticationError{
				Realm: "pve",
				TFA:   true,
			},
			expected: "authentication failed for realm pve (TFA required)",
		},
		{
			name: "with message",
			err: AuthenticationError{
				APIError: APIError{
					Message: "invalid credentials",
				},
			},
			expected: "authentication failed: invalid credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTFARequiredError_Error(t *testing.T) {
	err := TFARequiredError{
		Types: []string{"totp", "yubico"},
	}

	expected := "two-factor authentication required (available types: totp, yubico)"
	result := err.Error()
	if result != expected {
		t.Errorf("Error() = %v, want %v", result, expected)
	}
}

func TestConnectionError(t *testing.T) {
	t.Run("Error message", func(t *testing.T) {
		err := ConnectionError{
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
		cause := &APIError{Message: "timeout"}
		err := ConnectionError{
			Host:  "pve.example.com",
			Port:  8006,
			Cause: cause,
		}

		if err.Unwrap() != cause {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), cause)
		}
	})
}

func TestSSLError(t *testing.T) {
	t.Run("Error message", func(t *testing.T) {
		err := SSLError{
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
		cause := &APIError{Message: "expired"}
		err := SSLError{
			Host:  "pve.example.com",
			Cause: cause,
		}

		if err.Unwrap() != cause {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), cause)
		}
	})
}

func TestTimeoutError_Error(t *testing.T) {
	err := TimeoutError{
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
	tests := []struct {
		name       string
		statusCode int
		body       []byte
		wantType   string
	}{
		{
			name:       "401 authentication error",
			statusCode: 401,
			body:       []byte(`{"message": "unauthorized"}`),
			wantType:   "*errors.AuthenticationError",
		},
		{
			name:       "403 permission error",
			statusCode: 403,
			body:       []byte(`{"message": "forbidden"}`),
			wantType:   "*errors.PermissionError",
		},
		{
			name:       "400 parameter error",
			statusCode: 400,
			body:       []byte(`{"message": "bad request"}`),
			wantType:   "*errors.ParameterError",
		},
		{
			name:       "500 generic error",
			statusCode: 500,
			body:       []byte(`{"message": "internal server error"}`),
			wantType:   "*errors.APIError",
		},
		{
			name:       "TFA required",
			statusCode: 401,
			body:       []byte(`{"ticket": "partial", "types": ["totp"]}`),
			wantType:   "*errors.TFARequiredError",
		},
		{
			name:       "invalid JSON",
			statusCode: 500,
			body:       []byte(`not json`),
			wantType:   "*errors.APIError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ParseAPIError(tt.statusCode, tt.body)
			if err == nil {
				t.Errorf("ParseAPIError() returned nil error")
				return
			}

			// Check error type
			gotType := typeOf(err)
			if gotType != tt.wantType {
				t.Errorf("ParseAPIError() returned type %v, want %v", gotType, tt.wantType)
			}
		})
	}
}

func TestIsErrorType(t *testing.T) {
	apiErr := &APIError{Message: "test"}
	permErr := &PermissionError{APIError: APIError{Message: "test"}}
	authErr := &AuthenticationError{APIError: APIError{Message: "test"}}
	connErr := &ConnectionError{Message: "test"}
	sslErr := &SSLError{Message: "test"}
	timeoutErr := &TimeoutError{Operation: "test"}
	tfaErr := &TFARequiredError{Types: []string{"totp"}}

	tests := []struct {
		name     string
		err      error
		checkFn  func(error) bool
		expected bool
	}{
		{"IsAPIError with APIError", apiErr, IsAPIError, true},
		{"IsAPIError with PermissionError", permErr, IsAPIError, true},
		{"IsAPIError with ConnectionError", connErr, IsAPIError, false},
		{"IsConnectionError with ConnectionError", connErr, IsConnectionError, true},
		{"IsConnectionError with APIError", apiErr, IsConnectionError, false},
		{"IsSSLError with SSLError", sslErr, IsSSLError, true},
		{"IsSSLError with APIError", apiErr, IsSSLError, false},
		{"IsTimeoutError with TimeoutError", timeoutErr, IsTimeoutError, true},
		{"IsTimeoutError with APIError", apiErr, IsTimeoutError, false},
		{"IsTFARequired with TFARequiredError", tfaErr, IsTFARequired, true},
		{"IsTFARequired with AuthenticationError", authErr, IsTFARequired, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.checkFn(tt.err)
			if result != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

// Helper functions
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
	case *APIError:
		return "*errors.APIError"
	case *PermissionError:
		return "*errors.PermissionError"
	case *ParameterError:
		return "*errors.ParameterError"
	case *AuthenticationError:
		return "*errors.AuthenticationError"
	case *TFARequiredError:
		return "*errors.TFARequiredError"
	default:
		return "unknown"
	}
}
