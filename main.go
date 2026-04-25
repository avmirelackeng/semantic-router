// Package main is the entry point for the semantic-router service.
// semantic-router is a fork of vllm-project/semantic-router that provides
// intelligent request routing based on semantic similarity.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/semantic-router/semantic-router/internal/config"
	"github.com/semantic-router/semantic-router/internal/server"
)

var (
	// Version is set at build time via ldflags.
	Version = "dev"
	// Commit is set at build time via ldflags.
	Commit = "none"
	// BuildDate is set at build time via ldflags.
	BuildDate = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting semantic-router",
		"version", Version,
		"commit", Commit,
		"build_date", BuildDate,
	)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	if cfg.Debug {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
		// Also log the loaded config summary in debug mode for easier local dev.
		slog.Debug("configuration loaded", "host", cfg.Host, "port", cfg.Port)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := server.New(cfg)
	if err != nil {
		slog.Error("failed to initialize server", "error", err)
		os.Exit(1)
	}

	// Handle graceful shutdown on SIGINT or SIGTERM.
	// Also handle SIGHUP so the process can be cleanly stopped by some
	// process managers (e.g. supervisord) that send SIGHUP on restart.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		sig := <-quit
		slog.Info("received shutdown signal", "signal", sig.String())
		cancel()
	}()

	slog.Info("server listening", "address", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))

	if err := srv.Run(ctx); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}

	// NOTE: srv.Run blocks until ctx is cancelled, so reaching here means a
	// clean shutdown completed. Log before returning so the process manager
	// (e.g. systemd) can capture the final message before the process exits.
	// Using os.Exit(0) explicitly here to make the exit code clear when tailing
	// logs alongside non-zero exits from the error path above.
	slog.Info("server shutdown complete")
	os.Exit(0)
}
