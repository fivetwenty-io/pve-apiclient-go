package context

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExecutionMode_String(t *testing.T) {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("ExecutionMode.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetector_DetectMode_AllChecksFail(t *testing.T) {
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
	// Create temporary directory structure
	tmpDir := t.TempDir()
	pveDir := filepath.Join(tmpDir, "pve")

	err := os.MkdirAll(pveDir, 0755)
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
	// Create temporary directory structure
	tmpDir := t.TempDir()
	pveDir := filepath.Join(tmpDir, "pve")
	nodesDir := filepath.Join(pveDir, "nodes", "test-node")
	pveshPath := filepath.Join(tmpDir, "pvesh")

	err := os.MkdirAll(nodesDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Create fake pvesh binary
	err := os.WriteFile(pveshPath, []byte("#!/bin/sh\n"), 0755)
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

				_ = os.MkdirAll(nodesDir, 0755)
				_ = os.WriteFile(pveshPath, []byte("#!/bin/sh\n"), 0755)

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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := tt.setup()
			if got := detector.IsLocal(); got != tt.wantLocal {
				t.Errorf("IsLocal() = %v, want %v", got, tt.wantLocal)
			}
		})
	}
}

func TestDetector_IsRemote(t *testing.T) {
	detector := NewDetector(
		WithPVEPath("/nonexistent"),
		WithPVESHPath("/nonexistent"),
	)

	if !detector.IsRemote() {
		t.Error("IsRemote() = false, want true for non-PVE environment")
	}
}

func TestDetector_GetNodeName(t *testing.T) {
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

				_ = os.MkdirAll(nodesDir, 0755)
				_ = os.WriteFile(pveshPath, []byte("#!/bin/sh\n"), 0755)

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

				_ = os.MkdirAll(nodesDir, 0755)
				_ = os.WriteFile(pveshPath, []byte("#!/bin/sh\n"), 0755)

				return NewDetector(
					WithPVEPath(pveDir),
					WithPVESHPath(pveshPath),
					WithDpkgPath(""),
					WithHostnameFunc(func() (string, error) {
						return "", errors.New("hostname error")
					}),
				)
			},
			wantName:  "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := tt.setup()
			name, err := detector.GetNodeName()

			if (err != nil) != tt.wantError {
				t.Errorf("GetNodeName() error = %v, wantError %v", err, tt.wantError)

				return
			}

			if name != tt.wantName {
				t.Errorf("GetNodeName() = %v, want %v", name, tt.wantName)
			}

			if tt.wantError && !errors.Is(err, ErrNotOnPVENode) && err.Error() != "hostname error" {
				t.Errorf("GetNodeName() error = %v, want ErrNotOnPVENode or hostname error", err)
			}
		})
	}
}

func TestDetector_checkPVEDirectory(t *testing.T) {
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
				_ = os.WriteFile(file, []byte("test"), 0644)

				return file
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()
			detector := NewDetector(WithPVEPath(path))

			if got := detector.checkPVEDirectory(); got != tt.want {
				t.Errorf("checkPVEDirectory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetector_checkPVESH(t *testing.T) {
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
				_ = os.WriteFile(binary, []byte("#!/bin/sh\n"), 0755)

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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()
			detector := NewDetector(WithPVESHPath(path))

			if got := detector.checkPVESH(); got != tt.want {
				t.Errorf("checkPVESH() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetector_checkNodeRegistration(t *testing.T) {
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
				_ = os.MkdirAll(nodesDir, 0755)

				return pveDir, "registered-node"
			},
			want: true,
		},
		{
			name: "Node not registered",
			setup: func() (string, string) {
				tmpDir := t.TempDir()
				pveDir := filepath.Join(tmpDir, "pve")
				_ = os.MkdirAll(pveDir, 0755)

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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pveDir, hostname := tt.setup()

			detector := NewDetector(
				WithPVEPath(pveDir),
				WithHostnameFunc(func() (string, error) {
					if hostname == "" {
						return "", errors.New("hostname error")
					}

					return hostname, nil
				}),
			)

			if got := detector.checkNodeRegistration(); got != tt.want {
				t.Errorf("checkNodeRegistration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	// Just verify the convenience function works
	mode := Detect()
	// Should return Remote or Unknown on non-PVE systems
	if mode == ExecutionModeLocal {
		// Only possible if running tests on actual PVE node
		t.Logf("Detected local execution mode (running on PVE node?)")
	}
}

func TestIsRunningOnPVENode(t *testing.T) {
	// Just verify the convenience function works
	isLocal := IsRunningOnPVENode()
	// Should return false on non-PVE systems
	if isLocal {
		// Only possible if running tests on actual PVE node
		t.Logf("Detected running on PVE node")
	}
}

func TestDetector_checkPVEManager(t *testing.T) {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewDetector(WithDpkgPath(tt.dpkgPath))

			if got := detector.checkPVEManager(); got != tt.want {
				t.Errorf("checkPVEManager() = %v, want %v", got, tt.want)
			}
		})
	}
}
