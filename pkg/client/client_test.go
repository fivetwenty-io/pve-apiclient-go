package client_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fivetwenty-io/pve-apiclient-go/pkg/client"
)

const (
	testVersionEndpoint = "/api2/json/version"
)

type newClientTest struct {
	name    string
	opts    client.Options
	wantErr bool
	errMsg  string
}

func getValidTestCases() []newClientTest {
	return []newClientTest{
		{
			name: "valid with username/password",
			opts: client.Options{
				Host:     "pve.example.com",
				Username: "root@pam",
				Password: "secret",
			},
			wantErr: false,
		},
		{
			name: "valid with API token",
			opts: client.Options{
				Host:     "pve.example.com",
				APIToken: "root@pam!token=secret",
			},
			wantErr: false,
		},
	}
}

func getInvalidTestCases() []newClientTest {
	return []newClientTest{
		{
			name: "missing host",
			opts: client.Options{
				Username: "root@pam",
				Password: "secret",
			},
			wantErr: true,
			errMsg:  "host is required",
		},
		{
			name: "missing credentials",
			opts: client.Options{
				Host: "pve.example.com",
			},
			wantErr: true,
			errMsg:  "authentication credentials required",
		},
		{
			name: "username without password",
			opts: client.Options{
				Host:     "pve.example.com",
				Username: "root@pam",
			},
			wantErr: true,
			errMsg:  "password required when using username authentication",
		},
		{
			name: "invalid protocol",
			opts: client.Options{
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
			opts: client.Options{
				Host:     "pve.example.com",
				Username: "root@pam",
				Password: "secret",
				Port:     70000,
			},
			wantErr: true,
			errMsg:  "invalid port",
		},
	}
}

func getNewClientTestCases() []newClientTest {
	tests := getValidTestCases()
	tests = append(tests, getInvalidTestCases()...)

	return tests
}

func runNewClientTest(t *testing.T, testCase newClientTest) {
	t.Helper()

	cli, err := client.NewClient(testCase.opts)

	if testCase.wantErr {
		if err == nil {
			t.Errorf("NewClient() expected error, got nil")

			return
		}

		if testCase.errMsg != "" && err.Error() != testCase.errMsg && !contains(err.Error(), testCase.errMsg) {
			t.Errorf("NewClient() error = %v, want containing %v", err, testCase.errMsg)
		}

		return
	}

	if err != nil {
		t.Errorf("NewClient() unexpected error = %v", err)

		return
	}

	if cli == nil {
		t.Errorf("NewClient() returned nil client")
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := getNewClientTestCases()

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runNewClientTest(t, testCase)
		})
	}
}

func TestClient_UpdateTicket(t *testing.T) {
	t.Parallel()

	opts := client.Options{
		Host:     "pve.example.com",
		Username: "root@pam",
		Password: "secret",
	}

	cli, err := client.NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	testTicket := "PVE:test:ticket"
	cli.UpdateTicket(testTicket)

	// We can't directly test the internal state without type assertion working
	// This is a limitation of the current design where client is a private type
	// In a real test, we would either:
	// 1. Make the client type public
	// 2. Add getter methods to verify the state
	// 3. Test the behavior rather than the state
	// For now, we'll just verify the method doesn't panic
}

func TestClient_UpdateCSRFToken(t *testing.T) {
	t.Parallel()

	opts := client.Options{
		Host:     "pve.example.com",
		Username: "root@pam",
		Password: "secret",
	}

	cli, err := client.NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	testToken := "test:csrf:token" //nolint:gosec // Test credential
	cli.UpdateCSRFToken(testToken)

	// See comment in TestClient_UpdateTicket about testing private state
}

func TestClient_SetTimeout(t *testing.T) {
	t.Parallel()

	opts := client.Options{
		Host:     "pve.example.com",
		Username: "root@pam",
		Password: "secret",
	}

	cli, err := client.NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	newTimeout := 60 * time.Second
	cli.SetTimeout(newTimeout)

	// See comment in TestClient_UpdateTicket about testing private state
}

func TestClient_SetKeepAlive(t *testing.T) {
	t.Parallel()

	opts := client.Options{
		Host:     "pve.example.com",
		Username: "root@pam",
		Password: "secret",
	}

	cli, err := client.NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	newKeepAlive := 20
	cli.SetKeepAlive(newKeepAlive)

	// See comment in TestClient_UpdateTicket about testing private state
}

func createTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api2/json") {
			http.Error(writer, "bad base path", http.StatusNotFound)

			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data": {"ok": true}, "success": 1}`))
	}))
}

func parseServerURL(serverURL string) (string, int) {
	u, _ := url.Parse(serverURL)
	host := strings.Split(u.Host, ":")[0]

	portStr := "80"
	if parts := strings.Split(u.Host, ":"); len(parts) == 2 {
		portStr = parts[1]
	}

	var port int
	for _, ch := range portStr {
		port = port*10 + int(ch-'0')
	}

	return host, port
}

func testHTTPMethod(t *testing.T, cli client.Client, method string, data map[string]interface{}) {
	t.Helper()

	var (
		result interface{}
		err    error
	)

	switch method {
	case "GET":
		result, err = cli.Get("/test", nil)
	case "POST":
		result, err = cli.Post("/test", data)
	case "PUT":
		result, err = cli.Put("/test", data)
	case "DELETE":
		result, err = cli.Delete("/test", nil)
	}

	if err != nil {
		t.Errorf("%s() unexpected error = %v", method, err)
	}

	if result == nil {
		t.Errorf("%s() returned nil data", method)
	}
}

func TestClient_HTTPMethods(t *testing.T) {
	t.Parallel()

	srv := createTestServer()
	defer srv.Close()

	host, port := parseServerURL(srv.URL)
	opts := client.Options{
		Host:     host,
		Port:     port,
		Protocol: "http",
		APIToken: "root@pam!token=secret",
	}

	cli, err := client.NewClient(opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	testHTTPMethod(t, cli, "GET", nil)

	resp, err := cli.GetRaw("/test", nil)
	if err != nil {
		t.Errorf("GetRaw() unexpected error = %v", err)
	}

	if resp == nil {
		t.Errorf("GetRaw() returned nil response")
	}

	testData := map[string]interface{}{"key": "value"}
	testHTTPMethod(t, cli, "POST", testData)
	testHTTPMethod(t, cli, "PUT", testData)
	testHTTPMethod(t, cli, "DELETE", nil)
}

// Helper function to check if string contains substring.
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
