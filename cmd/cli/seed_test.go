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

// TestRunSeed_IsIdempotent: migrate runs the seed on every deploy, so running it
// again must add no city and must not undo an administrator's choices.
func TestRunSeed_IsIdempotent(t *testing.T) {
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
	sys := database.AsSystem(ctx)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := runSeed(ctx, db, log); err != nil {
		t.Fatalf("first runSeed: %v", err)
	}
	count := func(q string) (n int) {
		t.Helper()
		if err := db.InReadTx(sys, func(txCtx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(txCtx, q).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	const cities = `SELECT count(*) FROM platform_admin.cities`
	const defaults = `SELECT (SELECT count(*) FROM platform_admin.languages WHERE is_default) * 100 + (SELECT count(*) FROM platform_admin.currencies WHERE is_default)`
	citiesBefore, defaultsBefore := count(cities), count(defaults)

	// An administrator deactivates the country; a deploy must not switch it back on.
	exec := func(q string) {
		t.Helper()
		if err := db.InTx(sys, func(txCtx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(txCtx, q)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE platform_admin.countries SET is_active = false WHERE code = 'EG'`)
	defer exec(`UPDATE platform_admin.countries SET is_active = true WHERE code = 'EG'`)

	for i := 0; i < 3; i++ {
		if err := runSeed(ctx, db, log); err != nil {
			t.Fatalf("runSeed repeat %d: %v", i, err)
		}
	}
	if got := count(cities); got != citiesBefore {
		t.Fatalf("cities after repeated seeds = %d, want %d: the seed added rows", got, citiesBefore)
	}
	if got := count(defaults); got != defaultsBefore {
		t.Fatalf("default language/currency counts changed: %d, want %d", got, defaultsBefore)
	}
	if n := count(`SELECT count(*) FROM platform_admin.countries WHERE code = 'EG' AND is_active`); n != 0 {
		t.Fatal("the seed switched a deactivated country back on")
	}
}
