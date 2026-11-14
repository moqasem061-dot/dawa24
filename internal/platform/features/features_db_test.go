package features

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
)

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := database.Open(ctx, config.Database{URL: dbURL, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: 30 * time.Minute, StatementTimeout: 10 * time.Second})
	if err != nil {
		t.Skipf("cannot connect to database: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// Migration 262 seeds the ERP switch on; Set stores a change, applies it in this
// process at once, writes the audit entry in the same transaction, and a second
// engine (another server instance) sees it on its next reload.
func TestSetStoresAppliesAndAuditsTheERPSwitch(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(c context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(c, `UPDATE platform_admin.feature_flags SET is_enabled = true WHERE key = $1`, KeyERPEnabled)
			return err
		})
	})

	a := &Engine{db: db, log: log, flags: map[string]bool{}}
	b := &Engine{db: db, log: log, flags: map[string]bool{}}
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if !a.Enabled(KeyERPEnabled) {
		t.Fatal("migration 262 should seed erp.enabled on")
	}

	var auditBefore int
	count := func() (n int) {
		_ = db.InReadTx(database.AsSystem(ctx), func(c context.Context, tx pgx.Tx) error {
			return tx.QueryRow(c, `SELECT count(*) FROM platform.audit_log WHERE action = 'platform.feature_flag.set' AND entity_id = $1`, KeyERPEnabled).Scan(&n)
		})
		return n
	}
	auditBefore = count()

	if err := a.Set(ctx, KeyERPEnabled, false, 0); err != nil {
		t.Fatal(err)
	}
	if a.Enabled(KeyERPEnabled) {
		t.Error("the engine that wrote the flag must apply it immediately")
	}
	if got := count(); got != auditBefore+1 {
		t.Errorf("audit rows = %d, want %d", got, auditBefore+1)
	}
	var stored bool
	if err := db.InReadTx(database.AsSystem(ctx), func(c context.Context, tx pgx.Tx) error {
		return tx.QueryRow(c, `SELECT is_enabled FROM platform_admin.feature_flags WHERE key = $1`, KeyERPEnabled).Scan(&stored)
	}); err != nil || stored {
		t.Errorf("stored is_enabled = %v (err %v), want false", stored, err)
	}

	if err := b.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if b.Enabled(KeyERPEnabled) {
		t.Error("another instance must see the change after its next reload")
	}

	if err := a.Set(ctx, KeyERPEnabled, true, 0); err != nil {
		t.Fatal(err)
	}
	if !a.Enabled(KeyERPEnabled) {
		t.Error("switching back on must apply immediately")
	}
}

// A failed reload keeps what is in memory: replacing the flags with defaults
// would switch an integration an admin turned off back on.
func TestReloadFailureKeepsTheFlagsInMemory(t *testing.T) {
	db := openTestDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := &Engine{db: db, log: log, flags: map[string]bool{KeyERPEnabled: false}}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.Reload(cancelled); err == nil {
		t.Skip("reload unexpectedly succeeded with a cancelled context")
	}
	if e.Enabled(KeyERPEnabled) {
		t.Error("a failed reload reset a flag that an admin turned off")
	}
}
