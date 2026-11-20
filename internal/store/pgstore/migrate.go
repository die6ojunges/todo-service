package pgstore

import (
	"context"
	_ "embed" // schema.sql is compiled into the binary, so no migration files ship with the image
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// migrateLockKey identifies this service's advisory lock. Two replicas running
// CREATE TABLE IF NOT EXISTS at the same moment can still collide on the catalog
// (unique violation in pg_type), so Migrate serialises them. The value is arbitrary.
const migrateLockKey int64 = 0x746f646f // "todo"

// Migrate creates the schema if it is missing. It is idempotent and safe to call from
// every replica on startup.
//
//	if err := pgstore.Migrate(ctx, pool); err != nil { return err }
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		// The xact lock is released automatically on commit or rollback.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrateLockKey); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, schemaSQL)
		return err
	})
	if err != nil {
		return fmt.Errorf("migrate todo schema: %w", err)
	}
	return nil
}
