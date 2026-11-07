package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func earningsStatusByShipment(t *testing.T, db *database.DB, vendor int64) map[int64]string {
	t.Helper()
	sum, err := postgres.NewRepository(db).GetVendorFinancialSummary(context.Background(), vendor, "all")
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]string{}
	for _, s := range sum.Shipments {
		out[s.ShipmentID] = s.PaymentStatus
	}
	return out
}

// A wallet order paid in full at checkout before per-shipment charging existed has
// no wallet_paid_at on its shipments; the earnings page must show it as paid, not
// unpaid. Where per-shipment charging is in use, an uncharged shipment is unpaid.
func TestEarningsStatusOfWalletOrders(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)

	// Old style: order paid, no shipment carries a charge time.
	oldOrder, oldShips, _ := twoPartOrder(t, db, "earn-old", "wallet", "confirmed", "cancelled")
	// New style: the first shipment was charged, the second was not.
	newOrder, newFirst, newSecond := twoPartOrder(t, db, "earn-new", "wallet", "confirmed", "confirmed")
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `UPDATE commerce.orders SET payment_status='paid' WHERE id=$1`, oldOrder); err != nil {
			return err
		}
		_, err := tx.Exec(txCtx, `UPDATE commerce.order_shipments SET wallet_paid_at=now() WHERE id=$1`, newFirst)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = newOrder

	got := earningsStatusByShipment(t, db, testCommerceVendorID)
	if got[oldShips] != "paid" {
		t.Errorf("old wallet order shipment shows %q, want paid", got[oldShips])
	}
	if got[newFirst] != "paid" {
		t.Errorf("charged shipment shows %q, want paid", got[newFirst])
	}
	if got[newSecond] != "unpaid" {
		t.Errorf("uncharged shipment of a per-shipment order shows %q, want unpaid", got[newSecond])
	}
}
