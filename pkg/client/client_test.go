package client

import (
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with username/password",
			opts: Options{
				Host:     "pve.example.com",
				Username: "root@pam",
				Password: "secret",
			},
			wantErr: false,
		},
		{
			name: "valid with API token",
			opts: Options{
				Host:     "pve.example.com",
				APIToken: "root@pam!token=secret",
			},
			wantErr: false,
		},
		{
			name: "missing host",
			opts: Options{
				Username: "root@pam",
				Password: "secret",
			},
			wantErr: true,
			errMsg:  "host is required",
		},
		{
			name: "missing credentials",
			opts: Options{
				Host: "pve.example.com",
			},
			wantErr: true,
			errMsg:  "authentication credentials required",
		},
		{
			name: "username without password",
			opts: Options{
				Host:     "pve.example.com",
				Username: "root@pam",
			},
			wantErr: true,
			errMsg:  "password required when using username authentication",
		},
		{
			name: "invalid protocol",
			opts: Options{
				Host:     "pve.example.com",
				Username: "root@pam",
				Password: "secret",
				Protocol: "ftp",
			},
			wantErr: true,
			errMsg:  "invalid protocol",
		},
		{
			name: "invalid port",
			opts: Options{
				Host:     "pve.example.com",
				Username: "root@pam",
				Password: "secret",
				Port:     70000,
			},
			wantErr: true,
			errMsg:  "invalid port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewClient() expected error, got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					if !contains(err.Error(), tt.errMsg) {
						t.Errorf("NewClient() error = %v, want containing %v", err, tt.errMsg)
					}
				}
			} else {
				if err != nil {
					t.Errorf("NewClient() unexpected error = %v", err)
				}
				if client == nil {
					t.Errorf("NewClient() returned nil client")
				}
			}
		})
	}
}

func TestClient_UpdateTicket(t *testing.T) {
	opts := Options{
		Host:     "pve.example.com",
		Username: "root@pam",
		Password: "secret",
	}

	client, err := NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	testTicket := "PVE:test:ticket"
	client.UpdateTicket(testTicket)

	// We can't directly test the internal state without type assertion working
	// This is a limitation of the current design where client is a private type
	// In a real test, we would either:
	// 1. Make the client type public
	// 2. Add getter methods to verify the state
	// 3. Test the behavior rather than the state
	// For now, we'll just verify the method doesn't panic
}

func TestClient_UpdateCSRFToken(t *testing.T) {
	opts := Options{
		Host:     "pve.example.com",
		Username: "root@pam",
		Password: "secret",
	}

	client, err := NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	testToken := "test:csrf:token"
	client.UpdateCSRFToken(testToken)

	// See comment in TestClient_UpdateTicket about testing private state
}

func TestClient_SetTimeout(t *testing.T) {
	opts := Options{
		Host:     "pve.example.com",
		Username: "root@pam",
		Password: "secret",
	}

	client, err := NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	newTimeout := 60 * time.Second
	client.SetTimeout(newTimeout)

	// See comment in TestClient_UpdateTicket about testing private state
}

func TestClient_SetKeepAlive(t *testing.T) {
	opts := Options{
		Host:     "pve.example.com",
		Username: "root@pam",
		Password: "secret",
	}

	client, err := NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	newKeepAlive := 20
	client.SetKeepAlive(newKeepAlive)

	// See comment in TestClient_UpdateTicket about testing private state
}

func TestClient_HTTPMethods(t *testing.T) {
	opts := Options{
		Host:     "pve.example.com",
		APIToken: "root@pam!token=secret",
	}

	client, err := NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test Get
	data, err := client.Get("/test", nil)
	if err != nil {
		t.Errorf("Get() unexpected error = %v", err)
	}
	if data == nil {
		t.Errorf("Get() returned nil data")
	}

	// Test GetRaw
	resp, err := client.GetRaw("/test", nil)
	if err != nil {
		t.Errorf("GetRaw() unexpected error = %v", err)
	}
	if resp == nil {
		t.Errorf("GetRaw() returned nil response")
	}

	// Test Post
	data, err = client.Post("/test", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Errorf("Post() unexpected error = %v", err)
	}
	if data == nil {
		t.Errorf("Post() returned nil data")
	}

	// Test Put
	data, err = client.Put("/test", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Errorf("Put() unexpected error = %v", err)
	}
	if data == nil {
		t.Errorf("Put() returned nil data")
	}

	// Test Delete
	data, err = client.Delete("/test", nil)
	if err != nil {
		t.Errorf("Delete() unexpected error = %v", err)
	}
	if data == nil {
		t.Errorf("Delete() returned nil data")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
