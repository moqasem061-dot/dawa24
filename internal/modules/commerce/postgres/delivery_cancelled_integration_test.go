package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// twoPartOrder creates an order of the given payment method and total with two
// shipments: the first out for delivery under the PIN rule (60.00), the second
// in secondStatus (40.00). It returns the order id and both shipment ids.
func twoPartOrder(t *testing.T, db *database.DB, label, method, firstStatus, secondStatus string) (int64, int64, int64) {
	t.Helper()
	var orderID, first, second int64
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `INSERT INTO commerce.orders (order_number,customer_id,organization_id,status,payment_method,total_amount) VALUES ($1,$2,$3,'shipped',$4,100.00) RETURNING id`,
			"two-"+label, testCommerceUserID, testCommerceCustID, method).Scan(&orderID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `INSERT INTO commerce.order_shipments (order_id,organization_id,shipment_number,status,delivery_code,delivery_pin_required,total_amount) VALUES ($1,$2,$3,$4,$5,true,60.00) RETURNING id`,
			orderID, testCommerceVendorID, "two-"+label+"-1", firstStatus, pinIntegrationCode).Scan(&first); err != nil {
			return err
		}
		return tx.QueryRow(txCtx, `INSERT INTO commerce.order_shipments (order_id,organization_id,shipment_number,status,total_amount) VALUES ($1,$2,$3,$4,40.00) RETURNING id`,
			orderID, testCommerceVendorID, "two-"+label+"-2", secondStatus).Scan(&second)
	})
	if err != nil {
		t.Fatal(err)
	}
	return orderID, first, second
}

func orderStateOf(t *testing.T, db *database.DB, id int64) (status, payment string) {
	t.Helper()
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT status, payment_status FROM commerce.orders WHERE id=$1`, id).Scan(&status, &payment)
	})
	if err != nil {
		t.Fatal(err)
	}
	return status, payment
}

// A cancelled part of the order must not keep the delivered part from closing
// the order, and cash collected for the live part settles it.
func TestCancelledShipmentDoesNotBlockDeliveredOrder(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	repo := postgres.NewRepository(db)
	orderID, first, _ := twoPartOrder(t, db, "cancelled-sibling", "cod", "out_for_delivery", "cancelled")

	if _, err := repo.VerifyAndCompleteDelivery(context.Background(), first, pinIntegrationCode, "", 6000); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	status, payment := orderStateOf(t, db, orderID)
	if status != "delivered" {
		t.Fatalf("order status = %s, want delivered", status)
	}
	if payment != "paid" {
		t.Fatalf("payment status = %s, want paid (60.00 collected of the 60.00 that went ahead)", payment)
	}
}

// A sibling that is still open keeps the order from being delivered.
func TestOpenShipmentKeepsOrderNotDelivered(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	repo := postgres.NewRepository(db)
	orderID, first, _ := twoPartOrder(t, db, "open-sibling", "cod", "out_for_delivery", "shipped")

	if _, err := repo.VerifyAndCompleteDelivery(context.Background(), first, pinIntegrationCode, "", 6000); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if status, _ := orderStateOf(t, db, orderID); status == "delivered" {
		t.Fatal("order became delivered while a shipment is still open")
	}
}

// A wallet order is charged when its shipment is confirmed, so a parcel that
// was never confirmed cannot be delivered, with or without the PIN override.
func TestUnconfirmedWalletShipmentCannotBeDelivered(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	repo := postgres.NewRepository(db)
	for _, st := range []string{"pending", "processing", "on_hold"} {
		_, first, _ := twoPartOrder(t, db, "wallet-"+st, "wallet", st, "cancelled")
		if _, err := repo.ConfirmDeliveryOverride(context.Background(), first, testCommerceUserID, "buyer lost the PIN"); err == nil {
			t.Fatalf("override delivered a %s wallet shipment that was never charged", st)
		}
		if _, err := repo.VerifyAndCompleteDelivery(context.Background(), first, pinIntegrationCode, "", -1); err == nil {
			t.Fatalf("PIN delivered a %s wallet shipment that was never charged", st)
		}
		if got := shipmentStatusOf(t, db, first); got != st {
			t.Fatalf("status after refused delivery = %s, want %s", got, st)
		}
	}
}
