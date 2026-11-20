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
	"time"

	"github.com/google/uuid"

	"github.com/die6ojunges/todo-service/internal/config"
	"github.com/die6ojunges/todo-service/internal/httpapi"
	"github.com/die6ojunges/todo-service/internal/retry"
	"github.com/die6ojunges/todo-service/internal/server"
	"github.com/die6ojunges/todo-service/internal/store/pgstore"
	"github.com/die6ojunges/todo-service/internal/todo"
)

// startupBackoff paces reconnects while PostgreSQL is not up yet; /readyz stays 503 meanwhile.
var startupBackoff = retry.Backoff{Initial: 500 * time.Millisecond, Max: 10 * time.Second}

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

	listener, err := listen(ctx, cfg.Port)
	if err != nil {
		return err
	}
	pool, err := pgstore.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	// Deferred, so it runs after Serve returns: in-flight requests finish before the pool closes (SPEC D5 step 4).
	defer pool.Close()
	readiness := pgstore.NewSchemaReadiness(pool)
	go migrateInBackground(ctx, readiness, logger)
	return serve(ctx, listener, cfg, todo.NewService(pgstore.New(pool), uuid.New), readiness, logger)
}

func listen(ctx context.Context, port int) (net.Listener, error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("listen on port %d: %w", port, err)
	}
	return listener, nil
}

// migrateInBackground lets the server answer probes while waiting for PostgreSQL;
// it only gives up when the service is shutting down.
func migrateInBackground(ctx context.Context, readiness *pgstore.SchemaReadiness, logger *slog.Logger) {
	if err := readiness.MigrateWithRetry(ctx, startupBackoff, logger); err != nil {
		logger.Warn("stopped waiting for database", "error", err.Error())
	}
}

func serve(ctx context.Context, listener net.Listener, cfg config.Config, service *todo.Service, readiness httpapi.Pinger, logger *slog.Logger) error {
	health := httpapi.NewHealthChecks(readiness)
	router := httpapi.NewRouter(health, httpapi.NewTodoAPI(service, logger), logger)
	timeouts := server.Timeouts{Drain: cfg.ReadinessDrainDelay, Shutdown: cfg.ShutdownTimeout}
	return server.New(router, health, timeouts, logger).Serve(ctx, listener)
}
