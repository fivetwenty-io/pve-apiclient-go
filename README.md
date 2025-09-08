# PVE API Client for Go

A Go client library for the Proxmox Virtual Environment (PVE) API.

## Features

- Full authentication support (username/password, API tokens, TFA)
- SSL/TLS certificate verification with fingerprint support
- Complete HTTP method coverage (GET, POST, PUT, DELETE)
- Error handling with detailed error types
- Connection pooling and keep-alive support
- Comprehensive test coverage
- Structured logging with redaction and per-request controls
- Metrics: snapshots and Prometheus-friendly export

## Installation

```bash
go get github.com/fivetwenty-io/pve-apiclient-go
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    
    pve "github.com/fivetwenty-io/pve-apiclient-go/pkg/client"
)

func main() {
    client, err := pve.NewClient(pve.Options{
        Host:     "pve.example.com",
        Username: "root@pam",
        Password: "secret",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Get cluster status
    status, err := client.Get("/cluster/status", nil)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Cluster status: %v\n", status)
}
```

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
    ih "github.com/fivetwenty-io/pve-apiclient-go/internal/http"
    za "github.com/fivetwenty-io/pve-apiclient-go/pkg/logging/zapadapter"
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

## Documentation

See the [documentation](https://pkg.go.dev/github.com/fivetwenty-io/pve-apiclient-go) for detailed API reference.

## Examples

Check the `cmd/examples/` directory for more comprehensive examples:

- `basic/` - Basic authentication and API calls
- `auth/` - Advanced authentication scenarios including TFA
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
