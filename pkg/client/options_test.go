package client

import (
	"testing"
	"time"
)

func TestOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with username and password",
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
			name: "valid with ticket",
			opts: Options{
				Host:   "pve.example.com",
				Ticket: "PVE:ticket:data",
			},
			wantErr: false,
		},
		{
			name:    "missing host",
			opts:    Options{},
			wantErr: true,
			errMsg:  "host is required",
		},
		{
			name: "missing authentication",
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
				Protocol: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid protocol",
		},
		{
			name: "negative port",
			opts: Options{
				Host:     "pve.example.com",
				Username: "root@pam",
				Password: "secret",
				Port:     -1,
			},
			wantErr: true,
			errMsg:  "invalid port",
		},
		{
			name: "port too high",
			opts: Options{
				Host:     "pve.example.com",
				Username: "root@pam",
				Password: "secret",
				Port:     70000,
			},
			wantErr: true,
			errMsg:  "invalid port",
		},
		{
			name: "client cert without key",
			opts: Options{
				Host:     "pve.example.com",
				Username: "root@pam",
				Password: "secret",
				SSLOptions: &SSLOptions{
					ClientCert: "/path/to/cert",
				},
			},
			wantErr: true,
			errMsg:  "client key required when client certificate is specified",
		},
		{
			name: "client key without cert",
			opts: Options{
				Host:     "pve.example.com",
				Username: "root@pam",
				Password: "secret",
				SSLOptions: &SSLOptions{
					ClientKey: "/path/to/key",
				},
			},
			wantErr: true,
			errMsg:  "client certificate required when client key is specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want containing %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestOptions_setDefaults(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		expected Options
	}{
		{
			name: "empty options",
			opts: Options{},
			expected: Options{
				Protocol:           "https",
				Port:               8006,
				Timeout:            30 * time.Second,
				KeepAlive:          10,
				CookieName:         "PVEAuthCookie",
				CachedFingerprints: map[string]bool{},
				SSLOptions: &SSLOptions{
					VerifyMode:     SSLVerifyPeer,
					VerifyHostname: true,
				},
			},
		},
		{
			name: "http protocol",
			opts: Options{
				Protocol: "http",
			},
			expected: Options{
				Protocol:           "http",
				Port:               8006,
				Timeout:            30 * time.Second,
				KeepAlive:          10,
				CookieName:         "PVEAuthCookie",
				CachedFingerprints: map[string]bool{},
			},
		},
		{
			name: "custom values preserved",
			opts: Options{
				Protocol:   "https",
				Port:       443,
				Timeout:    60 * time.Second,
				KeepAlive:  20,
				CookieName: "CustomCookie",
			},
			expected: Options{
				Protocol:           "https",
				Port:               443,
				Timeout:            60 * time.Second,
				KeepAlive:          20,
				CookieName:         "CustomCookie",
				CachedFingerprints: map[string]bool{},
				SSLOptions: &SSLOptions{
					VerifyMode:     SSLVerifyPeer,
					VerifyHostname: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			opts.setDefaults()

			if opts.Protocol != tt.expected.Protocol {
				t.Errorf("setDefaults() Protocol = %v, want %v", opts.Protocol, tt.expected.Protocol)
			}
			if opts.Port != tt.expected.Port {
				t.Errorf("setDefaults() Port = %v, want %v", opts.Port, tt.expected.Port)
			}
			if opts.Timeout != tt.expected.Timeout {
				t.Errorf("setDefaults() Timeout = %v, want %v", opts.Timeout, tt.expected.Timeout)
			}
			if opts.KeepAlive != tt.expected.KeepAlive {
				t.Errorf("setDefaults() KeepAlive = %v, want %v", opts.KeepAlive, tt.expected.KeepAlive)
			}
			if opts.CookieName != tt.expected.CookieName {
				t.Errorf("setDefaults() CookieName = %v, want %v", opts.CookieName, tt.expected.CookieName)
			}
			if opts.CachedFingerprints == nil {
				t.Errorf("setDefaults() CachedFingerprints is nil")
			}
		})
	}
}

func TestOptions_GetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		expected string
	}{
		{
			name: "https default port",
			opts: Options{
				Protocol: "https",
				Host:     "pve.example.com",
				Port:     8006,
			},
			expected: "https://pve.example.com:8006/api2/json",
		},
		{
			name: "http custom port",
			opts: Options{
				Protocol: "http",
				Host:     "192.168.1.100",
				Port:     8080,
			},
			expected: "http://192.168.1.100:8080/api2/json",
		},
		{
			name: "https standard port",
			opts: Options{
				Protocol: "https",
				Host:     "pve.example.com",
				Port:     443,
			},
			expected: "https://pve.example.com:443/api2/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.GetBaseURL()
			if result != tt.expected {
				t.Errorf("GetBaseURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestOptions_AuthenticationMethods(t *testing.T) {
	t.Run("IsUsingAPIToken", func(t *testing.T) {
		opts := Options{APIToken: "token"}
		if !opts.IsUsingAPIToken() {
			t.Errorf("IsUsingAPIToken() = false, want true")
		}

		opts = Options{}
		if opts.IsUsingAPIToken() {
			t.Errorf("IsUsingAPIToken() = true, want false")
		}
	})

	t.Run("IsUsingTicket", func(t *testing.T) {
		opts := Options{Ticket: "ticket"}
		if !opts.IsUsingTicket() {
			t.Errorf("IsUsingTicket() = false, want true")
		}

		opts = Options{}
		if opts.IsUsingTicket() {
			t.Errorf("IsUsingTicket() = true, want false")
		}
	})

	t.Run("NeedsLogin", func(t *testing.T) {
		tests := []struct {
			name     string
			opts     Options
			expected bool
		}{
			{
				name:     "needs login with username",
				opts:     Options{Username: "root@pam"},
				expected: true,
			},
			{
				name:     "no login with API token",
				opts:     Options{APIToken: "token"},
				expected: false,
			},
			{
				name:     "no login with ticket",
				opts:     Options{Ticket: "ticket"},
				expected: false,
			},
			{
				name:     "no login without credentials",
				opts:     Options{},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := tt.opts.NeedsLogin()
				if result != tt.expected {
					t.Errorf("NeedsLogin() = %v, want %v", result, tt.expected)
				}
			})
		}
	})
}
