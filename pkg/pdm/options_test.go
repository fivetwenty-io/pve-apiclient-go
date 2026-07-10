package pdm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/fivetwenty-io/proxmox-apiclient-go/v3/pkg/client"
	"github.com/fivetwenty-io/proxmox-apiclient-go/v3/pkg/pdm"
	"github.com/fivetwenty-io/proxmox-apiclient-go/v3/pkg/pdm/version"
)

const testHost = "pdm.example.com"

func TestDefaultOptions_FillsPDMDefaults(t *testing.T) {
	t.Parallel()

	got := pdm.DefaultOptions(client.Options{Host: testHost})

	if got.Port != pdm.DefaultPort {
		t.Errorf("Port = %d, want %d", got.Port, pdm.DefaultPort)
	}

	if got.APITokenName != pdm.APITokenName {
		t.Errorf("APITokenName = %q, want %q", got.APITokenName, pdm.APITokenName)
	}

	if got.CookieName != pdm.CookieName {
		t.Errorf("CookieName = %q, want %q", got.CookieName, pdm.CookieName)
	}

	if got.Host != testHost {
		t.Errorf("Host = %q, want passthrough", got.Host)
	}
}

func TestDefaultOptions_PreservesExplicitValues(t *testing.T) {
	t.Parallel()

	base := client.Options{
		Host:         testHost,
		Port:         9999,
		APITokenName: "CustomToken",
		CookieName:   "CustomCookie",
	}

	got := pdm.DefaultOptions(base)

	if got.Port != 9999 {
		t.Errorf("Port = %d, want explicit 9999 preserved", got.Port)
	}

	if got.APITokenName != "CustomToken" {
		t.Errorf("APITokenName = %q, want explicit value preserved", got.APITokenName)
	}

	if got.CookieName != "CustomCookie" {
		t.Errorf("CookieName = %q, want explicit value preserved", got.CookieName)
	}
}

// TestNewClient_SendsPDMAPITokenHeader proves the preset wires the PDM
// token name all the way onto the Authorization header of a real request
// issued through a generated pkg/pdm service, using the ':' id/secret
// separator PDM's Rust auth API (proxmox-auth-api) expects rather than
// PVE's '='.
func TestNewClient_SendsPDMAPITokenHeader(t *testing.T) {
	t.Parallel()

	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"release":"1","repoid":"abc","version":"1.1"}}`))
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}

	cli, err := pdm.NewClient(client.Options{
		Host:     parsed.Hostname(),
		Port:     port,
		Protocol: "http",
		APIToken: "user@pdm!tok=secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = version.New(cli).Get(context.Background())
	if err != nil {
		t.Fatalf("version.Get: %v", err)
	}

	const wantAuth = "PDMAPIToken=user@pdm!tok:secret"
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
}

// TestNewClient_DefaultPortInBaseURL proves an options value without an
// explicit port targets 8443 rather than PVE's 8006.
func TestNewClient_DefaultPortInBaseURL(t *testing.T) {
	t.Parallel()

	opts := pdm.DefaultOptions(client.Options{
		Host:     testHost,
		APIToken: "user@pdm!tok=secret",
	})

	err := opts.Validate()
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if !strings.Contains(opts.GetBaseURL(), ":8443/") {
		t.Errorf("GetBaseURL() = %q, want port 8443", opts.GetBaseURL())
	}
}
