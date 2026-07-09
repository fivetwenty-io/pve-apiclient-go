package pve

import (
	"encoding/json"
	"testing"
)

// TestListRemotesLxcConfigResponseSparseRoundTrip guards against the defect
// that motivated treating numbered-slot response properties (dev0..dev255,
// mp0..mp255, net0..net31, unused0..unused255, ...) as implicitly optional:
// the PDM apidoc enumerates every possible slot for a guest config without
// ever marking the unused ones "optional", so a naively generated struct
// would decode/encode absent slots as fabricated empty strings rather than
// leaving them absent. A real container only ever populates a handful of
// slots, so marshaling a sparse config back out must reproduce only the
// slots that were actually present.
func TestListRemotesLxcConfigResponseSparseRoundTrip(t *testing.T) {
	t.Parallel()

	const sparse = `{"arch":"amd64","cores":2,"dev0":"/dev/foo"}`

	var resp ListRemotesLxcConfigResponse
	if err := json.Unmarshal([]byte(sparse), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if resp.Arch == nil || *resp.Arch != "amd64" {
		t.Fatalf("resp.Arch = %v, want \"amd64\"", resp.Arch)
	}

	if resp.Cores == nil || int64(*resp.Cores) != 2 {
		t.Fatalf("resp.Cores = %v, want 2", resp.Cores)
	}

	if resp.Dev0 == nil || *resp.Dev0 != "/dev/foo" {
		t.Fatalf("resp.Dev0 = %v, want \"/dev/foo\"", resp.Dev0)
	}

	if resp.Dev1 != nil {
		t.Fatalf("resp.Dev1 = %v, want nil (slot absent in source payload)", *resp.Dev1)
	}

	if resp.Dev255 != nil {
		t.Fatalf("resp.Dev255 = %v, want nil (slot absent in source payload)", *resp.Dev255)
	}

	out, err := json.Marshal(&resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var roundTripped map[string]any
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal(remarshaled) error = %v", err)
	}

	for absent := range map[string]bool{"dev1": true, "dev2": true, "dev255": true, "mp0": true, "net0": true, "unused0": true} {
		if _, present := roundTripped[absent]; present {
			t.Errorf("remarshaled sparse config fabricated field %q: %s", absent, out)
		}
	}

	for present, want := range map[string]any{"arch": "amd64", "dev0": "/dev/foo"} {
		if got := roundTripped[present]; got != want {
			t.Errorf("remarshaled sparse config field %q = %v, want %v", present, got, want)
		}
	}
}

// TestListRemotesQemuConfigResponseSparseRoundTrip is the QEMU-side sibling
// of TestListRemotesLxcConfigResponseSparseRoundTrip: same numbered-slot
// families (net, ide, sata, scsi, virtio, usb, hostpci, unused, ipconfig,
// serial, parallel, virtiofs), different guest type.
func TestListRemotesQemuConfigResponseSparseRoundTrip(t *testing.T) {
	t.Parallel()

	const sparse = `{"cores":4,"net0":"virtio=aa:bb:cc:dd:ee:ff,bridge=vmbr0"}`

	var resp ListRemotesQemuConfigResponse
	if err := json.Unmarshal([]byte(sparse), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if resp.Cores == nil || int64(*resp.Cores) != 4 {
		t.Fatalf("resp.Cores = %v, want 4", resp.Cores)
	}

	if resp.Net0 == nil || *resp.Net0 != "virtio=aa:bb:cc:dd:ee:ff,bridge=vmbr0" {
		t.Fatalf("resp.Net0 = %v, want the sample network device string", resp.Net0)
	}

	out, err := json.Marshal(&resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var roundTripped map[string]any
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal(remarshaled) error = %v", err)
	}

	for absent := range map[string]bool{"net1": true, "net31": true, "ide0": true, "scsi0": true, "usb0": true} {
		if _, present := roundTripped[absent]; present {
			t.Errorf("remarshaled sparse config fabricated field %q: %s", absent, out)
		}
	}

	if got := roundTripped["net0"]; got != "virtio=aa:bb:cc:dd:ee:ff,bridge=vmbr0" {
		t.Errorf(`remarshaled sparse config field "net0" = %v, want the sample network device string`, got)
	}
}
