package tasks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/api/tasks"
	pveclient "github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/client"
)

func optsFromServerURL(u string) pveclient.Options {
	parsed, _ := url.Parse(u)
	host := strings.Split(parsed.Host, ":")[0]
	port := 0

	if parts := strings.Split(parsed.Host, ":"); len(parts) == 2 {
		p, _ := strconv.Atoi(parts[1])
		port = p
	}

	return pveclient.Options{Host: host, Port: port, Protocol: "http", APIToken: "user@pam!tok=sec"}
}

func TestWaitTaskSuccess(t *testing.T) {
	t.Parallel()

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/node1/tasks/UPID123/status" {
			http.Error(writer, "bad path", http.StatusNotFound)

			return
		}

		calls++
		// first call running, second stopped OK
		var data map[string]any
		if calls < 2 {
			data = map[string]any{"status": "running"}
		} else {
			data = map[string]any{"status": "stopped", "exitstatus": "OK"}
		}

		_ = json.NewEncoder(writer).Encode(map[string]any{"data": data, "success": 1})
	}))
	defer srv.Close()

	cli, err := pveclient.NewClient(optsFromServerURL(srv.URL))
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	svc := tasks.New(cli)

	status, err := svc.Wait(context.Background(), "node1", "UPID123", &tasks.WaitOptions{TimeoutSeconds: 5, IntervalMillis: 10})
	if err != nil {
		t.Fatalf("wait err: %v", err)
	}

	if status == nil || status.Status != "stopped" || status.ExitStatus != "OK" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestWaitTaskTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow timeout test in short mode")
	}

	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always running
		_, _ = w.Write([]byte(`{"data": {"status": "running"}, "success": 1}`))
	}))
	defer srv.Close()

	parsed, _ := url.Parse(srv.URL)
	host := strings.Split(parsed.Host, ":")[0]

	p := 0
	if parts := strings.Split(parsed.Host, ":"); len(parts) == 2 {
		p, _ = strconv.Atoi(parts[1])
	}

	cli, err := pveclient.NewClient(pveclient.Options{Host: host, Port: p, Protocol: "http", APIToken: "u@pam!tok=sec"})
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	svc := tasks.New(cli)

	_, err = svc.Wait(context.Background(), "n1", "UPID123", &tasks.WaitOptions{TimeoutSeconds: 1, IntervalMillis: 5})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
