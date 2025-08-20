// Package pool provides connection pooling for the PVE API client.
package pool

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Config represents the connection pool configuration.
type Config struct {
	// MaxConnections is the maximum number of idle connections.
	MaxConnections int

	// MaxConnectionsPerHost is the maximum number of idle connections per host.
	MaxConnectionsPerHost int

	// IdleTimeout is the maximum amount of time a connection may be idle.
	IdleTimeout time.Duration

	// ConnectionTimeout is the maximum amount of time to wait for a connection.
	ConnectionTimeout time.Duration

	// MaxIdleTime is the maximum amount of time an idle connection is kept.
	MaxIdleTime time.Duration

	// EnableHTTP2 enables HTTP/2 support.
	EnableHTTP2 bool
}

// DefaultConfig returns the default pool configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxConnections:        100,
		MaxConnectionsPerHost: 10,
		IdleTimeout:           90 * time.Second,
		ConnectionTimeout:     30 * time.Second,
		MaxIdleTime:           600 * time.Second,
		EnableHTTP2:           true,
	}
}

// Pool manages a pool of HTTP connections.
type Pool struct {
	config    *Config
	transport *http.Transport
	client    *http.Client
	mu        sync.RWMutex
	stats     *Stats
	closed    bool
}

// Stats contains pool statistics.
type Stats struct {
	ActiveConnections   int64
	IdleConnections     int64
	TotalConnections    int64
	FailedConnections   int64
	RequestsServed      int64
	BytesSent           int64
	BytesReceived       int64
	AverageResponseTime time.Duration
	mu                  sync.RWMutex
}

// New creates a new connection pool with the given configuration.
func New(config *Config) *Pool {
	if config == nil {
		config = DefaultConfig()
	}

	transport := &http.Transport{
		MaxIdleConns:          config.MaxConnections,
		MaxIdleConnsPerHost:   config.MaxConnectionsPerHost,
		IdleConnTimeout:       config.IdleTimeout,
		ResponseHeaderTimeout: config.ConnectionTimeout,
		DisableCompression:    false,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     config.EnableHTTP2,
	}

	return &Pool{
		config:    config,
		transport: transport,
		client: &http.Client{
			Transport: transport,
			Timeout:   config.ConnectionTimeout,
		},
		stats: &Stats{},
	}
}

// Get returns an HTTP client from the pool.
func (p *Pool) Get() (*http.Client, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return nil, fmt.Errorf("pool is closed")
	}

	p.stats.mu.Lock()
	p.stats.ActiveConnections++
	p.stats.TotalConnections++
	p.stats.mu.Unlock()

	return p.client, nil
}

// Put returns a client to the pool.
func (p *Pool) Put(client *http.Client) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return
	}

	p.stats.mu.Lock()
	p.stats.ActiveConnections--
	p.stats.IdleConnections++
	p.stats.mu.Unlock()
}

// Do executes an HTTP request using a pooled connection.
func (p *Pool) Do(req *http.Request) (*http.Response, error) {
	return p.DoWithContext(context.Background(), req)
}

// DoWithContext executes an HTTP request with context using a pooled connection.
func (p *Pool) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error) {
	client, err := p.Get()
	if err != nil {
		return nil, err
	}
	defer p.Put(client)

	start := time.Now()
	req = req.WithContext(ctx)

	// Track request
	p.stats.mu.Lock()
	p.stats.RequestsServed++
	if req.ContentLength > 0 {
		p.stats.BytesSent += req.ContentLength
	}
	p.stats.mu.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		p.stats.mu.Lock()
		p.stats.FailedConnections++
		p.stats.mu.Unlock()
		return nil, err
	}

	// Track response
	elapsed := time.Since(start)
	p.stats.mu.Lock()
	if resp.ContentLength > 0 {
		p.stats.BytesReceived += resp.ContentLength
	}
	// Update average response time
	if p.stats.AverageResponseTime == 0 {
		p.stats.AverageResponseTime = elapsed
	} else {
		p.stats.AverageResponseTime = (p.stats.AverageResponseTime + elapsed) / 2
	}
	p.stats.mu.Unlock()

	return resp, nil
}

// Stats returns the current pool statistics.
func (p *Pool) Stats() Stats {
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()

	return Stats{
		ActiveConnections:   p.stats.ActiveConnections,
		IdleConnections:     p.stats.IdleConnections,
		TotalConnections:    p.stats.TotalConnections,
		FailedConnections:   p.stats.FailedConnections,
		RequestsServed:      p.stats.RequestsServed,
		BytesSent:           p.stats.BytesSent,
		BytesReceived:       p.stats.BytesReceived,
		AverageResponseTime: p.stats.AverageResponseTime,
	}
}

// Close closes the connection pool.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	p.transport.CloseIdleConnections()
	return nil
}

// SetMaxConnections updates the maximum number of connections.
func (p *Pool) SetMaxConnections(max int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config.MaxConnections = max
	p.transport.MaxIdleConns = max
}

// SetMaxConnectionsPerHost updates the maximum connections per host.
func (p *Pool) SetMaxConnectionsPerHost(max int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config.MaxConnectionsPerHost = max
	p.transport.MaxIdleConnsPerHost = max
}

// IsHealthy checks if the pool is healthy.
func (p *Pool) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return false
	}

	stats := p.Stats()
	// Consider unhealthy if too many failures
	if stats.TotalConnections > 0 {
		failureRate := float64(stats.FailedConnections) / float64(stats.TotalConnections)
		if failureRate > 0.5 {
			return false
		}
	}

	return true
}
