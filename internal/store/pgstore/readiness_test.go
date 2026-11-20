package pgstore_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/die6ojunges/todo-service/internal/retry"
	"github.com/die6ojunges/todo-service/internal/store/pgstore"
)

var fastBackoff = retry.Backoff{Initial: 10 * time.Millisecond, Max: 50 * time.Millisecond}

func TestOpenRejectsMalformedURLWithoutLeakingPassword(t *testing.T) {
	_, err := pgstore.Open(t.Context(), "postgres://todo:s3cret@db:notaport/todo")
	if err == nil {
		t.Fatal("Open(malformed URL) error = nil, want a parse error")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("Open() error = %q, must not contain the password", err)
	}
}

// Port 1 refuses connections, so this runs without a database.
func TestSchemaReadinessStaysNotReadyWhileDatabaseIsDown(t *testing.T) {
	pool, err := pgstore.Open(t.Context(), "postgres://todo:todo@127.0.0.1:1/todo?connect_timeout=1")
	if err != nil {
		t.Fatalf("Open() error = %v, want a lazy pool that connects later", err)
	}
	t.Cleanup(pool.Close)
	readiness := pgstore.NewSchemaReadiness(pool)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	if err := readiness.MigrateWithRetry(ctx, fastBackoff, slog.New(slog.DiscardHandler)); err == nil {
		t.Error("MigrateWithRetry() error = nil, want it to give up when ctx ends")
	}
	if err := readiness.Ping(t.Context()); err == nil {
		t.Error("Ping() error = nil, want not ready while the database is down")
	}
}

// Ready only once the schema exists: otherwise traffic would hit a missing table.
func TestSchemaReadinessBecomesReadyAfterMigration(t *testing.T) {
	pool := isolatedPool(t, testDatabaseURL)
	readiness := pgstore.NewSchemaReadiness(pool)
	if err := readiness.Ping(t.Context()); err == nil {
		t.Fatal("Ping() before migration error = nil, want not ready")
	}
	if err := readiness.MigrateWithRetry(t.Context(), fastBackoff, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("MigrateWithRetry() error = %v", err)
	}
	if err := readiness.Ping(t.Context()); err != nil {
		t.Errorf("Ping() after migration error = %v, want ready", err)
	}
	if _, err := pgstore.New(pool).List(t.Context()); err != nil {
		t.Errorf("List() after migration error = %v, want the todos table to exist", err)
	}
}
