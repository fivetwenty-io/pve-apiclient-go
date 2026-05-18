package websocket_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	pkgws "github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/websocket"
)

// --- helpers ---

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// newTestServer returns an httptest.Server that:
//   - POST /api2/json/* → replies with the given JSON body and status code
//   - GET /api2/json/*  → upgrades to WebSocket, sends one text frame, then closes
func newTestServer(t *testing.T, postBody string, postStatus int) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/api2/json/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(postStatus)
			_, _ = fmt.Fprint(w, postBody)

		case http.MethodGet:
			conn, err := wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				http.Error(w, "upgrade failed", http.StatusInternalServerError)

				return
			}

			defer func() { _ = conn.Close() }()

			// Send a single text frame then close normally.
			_ = conn.WriteMessage(websocket.TextMessage, []byte("hello"))
			_ = conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	return httptest.NewServer(mux)
}

// hostPort splits "host:port" from an httptest.Server URL.
func hostPort(srv *httptest.Server) (string, int) {
	addr := srv.Listener.Addr().String()
	// addr = "127.0.0.1:PORT"
	lastColon := strings.LastIndex(addr, ":")
	if lastColon < 0 {
		return addr, 80
	}

	portStr := addr[lastColon+1:]

	var port int

	_, _ = fmt.Sscanf(portStr, "%d", &port)

	return addr[:lastColon], port
}

// validVNCResponse wraps a VNCSession in the PVE data envelope.
func validVNCResponse(port int, ticket string) string {
	body, _ := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"port":   port,
			"ticket": ticket,
			"upid":   "UPID:pve1:00001234:00000000:6789ABCD:vncproxy:100:root@pam:",
			"user":   "root@pam",
			"cert":   "fakecert",
		},
	})

	return string(body)
}

func validTermResponse(port int, ticket string) string {
	body, _ := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"port":   port,
			"ticket": ticket,
			"upid":   "UPID:pve1:00001234:00000000:6789ABCD:termproxy:100:root@pam:",
			"user":   "root@pam",
		},
	})

	return string(body)
}

func validSpiceResponse() string {
	body, _ := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"host":     "192.0.2.1",
			"password": "spicepass",
			"proxy":    "192.0.2.1",
			"tls-port": 61000,
			"type":     "spice",
		},
	})

	return string(body)
}

// proxyClientFor builds a ProxyClient pointing at an httptest.Server (plain HTTP/WS).
func proxyClientFor(t *testing.T, srv *httptest.Server) *pkgws.ProxyClient {
	t.Helper()

	host, port := hostPort(srv)

	client, err := pkgws.NewProxyClient(&pkgws.ProxyConfig{
		Host:      host,
		Port:      port,
		Scheme:    "http", // plain http test server; ws (not wss) for WebSocket
		Insecure:  true,
		Ticket:    "PVECLUSTERID:testticket",
		CSRFToken: "testcsrf",
	})
	if err != nil {
		t.Fatalf("NewProxyClient: %v", err)
	}

	return client
}

// --- NewProxyClient validation tests ---

func TestNewProxyClient_NilConfig(t *testing.T) {
	_, err := pkgws.NewProxyClient(nil)
	if !errors.Is(err, pkgws.ErrNodeRequired) {
		t.Fatalf("expected ErrNodeRequired, got %v", err)
	}
}

func TestNewProxyClient_EmptyHost(t *testing.T) {
	_, err := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: ""})
	if !errors.Is(err, pkgws.ErrNodeRequired) {
		t.Fatalf("expected ErrNodeRequired, got %v", err)
	}
}

func TestNewProxyClient_DefaultPort(t *testing.T) {
	// Port 0 should be defaulted to 8006; construction should succeed.
	c, err := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1", Port: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c == nil {
		t.Fatal("expected non-nil ProxyClient")
	}
}

// --- VMVNCProxy happy path ---

func TestVMVNCProxy_HappyPath(t *testing.T) {
	srv := newTestServer(t, validVNCResponse(5900, "VNC:testticket"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)
	ctx := context.Background()

	session, err := client.VMVNCProxy(ctx, "pve1", 100)
	if err != nil {
		t.Fatalf("VMVNCProxy: %v", err)
	}

	if session.Port != 5900 {
		t.Errorf("port: want 5900, got %d", session.Port)
	}

	if session.Ticket != "VNC:testticket" {
		t.Errorf("ticket: want VNC:testticket, got %q", session.Ticket)
	}

	if session.User != "root@pam" {
		t.Errorf("user: want root@pam, got %q", session.User)
	}
}

// --- VMVNCProxy input validation ---

func TestVMVNCProxy_EmptyNode(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	_, err := c.VMVNCProxy(context.Background(), "", 100)

	if !errors.Is(err, pkgws.ErrNodeRequired) {
		t.Fatalf("expected ErrNodeRequired, got %v", err)
	}
}

func TestVMVNCProxy_InvalidVMID(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	_, err := c.VMVNCProxy(context.Background(), "pve1", 50)

	if !errors.Is(err, pkgws.ErrVMIDInvalid) {
		t.Fatalf("expected ErrVMIDInvalid, got %v", err)
	}
}

// --- VMVNCProxy 4xx error ---

func TestVMVNCProxy_HTTPError(t *testing.T) {
	errBody := `{"errors":{"permission":"VM.Console required"}}`

	srv := newTestServer(t, errBody, http.StatusForbidden)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	_, err := client.VMVNCProxy(context.Background(), "pve1", 100)
	if err == nil {
		t.Fatal("expected error on 403 response")
	}

	if !errors.Is(err, pkgws.ErrHTTPError) {
		t.Errorf("expected ErrHTTPError, got: %v", err)
	}
}

// --- VMVNCConnect happy path ---

func TestVMVNCConnect_HappyPath(t *testing.T) {
	// POST returns VNC session; GET upgrades WebSocket.
	srv := newTestServer(t, validVNCResponse(5900, "VNC:testticket"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)
	ctx := context.Background()

	session := &pkgws.VNCSession{Port: 5900, Ticket: "VNC:testticket", User: "root@pam"}

	conn, err := client.VMVNCConnect(ctx, "pve1", 100, session)
	if err != nil {
		t.Fatalf("VMVNCConnect: %v", err)
	}

	defer func() { _ = conn.Close() }()

	// Read the "hello" frame the test server sends.
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if msgType != websocket.TextMessage {
		t.Errorf("message type: want %d, got %d", websocket.TextMessage, msgType)
	}

	if string(data) != "hello" {
		t.Errorf("data: want hello, got %q", string(data))
	}
}

// --- VMVNCConnect input validation ---

func TestVMVNCConnect_NilSession(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	_, err := c.VMVNCConnect(context.Background(), "pve1", 100, nil)

	if !errors.Is(err, pkgws.ErrNilSession) {
		t.Fatalf("expected ErrNilSession, got %v", err)
	}
}

func TestVMVNCConnect_NoTicket(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	session := &pkgws.VNCSession{Port: 5900}
	_, err := c.VMVNCConnect(context.Background(), "pve1", 100, session)

	if !errors.Is(err, pkgws.ErrSessionNoTicket) {
		t.Fatalf("expected ErrSessionNoTicket, got %v", err)
	}
}

func TestVMVNCConnect_NoPort(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	session := &pkgws.VNCSession{Ticket: "VNC:tk"}
	_, err := c.VMVNCConnect(context.Background(), "pve1", 100, session)

	if !errors.Is(err, pkgws.ErrSessionNoPort) {
		t.Fatalf("expected ErrSessionNoPort, got %v", err)
	}
}

func TestVMVNCConnect_PortOutOfRange(t *testing.T) {
	srv := newTestServer(t, validVNCResponse(5900, "VNC:tk"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)
	// Port outside 5900-5999 should be rejected before dialing.
	session := &pkgws.VNCSession{Port: 9999, Ticket: "VNC:tk"}
	_, err := client.VMVNCConnect(context.Background(), "pve1", 100, session)

	if !errors.Is(err, pkgws.ErrPortOutOfRange) {
		t.Fatalf("expected ErrPortOutOfRange, got %v", err)
	}
}

// --- VMTermProxy ---

func TestVMTermProxy_HappyPath(t *testing.T) {
	srv := newTestServer(t, validTermResponse(5901, "TERM:tk"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	session, err := client.VMTermProxy(context.Background(), "pve1", 100, "")
	if err != nil {
		t.Fatalf("VMTermProxy: %v", err)
	}

	if session.Port != 5901 {
		t.Errorf("port: want 5901, got %d", session.Port)
	}
}

func TestVMTermProxy_WithSerial(t *testing.T) {
	srv := newTestServer(t, validTermResponse(5902, "TERM:serial"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	session, err := client.VMTermProxy(context.Background(), "pve1", 100, "serial0")
	if err != nil {
		t.Fatalf("VMTermProxy with serial: %v", err)
	}

	if session.Ticket != "TERM:serial" {
		t.Errorf("ticket: want TERM:serial, got %q", session.Ticket)
	}
}

func TestVMTermProxy_InputValidation(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})

	_, err := c.VMTermProxy(context.Background(), "", 100, "")
	if !errors.Is(err, pkgws.ErrNodeRequired) {
		t.Errorf("empty node: expected ErrNodeRequired, got %v", err)
	}

	_, err = c.VMTermProxy(context.Background(), "pve1", 99, "")
	if !errors.Is(err, pkgws.ErrVMIDInvalid) {
		t.Errorf("vmid 99: expected ErrVMIDInvalid, got %v", err)
	}
}

// --- VMSpiceProxy ---

func TestVMSpiceProxy_HappyPath(t *testing.T) {
	srv := newTestServer(t, validSpiceResponse(), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	session, err := client.VMSpiceProxy(context.Background(), "pve1", 100, "")
	if err != nil {
		t.Fatalf("VMSpiceProxy: %v", err)
	}

	if session.Host != "192.0.2.1" {
		t.Errorf("host: want 192.0.2.1, got %q", session.Host)
	}

	if session.TLSPort != 61000 {
		t.Errorf("tls-port: want 61000, got %d", session.TLSPort)
	}
}

func TestVMSpiceProxy_InputValidation(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})

	_, err := c.VMSpiceProxy(context.Background(), "pve1", 50, "")
	if !errors.Is(err, pkgws.ErrVMIDInvalid) {
		t.Errorf("expected ErrVMIDInvalid, got %v", err)
	}
}

// --- VMTunnelWebSocket ---

func TestVMTunnelWebSocket_HappyPath(t *testing.T) {
	srv := newTestServer(t, "", http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	conn, err := client.VMTunnelWebSocket(
		context.Background(), "pve1", 100, "/var/run/qemu-100.sock", "tunnel-ticket",
	)
	if err != nil {
		t.Fatalf("VMTunnelWebSocket: %v", err)
	}

	defer func() { _ = conn.Close() }()
}

func TestVMTunnelWebSocket_MissingSocket(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	_, err := c.VMTunnelWebSocket(context.Background(), "pve1", 100, "", "ticket")

	if !errors.Is(err, pkgws.ErrSocketRequired) {
		t.Fatalf("expected ErrSocketRequired, got %v", err)
	}
}

func TestVMTunnelWebSocket_MissingTicket(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	_, err := c.VMTunnelWebSocket(context.Background(), "pve1", 100, "/run/sock", "")

	if !errors.Is(err, pkgws.ErrTicketRequired) {
		t.Fatalf("expected ErrTicketRequired, got %v", err)
	}
}

// --- LXC endpoints ---

func TestLXCVNCProxy_HappyPath(t *testing.T) {
	srv := newTestServer(t, validVNCResponse(5910, "VNC:lxc"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	session, err := client.LXCVNCProxy(context.Background(), "pve1", 200)
	if err != nil {
		t.Fatalf("LXCVNCProxy: %v", err)
	}

	if session.Port != 5910 {
		t.Errorf("port: want 5910, got %d", session.Port)
	}
}

func TestLXCVNCConnect_HappyPath(t *testing.T) {
	srv := newTestServer(t, "", http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)
	session := &pkgws.VNCSession{Port: 5910, Ticket: "VNC:lxc", User: "root@pam"}

	conn, err := client.LXCVNCConnect(context.Background(), "pve1", 200, session)
	if err != nil {
		t.Fatalf("LXCVNCConnect: %v", err)
	}

	defer func() { _ = conn.Close() }()
}

func TestLXCTermProxy_HappyPath(t *testing.T) {
	srv := newTestServer(t, validTermResponse(5911, "TERM:lxc"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	session, err := client.LXCTermProxy(context.Background(), "pve1", 200)
	if err != nil {
		t.Fatalf("LXCTermProxy: %v", err)
	}

	if session.Ticket != "TERM:lxc" {
		t.Errorf("ticket: want TERM:lxc, got %q", session.Ticket)
	}
}

func TestLXCSpiceProxy_HappyPath(t *testing.T) {
	srv := newTestServer(t, validSpiceResponse(), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	session, err := client.LXCSpiceProxy(context.Background(), "pve1", 200, "")
	if err != nil {
		t.Fatalf("LXCSpiceProxy: %v", err)
	}

	if session.Type != "spice" {
		t.Errorf("type: want spice, got %q", session.Type)
	}
}

func TestLXCTunnelWebSocket_HappyPath(t *testing.T) {
	srv := newTestServer(t, "", http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	conn, err := client.LXCTunnelWebSocket(
		context.Background(), "pve1", 200, "/var/run/lxc-200.sock", "lxc-tunnel-ticket",
	)
	if err != nil {
		t.Fatalf("LXCTunnelWebSocket: %v", err)
	}

	defer func() { _ = conn.Close() }()
}

func TestLXCTunnelWebSocket_InputValidation(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})

	_, err := c.LXCTunnelWebSocket(context.Background(), "pve1", 200, "", "ticket")
	if !errors.Is(err, pkgws.ErrSocketRequired) {
		t.Errorf("expected ErrSocketRequired, got %v", err)
	}

	_, err = c.LXCTunnelWebSocket(context.Background(), "pve1", 200, "/run/s", "")
	if !errors.Is(err, pkgws.ErrTicketRequired) {
		t.Errorf("expected ErrTicketRequired, got %v", err)
	}
}

func TestLXCEndpoints_InputValidation(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	ctx := context.Background()

	if _, err := c.LXCVNCProxy(ctx, "", 200); !errors.Is(err, pkgws.ErrNodeRequired) {
		t.Errorf("LXCVNCProxy empty node: got %v", err)
	}

	if _, err := c.LXCTermProxy(ctx, "pve1", 50); !errors.Is(err, pkgws.ErrVMIDInvalid) {
		t.Errorf("LXCTermProxy low vmid: got %v", err)
	}

	if _, err := c.LXCSpiceProxy(ctx, "pve1", 50, ""); !errors.Is(err, pkgws.ErrVMIDInvalid) {
		t.Errorf("LXCSpiceProxy low vmid: got %v", err)
	}

	if _, err := c.LXCVNCConnect(ctx, "pve1", 200, nil); !errors.Is(err, pkgws.ErrNilSession) {
		t.Errorf("LXCVNCConnect nil session: got %v", err)
	}
}

// --- Node-level endpoints ---

func TestNodeVNCShell_HappyPath(t *testing.T) {
	srv := newTestServer(t, validVNCResponse(5920, "VNC:node"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	session, err := client.NodeVNCShell(context.Background(), "pve1", "login")
	if err != nil {
		t.Fatalf("NodeVNCShell: %v", err)
	}

	if session.Port != 5920 {
		t.Errorf("port: want 5920, got %d", session.Port)
	}
}

func TestNodeVNCShell_EmptyCmd(t *testing.T) {
	srv := newTestServer(t, validVNCResponse(5920, "VNC:node2"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)
	// Empty cmd is valid (server uses its default).
	session, err := client.NodeVNCShell(context.Background(), "pve1", "")
	if err != nil {
		t.Fatalf("NodeVNCShell empty cmd: %v", err)
	}

	if session.Ticket != "VNC:node2" {
		t.Errorf("ticket mismatch: got %q", session.Ticket)
	}
}

func TestNodeVNCShell_InputValidation(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	_, err := c.NodeVNCShell(context.Background(), "", "login")

	if !errors.Is(err, pkgws.ErrNodeRequired) {
		t.Fatalf("expected ErrNodeRequired, got %v", err)
	}
}

func TestNodeVNCConnect_HappyPath(t *testing.T) {
	srv := newTestServer(t, "", http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)
	session := &pkgws.VNCSession{Port: 5920, Ticket: "VNC:node", User: "root@pam"}

	conn, err := client.NodeVNCConnect(context.Background(), "pve1", session)
	if err != nil {
		t.Fatalf("NodeVNCConnect: %v", err)
	}

	defer func() { _ = conn.Close() }()
}

func TestNodeVNCConnect_InputValidation(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})

	_, err := c.NodeVNCConnect(context.Background(), "", &pkgws.VNCSession{Port: 5920, Ticket: "t"})
	if !errors.Is(err, pkgws.ErrNodeRequired) {
		t.Errorf("empty node: got %v", err)
	}

	_, err = c.NodeVNCConnect(context.Background(), "pve1", nil)
	if !errors.Is(err, pkgws.ErrNilSession) {
		t.Errorf("nil session: got %v", err)
	}
}

func TestNodeTermProxy_HappyPath(t *testing.T) {
	srv := newTestServer(t, validTermResponse(5921, "TERM:node"), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	session, err := client.NodeTermProxy(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("NodeTermProxy: %v", err)
	}

	if session.Port != 5921 {
		t.Errorf("port: want 5921, got %d", session.Port)
	}
}

func TestNodeTermProxy_InputValidation(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	_, err := c.NodeTermProxy(context.Background(), "")

	if !errors.Is(err, pkgws.ErrNodeRequired) {
		t.Fatalf("expected ErrNodeRequired, got %v", err)
	}
}

func TestNodeSpiceShell_HappyPath(t *testing.T) {
	srv := newTestServer(t, validSpiceResponse(), http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	session, err := client.NodeSpiceShell(context.Background(), "pve1", "login", "")
	if err != nil {
		t.Fatalf("NodeSpiceShell: %v", err)
	}

	if session.Password != "spicepass" {
		t.Errorf("password: want spicepass, got %q", session.Password)
	}
}

func TestNodeSpiceShell_InputValidation(t *testing.T) {
	c, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{Host: "pve1"})
	_, err := c.NodeSpiceShell(context.Background(), "", "login", "")

	if !errors.Is(err, pkgws.ErrNodeRequired) {
		t.Fatalf("expected ErrNodeRequired, got %v", err)
	}
}

// --- Conn.WriteMessage after Close ---

func TestConn_WriteAfterClose(t *testing.T) {
	srv := newTestServer(t, "", http.StatusOK)
	defer srv.Close()

	client := proxyClientFor(t, srv)

	// Use VMTunnelWebSocket to open a raw Conn without needing a valid VNC port.
	conn, err := client.VMTunnelWebSocket(
		context.Background(), "pve1", 100, "/run/test.sock", "ticket",
	)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	// Second close is a no-op.
	if err := conn.Close(); err != nil {
		t.Errorf("second close should be no-op, got %v", err)
	}

	// Write after close returns ErrWriteAfterClose.
	err = conn.WriteMessage(websocket.TextMessage, []byte("after-close"))
	if !errors.Is(err, pkgws.ErrWriteAfterClose) {
		t.Errorf("expected ErrWriteAfterClose, got %v", err)
	}
}

// --- Context cancellation ---

func TestVMVNCConnect_ContextCancelled(t *testing.T) {
	// Server that delays the WebSocket upgrade long enough for context to cancel.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			time.Sleep(500 * time.Millisecond)

			_, _ = wsUpgrader.Upgrade(w, r, nil)
		}
	}))
	defer slow.Close()

	host, port := hostPort(slow)
	client, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{
		Host:     host,
		Port:     port,
		Scheme:   "http",
		Insecure: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	session := &pkgws.VNCSession{Port: 5900, Ticket: "VNC:tk"}

	_, err := client.VMVNCConnect(ctx, "pve1", 100, session)
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

// --- Auth header injection ---

func TestAuthHeaders_InjectedOnConnect(t *testing.T) {
	var gotCookie, gotCSRF string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gotCookie = r.Header.Get("Cookie")
			gotCSRF = r.Header.Get("Csrfpreventiontoken")

			conn, err := wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}

			defer func() { _ = conn.Close() }()
		}
	}))
	defer srv.Close()

	host, port := hostPort(srv)
	client, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{
		Host:      host,
		Port:      port,
		Scheme:    "http",
		Insecure:  true,
		Ticket:    "MYTICKET",
		CSRFToken: "MYCSRF",
	})

	session := &pkgws.VNCSession{Port: 5900, Ticket: "VNC:tk"}

	conn, err := client.VMVNCConnect(context.Background(), "pve1", 100, session)
	if err != nil {
		t.Fatalf("VMVNCConnect: %v", err)
	}

	defer func() { _ = conn.Close() }()

	if !strings.Contains(gotCookie, "MYTICKET") {
		t.Errorf("cookie header: want MYTICKET in %q", gotCookie)
	}

	if gotCSRF != "MYCSRF" {
		t.Errorf("CSRF header: want MYCSRF, got %q", gotCSRF)
	}
}

// --- HTTPDoer integration ---

type fakeHTTPDoer struct {
	response interface{}
	err      error
}

func (f *fakeHTTPDoer) PostJSON(
	_ context.Context, _ string, _ map[string]string, dst interface{},
) error {
	if f.err != nil {
		return f.err
	}
	// dst is *pveDataEnvelope; marshal/unmarshal into it via JSON round-trip.
	b, err := json.Marshal(f.response)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, dst)
}

func TestVMVNCProxy_WithHTTPDoer(t *testing.T) {
	envelope := map[string]interface{}{
		"data": map[string]interface{}{
			"port":   5930,
			"ticket": "VNC:doer",
			"upid":   "UPID:x",
			"user":   "root@pam",
			"cert":   "",
		},
	}

	doer := &fakeHTTPDoer{response: envelope}
	client, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{
		Host:       "pve1",
		HTTPClient: doer,
	})

	session, err := client.VMVNCProxy(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMVNCProxy with HTTPDoer: %v", err)
	}

	if session.Port != 5930 {
		t.Errorf("port: want 5930, got %d", session.Port)
	}
}

var errFakeConnRefused = errors.New("connection refused")

func TestVMVNCProxy_HTTPDoer_Error(t *testing.T) {
	doer := &fakeHTTPDoer{err: errFakeConnRefused}
	client, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{
		Host:       "pve1",
		HTTPClient: doer,
	})

	_, err := client.VMVNCProxy(context.Background(), "pve1", 100)
	if !errors.Is(err, errFakeConnRefused) {
		t.Fatalf("expected errFakeConnRefused, got %v", err)
	}
}

// --- Connection refused (no server) ---

func TestVMVNCProxy_ConnectionRefused(t *testing.T) {
	// Port 1 is reserved and will always be refused.
	client, _ := pkgws.NewProxyClient(&pkgws.ProxyConfig{
		Host:     "127.0.0.1",
		Port:     1,
		Insecure: true,
	})

	_, err := client.VMVNCProxy(context.Background(), "pve1", 100)
	if err == nil {
		t.Fatal("expected connection refused error")
	}
}
