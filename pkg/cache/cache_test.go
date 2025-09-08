package cache

import (
	"sync"
	"testing"
	"time"
)

func TestCache_BasicGetSet(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	c := NewCache(config)
	defer c.Close()

	// Test Set and Get
	c.Set("key1", "value1", 1*time.Minute)

	val, found := c.Get("key1")
	if !found {
		t.Fatal("Expected to find key1")
	}

	if val.(string) != "value1" {
		t.Errorf("Expected 'value1', got %v", val)
	}

	// Test Get non-existent key
	_, found = c.Get("nonexistent")
	if found {
		t.Error("Expected not to find nonexistent key")
	}
}

func TestCache_TTLExpiration(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.DefaultTTL = 100 * time.Millisecond
	c := NewCache(config)
	defer c.Close()

	// Set with short TTL
	c.Set("expire-key", "expire-value", 50*time.Millisecond)

	// Should be available immediately
	val, found := c.Get("expire-key")
	if !found {
		t.Fatal("Expected to find expire-key immediately")
	}
	if val.(string) != "expire-value" {
		t.Errorf("Expected 'expire-value', got %v", val)
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	_, found = c.Get("expire-key")
	if found {
		t.Error("Expected expire-key to be expired")
	}

	// Check that miss counter increased
	stats := c.Stats()
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
}

func TestCache_LRUEviction(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.MaxSize = 3 * 1024 // 3KB total (3 entries with 1KB each)
	c := NewCache(config)
	defer c.Close()

	// Add 3 entries (fills cache)
	c.Set("key1", "value1", 1*time.Minute)
	c.Set("key2", "value2", 1*time.Minute)
	c.Set("key3", "value3", 1*time.Minute)

	// Access in order: key1, key2, key3 (key1 becomes LRU)
	if _, found := c.Get("key1"); !found {
		t.Error("Expected key1 to be present")
	}
	if _, found := c.Get("key2"); !found {
		t.Error("Expected key2 to be present")
	}
	if _, found := c.Get("key3"); !found {
		t.Error("Expected key3 to be present")
	}

	// Add 4th entry (should evict oldest which is key1 since it was accessed first)
	c.Set("key4", "value4", 1*time.Minute)

	// key1 should be evicted (it's the LRU)
	if _, found := c.Get("key1"); found {
		t.Error("Expected key1 to be evicted")
	}

	// Others should still be present
	if _, found := c.Get("key2"); !found {
		t.Error("Expected key2 to be present after eviction")
	}
	if _, found := c.Get("key3"); !found {
		t.Error("Expected key3 to be present after eviction")
	}
	if _, found := c.Get("key4"); !found {
		t.Error("Expected key4 to be present after eviction")
	}

	stats := c.Stats()
	if stats.Evictions < 1 {
		t.Errorf("Expected at least 1 eviction, got %d", stats.Evictions)
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	c := NewCache(config)
	defer c.Close()

	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent writes
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := "concurrent-" + string(rune(id))
				c.Set(key, id, 1*time.Minute)
			}
		}(i)
	}

	wg.Wait()

	// Concurrent reads
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := "concurrent-" + string(rune(id))
				c.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// Should have entries
	stats := c.Stats()
	if stats.Entries == 0 {
		t.Error("Expected some entries after concurrent access")
	}
}

func TestCache_PatternInvalidation(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	c := NewCache(config)
	defer c.Close()

	// Add entries with different path patterns
	c.Set("/nodes/node1/status", "data1", 1*time.Minute)
	c.Set("/nodes/node2/status", "data2", 1*time.Minute)
	c.Set("/storage/local/status", "data3", 1*time.Minute)
	c.Set("/version", "data4", 1*time.Minute)

	// Invalidate all /nodes/* entries
	removed := c.Invalidate("/nodes/*")
	if removed != 2 {
		t.Errorf("Expected to invalidate 2 entries, got %d", removed)
	}

	// /nodes entries should be gone
	if _, found := c.Get("/nodes/node1/status"); found {
		t.Error("Expected /nodes/node1/status to be invalidated")
	}
	if _, found := c.Get("/nodes/node2/status"); found {
		t.Error("Expected /nodes/node2/status to be invalidated")
	}

	// Others should remain
	if _, found := c.Get("/storage/local/status"); !found {
		t.Error("Expected /storage/local/status to remain")
	}
	if _, found := c.Get("/version"); !found {
		t.Error("Expected /version to remain")
	}
}

func TestCache_ExactInvalidation(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	c := NewCache(config)
	defer c.Close()

	c.Set("exact-key", "value", 1*time.Minute)
	c.Set("exact-key-other", "value2", 1*time.Minute)

	// Exact match invalidation (no wildcard)
	removed := c.Invalidate("exact-key")
	if removed != 1 {
		t.Errorf("Expected to invalidate 1 entry, got %d", removed)
	}

	if _, found := c.Get("exact-key"); found {
		t.Error("Expected exact-key to be invalidated")
	}
	if _, found := c.Get("exact-key-other"); !found {
		t.Error("Expected exact-key-other to remain")
	}
}

func TestCache_Clear(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	c := NewCache(config)
	defer c.Close()

	// Add multiple entries
	c.Set("key1", "value1", 1*time.Minute)
	c.Set("key2", "value2", 1*time.Minute)
	c.Set("key3", "value3", 1*time.Minute)

	stats := c.Stats()
	if stats.Entries != 3 {
		t.Errorf("Expected 3 entries, got %d", stats.Entries)
	}

	// Clear all
	c.Clear()

	stats = c.Stats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries after Clear, got %d", stats.Entries)
	}
	if stats.Size != 0 {
		t.Errorf("Expected 0 size after Clear, got %d", stats.Size)
	}
}

func TestCache_Stats(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	c := NewCache(config)
	defer c.Close()

	// Initial stats
	stats := c.Stats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Evictions != 0 {
		t.Error("Expected all stats to be 0 initially")
	}

	// Add entry and hit it
	c.Set("key1", "value1", 1*time.Minute)
	c.Get("key1") // hit
	c.Get("key1") // hit

	stats = c.Stats()
	if stats.Hits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.Hits)
	}
	if stats.HitRate != 1.0 {
		t.Errorf("Expected hit rate 1.0, got %f", stats.HitRate)
	}

	// Miss
	c.Get("nonexistent")

	stats = c.Stats()
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	expectedHitRate := 2.0 / 3.0
	if stats.HitRate < expectedHitRate-0.01 || stats.HitRate > expectedHitRate+0.01 {
		t.Errorf("Expected hit rate ~0.67, got %f", stats.HitRate)
	}
}

func TestCache_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false // Disabled
	c := NewCache(config)
	defer c.Close()

	// Operations should be no-ops
	c.Set("key1", "value1", 1*time.Minute)

	_, found := c.Get("key1")
	if found {
		t.Error("Expected cache to be disabled, but Get returned found=true")
	}

	stats := c.Stats()
	if stats.Entries != 0 {
		t.Error("Expected 0 entries when cache is disabled")
	}
}

func TestCache_UpdateExisting(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	c := NewCache(config)
	defer c.Close()

	// Set initial value
	c.Set("key1", "value1", 1*time.Minute)

	val, found := c.Get("key1")
	if !found || val.(string) != "value1" {
		t.Fatal("Expected to find initial value")
	}

	// Update with new value
	c.Set("key1", "value2", 1*time.Minute)

	val, found = c.Get("key1")
	if !found {
		t.Fatal("Expected to find updated value")
	}
	if val.(string) != "value2" {
		t.Errorf("Expected 'value2', got %v", val)
	}

	// Should still have only 1 entry
	stats := c.Stats()
	if stats.Entries != 1 {
		t.Errorf("Expected 1 entry after update, got %d", stats.Entries)
	}
}

func TestCache_CleanupLoop(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.CleanupInterval = 50 * time.Millisecond
	c := NewCache(config)
	defer c.Close()

	// Add entries with short TTL
	c.Set("cleanup1", "value1", 30*time.Millisecond)
	c.Set("cleanup2", "value2", 30*time.Millisecond)
	c.Set("cleanup3", "value3", 5*time.Minute) // This one won't expire

	// Wait for cleanup to run
	time.Sleep(150 * time.Millisecond)

	// Expired entries should be cleaned up
	if _, found := c.Get("cleanup1"); found {
		t.Error("Expected cleanup1 to be cleaned up")
	}
	if _, found := c.Get("cleanup2"); found {
		t.Error("Expected cleanup2 to be cleaned up")
	}

	// Non-expired should remain
	if _, found := c.Get("cleanup3"); !found {
		t.Error("Expected cleanup3 to remain")
	}
}

func TestGenerateKey(t *testing.T) {
	// Test consistent key generation
	key1 := GenerateKey("GET", "/nodes", map[string]interface{}{"node": "pve1"})
	key2 := GenerateKey("GET", "/nodes", map[string]interface{}{"node": "pve1"})

	if key1 != key2 {
		t.Error("Expected identical keys for identical inputs")
	}

	// Different method should produce different key
	key3 := GenerateKey("POST", "/nodes", map[string]interface{}{"node": "pve1"})
	if key1 == key3 {
		t.Error("Expected different keys for different methods")
	}

	// Different path should produce different key
	key4 := GenerateKey("GET", "/storage", map[string]interface{}{"node": "pve1"})
	if key1 == key4 {
		t.Error("Expected different keys for different paths")
	}

	// Different params should produce different key
	key5 := GenerateKey("GET", "/nodes", map[string]interface{}{"node": "pve2"})
	if key1 == key5 {
		t.Error("Expected different keys for different params")
	}

	// Param order shouldn't matter (sorted internally)
	key6 := GenerateKey("GET", "/nodes", map[string]interface{}{
		"node": "pve1",
		"type": "qemu",
	})
	key7 := GenerateKey("GET", "/nodes", map[string]interface{}{
		"type": "qemu",
		"node": "pve1",
	})
	if key6 != key7 {
		t.Error("Expected identical keys regardless of param order")
	}
}

func TestGenerateKeyFromURL(t *testing.T) {
	key1 := GenerateKeyFromURL("GET", "https://pve.example.com/api2/json/nodes/pve1")
	key2 := GenerateKeyFromURL("GET", "https://pve.example.com/api2/json/nodes/pve1")

	if key1 != key2 {
		t.Error("Expected identical keys for identical URLs")
	}

	key3 := GenerateKeyFromURL("GET", "https://pve.example.com/api2/json/nodes/pve2")
	if key1 == key3 {
		t.Error("Expected different keys for different URLs")
	}
}

func TestShouldCache(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"GET", true},
		{"get", true},
		{"Get", true},
		{"POST", false},
		{"PUT", false},
		{"DELETE", false},
		{"PATCH", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := ShouldCache(tt.method); got != tt.want {
				t.Errorf("ShouldCache(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		key     string
		pattern string
		want    bool
	}{
		{"/nodes/pve1", "/nodes/*", true},
		{"/nodes/pve2", "/nodes/*", true},
		{"/storage/local", "/nodes/*", false},
		{"/nodes/pve1/status", "/nodes/pve1/*", true},
		{"/nodes/pve2/status", "/nodes/pve1/*", false},
		{"/version", "/version", true},
		{"/version2", "/version", false},
		{"", "", false},
		{"/nodes", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.key+" vs "+tt.pattern, func(t *testing.T) {
			if got := matchPattern(tt.key, tt.pattern); got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.key, tt.pattern, got, tt.want)
			}
		})
	}
}
