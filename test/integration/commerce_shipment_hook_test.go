package integration_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	commercepg "github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// The in-transaction hook of a shipment transition (S4, owner decision on T163).
// The erp module writes its audit row through this hook; the two must commit or
// roll back together, so a failing hook leaves the shipment exactly as it was.

func TestCommerceShipmentHook_FailureRollsBackStatus(t *testing.T) {
	db := decideDB(t)
	fx := seedDecideFixture(t, db, decideBase(), 0)
	shipment := fx.seedDecideOrder(t, db, fx.orderNumber, "cash")
	comm := commerce.NewService(commercepg.NewRepository(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := database.WithTenant(context.Background(), fx.supplier)
	boom := errors.New("audit write failed")

	_, err := comm.TransitionShipmentStatusWithWallet(ctx, shipment, commerce.StatusConfirmed, nil, "", 0,
		func(context.Context, pgx.Tx) error { return boom })
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("transition error = %v, want the hook error", err)
	}
	if got := decideShipmentStatus(t, db, shipment); got != "pending" {
		t.Fatalf("shipment status after failed hook = %q, want pending", got)
	}
	if n := len(decideHistory(t, db, shipment)); n != 0 {
		t.Fatalf("%d history rows after failed hook, want 0", n)
	}
}

func TestCommerceShipmentHook_SuccessCommitsWithChange(t *testing.T) {
	db := decideDB(t)
	fx := seedDecideFixture(t, db, decideBase(), 0)
	shipment := fx.seedDecideOrder(t, db, fx.orderNumber, "cash")
	comm := commerce.NewService(commercepg.NewRepository(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := database.WithTenant(context.Background(), fx.supplier)
	calls := 0

	if _, err := comm.TransitionShipmentStatusWithWallet(ctx, shipment, commerce.StatusConfirmed, nil, "", 0,
		func(context.Context, pgx.Tx) error { calls++; return nil }); err != nil {
		t.Fatalf("transition with hook: %v", err)
	}
	if calls != 1 {
		t.Fatalf("hook ran %d times, want 1", calls)
	}
	if got := decideShipmentStatus(t, db, shipment); got != "confirmed" {
		t.Fatalf("shipment status = %q, want confirmed", got)
	}
}
