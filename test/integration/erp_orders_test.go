package integration_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// ordersFixture is one buyer pharmacy with two suppliers, each with its own
// integration, one catalog variant linked to an external code, and an order
// split into one shipment per supplier. Every id is time based.
type ordersFixture struct {
	buyer, buyerUser, buyerIntegration int64
	supplierA, supplierB               int64
	integA, integB                     int64
	variantA, variantB                 int64
	variantPublicA                     string
}

func seedOrdersFixture(t *testing.T, db *database.DB, base int64) ordersFixture {
	t.Helper()
	fx := ordersFixture{buyer: base, buyerUser: base, supplierA: base + 1, supplierB: base + 2}
	err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Buyer pharmacy"}'), ($2, '{"en":"Supplier A"}'), ($3, '{"en":"Supplier B"}')`, fx.buyer, fx.supplierA, fx.supplierB); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity.users (id, email, password_hash) VALUES ($1, $2, 'x')`, fx.buyerUser, fmt.Sprintf("erp-orders-%d@example.test", base)); err != nil {
			return err
		}
		for _, row := range []struct {
			org   int64
			integ *int64
			code  string
			varID *int64
			pub   *string
		}{
			{fx.buyer, &fx.buyerIntegration, "", nil, nil},
			{fx.supplierA, &fx.integA, "ORG-100", &fx.variantA, &fx.variantPublicA},
			{fx.supplierB, &fx.integB, "ORG-200", &fx.variantB, nil},
		} {
			if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider) VALUES ($1, $2) RETURNING id`, row.org, provider.DefaultProvider).Scan(row.integ); err != nil {
				return err
			}
			if row.code == "" {
				continue
			}
			var productID int64
			if err := tx.QueryRow(ctx, `INSERT INTO catalog.products (organization_id, name) VALUES ($1, '{"en":"Panadol"}') RETURNING id`, row.org).Scan(&productID); err != nil {
				return err
			}
			var pub string
			if err := tx.QueryRow(ctx, `INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, barcode, price) VALUES ($1, $2, '{"en":"Panadol Extra"}', $3, '6221024000123', 54.69) RETURNING id, public_id::text`, row.org, productID, row.code).Scan(row.varID, &pub); err != nil {
				return err
			}
			if row.pub != nil {
				*row.pub = pub
			}
			if _, err := tx.Exec(ctx, `INSERT INTO erp.product_links (organization_id, integration_id, external_code, variant_id, link_source) VALUES ($1, $2, $3, $4, 'manual_review')`, row.org, *row.integ, row.code, *row.varID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("orders fixture: %v", err)
	}
	return fx
}

// seedSplitOrder writes one order with a shipment and one line per supplier
// (A and B). It returns the order id and the two shipment ids.
func seedSplitOrder(t *testing.T, db *database.DB, fx ordersFixture, orderNumber string) (orderID, shipA, shipB int64) {
	t.Helper()
	err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO commerce.orders (order_number, customer_id, organization_id, status, subtotal, shipping_fee, total_amount, notes)
			VALUES ($1, $2, $3, 'confirmed', 1343.80, 25.00, 1368.80, 'split order') RETURNING id`, orderNumber, fx.buyerUser, fx.buyer).Scan(&orderID); err != nil {
			return err
		}
		for _, s := range []struct {
			org, integ, variant int64
			ship                *int64
			suffix              string
		}{
			{fx.supplierA, fx.integA, fx.variantA, &shipA, "1"},
			{fx.supplierB, fx.integB, fx.variantB, &shipB, "2"},
		} {
			if err := tx.QueryRow(ctx, `INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, shipping_fee, total_amount)
				VALUES ($1, $2, $3, 'pending', 1093.80, 25.00, 1118.80) RETURNING id`, orderID, s.org, orderNumber+"-"+s.suffix).Scan(s.ship); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO commerce.order_lines (order_id, shipment_id, organization_id, product_variant_id, product_name, sku, unit_price, quantity, discount_amount, total_price)
				VALUES ($1, $2, $3, $4, '{"en":"Panadol Extra"}', 'SKU', 54.69, 20, 0.00, 1093.80)`, orderID, *s.ship, s.org, s.variant); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return orderID, shipA, shipB
}

func shipmentIDs(rows []erp.SalesRow) map[int64]erp.SalesRow {
	out := map[int64]erp.SalesRow{}
	for _, r := range rows {
		out[r.ShipmentID] = r
	}
	return out
}

func TestERPOrders_SupplierSeesOnlyOwnShipment(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 1000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	_, shipA, shipB := seedSplitOrder(t, db, fx, fmt.Sprintf("ORD-%d", base))
	repo := erppg.New(db)

	rows, err := repo.ListSalesShipmentsAsSystem(ctx, fx.supplierA, fx.integA, erp.OrderFilter{Limit: 100, State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	seen := shipmentIDs(rows)
	if _, ok := seen[shipA]; !ok {
		t.Fatalf("supplier A does not see its own shipment %d", shipA)
	}
	if _, ok := seen[shipB]; ok {
		t.Fatalf("supplier A sees supplier B shipment %d", shipB)
	}
	row := seen[shipA]
	if len(row.Lines) != 1 || row.Lines[0].TotalPrice.String() != "1093.80" {
		t.Fatalf("supplier A lines = %+v, want one line at 1093.80", row.Lines)
	}
	if row.Total.String() != "1118.80" {
		t.Fatalf("supplier A total = %s, want its own shipment total 1118.80", row.Total.String())
	}
}

func TestERPOrders_BuyerKeySeesOnlyOwnOrders(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 2000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	otherFx := seedOrdersFixture(t, db, base+100)
	cleanupERPOrgs(t, db, ctx, otherFx.buyer, otherFx.supplierA, otherFx.supplierB)
	orderID, _, _ := seedSplitOrder(t, db, fx, fmt.Sprintf("BUY-%d", base))
	seedSplitOrder(t, db, otherFx, fmt.Sprintf("BUY-%d", base+100))
	repo := erppg.New(db)

	rows, err := repo.ListPurchaseOrders(ctx, fx.buyer, fx.buyerIntegration, erp.OrderFilter{Limit: 100, State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].OrderID != orderID {
		t.Fatalf("buyer rows = %+v, want exactly order %d", rows, orderID)
	}
	shipments, err := repo.ListPurchaseShipmentsAsSystem(ctx, fx.buyer, []int64{orderID})
	if err != nil {
		t.Fatal(err)
	}
	if len(shipments) != 2 {
		t.Fatalf("buyer shipments = %d, want the two supplier shipments", len(shipments))
	}
}

func TestERPOrders_NewAckImportedChanged(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 3000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	orderNumber := fmt.Sprintf("ACK-%d", base)
	orderID, shipA, _ := seedSplitOrder(t, db, fx, orderNumber)
	repo := erppg.New(db)
	stateOf := func() string {
		rows, err := repo.ListSalesShipmentsAsSystem(ctx, fx.supplierA, fx.integA, erp.OrderFilter{Limit: 500, State: "all"})
		if err != nil {
			t.Fatal(err)
		}
		row, ok := shipmentIDs(rows)[shipA]
		if !ok {
			t.Fatalf("shipment %d missing from supplier A feed", shipA)
		}
		return row.ImportState
	}
	if got := stateOf(); got != "new" {
		t.Fatalf("before ack state = %q, want new", got)
	}
	imp := erp.OrderImport{OrganizationID: fx.supplierA, IntegrationID: fx.integA, OrderID: orderID, ShipmentID: &shipA, Direction: "sales", OrderNumber: orderNumber, ErpReference: "ORGA-1", AckedStatus: "pending"}
	audit := erp.AuditRecord{Action: "erp.order.ack", EntityType: "order_import", EntityID: orderNumber, ActorKind: "api_key"}
	out, err := repo.UpsertOrderImport(ctx, imp, audit)
	if err != nil || out.Result != "acknowledged" {
		t.Fatalf("first ack = %+v, %v; want acknowledged", out, err)
	}
	if got := stateOf(); got != "imported" {
		t.Fatalf("after ack state = %q, want imported", got)
	}
	err = db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE commerce.order_shipments SET status = 'processing' WHERE id = $1`, shipA)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(); got != "changed" {
		t.Fatalf("after status change state = %q, want changed", got)
	}
	// The re-ack carries the shipment's current status, as the service does.
	reack := imp
	reack.AckedStatus = "processing"
	out, err = repo.UpsertOrderImport(ctx, reack, audit)
	if err != nil || out.Result != "already_acknowledged" {
		t.Fatalf("re-ack = %+v, %v; want already_acknowledged", out, err)
	}
	if got := stateOf(); got != "imported" {
		t.Fatalf("after re-ack state = %q, want imported", got)
	}
	conflict, err := repo.UpsertOrderImport(ctx, erp.OrderImport{OrganizationID: fx.supplierA, IntegrationID: fx.integA, OrderID: orderID, ShipmentID: &shipA, Direction: "sales", OrderNumber: orderNumber, ErpReference: "ORGA-2", AckedStatus: "processing"}, audit)
	if err != nil || conflict.Result != "conflict" || conflict.Code != "ERP_ACK_REFERENCE_CONFLICT" || conflict.ExistingReference != "ORGA-1" {
		t.Fatalf("different reference = %+v, %v; want conflict with ORGA-1", conflict, err)
	}
}

func TestERPOrders_ExternalProductCodeInLines(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 4000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	_, shipA, _ := seedSplitOrder(t, db, fx, fmt.Sprintf("LINE-%d", base))
	repo := erppg.New(db)

	rows, err := repo.ListSalesShipmentsAsSystem(ctx, fx.supplierA, fx.integA, erp.OrderFilter{Limit: 100, State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	row := shipmentIDs(rows)[shipA]
	if len(row.Lines) != 1 {
		t.Fatalf("lines = %+v, want one", row.Lines)
	}
	line := row.Lines[0]
	if line.ExternalProductCode != "ORG-100" || line.VariantPublicID != strconv.FormatInt(fx.variantA, 10) || line.Barcode != "6221024000123" {
		t.Fatalf("line = %+v, want ORG-100, the decimal variant id and the barcode", line)
	}
}

func TestTenantIsolation_ERPOrderImports(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("isolation test requires a database")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 5000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	orderID, shipA, _ := seedSplitOrder(t, db, fx, fmt.Sprintf("ISO-%d", base))
	repo := erppg.New(db)
	if _, err := repo.UpsertOrderImport(ctx, erp.OrderImport{OrganizationID: fx.supplierA, IntegrationID: fx.integA, OrderID: orderID, ShipmentID: &shipA, Direction: "sales", OrderNumber: fmt.Sprintf("ISO-%d", base), ErpReference: "ORGA-9", AckedStatus: "pending"}, erp.AuditRecord{Action: "erp.order.ack", EntityType: "order_import", EntityID: "iso", ActorKind: "api_key"}); err != nil {
		t.Fatal(err)
	}
	var importID int64
	err := db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM erp.order_imports WHERE organization_id = $1 AND order_id = $2`, fx.supplierA, orderID).Scan(&importID)
	})
	if err != nil {
		t.Fatalf("read import id: %v", err)
	}
	assertERPCrossTenantZero(t, db, ctx, "order_imports", importID, fx.supplierA, fx.supplierB)
}
