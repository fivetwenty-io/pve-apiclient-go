# PVE API Client for Go (v3)

A Go client library for the Proxmox Virtual Environment (PVE) API v3.

> **Version Note:** This is version 3 of the library, designed for compatibility with Proxmox VE API v3. The module path includes `/v3` as per Go module versioning conventions.

## Features

- Full authentication support (username/password, API tokens, TFA)
- **Auto-login**: Optional automatic authentication on first API call (v3.1.0+)
- **Request Caching**: Optional LRU cache with TTL for GET requests (v3.2.0+)
- **Context Detection**: Automatic detection of local vs remote execution (v3.2.0+)
- **LXC API**: Full container lifecycle management (v3.2.0+)
- SSL/TLS certificate verification with fingerprint support
- Complete HTTP method coverage (GET, POST, PUT, DELETE)
- Error handling with detailed error types
- Connection pooling and keep-alive support
- Structured logging with redaction and per-request controls
- Metrics: snapshots and Prometheus-friendly export

## Installation

```bash
go get github.com/fivetwenty-io/pve-apiclient-go/v3
```

> **Note:** This library uses Go module versioning. The `/v3` suffix indicates compatibility with Proxmox VE API v3.

## Quick Start

### With Auto-Login (Recommended)

```go
package main

import (
    "fmt"
    "log"

    pve "github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/client"
)

func main() {
    client, err := pve.NewClient(pve.Options{
        Host:      "pve.example.com",
        Username:  "root@pam",
        Password:  "secret",
        AutoLogin: true, // Authenticate automatically on first request
    })
    if err != nil {
        log.Fatal(err)
    }

    // No Login() needed - authentication happens automatically
    status, err := client.Get("/cluster/status", nil)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Cluster status: %v\n", status)
}
```

### Traditional Manual Login

```go
client, err := pve.NewClient(pve.Options{
    Host:     "pve.example.com",
    Username: "root@pam",
    Password: "secret",
    // AutoLogin: false (default)
})
if err != nil {
    log.Fatal(err)
}

// Explicit login call
err = client.Login()
if err != nil {
    log.Fatal(err)
}

// Now make API calls
status, err := client.Get("/cluster/status", nil)
if err != nil {
    log.Fatal(err)
}
```

## Authentication

### Auto-Login (v3.1.0+)

Auto-login provides convenient automatic authentication for simple scripts:

```go
client, _ := pve.NewClient(pve.Options{
    Host:      "pve.example.com",
    Username:  "root@pam",
    Password:  "secret",
    AutoLogin: true, // Enable auto-login
})

// Authentication happens automatically on first API call
// No need to call Login() explicitly
```

**Key Points:**
- Disabled by default (opt-in feature)
- Only applies to username/password authentication
- Thread-safe for concurrent first requests
- API tokens and pre-existing tickets don't use auto-login
- See [examples/auto-login](examples/auto-login/) for detailed examples

## Request Caching

Reduce API load and improve performance by caching GET request responses:

```go
import (
    "time"
    pve "github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/client"
)

// Configure caching
cacheConfig := pve.CacheConfig{
    Enabled:         true,
    MaxSize:         50 * 1024 * 1024,  // 50 MB
    DefaultTTL:      5 * time.Minute,   // Cache for 5 minutes
    CleanupInterval: 1 * time.Minute,   // Cleanup every minute
}

client, _ := pve.NewClient(pve.Options{
    Host:        "pve.example.com",
    Username:    "root@pam",
    Password:    "secret",
    CacheConfig: &cacheConfig,
})

// First request - cache miss (hits API)
version, _ := client.Get("/version", nil)

// Second request - cache hit (served from cache, much faster)
version, _ = client.Get("/version", nil)

// Check cache statistics
stats := client.CacheStats()
fmt.Printf("Hit rate: %.2f%%\n", stats.HitRate * 100)

// Invalidate cache entries by pattern
client.InvalidateCache("/nodes/*")  // Remove all /nodes/* entries

// Clear entire cache
client.ClearCache()
```

**Features:**
- LRU eviction when cache exceeds MaxSize
- TTL expiration for time-sensitive data
- Pattern-based invalidation (wildcards supported)
- Thread-safe concurrent access
- Real-time statistics (hits, misses, evictions, hit rate)
- Only caches GET requests (idempotent operations)

**See [examples/caching](examples/caching/) for detailed usage and best practices**

## Logging & Metrics

The client includes a retrying HTTP stack with middleware hooks, structured logging (with redaction), and basic metrics. You can toggle request logging per call and set retry behavior:

```go
ctx := context.Background()
ctx = client.WithRetries(ctx, 5)
ctx = client.WithRetryDelay(ctx, 300*time.Millisecond)
ctx = client.WithLogging(ctx, true)
ctx = client.WithLogFields(ctx, map[string]interface{}{"op":"create","vmid":100})

_, err := cli.PostRawCtx(ctx, "/nodes/pve/qemu", map[string]interface{}{"vmid":100})
if err != nil { /* ... */ }

// Per-call timeout helper
ctx, cancel := client.WithTimeout(ctx, 10*time.Second)
defer cancel()
```

Redaction and sampling are configurable via `LogConfig` on the underlying HTTP client.

### Metrics

Fetch a snapshot via:

```go
if m, ok := client.MetricsOf(cli); ok {
    fmt.Printf("requests=%d errors=%d total_ms=%d\n", m.Requests, m.Errors, m.TotalDuration/time.Millisecond)
}
```

Prometheus-style export (no external deps):

```go
// Attach a collector and expose /metrics
pm := metrics.NewDefaultMetrics()
cli.SetMetrics(pm)

http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/plain; version=0.0.4")
    _ = pm.Export(w)
})
log.Fatal(http.ListenAndServe(":2112", nil))
```
See a runnable example in `docs/examples/metrics-prom`.

### Zap Logger Adapter (optional)

For production-grade structured logging, you can enable a zap adapter behind a build tag.

1) Add the dependency and build with tag `zap`:

```
go get go.uber.org/zap
go build -tags zap ./...
```

2) Configure the logger:

```go
// only when built with -tags zap
import (
    ih "github.com/fivetwenty-io/pve-apiclient-go/v3/internal/http"
    za "github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/logging/zapadapter"
    "go.uber.org/zap"
)

// after creating your client via pve.NewClient(opts):
impl := cli // type client.Client

// internal details may change; a helper API may be added later
if c, ok := any(impl).(*pveclient.client); ok {
    if a, ok := any(c.httpClient).(*pveclient.internalHTTPAdapter); ok {
        zl, _ := zap.NewProduction()
        za.Set(a.inner, zl)
        a.inner.SetLogConfig(ih.LogConfig{
            Enabled: true,
            LogRequestHeader: false,
            LogQueryParams: true,
            LogBody: false,
            LogResponseHeader: false,
            LogResponseBody: false,
            MaxBodyBytes: 1024,
        })
    }
}
```

## Domain APIs

The library provides high-level APIs for common Proxmox VE resources:

### LXC Containers

Full lifecycle management for Linux Containers:

```go
import "github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/api/lxc"

lxcClient := lxc.NewClient(client, "pve")  // Node name
ctx := context.Background()

// Create container
config := lxc.ContainerConfig{
    VMID:         200,
    OSTemplate:   "local:vztmpl/ubuntu-22.04-standard_22.04-1_amd64.tar.zst",
    Hostname:     "test-ct",
    Memory:       1024,
    Cores:        2,
    RootFS:       "local-lvm:8",
    Net0:         "name=eth0,bridge=vmbr0,ip=dhcp",
    Unprivileged: true,
}
upid, _ := lxcClient.Create(ctx, config)

// List containers
containers, _ := lxcClient.List(ctx)

// Get status
status, _ := lxcClient.Status(ctx, 200)

// Start/Stop/Reboot
lxcClient.Start(ctx, 200)
lxcClient.Shutdown(ctx, 200, 60)  // 60s timeout
lxcClient.Stop(ctx, 200)
lxcClient.Reboot(ctx, 200)

// Update configuration
updates := map[string]interface{}{
    "memory": 2048,
    "cores":  4,
}
lxcClient.UpdateConfig(ctx, 200, updates)

// Clone container
cloneOpts := lxc.CloneOptions{
    Hostname: "test-ct-clone",
    Full:     true,
}
lxcClient.Clone(ctx, 200, 201, cloneOpts)

// Delete container
lxcClient.Delete(ctx, 200, true)  // purge=true
```

**See [examples/lxc-management](examples/lxc-management/) for complete usage**

## Documentation

See the [documentation](https://pkg.go.dev/github.com/fivetwenty-io/pve-apiclient-go/v3) for detailed API reference.

## Examples

Check the `examples/` directory for more examples:

- `basic/` - Basic authentication and API calls
- `auth/` - Advanced authentication scenarios including TFA
- `auto-login/` - Automatic authentication on first request
- `context-detection/` - Execution context detection (local vs remote)
- `caching/` - Request caching with LRU and TTL
- `lxc-management/` - LXC container lifecycle management
- `advanced/` - Advanced features like batching and streaming

## Development

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Running all checks

```bash
make all
```

## License

See LICENSE file for details.

## Contributing

Contributions are welcome! Please read the contributing guidelines before submitting PRs.
