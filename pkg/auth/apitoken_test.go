package auth_test

import (
	"testing"

	"github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/auth"
)

func TestNewAPITokenAuthenticator(t *testing.T) {
	t.Parallel()

	token := &auth.Token{
		ID:     "root@pam!mytoken",
		Secret: "secret-value",
	}

	authenticator := auth.NewAPITokenAuthenticator(token, "")
	if authenticator == nil {
		t.Fatal("NewAPITokenAuthenticator returned nil")
	}

	if authenticator.GetToken() != token {
		t.Errorf("NewAPITokenAuthenticator() token = %v, want %v", authenticator.GetToken(), token)
	}
}

type tokenFromStringTest struct {
	name        string
	tokenString string
	wantErr     bool
	wantID      string
	wantSecret  string
}

func getTokenFromStringTestCases() []tokenFromStringTest {
	return []tokenFromStringTest{
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
}

func runTokenFromStringTest(t *testing.T, testCase tokenFromStringTest) {
	t.Helper()

	authenticator, err := auth.NewAPITokenAuthenticatorFromString(testCase.tokenString)

	if testCase.wantErr {
		if err == nil {
			t.Errorf("NewAPITokenAuthenticatorFromString() expected error, got nil")
		}

		return
	}

	if err != nil {
		t.Errorf("NewAPITokenAuthenticatorFromString() unexpected error = %v", err)

		return
	}

	if authenticator == nil {
		t.Fatal("NewAPITokenAuthenticatorFromString() returned nil authenticator")
	}

	token := authenticator.GetToken()
	if token.ID != testCase.wantID {
		t.Errorf("token.ID = %v, want %v", token.ID, testCase.wantID)
	}

	if token.Secret != testCase.wantSecret {
		t.Errorf("token.Secret = %v, want %v", token.Secret, testCase.wantSecret)
	}
}

func TestNewAPITokenAuthenticatorFromString(t *testing.T) {
	t.Parallel()

	tests := getTokenFromStringTestCases()

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runTokenFromStringTest(t, testCase)
		})
	}
}

func TestAPITokenAuthenticator_Authenticate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		token   *auth.Token
		wantErr bool
	}{
		{
			name: "valid token",
			token: &auth.Token{
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
			token: &auth.Token{
				ID:     "",
				Secret: "secret",
			},
			wantErr: true,
		},
		{
			name: "empty secret",
			token: &auth.Token{
				ID:     "root@pam!mytoken",
				Secret: "",
			},
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			authenticator := auth.NewAPITokenAuthenticator(testCase.token, "")

			err := authenticator.Authenticate()
			if testCase.wantErr {
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
	t.Parallel()

	tests := []struct {
		name     string
		token    *auth.Token
		expected bool
	}{
		{
			name: "valid token",
			token: &auth.Token{
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
			token: &auth.Token{
				ID:     "",
				Secret: "secret",
			},
			expected: false,
		},
		{
			name: "empty secret",
			token: &auth.Token{
				ID:     "root@pam!mytoken",
				Secret: "",
			},
			expected: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			authenticator := auth.NewAPITokenAuthenticator(testCase.token, "")

			result := authenticator.IsAuthenticated()
			if result != testCase.expected {
				t.Errorf("IsAuthenticated() = %v, want %v", result, testCase.expected)
			}
		})
	}
}

func TestAPITokenAuthenticator_GetHeaders(t *testing.T) {
	t.Parallel()

	token := &auth.Token{
		ID:     "root@pam!mytoken",
		Secret: "secret-value",
	}

	authenticator := auth.NewAPITokenAuthenticator(token, "")
	headers := authenticator.GetHeaders()

	expected := "PVEAPIToken=root@pam!mytoken=secret-value"
	if headers["Authorization"] != expected {
		t.Errorf("GetHeaders()[Authorization] = %v, want %v", headers["Authorization"], expected)
	}

	// Test with unauthenticated
	authenticator = auth.NewAPITokenAuthenticator(nil, "")

	headers = authenticator.GetHeaders()
	if headers != nil {
		t.Errorf("GetHeaders() with nil token = %v, want nil", headers)
	}
}

type parseAPITokenTest struct {
	name       string
	tokenStr   string
	wantErr    bool
	wantID     string
	wantSecret string
}

func getParseAPITokenTestCases() []parseAPITokenTest {
	return []parseAPITokenTest{
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
}

func runParseAPITokenTest(t *testing.T, testCase parseAPITokenTest) {
	t.Helper()

	token, err := auth.ParseAPIToken(testCase.tokenStr)

	if testCase.wantErr {
		if err == nil {
			t.Errorf("ParseAPIToken() expected error, got nil")
		}

		return
	}

	if err != nil {
		t.Errorf("ParseAPIToken() unexpected error = %v", err)

		return
	}

	if token.ID != testCase.wantID {
		t.Errorf("ParseAPIToken() ID = %v, want %v", token.ID, testCase.wantID)
	}

	if token.Secret != testCase.wantSecret {
		t.Errorf("ParseAPIToken() Secret = %v, want %v", token.Secret, testCase.wantSecret)
	}
}

func TestParseAPIToken(t *testing.T) {
	t.Parallel()

	tests := getParseAPITokenTestCases()

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runParseAPITokenTest(t, testCase)
		})
	}
}

func TestFormatAPIToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    *auth.Token
		expected string
	}{
		{
			name: "valid token",
			token: &auth.Token{
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

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := auth.FormatAPIToken(testCase.token)
			if result != testCase.expected {
				t.Errorf("FormatAPIToken() = %v, want %v", result, testCase.expected)
			}
		})
	}
}

func TestValidateTokenID(t *testing.T) {
	t.Parallel()

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

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := auth.ValidateTokenID(testCase.id)
			if testCase.wantErr {
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

func TestAPITokenAuthenticator_GetHeaders_FormatDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		tokenID          string
		tokenSecret      string
		customTokenName  string
		expectedAuthHeader string
		description      string
	}{
		{
			name:             "default token name with raw token",
			tokenID:          "root@pam!mytoken",
			tokenSecret:      "secret-value",
			customTokenName:  "",
			expectedAuthHeader: "PVEAPIToken=root@pam!mytoken=secret-value",
			description:      "Should add PVEAPIToken= prefix to raw token",
		},
		{
			name:             "custom token name",
			tokenID:          "root@pam!mytoken",
			tokenSecret:      "secret-value",
			customTokenName:  "CustomAuth",
			expectedAuthHeader: "CustomAuth=root@pam!mytoken=secret-value",
			description:      "Should use custom token name prefix",
		},
		{
			name:             "bearer token format",
			tokenID:          "root@pam!mytoken",
			tokenSecret:      "secret-value",
			customTokenName:  "Bearer",
			expectedAuthHeader: "Bearer=root@pam!mytoken=secret-value",
			description:      "Should support Bearer token format",
		},
		{
			name:             "pre-formatted token should not double-prefix",
			tokenID:          "PVEAPIToken=user@pam!token",
			tokenSecret:      "secret",
			customTokenName:  "PVEAPIToken",
			expectedAuthHeader: "PVEAPIToken=user@pam!token=secret",
			description:      "Already formatted tokens should not be double-prefixed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			token := &auth.Token{
				ID:     tt.tokenID,
				Secret: tt.tokenSecret,
			}

			authenticator := auth.NewAPITokenAuthenticator(token, tt.customTokenName)
			headers := authenticator.GetHeaders()

			if headers == nil {
				t.Fatal("GetHeaders() returned nil")
			}

			authHeader, ok := headers["Authorization"]
			if !ok {
				t.Fatal("GetHeaders() missing Authorization header")
			}

			if authHeader != tt.expectedAuthHeader {
				t.Errorf("GetHeaders()[Authorization] = %v, want %v. %s",
					authHeader, tt.expectedAuthHeader, tt.description)
			}
		})
	}
}

func TestAPITokenAuthenticator_CustomTokenName(t *testing.T) {
	t.Parallel()

	token := &auth.Token{
		ID:     "root@pam!mytoken",
		Secret: "secret-value",
	}

	tests := []struct {
		name          string
		tokenName     string
		expectedPrefix string
	}{
		{
			name:          "empty token name defaults to PVEAPIToken",
			tokenName:     "",
			expectedPrefix: "PVEAPIToken=",
		},
		{
			name:          "explicit PVEAPIToken",
			tokenName:     "PVEAPIToken",
			expectedPrefix: "PVEAPIToken=",
		},
		{
			name:          "custom Bearer token",
			tokenName:     "Bearer",
			expectedPrefix: "Bearer=",
		},
		{
			name:          "custom API token",
			tokenName:     "X-API-Token",
			expectedPrefix: "X-API-Token=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			authenticator := auth.NewAPITokenAuthenticator(token, tt.tokenName)
			headers := authenticator.GetHeaders()

			authHeader := headers["Authorization"]
			if !hasPrefix(authHeader, tt.expectedPrefix) {
				t.Errorf("Authorization header %q does not start with expected prefix %q",
					authHeader, tt.expectedPrefix)
			}
		})
	}
}

// hasPrefix checks if a string has the given prefix
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
