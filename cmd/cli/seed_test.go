package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func TestBilingualJSON_ExactAndValid(t *testing.T) {
	got := bilingualJSON(`مصر "الجديدة"`, "Egypt")
	want := `{"ar":"مصر \"الجديدة\"","en":"Egypt"}`
	if got != want {
		t.Fatalf("bilingualJSON = %s, want %s", got, want)
	}
}

// TestRunSeed_InsertsReferenceData runs the seed against a migrated database.
// Before the fix the seed failed with SQLSTATE 42P08 and cmd/cli only logged a
// warning, so the migrate command reported success with an empty currency table.
func TestRunSeed_InsertsReferenceData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping seed test")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{URL: dbURL, MaxConns: 2, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: 30 * time.Minute, StatementTimeout: 10 * time.Second})
	if err != nil {
		t.Skipf("cannot connect to database: %v", err)
	}
	defer db.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := runSeed(ctx, db, log); err != nil {
		t.Fatalf("runSeed: %v", err)
	}

	var ar, en string
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT name->>'ar', name->>'en' FROM platform_admin.currencies WHERE code = 'EGP'`).Scan(&ar, &en)
	})
	if err != nil {
		t.Fatalf("read EGP currency: %v", err)
	}
	if en != "Egyptian Pound" || ar != i18n.TDefault("w4_mod.s_302_302") {
		t.Fatalf("EGP name = ar %q en %q, want ar %q en %q", ar, en, i18n.TDefault("w4_mod.s_302_302"), "Egyptian Pound")
	}
}
