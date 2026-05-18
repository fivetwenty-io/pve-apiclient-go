package websocket_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	pkgws "github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/websocket"
)

// newWSServer returns an httptest server that upgrades to WebSocket, sends
// a JSON event text frame, then stays open until the client closes.
func newWSServer(t *testing.T, frameJSON string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		defer func() { _ = conn.Close() }()

		if frameJSON != "" {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(frameJSON))
		}

		// Keep the connection open for up to 2 s or until client disconnects.
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))

	return srv
}

// wsURL builds a ws:// URL from an httptest.Server.
func wsURL(srv *httptest.Server, path string) string {
	return "ws://" + srv.Listener.Addr().String() + path
}

// newClient builds a pkgws.Client connected to addr (ws://...).
func newClient(t *testing.T, addr string) *pkgws.Client {
	t.Helper()

	cfg := pkgws.DefaultConfig()
	cfg.Host = "127.0.0.1"

	// Parse port from addr "ws://host:port/path"
	noScheme := strings.TrimPrefix(addr, "ws://")

	colonIdx := strings.LastIndex(noScheme, ":")
	if colonIdx >= 0 {
		portPath := noScheme[colonIdx+1:]
		slashIdx := strings.Index(portPath, "/")

		var portStr string
		if slashIdx >= 0 {
			portStr = portPath[:slashIdx]
			cfg.Path = portPath[slashIdx:]
		} else {
			portStr = portPath
		}

		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil {
			cfg.Port = port
		}
	}

	cfg.Secure = false
	cfg.PingInterval = 0 // disable ping for tests
	cfg.HandshakeTimeout = 2 * time.Second
	cfg.ReadTimeout = 2 * time.Second
	cfg.WriteTimeout = 2 * time.Second
	cfg.MaxReconnectAttempts = 1
	cfg.ReconnectInterval = 10 * time.Millisecond

	c, err := pkgws.New(cfg)
	if err != nil {
		t.Fatalf("pkgws.New: %v", err)
	}

	return c
}

// --- DefaultConfig / New ---

func TestDefaultConfig_NonNil(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	if cfg.Port <= 0 {
		t.Errorf("port should be > 0, got %d", cfg.Port)
	}
}

func TestNew_RequiresHost(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = ""

	_, err := pkgws.New(cfg)
	if err == nil {
		t.Fatal("expected error when host is empty")
	}
}

func TestNew_NilConfigUsesDefault(t *testing.T) {
	// nil config should get defaulted, but host is empty → error.
	_, err := pkgws.New(nil)
	if err == nil {
		t.Fatal("expected error for nil config (host empty)")
	}
}

func TestNew_ValidConfig(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, err := pkgws.New(cfg)
	if err != nil {
		t.Fatalf("pkgws.New: %v", err)
	}

	if c == nil {
		t.Fatal("expected non-nil Client")
	}
}

// --- SetHeaders / SetAuth ---

func TestSetHeaders(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)
	h := http.Header{}
	h.Set("X-Custom", "value")
	c.SetHeaders(h) // must not panic
}

func TestSetAuth(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)
	c.SetAuth("ticket-value", "csrf-value") // must not panic
}

func TestSetAuth_EmptyValues(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)
	c.SetAuth("", "") // must not panic
}

// --- Connect / Disconnect / IsConnected ---

func TestConnect_HappyPath(t *testing.T) {
	srv := newWSServer(t, "")
	defer srv.Close()

	addr := wsURL(srv, "/ws")
	c := newClient(t, addr)

	err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if !c.IsConnected() {
		t.Error("IsConnected should be true after Connect")
	}

	if err := c.Disconnect(); err != nil {
		t.Errorf("Disconnect: %v", err)
	}
}

func TestConnect_AlreadyConnected(t *testing.T) {
	srv := newWSServer(t, "")
	defer srv.Close()

	c := newClient(t, wsURL(srv, "/ws"))

	err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("first Connect: %v", err)
	}

	defer c.Disconnect() //nolint:errcheck

	err = c.Connect(context.Background())
	if err == nil {
		t.Error("second Connect should return errAlreadyConnected")
	}
}

func TestDisconnect_NotConnected(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)

	// Disconnect without connecting is a no-op.
	err := c.Disconnect()
	if err != nil {
		t.Errorf("Disconnect on unconnected client: %v", err)
	}
}

func TestIsConnected_False_BeforeConnect(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)

	if c.IsConnected() {
		t.Error("IsConnected should be false before Connect")
	}
}

// --- ConnectWithRetry ---

func TestConnectWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	srv := newWSServer(t, "")
	defer srv.Close()

	c := newClient(t, wsURL(srv, "/ws"))

	err := c.ConnectWithRetry(context.Background())
	if err != nil {
		t.Fatalf("ConnectWithRetry: %v", err)
	}

	defer c.Disconnect() //nolint:errcheck
}

func TestConnectWithRetry_FailsWhenRefused(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "127.0.0.1"
	cfg.Port = 1 // always refused
	cfg.Secure = false
	cfg.MaxReconnectAttempts = 1
	cfg.ReconnectInterval = 1 * time.Millisecond
	cfg.HandshakeTimeout = 50 * time.Millisecond

	c, _ := pkgws.New(cfg)

	err := c.ConnectWithRetry(context.Background())
	if err == nil {
		t.Error("expected error when connection refused")
	}
}

// --- Event handlers: On / OnAll / Off ---

func TestOn_ReceivesEvent(t *testing.T) {
	eventJSON := `{"type":"node.update","id":"ev1","resource":"node/pve1","action":"update","status":"ok"}`

	srv := newWSServer(t, eventJSON)
	defer srv.Close()

	c := newClient(t, wsURL(srv, "/ws"))

	received := make(chan *pkgws.Event, 1)

	c.On("node.update", func(e *pkgws.Event) {
		received <- e
	})

	err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	defer c.Disconnect() //nolint:errcheck

	select {
	case e := <-received:
		if e.Type != "node.update" {
			t.Errorf("event type: want node.update, got %q", e.Type)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestOnAll_ReceivesAllEvents(t *testing.T) {
	eventJSON := `{"type":"storage.update","id":"ev2"}`

	srv := newWSServer(t, eventJSON)
	defer srv.Close()

	c := newClient(t, wsURL(srv, "/ws"))

	var count int32

	c.OnAll(func(_ *pkgws.Event) {
		atomic.AddInt32(&count, 1)
	})

	err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	defer c.Disconnect() //nolint:errcheck

	// Wait for at least one event.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&count) > 0 {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	if atomic.LoadInt32(&count) == 0 {
		t.Error("OnAll handler never called")
	}
}

func TestOff_RemovesHandler(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)
	c.On("test.event", func(_ *pkgws.Event) {})
	c.Off("test.event") // must not panic
}

// --- Send / SendText ---

func TestSend_NotConnected(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)

	err := c.Send(map[string]string{"key": "value"})
	if err == nil {
		t.Error("Send without connection should error")
	}
}

func TestSendText_NotConnected(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)

	err := c.SendText("hello")
	if err == nil {
		t.Error("SendText without connection should error")
	}
}

func TestSend_HappyPath(t *testing.T) {
	srv := newWSServer(t, "")
	defer srv.Close()

	c := newClient(t, wsURL(srv, "/ws"))

	err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	defer c.Disconnect() //nolint:errcheck

	if err := c.Send(map[string]string{"action": "ping"}); err != nil {
		t.Errorf("Send: %v", err)
	}
}

func TestSendText_HappyPath(t *testing.T) {
	srv := newWSServer(t, "")
	defer srv.Close()

	c := newClient(t, wsURL(srv, "/ws"))

	err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	defer c.Disconnect() //nolint:errcheck

	err = c.SendText("hello")
	if err != nil {
		t.Errorf("SendText: %v", err)
	}
}

// --- Errors channel ---

func TestErrors_ChannelReturnedNonNil(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)

	ch := c.Errors()
	if ch == nil {
		t.Error("Errors() should return non-nil channel")
	}
}

// --- Subscription ---

func TestNewSubscription_Cancel(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)

	sub := c.NewSubscription("test.event", func(_ *pkgws.Event) {})
	sub.Cancel() // must not panic
}

// --- Stream ---

func TestNewStream_Events(t *testing.T) {
	eventJSON := `{"type":"vm.update","id":"s1"}`

	srv := newWSServer(t, eventJSON)
	defer srv.Close()

	c := newClient(t, wsURL(srv, "/ws"))

	stream := c.NewStream(10)
	defer stream.Stop()

	err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	defer c.Disconnect() //nolint:errcheck

	select {
	case e, ok := <-stream.Events():
		if !ok {
			t.Error("stream closed prematurely")
		}

		if e.Type != "vm.update" {
			t.Errorf("event type: want vm.update, got %q", e.Type)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for stream event")
	}
}

func TestNewStream_DefaultBuffer(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "pve1"

	c, _ := pkgws.New(cfg)

	stream := c.NewStream(0) // 0 → defaults to 100
	defer stream.Stop()

	if stream.Events() == nil {
		t.Error("Events() channel is nil")
	}
}

// --- raw / malformed message fallback ---

func TestHandleMessage_RawFallback(t *testing.T) {
	// Send a non-JSON payload; client should fall back to raw event type.
	rawSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		defer func() { _ = conn.Close() }()

		_ = conn.WriteMessage(websocket.TextMessage, []byte("not-json"))

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer rawSrv.Close()

	c := newClient(t, wsURL(rawSrv, "/ws"))

	rawReceived := make(chan *pkgws.Event, 1)

	c.On("raw", func(e *pkgws.Event) {
		rawReceived <- e
	})

	err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	defer c.Disconnect() //nolint:errcheck

	select {
	case e := <-rawReceived:
		if e.Type != "raw" {
			t.Errorf("expected raw event, got %q", e.Type)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for raw event")
	}
}

// --- Ping/pong keepalive ---

func TestConnect_WithPingEnabled(t *testing.T) {
	srv := newWSServer(t, "")
	defer srv.Close()

	cfg := pkgws.DefaultConfig()
	cfg.Host = "127.0.0.1"

	host, port := hostPort(srv)
	cfg.Host = host
	cfg.Port = port
	cfg.Secure = false
	cfg.Path = "/ws"
	cfg.PingInterval = 50 * time.Millisecond
	cfg.HandshakeTimeout = 2 * time.Second
	cfg.ReadTimeout = 500 * time.Millisecond
	cfg.WriteTimeout = 500 * time.Millisecond
	cfg.MaxReconnectAttempts = 1
	cfg.ReconnectInterval = 10 * time.Millisecond

	c, err := pkgws.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	time.Sleep(150 * time.Millisecond) // let at least two pings fire

	if err := c.Disconnect(); err != nil {
		t.Errorf("Disconnect: %v", err)
	}
}

// --- Reconnect on server close ---

func TestConnectWithRetry_ContextCancelled(t *testing.T) {
	cfg := pkgws.DefaultConfig()
	cfg.Host = "127.0.0.1"
	cfg.Port = 1 // always refused
	cfg.Secure = false
	cfg.MaxReconnectAttempts = 10
	cfg.ReconnectInterval = 200 * time.Millisecond
	cfg.HandshakeTimeout = 50 * time.Millisecond

	c, _ := pkgws.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	err := c.ConnectWithRetry(ctx)
	if err == nil {
		t.Error("expected error when context cancelled during retry")
	}
}
