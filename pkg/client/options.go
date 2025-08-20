package client

import (
	"crypto/x509"
	"errors"
	"fmt"
	"time"
)

// SSLVerifyMode defines how SSL certificates should be verified.
type SSLVerifyMode int

const (
	// SSLVerifyNone disables SSL verification (insecure).
	SSLVerifyNone SSLVerifyMode = iota
	// SSLVerifyPeer verifies the peer certificate.
	SSLVerifyPeer
	// SSLVerifyHost verifies the hostname matches the certificate.
	SSLVerifyHost
	// SSLVerifyFull performs full SSL verification.
	SSLVerifyFull
)

// Options contains configuration options for the PVE client.
type Options struct {
	// Connection
	Host     string // Hostname or IP address of the PVE server
	Port     int    // Port number (default: 8006)
	Protocol string // Protocol to use: "http" or "https" (default: "https")

	// Authentication
	Username  string // Username for authentication (e.g., "root@pam")
	Password  string // Password for authentication
	APIToken  string // API token for authentication (alternative to username/password)
	Ticket    string // Authentication ticket (obtained after login)
	CSRFToken string // CSRF prevention token

	// SSL/TLS
	SSLOptions                  *SSLOptions                  // SSL/TLS configuration
	CachedFingerprints          map[string]bool              // Cached certificate fingerprints
	ManualVerification          bool                         // Enable manual certificate verification
	RegisterFingerprintCallback func(string)                 // Callback for registering new fingerprints
	VerifyFingerprintCallback   func(*x509.Certificate) bool // Callback for verifying certificates

	// HTTP Client
	Timeout   time.Duration // Request timeout (default: 30s)
	KeepAlive int           // Number of keep-alive connections (default: 10)

	// Misc
	CookieName   string // Name of the authentication cookie (default: "PVEAuthCookie")
	PVENewFormat bool   // Use new PVE format for certain operations
}

// SSLOptions contains SSL/TLS specific configuration.
type SSLOptions struct {
	VerifyHostname bool          // Verify that the hostname matches the certificate
	VerifyMode     SSLVerifyMode // SSL verification mode
	CACert         string        // Path to CA certificate file
	ClientCert     string        // Path to client certificate file
	ClientKey      string        // Path to client key file
}

// Validate checks if the options are valid.
func (o *Options) Validate() error {
	if o.Host == "" {
		return errors.New("host is required")
	}

	if o.Username == "" && o.APIToken == "" && o.Ticket == "" {
		return errors.New("authentication credentials required (username/password, API token, or ticket)")
	}

	if o.Username != "" && o.Password == "" && o.Ticket == "" {
		return errors.New("password required when using username authentication")
	}

	if o.Protocol != "" && o.Protocol != "http" && o.Protocol != "https" {
		return fmt.Errorf("invalid protocol: %s (must be 'http' or 'https')", o.Protocol)
	}

	if o.Port < 0 || o.Port > 65535 {
		return fmt.Errorf("invalid port: %d", o.Port)
	}

	if o.SSLOptions != nil {
		if o.SSLOptions.ClientCert != "" && o.SSLOptions.ClientKey == "" {
			return errors.New("client key required when client certificate is specified")
		}
		if o.SSLOptions.ClientKey != "" && o.SSLOptions.ClientCert == "" {
			return errors.New("client certificate required when client key is specified")
		}
	}

	return nil
}

// setDefaults sets default values for unspecified options.
func (o *Options) setDefaults() {
	if o.Protocol == "" {
		o.Protocol = "https"
	}

	if o.Port == 0 {
		if o.Protocol == "https" {
			o.Port = 8006
		} else {
			o.Port = 8006 // PVE uses 8006 for both HTTP and HTTPS by default
		}
	}

	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}

	if o.KeepAlive == 0 {
		o.KeepAlive = 10
	}

	if o.CookieName == "" {
		o.CookieName = "PVEAuthCookie"
	}

	if o.CachedFingerprints == nil {
		o.CachedFingerprints = make(map[string]bool)
	}

	if o.SSLOptions == nil && o.Protocol == "https" {
		o.SSLOptions = &SSLOptions{
			VerifyMode:     SSLVerifyPeer,
			VerifyHostname: true,
		}
	}
}

// GetBaseURL returns the base URL for API requests.
func (o *Options) GetBaseURL() string {
	return fmt.Sprintf("%s://%s:%d/api2/json", o.Protocol, o.Host, o.Port)
}

// IsUsingAPIToken returns true if API token authentication is being used.
func (o *Options) IsUsingAPIToken() bool {
	return o.APIToken != ""
}

// IsUsingTicket returns true if ticket authentication is being used.
func (o *Options) IsUsingTicket() bool {
	return o.Ticket != ""
}

// NeedsLogin returns true if login is required.
func (o *Options) NeedsLogin() bool {
	return !o.IsUsingAPIToken() && !o.IsUsingTicket() && o.Username != ""
}
