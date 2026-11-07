package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// Batch 3 (S15): the SQL console role reads organization rows but never the AI
// gateway credentials of an organization (ai_virtual_key, ai_user_id).
func TestSQLConsoleCannotReadOrganizationAICredentials(t *testing.T) {
	db := connectTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	ctx := context.Background()

	var roleExists bool
	if err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_sql_console')`).Scan(&roleExists)
	}); err != nil {
		t.Fatalf("check console role: %v", err)
	}
	if !roleExists {
		t.Skip("dawa24_sql_console role not present")
	}

	// As the console role, a plain column read still works...
	runAsConsole(t, db, `SELECT id, name FROM org.organizations LIMIT 1`, false)
	// ...and the AI credential columns are refused.
	runAsConsole(t, db, `SELECT ai_virtual_key FROM org.organizations LIMIT 1`, true)
	runAsConsole(t, db, `SELECT ai_user_id FROM org.organizations LIMIT 1`, true)
}

// runAsConsole runs one statement as the SQL console role in its own
// transaction, and checks whether the database refused it.
func runAsConsole(t *testing.T, db *database.DB, stmt string, wantDenied bool) {
	t.Helper()
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `SET LOCAL ROLE dawa24_sql_console`); err != nil {
			return err
		}
		rows, err := tx.Query(txCtx, stmt)
		if err != nil {
			return err
		}
		rows.Close()
		return rows.Err()
	})
	denied := err != nil && strings.Contains(err.Error(), "permission denied")
	if wantDenied && !denied {
		t.Fatalf("%q as the console role: err=%v, want permission denied", stmt, err)
	}
	if !wantDenied && err != nil {
		t.Fatalf("%q as the console role: %v", stmt, err)
	}
}
