package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewBatch(t *testing.T) {
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
				MaxBatchSize:   50,
				MaxConcurrency: 5,
				Timeout:        2 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := New(tt.config)
			if batch == nil {
				t.Fatal("New() returned nil")
			}
			if batch.Size() != 0 {
				t.Errorf("Size() = %d, want 0", batch.Size())
			}
		})
	}
}

func TestBatchAdd(t *testing.T) {
	batch := New(&Config{MaxBatchSize: 3})

	// Add requests
	for i := 0; i < 3; i++ {
		req := &Request{
			ID:     fmt.Sprintf("req-%d", i),
			Method: "GET",
			Path:   fmt.Sprintf("/api/test/%d", i),
		}
		if err := batch.Add(req); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	}

	if batch.Size() != 3 {
		t.Errorf("Size() = %d, want 3", batch.Size())
	}

	// Try to exceed limit
	err := batch.Add(&Request{Method: "GET", Path: "/overflow"})
	if err == nil {
		t.Fatal("Add() should fail when exceeding MaxBatchSize")
	}
}

func TestBatchAddDuplicate(t *testing.T) {
	batch := New(DefaultConfig())

	req1 := &Request{ID: "duplicate", Method: "GET", Path: "/test1"}
	req2 := &Request{ID: "duplicate", Method: "GET", Path: "/test2"}

	if err := batch.Add(req1); err != nil {
		t.Fatalf("Add() first request error = %v", err)
	}

	if err := batch.Add(req2); err == nil {
		t.Fatal("Add() should fail for duplicate ID")
	}
}

func TestBatchAddMultiple(t *testing.T) {
	batch := New(DefaultConfig())

	requests := []*Request{
		{Method: "GET", Path: "/test1"},
		{Method: "POST", Path: "/test2"},
		{Method: "PUT", Path: "/test3"},
	}

	if err := batch.AddMultiple(requests...); err != nil {
		t.Fatalf("AddMultiple() error = %v", err)
	}

	if batch.Size() != 3 {
		t.Errorf("Size() = %d, want 3", batch.Size())
	}
}

func TestBatchClear(t *testing.T) {
	batch := New(DefaultConfig())

	// Add some requests
	batch.Add(&Request{Method: "GET", Path: "/test"})
	batch.Add(&Request{Method: "POST", Path: "/test"})

	if batch.Size() != 2 {
		t.Errorf("Size() before clear = %d, want 2", batch.Size())
	}

	batch.Clear()

	if batch.Size() != 0 {
		t.Errorf("Size() after clear = %d, want 0", batch.Size())
	}
}

type mockHTTPClient struct {
	responses map[string]*http.Response
	callCount int32
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&m.callCount, 1)

	// Return predefined response if exists
	if resp, ok := m.responses[req.URL.Path]; ok {
		return resp, nil
	}

	// Default response
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}

func TestExecutor(t *testing.T) {
	client := &mockHTTPClient{
		responses: make(map[string]*http.Response),
	}

	executor := NewExecutor(client, DefaultConfig())

	batch := New(DefaultConfig())
	batch.Add(&Request{ID: "1", Method: "GET", Path: "/test1"})
	batch.Add(&Request{ID: "2", Method: "GET", Path: "/test2"})
	batch.Add(&Request{ID: "3", Method: "GET", Path: "/test3"})

	ctx := context.Background()
	result, err := executor.Execute(ctx, batch)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.SuccessCount != 3 {
		t.Errorf("SuccessCount = %d, want 3", result.SuccessCount)
	}

	if result.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", result.FailureCount)
	}

	if len(result.Responses) != 3 {
		t.Errorf("len(Responses) = %d, want 3", len(result.Responses))
	}
}

func TestExecutorWithCallback(t *testing.T) {
	client := &mockHTTPClient{
		responses: make(map[string]*http.Response),
	}

	executor := NewExecutor(client, DefaultConfig())

	batch := New(DefaultConfig())
	batch.Add(&Request{ID: "1", Method: "GET", Path: "/test"})

	callbackCalled := false
	callback := func(req *Request, resp *Response) {
		callbackCalled = true
		if req.ID != resp.ID {
			t.Errorf("Callback: req.ID = %s, resp.ID = %s", req.ID, resp.ID)
		}
	}

	ctx := context.Background()
	_, err := executor.ExecuteWithCallback(ctx, batch, callback)
	if err != nil {
		t.Fatalf("ExecuteWithCallback() error = %v", err)
	}

	if !callbackCalled {
		t.Error("Callback was not called")
	}
}

func TestExecutorConcurrency(t *testing.T) {
	// Create test server that tracks concurrent requests
	var maxConcurrent int32
	var currentConcurrent int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Increment current concurrent count
		current := atomic.AddInt32(&currentConcurrent, 1)

		// Update max if needed
		for {
			max := atomic.LoadInt32(&maxConcurrent)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
				break
			}
		}

		// Simulate work
		time.Sleep(50 * time.Millisecond)

		// Decrement current concurrent count
		atomic.AddInt32(&currentConcurrent, -1)

		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := &http.Client{}
	config := &Config{
		MaxBatchSize:   10,
		MaxConcurrency: 3, // Limit concurrency to 3
		Timeout:        5 * time.Second,
	}
	executor := NewExecutor(client, config)

	batch := New(config)
	// Add 10 requests
	for i := 0; i < 10; i++ {
		batch.Add(&Request{
			ID:     fmt.Sprintf("req-%d", i),
			Method: "GET",
			Path:   server.URL,
		})
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, batch)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.SuccessCount != 10 {
		t.Errorf("SuccessCount = %d, want 10", result.SuccessCount)
	}

	// Check that max concurrent requests didn't exceed limit
	if maxConcurrent > 3 {
		t.Errorf("Max concurrent requests = %d, want <= 3", maxConcurrent)
	}
}

func TestExecutorTimeout(t *testing.T) {
	// Create slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := &http.Client{}
	config := &Config{
		MaxBatchSize:   10,
		MaxConcurrency: 1,
		Timeout:        100 * time.Millisecond, // Short timeout
	}
	executor := NewExecutor(client, config)

	batch := New(config)
	batch.Add(&Request{ID: "1", Method: "GET", Path: server.URL})

	ctx := context.Background()
	result, err := executor.Execute(ctx, batch)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Should have failed due to timeout
	if result.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", result.FailureCount)
	}
}

func TestBuilder(t *testing.T) {
	builder := NewBuilder(DefaultConfig())

	batch := builder.
		AddRequest("GET", "/test1").
		AddRequest("POST", "/test2").
		AddRequestWithParams("PUT", "/test3", map[string]interface{}{
			"key": "value",
		}).
		Build()

	if batch.Size() != 3 {
		t.Errorf("Size() = %d, want 3", batch.Size())
	}

	// Check third request has params
	if batch.requests[2].Params["key"] != "value" {
		t.Error("Third request should have params")
	}
}

func TestPipeline(t *testing.T) {
	client := &mockHTTPClient{
		responses: make(map[string]*http.Response),
	}
	executor := NewExecutor(client, DefaultConfig())

	// Create pipeline with 2 batches
	pipeline := NewPipeline(executor)

	batch1 := New(DefaultConfig())
	batch1.Add(&Request{ID: "1-1", Method: "GET", Path: "/batch1/test1"})
	batch1.Add(&Request{ID: "1-2", Method: "GET", Path: "/batch1/test2"})

	batch2 := New(DefaultConfig())
	batch2.Add(&Request{ID: "2-1", Method: "GET", Path: "/batch2/test1"})
	batch2.Add(&Request{ID: "2-2", Method: "GET", Path: "/batch2/test2"})

	pipeline.AddBatch(batch1).AddBatch(batch2)

	ctx := context.Background()
	results, err := pipeline.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	// Check first batch results
	if results[0].SuccessCount != 2 {
		t.Errorf("Batch 1 SuccessCount = %d, want 2", results[0].SuccessCount)
	}

	// Check second batch results
	if results[1].SuccessCount != 2 {
		t.Errorf("Batch 2 SuccessCount = %d, want 2", results[1].SuccessCount)
	}
}
