// Package websocket provides WebSocket support for real-time PVE events.
package websocket

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/fivetwenty-io/pve-apiclient-go/v3/internal/constants"
	"github.com/gorilla/websocket"
)

var (
	errHostRequired             = errors.New("host is required")
	errAlreadyConnected         = errors.New("already connected")
	errFailedToConnect          = errors.New("failed to connect after %d attempts")
	errNotConnected             = errors.New("not connected")
	errPanicInReadLoop          = errors.New("panic in read loop")
	errDisconnectedReconnecting = errors.New("disconnected, attempting to reconnect")
	errReconnected              = errors.New("reconnected after %d attempts")
	errFailedToReconnect        = errors.New("failed to reconnect after %d attempts")
)

// Client represents a WebSocket client for PVE events.
type Client struct {
	conn       *websocket.Conn
	config     *Config
	url        *url.URL
	headers    http.Header
	dialer     *websocket.Dialer
	handlers   map[string][]EventHandler
	mu         sync.RWMutex
	closed     bool
	closeChan  chan struct{}
	errorChan  chan error
	reconnect  bool
	pingTicker *time.Ticker
}

// Config represents WebSocket client configuration.
type Config struct {
	// Host is the PVE host.
	Host string

	// Port is the WebSocket port (default: 8006).
	Port int

	// Path is the WebSocket endpoint path.
	Path string

	// Secure indicates whether to use WSS (default: true).
	Secure bool

	// TLSConfig is the TLS configuration.
	TLSConfig *tls.Config

	// HandshakeTimeout is the handshake timeout.
	HandshakeTimeout time.Duration

	// ReadTimeout is the read timeout.
	ReadTimeout time.Duration

	// WriteTimeout is the write timeout.
	WriteTimeout time.Duration

	// PingInterval is the interval for ping messages.
	PingInterval time.Duration

	// ReconnectInterval is the interval between reconnection attempts.
	ReconnectInterval time.Duration

	// MaxReconnectAttempts is the maximum number of reconnection attempts.
	MaxReconnectAttempts int

	// BufferSize is the read/write buffer size.
	BufferSize int
}

// DefaultConfig returns the default WebSocket configuration.
func DefaultConfig() *Config {
	return &Config{
		Host:                 "",
		Port:                 constants.ProxmoxDefaultPort,
		Path:                 "/api2/json/nodes/localhost/console",
		Secure:               true,
		TLSConfig:            nil,
		HandshakeTimeout:     constants.WebSocketHandshakeTimeout(),
		ReadTimeout:          constants.DefaultClientTimeout(),
		WriteTimeout:         constants.ShortTimeout(),
		PingInterval:         constants.DefaultClientTimeout(),
		ReconnectInterval:    constants.WebSocketReconnectInterval(),
		MaxReconnectAttempts: constants.WebSocketMaxReconnectAttempts,
		BufferSize:           constants.LargeBufferSize,
	}
}

// Event represents a PVE event.
type Event struct {
	Type      string                 `json:"type"`
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Node      string                 `json:"node,omitempty"`
	Resource  string                 `json:"resource,omitempty"`
	Action    string                 `json:"action,omitempty"`
	User      string                 `json:"user,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// EventHandler handles WebSocket events.
type EventHandler func(event *Event)

// New creates a new WebSocket client.
func New(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.Host == "" {
		return nil, errHostRequired
	}

	// Build WebSocket URL
	scheme := "wss"
	if !config.Secure {
		scheme = "ws"
	}

	wsURL := &url.URL{
		Scheme:      scheme,
		Opaque:      "",
		User:        nil,
		Host:        fmt.Sprintf("%s:%d", config.Host, config.Port),
		Path:        config.Path,
		RawPath:     "",
		OmitHost:    false,
		ForceQuery:  false,
		RawQuery:    "",
		Fragment:    "",
		RawFragment: "",
	}

	// Create dialer
	dialer := &websocket.Dialer{
		NetDial:           nil,
		NetDialContext:    nil,
		NetDialTLSContext: nil,
		Proxy:             nil,
		TLSClientConfig:   config.TLSConfig,
		HandshakeTimeout:  config.HandshakeTimeout,
		ReadBufferSize:    config.BufferSize,
		WriteBufferSize:   config.BufferSize,
		WriteBufferPool:   nil,
		Subprotocols:      nil,
		EnableCompression: false,
		Jar:               nil,
	}

	return &Client{
		conn:       nil,
		config:     config,
		url:        wsURL,
		headers:    make(http.Header),
		dialer:     dialer,
		handlers:   make(map[string][]EventHandler),
		mu:         sync.RWMutex{},
		closed:     false,
		closeChan:  make(chan struct{}),
		errorChan:  make(chan error, constants.ErrorChannelSize),
		reconnect:  true,
		pingTicker: nil,
	}, nil
}

// SetHeaders sets custom headers for the WebSocket connection.
func (c *Client) SetHeaders(headers http.Header) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.headers = headers
}

// SetAuth sets authentication headers.
func (c *Client) SetAuth(ticket, csrfToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ticket != "" {
		c.headers.Set("Cookie", "PVEAuthCookie="+ticket)
	}

	if csrfToken != "" {
		c.headers.Set("Csrfpreventiontoken", csrfToken)
	}
}

// Connect establishes the WebSocket connection.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return errAlreadyConnected
	}

	// Connect with context
	conn, resp, err := c.dialer.DialContext(ctx, c.url.String(), c.headers)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}

		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.closed = false

	// Set timeouts
	if c.config.ReadTimeout > 0 {
		err := c.conn.SetReadDeadline(time.Now().Add(c.config.ReadTimeout))
		if err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}
	}

	if c.config.WriteTimeout > 0 {
		err := c.conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
		if err != nil {
			return fmt.Errorf("failed to set write deadline: %w", err)
		}
	}

	// Set pong handler
	c.conn.SetPongHandler(func(string) error {
		if c.config.ReadTimeout > 0 {
			err := c.conn.SetReadDeadline(time.Now().Add(c.config.ReadTimeout))
			if err != nil {
				return fmt.Errorf("failed to set read deadline: %w", err)
			}
		}

		return nil
	})

	// Start ping ticker
	if c.config.PingInterval > 0 {
		c.pingTicker = time.NewTicker(c.config.PingInterval)
		go c.pingLoop()
	}

	// Start read loop
	go c.readLoop(ctx)

	return nil
}

// ConnectWithRetry connects with automatic retry on failure.
func (c *Client) ConnectWithRetry(ctx context.Context) error {
	attempts := 0

	maxAttempts := c.config.MaxReconnectAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	for attempts < maxAttempts {
		if attempts > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during reconnect: %w", ctx.Err())
			case <-time.After(c.config.ReconnectInterval):
			}
		}

		err := c.Connect(ctx)
		if err == nil {
			return nil
		}

		attempts++
		if attempts < maxAttempts {
			c.sendError(fmt.Errorf("connection attempt %d failed: %w", attempts, err))
		}
	}

	return fmt.Errorf("%w: %d", errFailedToConnect, maxAttempts)
}

// Disconnect closes the WebSocket connection.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	c.reconnect = false
	close(c.closeChan)

	if c.pingTicker != nil {
		c.pingTicker.Stop()
		c.pingTicker = nil
	}

	if c.conn != nil {
		// Send close message
		_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		err := c.conn.Close()
		c.conn = nil

		if err != nil {
			return fmt.Errorf("failed to close websocket connection: %w", err)
		}

		return nil
	}

	return nil
}

// On registers an event handler for a specific event type.
func (c *Client) On(eventType string, handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.handlers[eventType] = append(c.handlers[eventType], handler)
}

// OnAll registers a handler for all events.
func (c *Client) OnAll(handler EventHandler) {
	c.On("*", handler)
}

// Off removes event handlers for a specific event type.
func (c *Client) Off(eventType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.handlers, eventType)
}

// Send sends a message to the server.
func (c *Client) Send(data interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn == nil {
		return errNotConnected
	}

	// Set write deadline
	if c.config.WriteTimeout > 0 {
		err := c.conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
		if err != nil {
			return fmt.Errorf("failed to set write deadline: %w", err)
		}
	}

	err := c.conn.WriteJSON(data)
	if err != nil {
		return fmt.Errorf("failed to send JSON message: %w", err)
	}

	return nil
}

// SendText sends a text message to the server.
func (c *Client) SendText(text string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn == nil {
		return errNotConnected
	}

	// Set write deadline
	if c.config.WriteTimeout > 0 {
		err := c.conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
		if err != nil {
			return fmt.Errorf("failed to set write deadline: %w", err)
		}
	}

	err := c.conn.WriteMessage(websocket.TextMessage, []byte(text))
	if err != nil {
		return fmt.Errorf("failed to send text message: %w", err)
	}

	return nil
}

// Errors returns the error channel.
func (c *Client) Errors() <-chan error {
	return c.errorChan
}

// IsConnected returns whether the client is connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.conn != nil && !c.closed
}

// Subscription manages event subscriptions.
type Subscription struct {
	client    *Client
	eventType string
	handler   EventHandler
	cancel    context.CancelFunc
}

// NewSubscription creates a new subscription.
func (c *Client) NewSubscription(eventType string, handler EventHandler) *Subscription {
	ctx, cancel := context.WithCancel(context.Background())

	sub := &Subscription{
		client:    c,
		eventType: eventType,
		handler:   handler,
		cancel:    cancel,
	}

	// Wrapper handler that checks context
	wrappedHandler := func(event *Event) {
		select {
		case <-ctx.Done():
			return
		default:
			handler(event)
		}
	}

	c.On(eventType, wrappedHandler)

	return sub
}

// Cancel cancels the subscription.
func (s *Subscription) Cancel() {
	s.cancel()
	// Note: This doesn't remove the handler from the client's handler map
	// In a production implementation, you'd want to track and remove handlers
}

// Stream provides a channel-based interface for events.
type Stream struct {
	client    *Client
	eventChan chan *Event
	stopChan  chan struct{}
}

// NewStream creates a new event stream.
func (c *Client) NewStream(bufferSize int) *Stream {
	if bufferSize <= 0 {
		bufferSize = 100
	}

	stream := &Stream{
		client:    c,
		eventChan: make(chan *Event, bufferSize),
		stopChan:  make(chan struct{}),
	}

	// Register handler that sends to channel
	handler := func(event *Event) {
		select {
		case stream.eventChan <- event:
		case <-stream.stopChan:
			return
		default:
			// Channel is full, drop the event
		}
	}

	c.OnAll(handler)

	return stream
}

func (c *Client) readLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.sendError(fmt.Errorf("%w: %v", errPanicInReadLoop, r))
		}

		c.handleDisconnect(ctx)
	}()

	for {
		select {
		case <-c.closeChan:
			return
		default:
			// Set read deadline
			if c.config.ReadTimeout > 0 {
				err := c.conn.SetReadDeadline(time.Now().Add(c.config.ReadTimeout))
				if err != nil {
					c.sendError(fmt.Errorf("failed to set read deadline: %w", err))

					continue
				}
			}

			messageType, data, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					c.sendError(fmt.Errorf("read error: %w", err))
				}

				return
			}

			if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
				c.handleMessage(data)
			}
		}
	}
}

func (c *Client) pingLoop() {
	defer c.pingTicker.Stop()

	for {
		select {
		case <-c.closeChan:
			return
		case <-c.pingTicker.C:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				return
			}

			// Set write deadline
			if c.config.WriteTimeout > 0 {
				err := conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
				if err != nil {
					c.sendError(fmt.Errorf("failed to set write deadline: %w", err))

					return
				}
			}

			err := conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				c.sendError(fmt.Errorf("ping error: %w", err))

				return
			}
		}
	}
}

func (c *Client) handleMessage(data []byte) {
	var event Event

	err := json.Unmarshal(data, &event)
	if err != nil {
		// Try to handle as raw message
		event = Event{
			Type:      "raw",
			ID:        "",
			Timestamp: time.Now(),
			Node:      "",
			Resource:  "",
			Action:    "",
			User:      "",
			Status:    "",
			Data: map[string]interface{}{
				"message": string(data),
			},
		}
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Call specific handlers
	if handlers, ok := c.handlers[event.Type]; ok {
		for _, handler := range handlers {
			go handler(&event)
		}
	}

	// Call wildcard handlers
	if handlers, ok := c.handlers["*"]; ok {
		for _, handler := range handlers {
			go handler(&event)
		}
	}
}

func (c *Client) handleDisconnect(ctx context.Context) {
	c.mu.Lock()
	wasConnected := c.conn != nil
	c.conn = nil
	shouldReconnect := c.reconnect && !c.closed
	c.mu.Unlock()

	if wasConnected && shouldReconnect {
		c.sendError(errDisconnectedReconnecting)

		go c.reconnectLoop(ctx)
	}
}

func (c *Client) reconnectLoop(ctx context.Context) {
	attempts := 0
	maxAttempts := c.config.MaxReconnectAttempts

	for attempts < maxAttempts {
		select {
		case <-c.closeChan:
			return
		case <-time.After(c.config.ReconnectInterval):
			attempts++

			connectCtx, cancel := context.WithTimeout(ctx, c.config.HandshakeTimeout)
			err := c.Connect(connectCtx)

			cancel()

			if err == nil {
				c.sendError(fmt.Errorf("%w: %d", errReconnected, attempts))

				return
			}

			if attempts < maxAttempts {
				c.sendError(fmt.Errorf("reconnection attempt %d failed: %w", attempts, err))
			}
		}
	}

	c.sendError(fmt.Errorf("%w: %d", errFailedToReconnect, maxAttempts))
}

func (c *Client) sendError(err error) {
	select {
	case c.errorChan <- err:
	default:
		// Channel is full, drop the error
	}
}

// Events returns the event channel.
func (s *Stream) Events() <-chan *Event {
	return s.eventChan
}

// Stop stops the stream.
func (s *Stream) Stop() {
	close(s.stopChan)
	close(s.eventChan)
}
