package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/fivetwenty-io/pve-apiclient-go/internal/constants"
	pve "github.com/fivetwenty-io/pve-apiclient-go/pkg/client"
	"github.com/fivetwenty-io/pve-apiclient-go/pkg/logging/slogadapter"
)

func main() {
	// Create a basic client (no call performed in this example)
	cli, _ := pve.NewClient(pve.Options{
		Host:     "localhost",
		Protocol: "https",
		Port:     constants.ProxmoxDefaultPort,
		APIToken: "user@pam!tok=secret",
	})

	// Configure slog JSON logger to stdout
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cli.SetLogger(slogadapter.New(logger))

	// Optional: tweak log configuration (redaction, headers/body sampling)
	cfg := cli.GetLogConfig()
	cfg.LogRequestHeader = true
	cfg.LogQueryParams = true
	cfg.LogResponseHeader = true
	cfg.LogResponseBody = false
	cfg.MaxBodyBytes = 1024
	cli.SetLogConfig(cfg)

	// Show per-request fields and toggles
	ctx := context.Background()
	ctx = pve.WithLogFields(ctx, map[string]interface{}{"example": "logging-slog"})
	ctx = pve.WithLogging(ctx, true)

	// Make a cheap call to demonstrate logging:
	_, _ = cli.GetRawCtx(ctx, "/version", nil)
}
