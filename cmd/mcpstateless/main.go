// Command mcpstateless serves an MCP endpoint whose core holds no session.
package main

import (
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/epequeno/mcp-stateless/internal/server"
	"github.com/epequeno/mcp-stateless/internal/tools"
)

func main() {
	var (
		addr    = flag.String("addr", ":8080", "listen address")
		pinned  = flag.Bool("pinned", false, "serve the session-pinned counterfactual instead of the stateless core")
		verbose = flag.Bool("v", false, "debug logging")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	registry := tools.NewRegistry()
	// TODO(A1): register the extension surface (Tasks or MCP Apps) here. The
	// deliverable requires at least one; the placeholder builtin is not it.

	srv := &http.Server{
		Addr: *addr,
		Handler: server.New(server.Config{
			Stateless: !*pinned,
			Registry:  registry,
			Logger:    logger,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("listening", "addr", *addr, "stateless", !*pinned, "tools", registry.Count())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("listen failed", "err", err)
		os.Exit(1)
	}
}
