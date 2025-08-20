package pool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewPool(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "default config",
			config: nil,
		},
		{
			name: "custom config",
			config: &Config{
				MaxConnections:        50,
				MaxConnectionsPerHost: 5,
				IdleTimeout:           60 * time.Second,
				ConnectionTimeout:     20 * time.Second,
				EnableHTTP2:           false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := New(tt.config)
			if pool == nil {
				t.Fatal("New() returned nil")
			}
			if pool.stats == nil {
				t.Fatal("pool stats not initialized")
			}
			if pool.closed {
				t.Fatal("new pool should not be closed")
			}
		})
	}
}

func TestPoolGetPut(t *testing.T) {
	pool := New(DefaultConfig())
	defer pool.Close()

	// Get client
	client, err := pool.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if client == nil {
		t.Fatal("Get() returned nil client")
	}

	// Check stats
	stats := pool.Stats()
	if stats.ActiveConnections != 1 {
		t.Errorf("ActiveConnections = %d, want 1", stats.ActiveConnections)
	}

	// Put client back
	pool.Put(client)

	// Check stats after put
	stats = pool.Stats()
	if stats.ActiveConnections != 0 {
		t.Errorf("ActiveConnections after Put = %d, want 0", stats.ActiveConnections)
	}
	if stats.IdleConnections != 1 {
		t.Errorf("IdleConnections = %d, want 1", stats.IdleConnections)
	}
}

func TestPoolDo(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	pool := New(DefaultConfig())
	defer pool.Close()

	// Create request
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// Execute request
	resp, err := pool.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Check stats
	stats := pool.Stats()
	if stats.RequestsServed != 1 {
		t.Errorf("RequestsServed = %d, want 1", stats.RequestsServed)
	}
}

func TestPoolDoWithContext(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pool := New(DefaultConfig())
	defer pool.Close()

	// Test with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// Should timeout
	_, err = pool.DoWithContext(ctx, req)
	if err == nil {
		t.Fatal("DoWithContext() should have timed out")
	}

	// Check failed connection stat
	stats := pool.Stats()
	if stats.FailedConnections != 1 {
		t.Errorf("FailedConnections = %d, want 1", stats.FailedConnections)
	}
}

func TestPoolClose(t *testing.T) {
	pool := New(DefaultConfig())

	// Close pool
	err := pool.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Try to get client from closed pool
	_, err = pool.Get()
	if err == nil {
		t.Fatal("Get() should fail on closed pool")
	}

	// Close again should not error
	err = pool.Close()
	if err != nil {
		t.Fatalf("Close() on already closed pool error = %v", err)
	}
}

func TestPoolSetters(t *testing.T) {
	pool := New(DefaultConfig())
	defer pool.Close()

	// Set max connections
	pool.SetMaxConnections(200)
	if pool.config.MaxConnections != 200 {
		t.Errorf("MaxConnections = %d, want 200", pool.config.MaxConnections)
	}

	// Set max connections per host
	pool.SetMaxConnectionsPerHost(20)
	if pool.config.MaxConnectionsPerHost != 20 {
		t.Errorf("MaxConnectionsPerHost = %d, want 20", pool.config.MaxConnectionsPerHost)
	}
}

func TestPoolIsHealthy(t *testing.T) {
	pool := New(DefaultConfig())
	defer pool.Close()

	// New pool should be healthy
	if !pool.IsHealthy() {
		t.Fatal("New pool should be healthy")
	}

	// Simulate failures
	pool.stats.TotalConnections = 10
	pool.stats.FailedConnections = 6

	// Should be unhealthy with >50% failure rate
	if pool.IsHealthy() {
		t.Fatal("Pool should be unhealthy with high failure rate")
	}

	// Close pool
	pool.Close()

	// Closed pool should be unhealthy
	if pool.IsHealthy() {
		t.Fatal("Closed pool should be unhealthy")
	}
}

func TestPoolStats(t *testing.T) {
	pool := New(DefaultConfig())
	defer pool.Close()

	// Initial stats should be zero
	stats := pool.Stats()
	if stats.RequestsServed != 0 {
		t.Errorf("Initial RequestsServed = %d, want 0", stats.RequestsServed)
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "13")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	// Make multiple requests
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", server.URL, nil)
		resp, err := pool.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		resp.Body.Close()
	}

	// Check accumulated stats
	stats = pool.Stats()
	if stats.RequestsServed != 3 {
		t.Errorf("RequestsServed = %d, want 3", stats.RequestsServed)
	}
	if stats.AverageResponseTime == 0 {
		t.Error("AverageResponseTime should be > 0")
	}
}
