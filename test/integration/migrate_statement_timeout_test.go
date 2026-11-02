package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// A migration must not inherit the request-sized statement timeout.
//
// Every pooled connection carries DB_STATEMENT_TIMEOUT (20s in production), and
// the migration runner takes a connection like any other. Building an index on a
// few million rows or backfilling a column is minutes of legitimate work; being
// cancelled at twenty seconds leaves the schema half-migrated and the deploy
// looking like it succeeded. Migration 246 (the market board's ordering column)
// is exactly this shape: two index builds and a table-wide backfill.
//
// The probe migration below asks the database what its deadline is and raises it
// in the error, so the assertion is about the value the SQL actually saw. It
// fails on purpose, which also means it leaves no row in schema_migrations.
func TestMigrationLiftsTheStatementTimeout(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(db.Close)
	ctx := context.Background()

	migrations := []database.Migration{{
		Version: 999999,
		Name:    "statement_timeout_probe",
		Hash:    "probe",
		SQL: `DO $$
		      BEGIN
		        RAISE EXCEPTION 'migration statement_timeout=%', current_setting('statement_timeout');
		      END $$;`,
	}}

	err := db.Migrate(ctx, migrations, func(string, ...any) {})
	if err == nil {
		t.Fatal("the probe migration should have failed; without the failure the deadline is untested")
	}
	if msg := err.Error(); !strings.Contains(msg, "migration statement_timeout=0") {
		t.Fatalf("the migration ran under the pool's statement_timeout, not the deploy's: %v", msg)
	}
}

// Lifting the deadline is scoped to the migration transaction: traffic keeps the
// request-sized one, or every request in the process becomes unbounded.
func TestStatementTimeoutStaysForTraffic(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(db.Close)
	ctx := context.Background()

	var timeout string
	if err := db.QueryRowUnscoped(ctx, "SELECT current_setting('statement_timeout')").Scan(&timeout); err != nil {
		t.Fatal(err)
	}
	if timeout == "0" {
		t.Fatal("statement_timeout is 0 on a pool connection; a runaway query would never be cancelled")
	}
}
