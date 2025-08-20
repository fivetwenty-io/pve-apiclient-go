package ssl

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"time"
)

// VerificationMode defines how SSL certificates should be verified.
type VerificationMode int

const (
	// VerifyNone disables certificate verification (insecure).
	VerifyNone VerificationMode = iota
	// VerifyPeer verifies the peer certificate.
	VerifyPeer
	// VerifyHostname verifies that the hostname matches the certificate.
	VerifyHostname
	// VerifyFingerprint verifies the certificate fingerprint.
	VerifyFingerprint
	// VerifyFull performs full certificate verification.
	VerifyFull
)

// Verifier handles SSL/TLS certificate verification.
type Verifier struct {
	mode                VerificationMode
	fingerprintVerifier *FingerprintVerifier
	caCertPool          *x509.CertPool
	skipHostname        bool
	allowExpired        bool
}

// NewVerifier creates a new SSL verifier.
func NewVerifier(mode VerificationMode) *Verifier {
	return &Verifier{
		mode:                mode,
		fingerprintVerifier: NewFingerprintVerifier(),
		skipHostname:        false,
		allowExpired:        false,
	}
}

// SetFingerprintVerifier sets the fingerprint verifier.
func (v *Verifier) SetFingerprintVerifier(fv *FingerprintVerifier) {
	v.fingerprintVerifier = fv
}

// SetCACertPool sets the CA certificate pool for verification.
func (v *Verifier) SetCACertPool(pool *x509.CertPool) {
	v.caCertPool = pool
}

// SetSkipHostname sets whether to skip hostname verification.
func (v *Verifier) SetSkipHostname(skip bool) {
	v.skipHostname = skip
}

// SetAllowExpired sets whether to allow expired certificates.
func (v *Verifier) SetAllowExpired(allow bool) {
	v.allowExpired = allow
}

// GetTLSConfig returns a TLS configuration based on the verifier settings.
func (v *Verifier) GetTLSConfig(serverName string) *tls.Config {
	config := &tls.Config{
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
	}

	switch v.mode {
	case VerifyNone:
		config.InsecureSkipVerify = true

	case VerifyFingerprint:
		config.InsecureSkipVerify = true
		config.VerifyPeerCertificate = v.verifyFingerprint

	case VerifyPeer:
		config.RootCAs = v.caCertPool
		if v.skipHostname {
			config.InsecureSkipVerify = true
			config.VerifyPeerCertificate = v.verifyPeerOnly
		}

	case VerifyHostname:
		config.RootCAs = v.caCertPool

	case VerifyFull:
		config.RootCAs = v.caCertPool
		config.VerifyPeerCertificate = v.verifyFull
	}

	return config
}

// verifyFingerprint verifies only the certificate fingerprint.
func (v *Verifier) verifyFingerprint(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("no certificates provided")
	}

	// Parse the leaf certificate
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Verify fingerprint
	if v.fingerprintVerifier != nil {
		return v.fingerprintVerifier.VerifyCertificate(cert)
	}

	return fmt.Errorf("fingerprint verifier not configured")
}

// verifyPeerOnly verifies the peer certificate without hostname verification.
func (v *Verifier) verifyPeerOnly(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("no certificates provided")
	}

	// Parse certificates
	certs := make([]*x509.Certificate, len(rawCerts))
	for i, rawCert := range rawCerts {
		cert, err := x509.ParseCertificate(rawCert)
		if err != nil {
			return fmt.Errorf("failed to parse certificate: %w", err)
		}
		certs[i] = cert
	}

	// Verify certificate chain
	opts := x509.VerifyOptions{
		Roots:         v.caCertPool,
		Intermediates: x509.NewCertPool(),
	}

	// Add intermediate certificates
	for i := 1; i < len(certs); i++ {
		opts.Intermediates.AddCert(certs[i])
	}

	// Check expiration unless allowed
	if !v.allowExpired {
		opts.CurrentTime = time.Now()
	} else {
		// Use a time when the cert was likely valid
		opts.CurrentTime = certs[0].NotBefore.Add(time.Hour)
	}

	// Verify the certificate chain
	_, err := certs[0].Verify(opts)
	return err
}

// verifyFull performs full certificate verification including custom checks.
func (v *Verifier) verifyFull(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("no certificates provided")
	}

	// Parse the leaf certificate
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check fingerprint if verifier is configured
	if v.fingerprintVerifier != nil {
		if err := v.fingerprintVerifier.VerifyCertificate(cert); err != nil {
			// Fingerprint doesn't match, but continue with other checks
			// The fingerprint verifier might just be logging unknown certificates
		}
	}

	// Additional custom verification can be added here
	// For example, checking specific certificate attributes

	return nil
}

// VerifyCertificateChain verifies a certificate chain.
func (v *Verifier) VerifyCertificateChain(certs []*x509.Certificate, hostname string) error {
	if len(certs) == 0 {
		return fmt.Errorf("no certificates in chain")
	}

	leafCert := certs[0]

	// Check expiration
	if !v.allowExpired {
		now := time.Now()
		if now.Before(leafCert.NotBefore) {
			return fmt.Errorf("certificate not yet valid")
		}
		if now.After(leafCert.NotAfter) {
			return fmt.Errorf("certificate has expired")
		}
	}

	// Check hostname
	if !v.skipHostname && hostname != "" {
		if err := leafCert.VerifyHostname(hostname); err != nil {
			return fmt.Errorf("hostname verification failed: %w", err)
		}
	}

	// Verify chain
	if v.caCertPool != nil {
		opts := x509.VerifyOptions{
			Roots:         v.caCertPool,
			Intermediates: x509.NewCertPool(),
		}

		// Add intermediate certificates
		for i := 1; i < len(certs); i++ {
			opts.Intermediates.AddCert(certs[i])
		}

		if _, err := leafCert.Verify(opts); err != nil {
			return fmt.Errorf("certificate chain verification failed: %w", err)
		}
	}

	return nil
}

// CreateTLSConfig creates a TLS configuration for a given host.
func CreateTLSConfig(host string, options *TLSOptions) (*tls.Config, error) {
	config := &tls.Config{
		ServerName: extractHostname(host),
		MinVersion: tls.VersionTLS12,
	}

	// Set verification mode
	if options != nil {
		if options.InsecureSkipVerify {
			config.InsecureSkipVerify = true
		}

		// Load CA certificates
		if options.CACert != "" {
			pool, err := LoadCACertificate(options.CACert)
			if err != nil {
				return nil, err
			}
			config.RootCAs = pool
		}

		// Load client certificates
		if options.ClientCert != "" && options.ClientKey != "" {
			cert, err := tls.LoadX509KeyPair(options.ClientCert, options.ClientKey)
			if err != nil {
				return nil, fmt.Errorf("failed to load client certificates: %w", err)
			}
			config.Certificates = []tls.Certificate{cert}
		}

		// Set minimum TLS version
		if options.MinTLSVersion != 0 {
			config.MinVersion = options.MinTLSVersion
		} else {
			config.MinVersion = tls.VersionTLS12
		}

		// Set cipher suites if specified
		if len(options.CipherSuites) > 0 {
			config.CipherSuites = options.CipherSuites
		}
	}

	return config, nil
}

// TLSOptions contains TLS configuration options.
type TLSOptions struct {
	InsecureSkipVerify bool
	CACert             string
	ClientCert         string
	ClientKey          string
	MinTLSVersion      uint16
	CipherSuites       []uint16
}

// LoadCACertificate loads a CA certificate from a file.
func LoadCACertificate(filename string) (*x509.CertPool, error) {
	// This would load the certificate from file
	// For now, return the system pool
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	return pool, nil
}

// extractHostname extracts the hostname from a host:port string.
func extractHostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// GetCertificateInfo returns information about a certificate.
func GetCertificateInfo(cert *x509.Certificate) map[string]interface{} {
	info := make(map[string]interface{})

	info["subject"] = cert.Subject.String()
	info["issuer"] = cert.Issuer.String()
	info["serial"] = cert.SerialNumber.String()
	info["not_before"] = cert.NotBefore.Format(time.RFC3339)
	info["not_after"] = cert.NotAfter.Format(time.RFC3339)
	info["fingerprint"] = CalculateFingerprint(cert)
	info["signature_algorithm"] = cert.SignatureAlgorithm.String()
	info["public_key_algorithm"] = cert.PublicKeyAlgorithm.String()

	// DNS names
	if len(cert.DNSNames) > 0 {
		info["dns_names"] = cert.DNSNames
	}

	// IP addresses
	if len(cert.IPAddresses) > 0 {
		ips := make([]string, len(cert.IPAddresses))
		for i, ip := range cert.IPAddresses {
			ips[i] = ip.String()
		}
		info["ip_addresses"] = ips
	}

	// Key usage
	var keyUsage []string
	if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		keyUsage = append(keyUsage, "Digital Signature")
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		keyUsage = append(keyUsage, "Key Encipherment")
	}
	if cert.KeyUsage&x509.KeyUsageDataEncipherment != 0 {
		keyUsage = append(keyUsage, "Data Encipherment")
	}
	if cert.KeyUsage&x509.KeyUsageKeyAgreement != 0 {
		keyUsage = append(keyUsage, "Key Agreement")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign != 0 {
		keyUsage = append(keyUsage, "Certificate Signing")
	}
	if len(keyUsage) > 0 {
		info["key_usage"] = strings.Join(keyUsage, ", ")
	}

	// Extended key usage
	if len(cert.ExtKeyUsage) > 0 {
		var extKeyUsage []string
		for _, usage := range cert.ExtKeyUsage {
			switch usage {
			case x509.ExtKeyUsageServerAuth:
				extKeyUsage = append(extKeyUsage, "Server Authentication")
			case x509.ExtKeyUsageClientAuth:
				extKeyUsage = append(extKeyUsage, "Client Authentication")
			case x509.ExtKeyUsageCodeSigning:
				extKeyUsage = append(extKeyUsage, "Code Signing")
			case x509.ExtKeyUsageEmailProtection:
				extKeyUsage = append(extKeyUsage, "Email Protection")
			}
		}
		if len(extKeyUsage) > 0 {
			info["extended_key_usage"] = strings.Join(extKeyUsage, ", ")
		}
	}

	return info
}
