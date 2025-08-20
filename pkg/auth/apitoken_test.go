package auth

import (
	"testing"
)

func TestNewAPITokenAuthenticator(t *testing.T) {
	token := &Token{
		ID:     "root@pam!mytoken",
		Secret: "secret-value",
	}

	auth := NewAPITokenAuthenticator(token)
	if auth == nil {
		t.Fatal("NewAPITokenAuthenticator returned nil")
	}

	if auth.token != token {
		t.Errorf("NewAPITokenAuthenticator() token = %v, want %v", auth.token, token)
	}
}

func TestNewAPITokenAuthenticatorFromString(t *testing.T) {
	tests := []struct {
		name        string
		tokenString string
		wantErr     bool
		wantID      string
		wantSecret  string
	}{
		{
			name:        "valid token",
			tokenString: "root@pam!mytoken=secret-value",
			wantErr:     false,
			wantID:      "root@pam!mytoken",
			wantSecret:  "secret-value",
		},
		{
			name:        "empty string",
			tokenString: "",
			wantErr:     true,
		},
		{
			name:        "missing secret",
			tokenString: "root@pam!mytoken",
			wantErr:     true,
		},
		{
			name:        "invalid format",
			tokenString: "invalid-token-format",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewAPITokenAuthenticatorFromString(tt.tokenString)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewAPITokenAuthenticatorFromString() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewAPITokenAuthenticatorFromString() unexpected error = %v", err)
				}
				if auth == nil {
					t.Fatal("NewAPITokenAuthenticatorFromString() returned nil authenticator")
				}
				if auth.token.ID != tt.wantID {
					t.Errorf("token.ID = %v, want %v", auth.token.ID, tt.wantID)
				}
				if auth.token.Secret != tt.wantSecret {
					t.Errorf("token.Secret = %v, want %v", auth.token.Secret, tt.wantSecret)
				}
			}
		})
	}
}

func TestAPITokenAuthenticator_Authenticate(t *testing.T) {
	tests := []struct {
		name    string
		token   *Token
		wantErr bool
	}{
		{
			name: "valid token",
			token: &Token{
				ID:     "root@pam!mytoken",
				Secret: "secret",
			},
			wantErr: false,
		},
		{
			name:    "nil token",
			token:   nil,
			wantErr: true,
		},
		{
			name: "empty ID",
			token: &Token{
				ID:     "",
				Secret: "secret",
			},
			wantErr: true,
		},
		{
			name: "empty secret",
			token: &Token{
				ID:     "root@pam!mytoken",
				Secret: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewAPITokenAuthenticator(tt.token)
			err := auth.Authenticate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Authenticate() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Authenticate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestAPITokenAuthenticator_IsAuthenticated(t *testing.T) {
	tests := []struct {
		name     string
		token    *Token
		expected bool
	}{
		{
			name: "valid token",
			token: &Token{
				ID:     "root@pam!mytoken",
				Secret: "secret",
			},
			expected: true,
		},
		{
			name:     "nil token",
			token:    nil,
			expected: false,
		},
		{
			name: "empty ID",
			token: &Token{
				ID:     "",
				Secret: "secret",
			},
			expected: false,
		},
		{
			name: "empty secret",
			token: &Token{
				ID:     "root@pam!mytoken",
				Secret: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewAPITokenAuthenticator(tt.token)
			result := auth.IsAuthenticated()
			if result != tt.expected {
				t.Errorf("IsAuthenticated() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAPITokenAuthenticator_GetHeaders(t *testing.T) {
	token := &Token{
		ID:     "root@pam!mytoken",
		Secret: "secret-value",
	}

	auth := NewAPITokenAuthenticator(token)
	headers := auth.GetHeaders()

	expected := "PVEAPIToken=root@pam!mytoken=secret-value"
	if headers["Authorization"] != expected {
		t.Errorf("GetHeaders()[Authorization] = %v, want %v", headers["Authorization"], expected)
	}

	// Test with unauthenticated
	auth = NewAPITokenAuthenticator(nil)
	headers = auth.GetHeaders()
	if headers != nil {
		t.Errorf("GetHeaders() with nil token = %v, want nil", headers)
	}
}

func TestParseAPIToken(t *testing.T) {
	tests := []struct {
		name       string
		tokenStr   string
		wantErr    bool
		wantID     string
		wantSecret string
	}{
		{
			name:       "valid token",
			tokenStr:   "root@pam!mytoken=secret-value",
			wantErr:    false,
			wantID:     "root@pam!mytoken",
			wantSecret: "secret-value",
		},
		{
			name:     "empty string",
			tokenStr: "",
			wantErr:  true,
		},
		{
			name:     "missing equals",
			tokenStr: "root@pam!mytoken",
			wantErr:  true,
		},
		{
			name:     "missing realm",
			tokenStr: "root!mytoken=secret",
			wantErr:  true,
		},
		{
			name:     "missing token name",
			tokenStr: "root@pam=secret",
			wantErr:  true,
		},
		{
			name:     "empty secret",
			tokenStr: "root@pam!mytoken=",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ParseAPIToken(tt.tokenStr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseAPIToken() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ParseAPIToken() unexpected error = %v", err)
				}
				if token.ID != tt.wantID {
					t.Errorf("ParseAPIToken() ID = %v, want %v", token.ID, tt.wantID)
				}
				if token.Secret != tt.wantSecret {
					t.Errorf("ParseAPIToken() Secret = %v, want %v", token.Secret, tt.wantSecret)
				}
			}
		})
	}
}

func TestFormatAPIToken(t *testing.T) {
	tests := []struct {
		name     string
		token    *Token
		expected string
	}{
		{
			name: "valid token",
			token: &Token{
				ID:     "root@pam!mytoken",
				Secret: "secret-value",
			},
			expected: "root@pam!mytoken=secret-value",
		},
		{
			name:     "nil token",
			token:    nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatAPIToken(tt.token)
			if result != tt.expected {
				t.Errorf("FormatAPIToken() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateTokenID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid ID",
			id:      "root@pam!mytoken",
			wantErr: false,
		},
		{
			name:    "empty ID",
			id:      "",
			wantErr: true,
		},
		{
			name:    "missing @",
			id:      "rootpam!mytoken",
			wantErr: true,
		},
		{
			name:    "missing !",
			id:      "root@pammytoken",
			wantErr: true,
		},
		{
			name:    "empty user",
			id:      "@pam!mytoken",
			wantErr: true,
		},
		{
			name:    "empty realm",
			id:      "root@!mytoken",
			wantErr: true,
		},
		{
			name:    "empty token name",
			id:      "root@pam!",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTokenID(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateTokenID() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ValidateTokenID() unexpected error = %v", err)
				}
			}
		})
	}
}
