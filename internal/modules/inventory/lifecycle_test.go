package inventory_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Coverage for the two-phase transfer lifecycle and warehouse deletion rules.
//
// The property that matters most here: stock is conserved. Dispatching,
// receiving and cancelling must never create or destroy units, only move them
// between a warehouse and "in transit".

func newLifecycleFixture(t *testing.T) (context.Context, *mockInventoryRepo, *inventory.Service, int64, int64) {
	t.Helper()

	ctx := database.WithTenant(context.Background(), 10)
	repo := newMockInventoryRepo()
	svc := inventory.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

	wh1, err := svc.CreateWarehouse(ctx, &inventory.Warehouse{Name: "Cairo Main"})
	if err != nil {
		t.Fatalf("create source warehouse: %v", err)
	}
	wh2, err := svc.CreateWarehouse(ctx, &inventory.Warehouse{Name: "Giza Branch"})
	if err != nil {
		t.Fatalf("create destination warehouse: %v", err)
	}

	_ = repo.UpsertStock(ctx, &inventory.Stock{
		OrganizationID: 10, WarehouseID: wh1.ID,
		ProductID: 100, ProductVariantID: 501, Quantity: 100,
	})

	return ctx, repo, svc, wh1.ID, wh2.ID
}

func dispatch(t *testing.T, ctx context.Context, svc *inventory.Service, from, to int64, qty int) *inventory.WarehouseTransfer {
	t.Helper()
	tr, err := svc.TransferStock(ctx, &inventory.WarehouseTransfer{
		FromWarehouseID: from, ToWarehouseID: to,
		ProductID: 100, ProductVariantID: 501, Quantity: qty,
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	return tr
}

func TestDispatchDeductsSourceButDoesNotCreditDestination(t *testing.T) {
	ctx, repo, svc, from, to := newLifecycleFixture(t)

	tr := dispatch(t, ctx, svc, from, to, 30)

	if tr.Status != inventory.TransferInTransit {
		t.Errorf("status = %s, want in_transit", tr.Status)
	}

	src, _ := repo.GetStock(ctx, from, 501)
	if src.Quantity != 70 {
		t.Errorf("source = %d, want 70", src.Quantity)
	}

	// The critical assertion. Crediting here would let the destination sell
	// medicine that is still on a van.
	if _, err := repo.GetStock(ctx, to, 501); err == nil {
		t.Error("destination was credited at dispatch; it must only be credited on receipt")
	}
}

func TestReceiveCreditsDestinationAndCompletes(t *testing.T) {
	ctx, repo, svc, from, to := newLifecycleFixture(t)
	tr := dispatch(t, ctx, svc, from, to, 30)

	received, err := svc.ReceiveTransfer(ctx, tr.ID)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if received.Status != inventory.TransferCompleted {
		t.Errorf("status = %s, want completed", received.Status)
	}

	src, _ := repo.GetStock(ctx, from, 501)
	dst, err := repo.GetStock(ctx, to, 501)
	if err != nil {
		t.Fatalf("destination stock missing after receipt: %v", err)
	}

	if src.Quantity != 70 || dst.Quantity != 30 {
		t.Errorf("source=%d destination=%d, want 70 and 30", src.Quantity, dst.Quantity)
	}
	// Stock conservation: nothing was created or destroyed by the round trip.
	if src.Quantity+dst.Quantity != 100 {
		t.Errorf("total = %d, want 100 — the transfer changed the amount of stock in existence",
			src.Quantity+dst.Quantity)
	}
}

func TestReceivingTwiceIsRejected(t *testing.T) {
	ctx, repo, svc, from, to := newLifecycleFixture(t)
	tr := dispatch(t, ctx, svc, from, to, 30)

	if _, err := svc.ReceiveTransfer(ctx, tr.ID); err != nil {
		t.Fatalf("first receive: %v", err)
	}

	// Two staff members clicking "receive" together must not credit twice.
	_, err := svc.ReceiveTransfer(ctx, tr.ID)
	if err == nil {
		t.Fatal("second receive succeeded; stock would be created out of nothing")
	}
	if apperr.KindOf(err) != apperr.KindConflict {
		t.Errorf("kind = %s, want conflict", apperr.KindOf(err))
	}

	dst, _ := repo.GetStock(ctx, to, 501)
	if dst.Quantity != 30 {
		t.Errorf("destination = %d, want 30 — it was credited twice", dst.Quantity)
	}
}

func TestCancelInTransitReturnsStockToSource(t *testing.T) {
	ctx, repo, svc, from, to := newLifecycleFixture(t)
	tr := dispatch(t, ctx, svc, from, to, 30)

	cancelled, err := svc.CancelTransfer(ctx, tr.ID, "van broke down")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != inventory.TransferCancelled {
		t.Errorf("status = %s, want cancelled", cancelled.Status)
	}

	src, _ := repo.GetStock(ctx, from, 501)
	if src.Quantity != 100 {
		t.Errorf("source = %d, want 100 — cancelling destroyed stock", src.Quantity)
	}
}

func TestCancelAfterReceiptIsRejected(t *testing.T) {
	ctx, _, svc, from, to := newLifecycleFixture(t)
	tr := dispatch(t, ctx, svc, from, to, 30)

	if _, err := svc.ReceiveTransfer(ctx, tr.ID); err != nil {
		t.Fatalf("receive: %v", err)
	}

	// Completed is terminal: unwinding it would need a new reverse transfer,
	// not a status change.
	if _, err := svc.CancelTransfer(ctx, tr.ID, "changed my mind"); err == nil {
		t.Fatal("cancelling a completed transfer succeeded")
	}
}

func TestDispatchRefusesMoreThanSourceHolds(t *testing.T) {
	ctx, _, svc, from, to := newLifecycleFixture(t)

	_, err := svc.TransferStock(ctx, &inventory.WarehouseTransfer{
		FromWarehouseID: from, ToWarehouseID: to,
		ProductID: 100, ProductVariantID: 501, Quantity: 500,
	})
	if err == nil {
		t.Fatal("dispatched more stock than the source holds")
	}
	if apperr.KindOf(err) != apperr.KindValidation {
		t.Errorf("kind = %s, want validation", apperr.KindOf(err))
	}
}

func TestDeleteWarehouseRefusedWhileHoldingStock(t *testing.T) {
	ctx, _, svc, from, _ := newLifecycleFixture(t)

	err := svc.DeleteWarehouse(ctx, from)
	if err == nil {
		t.Fatal("deleted a warehouse still holding 100 units; its stock would be orphaned")
	}
	if apperr.KindOf(err) != apperr.KindConflict {
		t.Errorf("kind = %s, want conflict", apperr.KindOf(err))
	}
}

func TestDeleteEmptyWarehouseSucceeds(t *testing.T) {
	ctx, _, svc, _, to := newLifecycleFixture(t)

	// The destination holds nothing until a transfer is received.
	if err := svc.DeleteWarehouse(ctx, to); err != nil {
		t.Fatalf("delete empty warehouse: %v", err)
	}
}

func TestUpdateWarehouseRejectsEmptyName(t *testing.T) {
	ctx, _, svc, from, _ := newLifecycleFixture(t)

	if _, err := svc.UpdateWarehouse(ctx, from, &inventory.Warehouse{Name: "   "}); err == nil {
		t.Fatal("accepted a blank warehouse name")
	}
}

func TestUpdateWarehouseCannotReassignTenant(t *testing.T) {
	ctx, _, svc, from, _ := newLifecycleFixture(t)

	// A request body carrying another organisation's id must not move the
	// warehouse between tenants.
	updated, err := svc.UpdateWarehouse(ctx, from, &inventory.Warehouse{
		Name:           "Renamed",
		OrganizationID: 9999,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.OrganizationID == 9999 {
		t.Error("organization_id was taken from the request body; a warehouse could be moved between tenants")
	}
}

func TestTenantIsRequiredForLifecycleOperations(t *testing.T) {
	repo := newMockInventoryRepo()
	svc := inventory.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bare := context.Background()

	if _, err := svc.ListLowStock(bare, 10, 0); err == nil {
		t.Error("ListLowStock ran without a tenant")
	}
	if _, err := svc.ListTransfers(bare, "", 10, 0); err == nil {
		t.Error("ListTransfers ran without a tenant")
	}
	if err := svc.DeleteWarehouse(bare, 1); err == nil {
		t.Error("DeleteWarehouse ran without a tenant")
	}
}

func TestListTransfersRejectsUnknownStatusFilter(t *testing.T) {
	ctx, _, svc, _, _ := newLifecycleFixture(t)

	if _, err := svc.ListTransfers(ctx, "definitely-not-a-status", 10, 0); err == nil {
		t.Fatal("accepted an unknown status filter")
	}
}
