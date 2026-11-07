package integration_test

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	promoPostgres "github.com/moqasem061-dot/dawa24/internal/modules/promo/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// openAppRoleDB connects as the non-superuser application role, for which
// row-level security is enforced. It skips when that role cannot log in.
func openAppRoleDB(t *testing.T) *database.DB {
	t.Helper()
	raw := os.Getenv("DATABASE_URL")
	if raw == "" {
		raw = os.Getenv("TEST_DATABASE_URL")
	}
	u, err := url.Parse(raw)
	if raw == "" || err != nil {
		t.Skip("no database URL")
	}
	u.User = url.UserPassword("dawa24_app", "dawa24_app_change_me")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := database.Open(ctx, config.Database{URL: u.String(), MaxConns: 5, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute, StatementTimeout: 10 * time.Second})
	if err != nil {
		t.Skipf("cannot connect as dawa24_app: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// A vendor buys a paid package from a wallet that has no organization of its
// own. Under row-level security the purchase must still see the wallet's ledger.
func TestPaidPackagePurchaseWorksUnderRowLevelSecurity(t *testing.T) {
	w := newSPWorld(t, "100.00", 10000)
	appDB := openAppRoleDB(t)
	svc := promo.NewService(promoPostgres.NewRepository(appDB), slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetWalletDebiter(func(ctx context.Context, tx pgx.Tx, orgID int64, amount money.Amount, description string) (*int64, error) {
		id, err := w.billRepo.WithdrawInTx(ctx, tx, w.user, amount, "sponsorship_package", nil, description)
		if err != nil {
			return nil, err
		}
		return &id, nil
	})
	if _, err := svc.PurchaseSponsorshipPackage(w.tenant(), w.pkg, false, "monthly"); err != nil {
		t.Fatalf("purchase under RLS: %v", err)
	}
	if got := w.balance(t); got != 0 {
		t.Fatalf("balance %d after a 100.00 package from a 100.00 wallet, want 0", got)
	}
	if n := w.scalar(t, `SELECT count(*) FROM promo.sponsorship_purchases WHERE organization_id = $1`, w.org); n != 1 {
		t.Fatalf("%d purchases, want 1", n)
	}
}
