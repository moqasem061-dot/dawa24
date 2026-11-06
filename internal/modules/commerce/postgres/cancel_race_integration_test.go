package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

var cancelRaceSeq atomic.Int64

// insertCancelRaceOrder creates a COD order with one shipment per status given.
func insertCancelRaceOrder(t *testing.T, db *database.DB, orderStatus string, shipmentStatuses ...string) int64 {
	t.Helper()
	n := cancelRaceSeq.Add(1)
	var id int64
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `INSERT INTO commerce.orders (order_number,customer_id,organization_id,status,payment_method) VALUES ($1,$2,$3,$4,'cod') RETURNING id`,
			fmt.Sprintf("batch1b-race-%d", n), testCommerceUserID, testCommerceCustID, orderStatus).Scan(&id); err != nil {
			return err
		}
		for i, s := range shipmentStatuses {
			if _, err := tx.Exec(txCtx, `INSERT INTO commerce.order_shipments (order_id,organization_id,shipment_number,status) VALUES ($1,$2,$3,$4)`,
				id, testCommerceVendorID, fmt.Sprintf("batch1b-race-%d-%d", n, i), s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func orderStatusOf(t *testing.T, db *database.DB, id int64) string {
	t.Helper()
	var status string
	err := db.InReadTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT status FROM commerce.orders WHERE id=$1`, id).Scan(&status)
	})
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// A second cancellation of an order that is already cancelled must be refused,
// not accepted again (cancelled -> cancelled was accepted before Batch 1B).
func TestRepeatedOrderCancelIsRefused(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	repo := postgres.NewRepository(db)
	ctx := context.Background()
	id := insertCancelRaceOrder(t, db, "pending", "pending")
	cancel := commerce.OrderStatusHistory{OrderID: id, ToStatus: string(commerce.StatusCancelled)}
	if err := repo.UpdateOrderStatus(ctx, id, commerce.StatusCancelled, cancel); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := repo.UpdateOrderStatus(ctx, id, commerce.StatusCancelled, cancel); err == nil {
		t.Fatal("second cancel of a cancelled order was accepted")
	}
	if err := repo.CancelAPIOrder(ctx, commerce.APICancelRequest{OrderID: id, UserID: testCommerceUserID, OrganizationID: testCommerceCustID}); err == nil {
		t.Fatal("API cancel of a cancelled order was accepted")
	}
}

// Two status-route cancels at once: exactly one may succeed, 20 times over.
func TestConcurrentOrderCancelsAdmitOne(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	repo := postgres.NewRepository(db)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		id := insertCancelRaceOrder(t, db, "pending", "pending")
		start := make(chan struct{})
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for j := 0; j < 2; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results <- repo.UpdateOrderStatus(ctx, id, commerce.StatusCancelled, commerce.OrderStatusHistory{OrderID: id, ToStatus: string(commerce.StatusCancelled)})
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("round %d: %d cancellations succeeded, want 1", i, successes)
		}
	}
}

// A buyer cancel races a supplier's cancel of its own shipment. Whatever order
// they run in, a cancelled order never keeps a shipped shipment.
func TestBuyerCancelRacesSupplierShipmentCancel(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	resetCommerceFixtures(t, db)
	repo := postgres.NewRepository(db)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		id := insertCancelRaceOrder(t, db, "pending", "pending", "pending")
		var shipmentID int64
		err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(txCtx, `SELECT id FROM commerce.order_shipments WHERE order_id=$1 ORDER BY id LIMIT 1`, id).Scan(&shipmentID)
		})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_ = repo.CancelAPIOrder(ctx, commerce.APICancelRequest{OrderID: id, UserID: testCommerceUserID, OrganizationID: testCommerceCustID})
		}()
		go func() {
			defer wg.Done()
			<-start
			from := string(commerce.StatusPending)
			_ = repo.UpdateShipmentStatus(database.WithTenant(ctx, testCommerceVendorID), shipmentID, commerce.StatusPending, commerce.StatusCancelled,
				commerce.OrderStatusHistory{OrderID: id, ShipmentID: &shipmentID, FromStatus: &from, ToStatus: string(commerce.StatusCancelled)})
		}()
		close(start)
		wg.Wait()
		if orderStatusOf(t, db, id) == "cancelled" {
			var open int
			err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
				return tx.QueryRow(txCtx, `SELECT count(*) FROM commerce.order_shipments WHERE order_id=$1 AND status NOT IN ('cancelled','failed','returned')`, id).Scan(&open)
			})
			if err != nil || open != 0 {
				t.Fatalf("round %d: order cancelled with %d open shipments (err %v)", i, open, err)
			}
		}
	}
}
