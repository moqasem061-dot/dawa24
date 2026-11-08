package integration_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// seedSupplierOrders writes n single-shipment orders for one supplier, with
// the buyer as the order's organization, and returns nothing: the test reads
// them back through the feed.
func seedSupplierOrders(t *testing.T, db *database.DB, fx ordersFixture, prefix string, n int) {
	t.Helper()
	err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO commerce.orders (order_number, customer_id, organization_id, status, subtotal, total_amount)
			SELECT $1 || g, $2, $3, 'confirmed', 1.00, 1.00 FROM generate_series(1, $4) g`, prefix, fx.buyerUser, fx.buyer, n); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount)
			SELECT o.id, $1, o.order_number || '-1', 'pending', 1.00, 1.00 FROM commerce.orders o
			WHERE o.organization_id = $2 AND o.order_number LIKE $3 || '%' AND NOT EXISTS (
				SELECT 1 FROM commerce.order_shipments s WHERE s.order_id = o.id AND s.organization_id = $1)`, fx.supplierA, fx.buyer, prefix)
		return err
	})
	if err != nil {
		t.Fatalf("seed %d orders: %v", n, err)
	}
}

// TestERPOrders_Paging5000 pages 5,000 supplier orders 100 at a time and adds
// 20 more between pages. No id may repeat, and every original id must appear
// (SC-007: keyset paging, research D-16).
func TestERPOrders_Paging5000(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 6000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	prefix := fmt.Sprintf("PG5K-%d-", base)
	seedSupplierOrders(t, db, fx, prefix, 5000)
	repo := erppg.New(db)

	originals := map[int64]bool{}
	seen := map[int64]bool{}
	var after int64
	inserted := 0
	for page := 0; ; page++ {
		rows, err := repo.ListSalesShipmentsAsSystem(ctx, fx.supplierA, fx.integA, erp.OrderFilter{After: after, Limit: 101, State: "all"})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) > 100 {
			rows = rows[:100]
		} else if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if seen[r.ShipmentID] {
				t.Fatalf("shipment %d returned twice", r.ShipmentID)
			}
			seen[r.ShipmentID] = true
			// The originals are prefix + digits; the mid-paging inserts carry an "x" marker.
			if strings.HasPrefix(r.OrderNumber, prefix) && !strings.Contains(r.OrderNumber[len(prefix):], "x") {
				originals[r.ShipmentID] = true
			}
		}
		after = rows[len(rows)-1].ShipmentID
		if inserted < 20 {
			seedSupplierOrders(t, db, fx, fmt.Sprintf("%sx%d-", prefix, inserted), 1)
			inserted++
		}
		if len(rows) < 100 {
			break
		}
	}
	if len(originals) != 5000 {
		t.Fatalf("paged %d original orders, want 5000 (none skipped)", len(originals))
	}
}

// TestERPOrders_PagingWithNewState pages the state=new feed and acknowledges
// every page's orders before fetching the next one. Nothing may be lost: each
// original order is seen exactly once while it is still new.
func TestERPOrders_PagingWithNewState(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 7000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	prefix := fmt.Sprintf("PGNEW-%d-", base)
	const total = 250
	seedSupplierOrders(t, db, fx, prefix, total)
	repo := erppg.New(db)
	audit := erp.AuditRecord{Action: "erp.order.ack", EntityType: "order_import", EntityID: "paging", ActorKind: "api_key"}

	seen := map[int64]bool{}
	var after int64
	for {
		rows, err := repo.ListSalesShipmentsAsSystem(ctx, fx.supplierA, fx.integA, erp.OrderFilter{After: after, Limit: 101, State: "new"})
		if err != nil {
			t.Fatal(err)
		}
		more := len(rows) > 100
		if more {
			rows = rows[:100]
		}
		for _, r := range rows {
			if seen[r.ShipmentID] {
				t.Fatalf("shipment %d returned twice in the new feed", r.ShipmentID)
			}
			seen[r.ShipmentID] = true
			imp := erp.OrderImport{OrganizationID: fx.supplierA, IntegrationID: fx.integA, OrderID: r.OrderID, ShipmentID: &r.ShipmentID, Direction: "sales", OrderNumber: r.OrderNumber, ErpReference: "REF-" + r.OrderNumber, AckedStatus: r.Status}
			if _, err := repo.UpsertOrderImport(ctx, imp, audit); err != nil {
				t.Fatal(err)
			}
		}
		if len(rows) == 0 || !more {
			break
		}
		after = rows[len(rows)-1].ShipmentID
	}
	// The supplier's only orders are the ones this test wrote, so the feed
	// must have delivered exactly that many.
	if len(seen) != total {
		t.Fatalf("new feed delivered %d of %d orders", len(seen), total)
	}
}
