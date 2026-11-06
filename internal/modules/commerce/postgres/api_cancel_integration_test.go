package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func TestAPICancelTransaction(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	ctx := context.Background()
	repo := postgres.NewRepository(db)
	sequence := 0
	newOrder := func(status, shipmentStatus, method string) int64 {
		t.Helper()
		sequence++
		var id int64
		err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			err := tx.QueryRow(txCtx, `INSERT INTO commerce.orders (order_number,customer_id,organization_id,status,payment_method) VALUES ($1,$2,$3,$4,$5) RETURNING id`, fmt.Sprintf("batch1a-cancel-%d", sequence), testCommerceUserID, testCommerceCustID, status, method).Scan(&id)
			if err != nil {
				return err
			}
			_, err = tx.Exec(txCtx, `INSERT INTO commerce.order_shipments (order_id,organization_id,shipment_number,status) VALUES ($1,$2,$3,$4)`, id, testCommerceVendorID, fmt.Sprintf("batch1a-shipment-%d", sequence), shipmentStatus)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	check := func(status, shipment, method string, allowed bool) {
		t.Helper()
		id := newOrder(status, shipment, method)
		err := repo.CancelAPIOrder(ctx, commerce.APICancelRequest{OrderID: id, UserID: testCommerceUserID, OrganizationID: testCommerceCustID})
		if (err == nil) != allowed {
			t.Fatalf("order=%s shipment=%s method=%s: err=%v", status, shipment, method, err)
		}
	}
	for _, status := range []string{"pending", "processing"} {
		t.Run("allowed_"+status, func(t *testing.T) { check(status, "pending", "cod", true) })
	}
	for _, status := range []string{"confirmed", "on_hold", "shipped", "in_transit", "out_for_delivery", "delivered", "completed", "cancelled", "failed", "returned", "refunded"} {
		t.Run("blocked_order_"+status, func(t *testing.T) { check(status, "pending", "cod", false) })
	}
	for _, status := range []string{"shipped", "in_transit", "out_for_delivery", "delivered", "completed"} {
		t.Run("blocked_shipment_"+status, func(t *testing.T) { check("pending", status, "cod", false) })
	}
	t.Run("wallet", func(t *testing.T) { check("pending", "pending", "wallet", false) })
	t.Run("supplier_cancels_only_own_shipment", func(t *testing.T) {
		id := newOrder("pending", "pending", "cod")
		var ownID int64
		err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			if err := tx.QueryRow(txCtx, `SELECT id FROM commerce.order_shipments WHERE order_id=$1`, id).Scan(&ownID); err != nil {
				return err
			}
			_, err := tx.Exec(txCtx, `INSERT INTO commerce.order_shipments (order_id,organization_id,shipment_number,status) VALUES ($1,$2,$3,'pending')`, id, testCommerceCustID, fmt.Sprintf("batch1a-other-%d", sequence))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		err = repo.CancelAPIVendorShipment(ctx, commerce.APIVendorCancelRequest{OrderID: id, ShipmentID: ownID, VendorOrgID: testCommerceVendorID, UserID: testCommerceUserID})
		if err != nil {
			t.Fatal(err)
		}
		var ownStatus, otherStatus, parentStatus string
		err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(txCtx, `SELECT o.status, a.status, b.status FROM commerce.orders o JOIN commerce.order_shipments a ON a.order_id=o.id AND a.id=$2 JOIN commerce.order_shipments b ON b.order_id=o.id AND b.id<>$2 WHERE o.id=$1`, id, ownID).Scan(&parentStatus, &ownStatus, &otherStatus)
		})
		if err != nil {
			t.Fatal(err)
		}
		if parentStatus != "pending" || ownStatus != "cancelled" || otherStatus != "pending" {
			t.Fatalf("parent=%s own=%s other=%s", parentStatus, ownStatus, otherStatus)
		}
	})
	t.Run("supplier_cannot_cancel_after_other_shipment_ships", func(t *testing.T) {
		id := newOrder("pending", "pending", "cod")
		var ownID int64
		err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			if err := tx.QueryRow(txCtx, `SELECT id FROM commerce.order_shipments WHERE order_id=$1`, id).Scan(&ownID); err != nil {
				return err
			}
			_, err := tx.Exec(txCtx, `INSERT INTO commerce.order_shipments (order_id,organization_id,shipment_number,status) VALUES ($1,$2,$3,'shipped')`, id, testCommerceCustID, fmt.Sprintf("batch1a-shipped-%d", sequence))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		err = repo.CancelAPIVendorShipment(ctx, commerce.APIVendorCancelRequest{OrderID: id, ShipmentID: ownID, VendorOrgID: testCommerceVendorID, UserID: testCommerceUserID})
		if err == nil {
			t.Fatal("supplier cancellation succeeded despite shipped shipment")
		}
		var status string
		err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(txCtx, `SELECT status FROM commerce.order_shipments WHERE id=$1`, ownID).Scan(&status)
		})
		if err != nil || status != "pending" {
			t.Fatalf("status=%s err=%v", status, err)
		}
	})
	t.Run("two_simultaneous_cancels", func(t *testing.T) {
		id := newOrder("pending", "pending", "cod")
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- repo.CancelAPIOrder(ctx, commerce.APICancelRequest{OrderID: id, UserID: testCommerceUserID, OrganizationID: testCommerceCustID})
			}()
		}
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("got %d successful cancellations, want 1", successes)
		}
	})
	t.Run("cancel_races_order_confirmation", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			id := newOrder("pending", "pending", "cod")
			results := make(chan error, 2)
			go func() {
				results <- repo.CancelAPIOrder(ctx, commerce.APICancelRequest{OrderID: id, UserID: testCommerceUserID, OrganizationID: testCommerceCustID})
			}()
			go func() {
				results <- repo.UpdateOrderStatus(ctx, id, commerce.StatusConfirmed, commerce.OrderStatusHistory{OrderID: id, ToStatus: string(commerce.StatusConfirmed)})
			}()
			first, second := <-results, <-results
			if (first == nil) == (second == nil) {
				t.Fatalf("exactly one transition must succeed: %v; %v", first, second)
			}
		}
	})
	t.Run("cancel_races_shipment", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			id := newOrder("pending", "pending", "cod")
			var shipmentID int64
			err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
				return tx.QueryRow(txCtx, `SELECT id FROM commerce.order_shipments WHERE order_id=$1`, id).Scan(&shipmentID)
			})
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			results := make(chan error, 2)
			wg.Add(2)
			go func() {
				defer wg.Done()
				results <- repo.CancelAPIOrder(ctx, commerce.APICancelRequest{OrderID: id, UserID: testCommerceUserID, OrganizationID: testCommerceCustID})
			}()
			go func() {
				defer wg.Done()
				from := string(commerce.StatusPending)
				results <- repo.UpdateShipmentStatus(database.WithTenant(ctx, testCommerceVendorID), shipmentID, commerce.StatusPending, commerce.StatusShipped, commerce.OrderStatusHistory{OrderID: id, ShipmentID: &shipmentID, FromStatus: &from, ToStatus: string(commerce.StatusShipped)})
			}()
			wg.Wait()
			close(results)
			successes := 0
			for err := range results {
				if err == nil {
					successes++
				}
			}
			if successes != 1 {
				t.Fatalf("got %d successful conflicting transitions, want 1", successes)
			}
			var orderStatus, shipmentStatus string
			err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
				return tx.QueryRow(txCtx, `SELECT o.status,s.status FROM commerce.orders o JOIN commerce.order_shipments s ON s.order_id=o.id WHERE o.id=$1`, id).Scan(&orderStatus, &shipmentStatus)
			})
			if err != nil {
				t.Fatal(err)
			}
			if orderStatus == "cancelled" && shipmentStatus == "shipped" {
				t.Fatal("cancelled order contains shipped shipment")
			}
		}
	})
}
