package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

func TestNeedsTransaction(t *testing.T) {
	for sql, want := range map[string]bool{
		"SELECT id FROM org.branches WHERE id = $1":                       false,
		"SET LOCAL ROLE dawa24_sql_console":                               true,
		"set local statement_timeout = '10s'":                             true,
		"SELECT set_config('statement_timeout', $1, true)":                true,
		"SELECT pg_advisory_xact_lock($1)":                                true,
		"SELECT status FROM commerce.orders WHERE id = $1 FOR UPDATE":     true,
		"SELECT * FROM billing.wallets FOR SHARE":                         true,
		"SELECT 'no update' AS label FROM platform_admin.system_settings": false,
	} {
		if got := needsTransaction(sql); got != want {
			t.Errorf("needsTransaction(%q) = %v, want %v", sql, got, want)
		}
	}
}

// TestReadsOpenATransactionOnlyWhenNeeded runs against a real server: a plain
// read executes outside any transaction, and a callback that switches role
// with SET LOCAL still runs its later statements as that role.
func TestReadsOpenATransactionOnlyWhenNeeded(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := Open(ctx, config.Database{URL: dsn, MaxConns: 2, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute, StatementTimeout: 10 * time.Second})
	if err != nil {
		t.Skipf("connect: %v", err)
	}
	defer db.Close()
	if !db.rlsBypassed.Load() {
		t.Skip("test role does not bypass RLS; reads keep full transactions")
	}

	// One statement, one round trip: no BEGIN, no COMMIT.
	statsCtx, stats := WithQueryStats(ctx)
	var one int
	if err := db.InReadTx(statsCtx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT 1`).Scan(&one)
	}); err != nil {
		t.Fatal(err)
	}
	if got := stats.RoundTrips(); got != 1 {
		t.Fatalf("a one-statement read took %d round trips, want 1", got)
	}

	var role string
	err = db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE dawa24_sql_console"); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT current_user`).Scan(&role)
	})
	if err != nil {
		t.Skipf("role unavailable: %v", err)
	}
	if role != "dawa24_sql_console" {
		t.Fatalf("SET LOCAL ROLE did not apply: running as %q", role)
	}

	// The role ended with the transaction: the next read is back to normal.
	err = db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT current_user`).Scan(&role)
	})
	if err != nil || role == "dawa24_sql_console" {
		t.Fatalf("role leaked past its transaction: %q %v", role, err)
	}
}
