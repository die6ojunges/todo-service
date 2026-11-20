package pgstore_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/die6ojunges/todo-service/internal/store/pgstore"
	"github.com/die6ojunges/todo-service/internal/todo"
	"github.com/die6ojunges/todo-service/internal/todo/todotest"
)

func TestPgStoreSatisfiesStoreContract(t *testing.T) {
	url := testDatabaseURL
	todotest.RunStoreContract(t, func(t *testing.T) todo.Store {
		pool := isolatedPool(t, url)
		mustMigrate(t, pool)
		return pgstore.New(pool)
	})
}

// Replicas start together, so Migrate must tolerate concurrent and repeated calls.
func TestMigrateIsIdempotentUnderConcurrency(t *testing.T) {
	pool := isolatedPool(t, testDatabaseURL)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { mustMigrate(t, pool) })
	}
	wg.Wait()
	mustMigrate(t, pool)
}

// isolatedPool gives each test its own empty schema, dropped afterwards. Tests stay independent
// and never touch tables a developer uses in the same database.
func isolatedPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin := connectPool(t, url, "")
	mustExec(t, admin, "CREATE SCHEMA "+schema)
	t.Cleanup(func() { mustExec(t, admin, "DROP SCHEMA "+schema+" CASCADE") })
	return connectPool(t, url, schema)
}

// connectPool opens a pool closed at cleanup; a non-empty schema becomes its search_path.
func connectPool(t *testing.T, url, schema string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	if schema != "" {
		cfg.ConnConfig.RuntimeParams["search_path"] = schema
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// mustExec uses a background context because it also runs in cleanup, after t.Context is done.
func mustExec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func mustMigrate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if err := pgstore.Migrate(t.Context(), pool); err != nil {
		t.Errorf("Migrate() error: %v", err)
	}
}
