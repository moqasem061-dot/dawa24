package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// TestUnscopedTablesHaveNoRLS is the check that keeps database.QueryUnscoped
// honest.
//
// QueryUnscoped skips the transaction and the SET LOCAL that arms row-level
// security, which is safe only while none of the tables it reads HAS a policy.
// That is a property of the live schema, and a migration can change it without
// touching a line of Go — so asserting it in a comment would be asserting it
// nowhere. This asks the database.
//
// It needs a real database and skips without one, which means it does not run
// on a bare `go test ./...`. Wire TEST_DATABASE_URL into CI so it does: a
// migration that enables RLS on identity.users would otherwise turn every
// unscoped read of that table into a silent empty result.
func TestUnscopedTablesHaveNoRLS(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping live schema check")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))

	for qualified := range database.UnscopedTables {
		var (
			exists bool
			rls    bool
		)
		err := conn.QueryRow(ctx, `
			SELECT true, c.relrowsecurity
			  FROM pg_class c
			  JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE n.nspname || '.' || c.relname = $1
			   AND c.relkind = 'r';
		`, qualified).Scan(&exists, &rls)
		switch {
		case err == pgx.ErrNoRows:
			t.Errorf("%s is listed in database.UnscopedTables but does not exist; "+
				"remove it or fix the name", qualified)
		case err != nil:
			t.Fatalf("%s: %v", qualified, err)
		case rls:
			t.Errorf("%s has row-level security ENABLED but is listed in "+
				"database.UnscopedTables.\n"+
				"Every QueryUnscoped read of it now returns zero rows instead of "+
				"an error, because no organisation is set outside a transaction.\n"+
				"Move those call sites back onto InReadTx and drop the table from "+
				"the list.", qualified)
		}
	}
}
