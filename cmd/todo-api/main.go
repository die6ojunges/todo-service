// Command todo-api runs the ToDo microservice. It only wires dependencies;
// behavior lives in internal/ packages.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/die6ojunges/todo-service/internal/config"
	"github.com/die6ojunges/todo-service/internal/httpapi"
	"github.com/die6ojunges/todo-service/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("service exited with error", "error", err.Error())
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	// SIGTERM is what Kubernetes sends on pod termination; SIGINT covers Ctrl+C locally.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", cfg.Port, err)
	}
	health := httpapi.NewHealthChecks(noDatabaseYet{})
	timeouts := server.Timeouts{Drain: cfg.ReadinessDrainDelay, Shutdown: cfg.ShutdownTimeout}
	return server.New(health.Routes(), health, timeouts, logger).Serve(ctx, listener)
}

// noDatabaseYet is a temporary readiness dependency until the Postgres store lands;
// it will be replaced by the database pool.
type noDatabaseYet struct{}

func (noDatabaseYet) Ping(context.Context) error { return nil }
