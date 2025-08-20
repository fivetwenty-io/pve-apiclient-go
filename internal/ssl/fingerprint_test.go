package ssl

import (
	"testing"
)

func TestNewFingerprintVerifier(t *testing.T) {
	fv := NewFingerprintVerifier()
	if fv == nil {
		t.Fatal("NewFingerprintVerifier returned nil")
	}
	if fv.cache == nil {
		t.Error("NewFingerprintVerifier() cache is nil")
	}
	if fv.manualVerification {
		t.Error("NewFingerprintVerifier() manualVerification should be false by default")
	}
}

func TestFingerprintVerifier_AddTrustedFingerprint(t *testing.T) {
	fv := NewFingerprintVerifier()

	fingerprint := "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99"
	fv.AddTrustedFingerprint(fingerprint)

	normalized := NormalizeFingerprint(fingerprint)
	if !fv.cache[normalized] {
		t.Errorf("AddTrustedFingerprint() failed to add fingerprint to cache")
	}
}

func TestFingerprintVerifier_AddTrustedFingerprints(t *testing.T) {
	fv := NewFingerprintVerifier()

	fingerprints := []string{
		"AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		"11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00",
	}

	fv.AddTrustedFingerprints(fingerprints)

	for _, fp := range fingerprints {
		normalized := NormalizeFingerprint(fp)
		if !fv.cache[normalized] {
			t.Errorf("AddTrustedFingerprints() failed to add fingerprint %s to cache", fp)
		}
	}
}

func TestFingerprintVerifier_RemoveTrustedFingerprint(t *testing.T) {
	fv := NewFingerprintVerifier()

	fingerprint := "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99"
	fv.AddTrustedFingerprint(fingerprint)

	// Verify it was added
	normalized := NormalizeFingerprint(fingerprint)
	if !fv.cache[normalized] {
		t.Fatal("Fingerprint was not added to cache")
	}

	// Remove it
	fv.RemoveTrustedFingerprint(fingerprint)

	// Verify it was removed
	if fv.cache[normalized] {
		t.Errorf("RemoveTrustedFingerprint() failed to remove fingerprint from cache")
	}
}

func TestFingerprintVerifier_GetTrustedFingerprints(t *testing.T) {
	fv := NewFingerprintVerifier()

	fingerprints := []string{
		"AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		"11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00",
	}

	fv.AddTrustedFingerprints(fingerprints)

	trusted := fv.GetTrustedFingerprints()
	if len(trusted) != len(fingerprints) {
		t.Errorf("GetTrustedFingerprints() returned %d fingerprints, want %d", len(trusted), len(fingerprints))
	}
}

func TestFingerprintVerifier_ClearCache(t *testing.T) {
	fv := NewFingerprintVerifier()

	// Add some fingerprints
	fv.AddTrustedFingerprint("AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99")
	fv.AddTrustedFingerprint("11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00")

	if len(fv.cache) == 0 {
		t.Fatal("No fingerprints in cache")
	}

	// Clear cache
	fv.ClearCache()

	if len(fv.cache) != 0 {
		t.Errorf("ClearCache() failed to clear cache, still has %d entries", len(fv.cache))
	}
}

func TestNormalizeFingerprint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already normalized",
			input:    "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			expected: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		},
		{
			name:     "lowercase",
			input:    "aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99",
			expected: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		},
		{
			name:     "no colons",
			input:    "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899",
			expected: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		},
		{
			name:     "with spaces",
			input:    "AA BB CC DD EE FF 00 11 22 33 44 55 66 77 88 99 AA BB CC DD EE FF 00 11 22 33 44 55 66 77 88 99",
			expected: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		},
		{
			name:     "with dashes",
			input:    "AA-BB-CC-DD-EE-FF-00-11-22-33-44-55-66-77-88-99-AA-BB-CC-DD-EE-FF-00-11-22-33-44-55-66-77-88-99",
			expected: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeFingerprint(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeFingerprint() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCompareFingerprints(t *testing.T) {
	tests := []struct {
		name     string
		fp1      string
		fp2      string
		expected bool
	}{
		{
			name:     "identical",
			fp1:      "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			fp2:      "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			expected: true,
		},
		{
			name:     "different format same value",
			fp1:      "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			fp2:      "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
			expected: true,
		},
		{
			name:     "different values",
			fp1:      "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			fp2:      "11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareFingerprints(tt.fp1, tt.fp2)
			if result != tt.expected {
				t.Errorf("CompareFingerprints() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatFingerprint(t *testing.T) {
	// Test with 32 bytes (SHA256)
	input := []byte{
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11,
		0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99,
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11,
		0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99,
	}

	expected := "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99"
	result := FormatFingerprint(input)

	if result != expected {
		t.Errorf("FormatFingerprint() = %v, want %v", result, expected)
	}
}

func TestParseFingerprint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid fingerprint with colons",
			input:   "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			wantErr: false,
		},
		{
			name:    "valid fingerprint without colons",
			input:   "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899",
			wantErr: false,
		},
		{
			name:    "invalid hex",
			input:   "ZZ:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			wantErr: true,
		},
		{
			name:    "wrong length",
			input:   "AA:BB:CC:DD",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFingerprint(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseFingerprint() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ParseFingerprint() unexpected error = %v", err)
				}
				if len(result) != 32 {
					t.Errorf("ParseFingerprint() returned %d bytes, want 32", len(result))
				}
			}
		})
	}
}
