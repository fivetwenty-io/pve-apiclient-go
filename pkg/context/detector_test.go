//nolint:testpackage // White-box testing required to test unexported functions
package context

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var errHostname = errors.New("hostname error")

func TestExecutionMode_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode ExecutionMode
		want string
	}{
		{"Remote mode", ExecutionModeRemote, "remote"},
		{"Local mode", ExecutionModeLocal, "local"},
		{"Unknown mode", ExecutionModeUnknown, "unknown"},
		{"Invalid mode", ExecutionMode(999), "invalid"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.mode.String(); got != testCase.want {
				t.Errorf("ExecutionMode.String() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestDetector_DetectMode_AllChecksFail(t *testing.T) {
	t.Parallel()

	detector := NewDetector(
		WithPVEPath("/nonexistent/pve"),
		WithPVESHPath("/nonexistent/pvesh"),
		WithDpkgPath("/nonexistent/dpkg"),
		WithHostnameFunc(func() (string, error) {
			return "test-host", nil
		}),
	)

	mode := detector.DetectMode()
	if mode != ExecutionModeRemote {
		t.Errorf("DetectMode() with all checks failing = %v, want %v", mode, ExecutionModeRemote)
	}
}

func TestDetector_DetectMode_PartialChecks(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure
	tmpDir := t.TempDir()
	pveDir := filepath.Join(tmpDir, "pve")

	err := os.MkdirAll(pveDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	detector := NewDetector(
		WithPVEPath(pveDir), // This will pass (+3 points)
		WithPVESHPath("/nonexistent/pvesh"),
		WithDpkgPath("/nonexistent/dpkg"),
	)

	mode := detector.DetectMode()
	// Score = 3 (only PVE directory exists)
	// 3 points = Unknown threshold
	if mode != ExecutionModeUnknown {
		t.Errorf("DetectMode() with partial checks = %v, want %v", mode, ExecutionModeUnknown)
	}
}

func TestDetector_DetectMode_HighScore(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure
	tmpDir := t.TempDir()
	pveDir := filepath.Join(tmpDir, "pve")
	nodesDir := filepath.Join(pveDir, "nodes", "test-node")
	pveshPath := filepath.Join(tmpDir, "pvesh")

	err := os.MkdirAll(nodesDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	// Create fake pvesh binary
	// #nosec G306 -- Test file requires executable permissions to simulate pvesh binary
	err = os.WriteFile(pveshPath, []byte("#!/bin/sh\n"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	detector := NewDetector(
		WithPVEPath(pveDir),      // +3 points (PVE dir exists)
		WithPVESHPath(pveshPath), // +2 points (pvesh exists)
		WithDpkgPath(""),         // 0 points (disabled)
		WithHostnameFunc(func() (string, error) {
			return "test-node", nil // +3 points (node registered)
		}),
	)

	mode := detector.DetectMode()
	// Score = 3 + 2 + 3 = 8 points
	// 8 >= 6 = Local
	if mode != ExecutionModeLocal {
		t.Errorf("DetectMode() with high score = %v, want %v", mode, ExecutionModeLocal)
	}
}

func TestDetector_IsLocal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func() *Detector
		wantLocal bool
	}{
		{
			name: "Local when high score",
			setup: func() *Detector {
				tmpDir := t.TempDir()
				pveDir := filepath.Join(tmpDir, "pve")
				nodesDir := filepath.Join(pveDir, "nodes", "local-node")
				pveshPath := filepath.Join(tmpDir, "pvesh")

				_ = os.MkdirAll(nodesDir, 0750)
				// #nosec G306 -- Test file requires executable permissions to simulate pvesh binary
				_ = os.WriteFile(pveshPath, []byte("#!/bin/sh\n"), 0750)

				return NewDetector(
					WithPVEPath(pveDir),
					WithPVESHPath(pveshPath),
					WithDpkgPath(""),
					WithHostnameFunc(func() (string, error) {
						return "local-node", nil
					}),
				)
			},
			wantLocal: true,
		},
		{
			name: "Not local when low score",
			setup: func() *Detector {
				return NewDetector(
					WithPVEPath("/nonexistent"),
					WithPVESHPath("/nonexistent"),
					WithDpkgPath(""),
				)
			},
			wantLocal: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			detector := testCase.setup()
			if got := detector.IsLocal(); got != testCase.wantLocal {
				t.Errorf("IsLocal() = %v, want %v", got, testCase.wantLocal)
			}
		})
	}
}

func TestDetector_IsRemote(t *testing.T) {
	t.Parallel()

	detector := NewDetector(
		WithPVEPath("/nonexistent"),
		WithPVESHPath("/nonexistent"),
	)

	if !detector.IsRemote() {
		t.Error("IsRemote() = false, want true for non-PVE environment")
	}
}

//nolint:funlen // Test case definitions with setup closures
func TestDetector_GetNodeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func() *Detector
		wantName  string
		wantError bool
	}{
		{
			name: "Success on local node",
			setup: func() *Detector {
				tmpDir := t.TempDir()
				pveDir := filepath.Join(tmpDir, "pve")
				nodesDir := filepath.Join(pveDir, "nodes", "my-node")
				pveshPath := filepath.Join(tmpDir, "pvesh")

				_ = os.MkdirAll(nodesDir, 0750)
				// #nosec G306 -- Test file requires executable permissions to simulate pvesh binary
				_ = os.WriteFile(pveshPath, []byte("#!/bin/sh\n"), 0750)

				return NewDetector(
					WithPVEPath(pveDir),
					WithPVESHPath(pveshPath),
					WithDpkgPath(""),
					WithHostnameFunc(func() (string, error) {
						return "my-node", nil
					}),
				)
			},
			wantName:  "my-node",
			wantError: false,
		},
		{
			name: "Error when remote",
			setup: func() *Detector {
				return NewDetector(
					WithPVEPath("/nonexistent"),
				)
			},
			wantName:  "",
			wantError: true,
		},
		{
			name: "Error when hostname fails",
			setup: func() *Detector {
				tmpDir := t.TempDir()
				pveDir := filepath.Join(tmpDir, "pve")
				nodesDir := filepath.Join(pveDir, "nodes", "test")
				pveshPath := filepath.Join(tmpDir, "pvesh")

				_ = os.MkdirAll(nodesDir, 0750)
				// #nosec G306 -- Test file requires executable permissions to simulate pvesh binary
				_ = os.WriteFile(pveshPath, []byte("#!/bin/sh\n"), 0750)

				return NewDetector(
					WithPVEPath(pveDir),
					WithPVESHPath(pveshPath),
					WithDpkgPath(""),
					WithHostnameFunc(func() (string, error) {
						return "", errHostname
					}),
				)
			},
			wantName:  "",
			wantError: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			detector := testCase.setup()
			name, err := detector.GetNodeName()

			if (err != nil) != testCase.wantError {
				t.Errorf("GetNodeName() error = %v, wantError %v", err, testCase.wantError)

				return
			}

			if name != testCase.wantName {
				t.Errorf("GetNodeName() = %v, want %v", name, testCase.wantName)
			}

			if testCase.wantError && !errors.Is(err, ErrNotOnPVENode) && err.Error() != "hostname error" {
				t.Errorf("GetNodeName() error = %v, want ErrNotOnPVENode or hostname error", err)
			}
		})
	}
}

func TestDetector_checkPVEDirectory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func() string
		want  bool
	}{
		{
			name: "Directory exists",
			setup: func() string {
				tmpDir := t.TempDir()

				return tmpDir
			},
			want: true,
		},
		{
			name: "Directory does not exist",
			setup: func() string {
				return "/this/path/does/not/exist"
			},
			want: false,
		},
		{
			name: "Path is a file not directory",
			setup: func() string {
				tmpDir := t.TempDir()
				file := filepath.Join(tmpDir, "file")
				_ = os.WriteFile(file, []byte("test"), 0600)

				return file
			},
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := testCase.setup()
			detector := NewDetector(WithPVEPath(path))

			if got := detector.checkPVEDirectory(); got != testCase.want {
				t.Errorf("checkPVEDirectory() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestDetector_checkPVESH(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func() string
		want  bool
	}{
		{
			name: "Binary exists",
			setup: func() string {
				tmpDir := t.TempDir()
				binary := filepath.Join(tmpDir, "pvesh")
				// #nosec G306 -- Test file requires executable permissions to simulate pvesh binary
				_ = os.WriteFile(binary, []byte("#!/bin/sh\n"), 0750)

				return binary
			},
			want: true,
		},
		{
			name: "Binary does not exist",
			setup: func() string {
				return "/this/path/does/not/exist/pvesh"
			},
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := testCase.setup()
			detector := NewDetector(WithPVESHPath(path))

			if got := detector.checkPVESH(); got != testCase.want {
				t.Errorf("checkPVESH() = %v, want %v", got, testCase.want)
			}
		})
	}
}

//nolint:funlen // Test case definitions with setup closures
func TestDetector_checkNodeRegistration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func() (pveDir string, hostname string)
		want  bool
	}{
		{
			name: "Node registered",
			setup: func() (string, string) {
				tmpDir := t.TempDir()
				pveDir := filepath.Join(tmpDir, "pve")
				nodesDir := filepath.Join(pveDir, "nodes", "registered-node")
				_ = os.MkdirAll(nodesDir, 0750)

				return pveDir, "registered-node"
			},
			want: true,
		},
		{
			name: "Node not registered",
			setup: func() (string, string) {
				tmpDir := t.TempDir()
				pveDir := filepath.Join(tmpDir, "pve")
				_ = os.MkdirAll(pveDir, 0750)

				return pveDir, "unregistered-node"
			},
			want: false,
		},
		{
			name: "Hostname error",
			setup: func() (string, string) {
				return "/nonexistent", ""
			},
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pveDir, hostname := testCase.setup()

			detector := NewDetector(
				WithPVEPath(pveDir),
				WithHostnameFunc(func() (string, error) {
					if hostname == "" {
						return "", errHostname
					}

					return hostname, nil
				}),
			)

			if got := detector.checkNodeRegistration(); got != testCase.want {
				t.Errorf("checkNodeRegistration() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()

	// Just verify the convenience function works
	mode := Detect()
	// Should return Remote or Unknown on non-PVE systems
	if mode == ExecutionModeLocal {
		// Only possible if running tests on actual PVE node
		t.Logf("Detected local execution mode (running on PVE node?)")
	}
}

func TestIsRunningOnPVENode(t *testing.T) {
	t.Parallel()

	// Just verify the convenience function works
	isLocal := IsRunningOnPVENode()
	// Should return false on non-PVE systems
	if isLocal {
		// Only possible if running tests on actual PVE node
		t.Logf("Detected running on PVE node")
	}
}

func TestDetector_checkPVEManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		dpkgPath string
		want     bool
	}{
		{
			name:     "Disabled when dpkg path empty",
			dpkgPath: "",
			want:     false,
		},
		{
			name:     "Returns false when dpkg missing",
			dpkgPath: "/nonexistent/dpkg",
			want:     false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			detector := NewDetector(WithDpkgPath(testCase.dpkgPath))

			if got := detector.checkPVEManager(); got != testCase.want {
				t.Errorf("checkPVEManager() = %v, want %v", got, testCase.want)
			}
		})
	}
}
