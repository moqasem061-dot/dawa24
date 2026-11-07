package postgres_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

const pinIntegrationCode = "482913"

// insertPINShipment creates an order with one shipment, out for delivery, under
// the PIN rule (pinRequired) or the old flow (!pinRequired), and returns the
// shipment id.
func insertPINShipment(t *testing.T, db *database.DB, label string, pinRequired bool, code string) int64 {
	t.Helper()
	var shipmentID int64
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		var orderID int64
		if err := tx.QueryRow(txCtx, `INSERT INTO commerce.orders (order_number,customer_id,organization_id,status,payment_method) VALUES ($1,$2,$3,'out_for_delivery','cod') RETURNING id`,
			"pin-"+label, testCommerceUserID, testCommerceCustID).Scan(&orderID); err != nil {
			return err
		}
		return tx.QueryRow(txCtx, `INSERT INTO commerce.order_shipments (order_id,organization_id,shipment_number,status,delivery_code,delivery_pin_required) VALUES ($1,$2,$3,'out_for_delivery',$4,$5) RETURNING id`,
			orderID, testCommerceVendorID, "pin-"+label+"-1", code, pinRequired).Scan(&shipmentID)
	})
	if err != nil {
		t.Fatal(err)
	}
	return shipmentID
}

func shipmentStatusOf(t *testing.T, db *database.DB, id int64) string {
	t.Helper()
	var status string
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT status FROM commerce.order_shipments WHERE id=$1`, id).Scan(&status)
	})
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// A shipment under the PIN rule is closed only by its PIN: a wrong or missing
// code leaves it out for delivery, and the right code delivers it.
func TestPINShipmentNeedsItsPIN(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	repo := postgres.NewRepository(db)
	ctx := context.Background()
	id := insertPINShipment(t, db, "needs-pin", true, pinIntegrationCode)

	if _, err := repo.VerifyAndCompleteDelivery(ctx, id, "000000", "", -1); err == nil {
		t.Fatal("wrong PIN closed the shipment")
	}
	if _, err := repo.VerifyAndCompleteDelivery(ctx, id, "", "", -1); err == nil {
		t.Fatal("empty PIN closed a shipment under the PIN rule")
	}
	if got := shipmentStatusOf(t, db, id); got != "out_for_delivery" {
		t.Fatalf("status after refused PIN = %s, want out_for_delivery", got)
	}
	if _, err := repo.VerifyAndCompleteDelivery(ctx, id, pinIntegrationCode, "", -1); err != nil {
		t.Fatalf("right PIN: %v", err)
	}
	if got := shipmentStatusOf(t, db, id); got != "delivered" {
		t.Fatalf("status after right PIN = %s, want delivered", got)
	}
}

// A shipment created before the rule keeps the old flow.
func TestLegacyShipmentKeepsOldFlow(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	repo := postgres.NewRepository(db)
	id := insertPINShipment(t, db, "legacy", false, "")

	if _, err := repo.VerifyAndCompleteDelivery(context.Background(), id, "", "", -1); err != nil {
		t.Fatalf("legacy shipment without PIN: %v", err)
	}
	if got := shipmentStatusOf(t, db, id); got != "delivered" {
		t.Fatalf("legacy status = %s, want delivered", got)
	}
}

// A staff override closes the shipment without the PIN and writes an audit row
// naming the actor and the reason.
func TestDeliveryOverrideIsAudited(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	repo := postgres.NewRepository(db)
	ctx := context.Background()
	id := insertPINShipment(t, db, "override", true, pinIntegrationCode)
	const reason = "buyer lost the PIN; confirmed by phone"

	got, err := repo.ConfirmDeliveryOverride(ctx, id, testCommerceUserID, reason)
	if err != nil {
		t.Fatalf("override: %v", err)
	}
	if got.Status != commerce.StatusDelivered {
		t.Fatalf("status = %s, want delivered", got.Status)
	}

	var n int
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform.audit_log WHERE action='shipment.delivery_override' AND entity_id=$1 AND actor_user_id=$2 AND after::text LIKE $3`,
			fmt.Sprint(id), testCommerceUserID, "%"+strings.ReplaceAll(reason, "'", "")+"%").Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("audit rows for override = %d, want 1", n)
	}
}
