package compatibility

import (
	"reflect"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  *Version
		expectErr bool
	}{
		{
			name:  "PVE 7.4-3",
			input: "7.4-3",
			expected: &Version{
				Major: 7,
				Minor: 4,
				Patch: 0,
				Build: "3",
			},
		},
		{
			name:  "PVE 8.0-2",
			input: "8.0-2",
			expected: &Version{
				Major: 8,
				Minor: 0,
				Patch: 0,
				Build: "2",
			},
		},
		{
			name:  "PVE 7.3.1",
			input: "7.3.1",
			expected: &Version{
				Major: 7,
				Minor: 3,
				Patch: 1,
				Build: "",
			},
		},
		{
			name:  "PVE 6.4",
			input: "6.4",
			expected: &Version{
				Major: 6,
				Minor: 4,
				Patch: 0,
				Build: "",
			},
		},
		{
			name:      "Invalid format",
			input:     "invalid",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := ParseVersion(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !reflect.DeepEqual(version, tt.expected) {
				t.Errorf("Got %+v, expected %+v", version, tt.expected)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name     string
		v1       *Version
		v2       *Version
		expected int
	}{
		{
			name:     "Equal versions",
			v1:       &Version{Major: 7, Minor: 4, Patch: 3},
			v2:       &Version{Major: 7, Minor: 4, Patch: 3},
			expected: 0,
		},
		{
			name:     "Major version difference",
			v1:       &Version{Major: 6, Minor: 4, Patch: 0},
			v2:       &Version{Major: 7, Minor: 4, Patch: 0},
			expected: -1,
		},
		{
			name:     "Minor version difference",
			v1:       &Version{Major: 7, Minor: 3, Patch: 0},
			v2:       &Version{Major: 7, Minor: 4, Patch: 0},
			expected: -1,
		},
		{
			name:     "Patch version difference",
			v1:       &Version{Major: 7, Minor: 4, Patch: 1},
			v2:       &Version{Major: 7, Minor: 4, Patch: 3},
			expected: -1,
		},
		{
			name:     "Higher version",
			v1:       &Version{Major: 8, Minor: 0, Patch: 0},
			v2:       &Version{Major: 7, Minor: 4, Patch: 0},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v1.Compare(tt.v2)
			if result != tt.expected {
				t.Errorf("Compare() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestIsAtLeast(t *testing.T) {
	version := &Version{Major: 7, Minor: 4, Patch: 3}

	tests := []struct {
		name     string
		major    int
		minor    int
		patch    int
		expected bool
	}{
		{
			name:     "Exact version",
			major:    7,
			minor:    4,
			patch:    3,
			expected: true,
		},
		{
			name:     "Lower version",
			major:    7,
			minor:    3,
			patch:    0,
			expected: true,
		},
		{
			name:     "Higher version",
			major:    7,
			minor:    5,
			patch:    0,
			expected: false,
		},
		{
			name:     "Much lower version",
			major:    6,
			minor:    0,
			patch:    0,
			expected: true,
		},
		{
			name:     "Much higher version",
			major:    8,
			minor:    0,
			patch:    0,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := version.IsAtLeast(tt.major, tt.minor, tt.patch)
			if result != tt.expected {
				t.Errorf("IsAtLeast(%d, %d, %d) = %v, expected %v",
					tt.major, tt.minor, tt.patch, result, tt.expected)
			}
		})
	}
}

func TestMatrixFeatureSupport(t *testing.T) {
	matrix := NewMatrix()

	tests := []struct {
		name       string
		feature    string
		version    *Version
		supported  bool
		hasWarning bool
	}{
		{
			name:      "PVE 6.0 supports storage_content",
			feature:   "storage_content",
			version:   &Version{Major: 6, Minor: 0, Patch: 0},
			supported: true,
		},
		{
			name:      "PVE 6.1 doesn't support cloud_init",
			feature:   "cloud_init",
			version:   &Version{Major: 6, Minor: 1, Patch: 0},
			supported: false,
		},
		{
			name:      "PVE 6.2 supports cloud_init",
			feature:   "cloud_init",
			version:   &Version{Major: 6, Minor: 2, Patch: 0},
			supported: true,
		},
		{
			name:      "PVE 7.0 supports PBS integration",
			feature:   "pbs_integration",
			version:   &Version{Major: 7, Minor: 0, Patch: 0},
			supported: true,
		},
		{
			name:      "PVE 6.4 doesn't support PBS integration",
			feature:   "pbs_integration",
			version:   &Version{Major: 6, Minor: 4, Patch: 0},
			supported: false,
		},
		{
			name:      "PVE 7.3 supports SDN",
			feature:   "sdn",
			version:   &Version{Major: 7, Minor: 3, Patch: 0},
			supported: true,
		},
		{
			name:      "PVE 7.2 doesn't support SDN",
			feature:   "sdn",
			version:   &Version{Major: 7, Minor: 2, Patch: 0},
			supported: false,
		},
		{
			name:      "PVE 8.1 supports notification system",
			feature:   "notification_system",
			version:   &Version{Major: 8, Minor: 1, Patch: 0},
			supported: true,
		},
		{
			name:       "OpenVZ deprecated in PVE 4",
			feature:    "openvz",
			version:    &Version{Major: 4, Minor: 0, Patch: 0},
			supported:  true,
			hasWarning: true,
		},
		{
			name:      "OpenVZ not available in PVE 6",
			feature:   "openvz",
			version:   &Version{Major: 6, Minor: 0, Patch: 0},
			supported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported, msg := matrix.IsFeatureSupported(tt.feature, tt.version)
			if supported != tt.supported {
				t.Errorf("IsFeatureSupported(%s, %v) = %v, expected %v",
					tt.feature, tt.version, supported, tt.supported)
			}
			if tt.hasWarning && msg == "" {
				t.Error("Expected warning message but got none")
			}
		})
	}
}

func TestGetSupportedFeatures(t *testing.T) {
	matrix := NewMatrix()

	tests := []struct {
		name     string
		version  *Version
		minCount int // Minimum expected features
	}{
		{
			name:     "PVE 6.0",
			version:  &Version{Major: 6, Minor: 0, Patch: 0},
			minCount: 3,
		},
		{
			name:     "PVE 7.0",
			version:  &Version{Major: 7, Minor: 0, Patch: 0},
			minCount: 6,
		},
		{
			name:     "PVE 7.4",
			version:  &Version{Major: 7, Minor: 4, Patch: 0},
			minCount: 10,
		},
		{
			name:     "PVE 8.1",
			version:  &Version{Major: 8, Minor: 1, Patch: 0},
			minCount: 14,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			features := matrix.GetSupportedFeatures(tt.version)
			if len(features) < tt.minCount {
				t.Errorf("GetSupportedFeatures(%v) returned %d features, expected at least %d",
					tt.version, len(features), tt.minCount)
			}
		})
	}
}

func TestChecker(t *testing.T) {
	checker, err := NewChecker("7.4-3")
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	// Test version parsing
	version := checker.GetVersion()
	if version.Major != 7 || version.Minor != 4 {
		t.Errorf("GetVersion() = %v, expected 7.4", version)
	}

	// Test feature checking
	tests := []struct {
		feature   string
		supported bool
	}{
		{"pbs_integration", true},
		{"sdn", true},
		{"notification_system", false},
		{"webauthn", true},
		{"openvz", false},
	}

	for _, tt := range tests {
		supported, _ := checker.Check(tt.feature)
		if supported != tt.supported {
			t.Errorf("Check(%s) = %v, expected %v", tt.feature, supported, tt.supported)
		}
	}
}

func TestEndpointRegistry(t *testing.T) {
	registry := NewEndpointRegistry()

	tests := []struct {
		name        string
		endpoint    string
		version     *Version
		expectError bool
	}{
		{
			name:     "VM config available in PVE 7",
			endpoint: "vm_config",
			version:  &Version{Major: 7, Minor: 0, Patch: 0},
		},
		{
			name:     "Cloud-init available in PVE 6.2",
			endpoint: "vm_cloud_init",
			version:  &Version{Major: 6, Minor: 2, Patch: 0},
		},
		{
			name:        "Cloud-init not available in PVE 6.1",
			endpoint:    "vm_cloud_init",
			version:     &Version{Major: 6, Minor: 1, Patch: 0},
			expectError: true,
		},
		{
			name:        "Unknown endpoint",
			endpoint:    "unknown",
			version:     &Version{Major: 7, Minor: 0, Patch: 0},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := registry.GetEndpoint(tt.endpoint, tt.version)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if endpoint == nil {
					t.Error("Got nil endpoint")
				}
			}
		})
	}
}

func TestGenerateReport(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		hasWarnings    bool
		hasRecommended bool
	}{
		{
			name:           "PVE 6.4 has warnings",
			version:        "6.4-1",
			hasWarnings:    true,
			hasRecommended: true,
		},
		{
			name:           "PVE 7.3 has recommendations",
			version:        "7.3-1",
			hasRecommended: true,
		},
		{
			name:           "PVE 8.1 is current",
			version:        "8.1-1",
			hasRecommended: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := GenerateReport(tt.version)
			if err != nil {
				t.Fatalf("GenerateReport() error = %v", err)
			}

			if tt.hasWarnings && len(report.Warnings) == 0 {
				t.Error("Expected warnings but got none")
			}
			if tt.hasRecommended && len(report.Recommendations) == 0 {
				t.Error("Expected recommendations but got none")
			}
			if len(report.SupportedFeatures) == 0 {
				t.Error("No supported features in report")
			}
		})
	}
}

func TestMigrationHelper(t *testing.T) {
	helper, err := NewMigrationHelper("6.4-1", "7.4-1")
	if err != nil {
		t.Fatalf("NewMigrationHelper() error = %v", err)
	}

	// Test migration steps
	steps := helper.GetMigrationSteps()
	if len(steps) == 0 {
		t.Error("No migration steps returned")
	}

	// Test new features
	newFeatures := helper.GetNewFeatures()
	if len(newFeatures) == 0 {
		t.Error("No new features found")
	}

	// Test breaking changes
	changes := helper.GetBreakingChanges()
	if len(changes) == 0 {
		t.Error("No breaking changes found for major version upgrade")
	}
}

func TestValidateConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		version *Version
		valid   bool
	}{
		{
			name: "Valid PVE 7 config",
			config: map[string]interface{}{
				"cluster": map[string]interface{}{
					"name": "test-cluster",
				},
			},
			version: &Version{Major: 7, Minor: 4, Patch: 0},
			valid:   true,
		},
		{
			name: "PVE 8 without notification config",
			config: map[string]interface{}{
				"cluster": map[string]interface{}{
					"name": "test-cluster",
				},
			},
			version: &Version{Major: 8, Minor: 0, Patch: 0},
			valid:   false,
		},
		{
			name: "OpenVZ config in PVE 6",
			config: map[string]interface{}{
				"openvz": map[string]interface{}{
					"enabled": true,
				},
			},
			version: &Version{Major: 6, Minor: 0, Patch: 0},
			valid:   false,
		},
		{
			name: "SDN config in PVE 7.2",
			config: map[string]interface{}{
				"sdn": map[string]interface{}{
					"zones": []string{"zone1"},
				},
			},
			version: &Version{Major: 7, Minor: 2, Patch: 0},
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, issues := ValidateConfiguration(tt.config, tt.version)
			if valid != tt.valid {
				t.Errorf("ValidateConfiguration() valid = %v, expected %v", valid, tt.valid)
			}
			if !tt.valid && len(issues) == 0 {
				t.Error("Invalid config but no issues reported")
			}
		})
	}
}

// Benchmark tests
func BenchmarkParseVersion(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseVersion("7.4-3")
	}
}

func BenchmarkVersionCompare(b *testing.B) {
	v1 := &Version{Major: 7, Minor: 4, Patch: 3}
	v2 := &Version{Major: 7, Minor: 3, Patch: 1}

	for i := 0; i < b.N; i++ {
		v1.Compare(v2)
	}
}

func BenchmarkFeatureCheck(b *testing.B) {
	matrix := NewMatrix()
	version := &Version{Major: 7, Minor: 4, Patch: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matrix.IsFeatureSupported("pbs_integration", version)
	}
}

func BenchmarkGetSupportedFeatures(b *testing.B) {
	matrix := NewMatrix()
	version := &Version{Major: 7, Minor: 4, Patch: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matrix.GetSupportedFeatures(version)
	}
}
