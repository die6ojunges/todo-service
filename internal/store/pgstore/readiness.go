package pgstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/die6ojunges/todo-service/internal/retry"
)

var errSchemaNotMigrated = errors.New("todo schema not migrated yet")

// Open builds a pool without connecting: connections are made on first use, so the service can
// start (and answer /livez) before PostgreSQL is up. Only a malformed URL fails here.
//
//	pool, err := pgstore.Open(ctx, cfg.DatabaseURL)
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		// pgx redacts the password in its parse errors; the URL itself is never echoed here.
		return nil, fmt.Errorf("open database (want a URL like postgres://user:password@host:5432/db): %w", err)
	}
	return pool, nil
}

// SchemaReadiness gates /readyz on the database: not ready until the schema is migrated,
// then ready while PostgreSQL answers a ping. It satisfies httpapi.Pinger.
type SchemaReadiness struct {
	pool     *pgxpool.Pool
	migrated atomic.Bool
}

// NewSchemaReadiness returns a gate that stays not ready until MigrateWithRetry succeeds.
//
//	readiness := pgstore.NewSchemaReadiness(pool)
//	go readiness.MigrateWithRetry(ctx, backoff, logger)
func NewSchemaReadiness(pool *pgxpool.Pool) *SchemaReadiness {
	return &SchemaReadiness{pool: pool}
}

// MigrateWithRetry runs Migrate until it succeeds or ctx ends, logging each failure, because
// PostgreSQL may become ready after the service (SPEC D5).
func (r *SchemaReadiness) MigrateWithRetry(ctx context.Context, backoff retry.Backoff, logger *slog.Logger) error {
	logFailure := func(err error, wait time.Duration) {
		logger.Warn("database not ready, retrying", "error", err.Error(), "retry_in", wait.String())
	}
	migrate := func(ctx context.Context) error { return Migrate(ctx, r.pool) }
	if err := retry.Until(ctx, backoff, migrate, logFailure); err != nil {
		return err
	}
	r.migrated.Store(true)
	logger.Info("database ready, schema migrated")
	return nil
}

// Ping reports whether the service can take traffic as far as the database is concerned.
func (r *SchemaReadiness) Ping(ctx context.Context) error {
	if !r.migrated.Load() {
		return errSchemaNotMigrated
	}
	return r.pool.Ping(ctx)
}
