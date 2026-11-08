package integration_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// S7 (US6) on the real database. The purchase placer is a port; these tests use
// a placer that writes the order row itself, drawing its number from the same
// sequence the website uses. The checkout path has its own test
// (TestOrderNumberSequenceConcurrent) and the placer's price rules have their own
// unit tests in cmd/server.

// poPlacer stands in for the website checkout behind erp.PurchasePlacer. It
// counts calls, and writes an order when it is not told to fail.
type poPlacer struct {
	db    *database.DB
	calls int
	// failAfterWrite makes Place write the order and then return failErr, the
	// shape of a crash between the order write and the answer.
	failAfterWrite bool
	failErr        error
}

func (p *poPlacer) Place(ctx context.Context, req erp.PurchaseRequest) (erp.PlacedOrder, error) {
	p.calls++
	if p.failErr != nil && !p.failAfterWrite {
		return erp.PlacedOrder{}, p.failErr
	}
	var placed erp.PlacedOrder
	err := p.db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `WITH n AS (SELECT nextval('commerce.order_number_seq')::text AS num)
			INSERT INTO commerce.orders (order_number, customer_id, organization_id, status, subtotal, shipping_fee, total_amount, notes)
			SELECT n.num, $1, $2, 'pending', 0, 0, 0, $3 FROM n RETURNING id, order_number`,
			req.BuyingUserID, req.BuyerOrgID, req.Notes).Scan(&placed.OrderID, &placed.OrderNumber)
	})
	if err != nil {
		return erp.PlacedOrder{}, err
	}
	if p.failAfterWrite {
		return erp.PlacedOrder{}, p.failErr
	}
	return placed, nil
}

// poFixture is one buyer pharmacy with an ERP key that holds erp.po.create, a
// linked supplier with one active priced variant in stock, and a second supplier
// that is not linked to the buyer.
type poFixture struct {
	buyer, buyerUser, supplier, unlinked int64
	integ, keyID, branch                 int64
	variant, unlinkedVariant             int64
	variantPublic, unlinkedPublic        string
	barcode                              string
}

func seedPOFixture(t *testing.T, db *database.DB, base int64) poFixture {
	t.Helper()
	ctx := context.Background()
	fx := poFixture{buyer: base, buyerUser: base, supplier: base + 1, unlinked: base + 2, barcode: fmt.Sprintf("PO%012d", base)}
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name, type, status) VALUES
			($1, '{"en":"PO buyer"}', 'pharmacy', 'approved'),
			($2, '{"en":"PO supplier"}', 'vendor', 'approved'),
			($3, '{"en":"PO unlinked supplier"}', 'vendor', 'approved')`, fx.buyer, fx.supplier, fx.unlinked); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity.users (id, email, password_hash) VALUES ($1, $2, 'x')`, fx.buyerUser, fmt.Sprintf("erp-po-%d@example.test", base)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org.members (organization_id, user_id, role_key, status) VALUES ($1, $2, (SELECT key FROM identity.roles ORDER BY key LIMIT 1), 'active')`, fx.buyer, fx.buyerUser); err != nil {
			return err
		}
		var warehouse int64
		if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id, name) VALUES ($1, '{"en":"PO branch"}') RETURNING id`, fx.buyer).Scan(&fx.branch); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id, name) VALUES ($1, 'PO warehouse') RETURNING id`, fx.buyer).Scan(&warehouse); err != nil {
			return err
		}
		var err error
		fx.integ, fx.keyID, err = seedERPKey(ctx, tx, fx.buyer, warehouse, fx.branch, []string{erp.ScopePurchaseCreate})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE erp.api_keys SET buying_user_id = $1 WHERE id = $2`, fx.buyerUser, fx.keyID); err != nil {
			return err
		}
		for _, row := range []struct {
			org     int64
			barcode string
			price   string
			out     *int64
			public  *string
			stock   int
			status  string
		}{
			{fx.supplier, fx.barcode, "62.50", &fx.variant, &fx.variantPublic, 100, "active"},
			{fx.unlinked, fx.barcode + "U", "62.50", &fx.unlinkedVariant, &fx.unlinkedPublic, 100, "active"},
		} {
			var product, warehouseID int64
			if err := tx.QueryRow(ctx, `INSERT INTO catalog.products (organization_id, name) VALUES ($1, '{"en":"Panadol"}') RETURNING id`, row.org).Scan(&product); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, barcode, price, discount, status)
				VALUES ($1, $2, '{"en":"Panadol Extra"}', $3, $4, $5, 12.50, $6) RETURNING id, public_id::text`, row.org, product, fmt.Sprintf("PO-%d", row.org), row.barcode, row.price, row.status).Scan(row.out, row.public); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id, name) VALUES ($1, 'PO stock') RETURNING id`, row.org).Scan(&warehouseID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity) VALUES ($1, $2, $3, $4, $5)`, row.org, warehouseID, product, *row.out, row.stock); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO org.user_organizations (user_id, customer_org_id, vendor_org_id, organization_number, status) VALUES ($1, $2, $3, 'PO-CUST', 'approved')`, fx.buyerUser, fx.buyer, fx.supplier)
		return err
	})
	if err != nil {
		t.Fatalf("po fixture: %v", err)
	}
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplier, fx.unlinked)
	return fx
}

// principal is the buyer's key as the authentication step would return it.
func (fx poFixture) principal() erp.Principal {
	user := fx.buyerUser
	return erp.Principal{OrganizationID: fx.buyer, IntegrationID: fx.integ, KeyID: fx.keyID, Provider: provider.DefaultProvider,
		Scopes: []string{erp.ScopePurchaseCreate}, BranchID: fx.branch, BuyingUserID: &user, OrgType: "pharmacy", MaxBatchItems: 50000, RatePerMinute: 600}
}

func poService(db *database.DB, placer erp.PurchasePlacer) *erp.Service {
	cfg := erp.Config{Pepper: []byte("po-integration-pepper-32-bytes-ok"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider}
	return erp.NewService(erppg.New(db), erp.Ports{Placer: placer}, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func poLine(variant string, quantity int, unit string) erp.PurchaseLine {
	line := erp.PurchaseLine{VariantID: variant, Quantity: quantity}
	if unit != "" {
		price := money.MustParse(unit)
		line.UnitPrice = &price
	}
	return line
}

// poOrderCount counts the orders that carry this external reference's marker.
func poOrderCount(t *testing.T, db *database.DB, fx poFixture, ref string) int {
	t.Helper()
	var n int
	err := db.InReadTx(database.WithTenant(context.Background(), fx.buyer), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM commerce.orders WHERE organization_id = $1 AND notes LIKE '%' || $2 || '%'`, fx.buyer, "[ERP:"+provider.DefaultProvider+":"+ref+"]").Scan(&n)
	})
	if err != nil {
		t.Fatalf("count orders: %v", err)
	}
	return n
}

func poCode(err error) string {
	var e *erp.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestERPPO_CreateAndRepeat100(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	fx := seedPOFixture(t, db, time.Now().UnixNano()/1000%1000000000*10)
	placer := &poPlacer{db: db}
	svc := poService(db, placer)
	ref := fmt.Sprintf("PO-REPEAT-%d", fx.buyer)
	req := erp.CreatePurchaseRequest{ExternalReference: ref, Lines: []erp.PurchaseLine{poLine(fx.variantPublic, 12, "54.69")}}

	first, err := svc.CreatePurchaseOrder(context.Background(), fx.principal(), req)
	if err != nil || first.Replay {
		t.Fatalf("first create: replay=%v err=%v", first.Replay, err)
	}
	for i := 0; i < 100; i++ {
		again, err := svc.CreatePurchaseOrder(context.Background(), fx.principal(), req)
		if err != nil {
			t.Fatalf("repeat %d: %v", i, err)
		}
		if !again.Replay || again.OrderNumber != first.OrderNumber {
			t.Fatalf("repeat %d: replay=%v number=%s, want replay of %s", i, again.Replay, again.OrderNumber, first.OrderNumber)
		}
	}
	if placer.calls != 1 {
		t.Fatalf("placer called %d times, want once", placer.calls)
	}
	if n := poOrderCount(t, db, fx, ref); n != 1 {
		t.Fatalf("orders for the reference = %d, want 1", n)
	}
}

func TestERPPO_WrongPriceRefused(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	fx := seedPOFixture(t, db, time.Now().UnixNano()/1000%1000000000*10+1)
	placer := &poPlacer{db: db}
	svc := poService(db, placer)
	ref := fmt.Sprintf("PO-PRICE-%d", fx.buyer)
	_, err := svc.CreatePurchaseOrder(context.Background(), fx.principal(), erp.CreatePurchaseRequest{ExternalReference: ref, Lines: []erp.PurchaseLine{poLine(fx.variantPublic, 2, "60.00")}})
	if poCode(err) != "ERP_PO_PRICE_MISMATCH" {
		t.Fatalf("code = %q (%v), want ERP_PO_PRICE_MISMATCH", poCode(err), err)
	}
	lines := err.(*erp.Error).Details["lines"].([]map[string]any)
	if lines[0]["sentPrice"] != "60.00" || lines[0]["correctPrice"] != "54.69" {
		t.Fatalf("details = %+v, want sentPrice 60.00 and correctPrice 54.69", lines)
	}
	if placer.calls != 0 || poOrderCount(t, db, fx, ref) != 0 {
		t.Fatalf("a refused price must write no order (calls %d)", placer.calls)
	}
}

func TestERPPO_NoActiveOffer(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	fx := seedPOFixture(t, db, time.Now().UnixNano()/1000%1000000000*10+2)
	if err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE catalog.product_variants SET status = 'inactive' WHERE id = $1`, fx.variant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	svc := poService(db, &poPlacer{db: db})
	_, err := svc.CreatePurchaseOrder(context.Background(), fx.principal(), erp.CreatePurchaseRequest{ExternalReference: "PO-NOOFFER", Lines: []erp.PurchaseLine{poLine(fx.variantPublic, 1, "")}})
	if poCode(err) != "ERP_PO_NO_ACTIVE_OFFER" {
		t.Fatalf("code = %q (%v), want ERP_PO_NO_ACTIVE_OFFER", poCode(err), err)
	}
}

func TestERPPO_SupplierNotLinked(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	fx := seedPOFixture(t, db, time.Now().UnixNano()/1000%1000000000*10+3)
	placer := &poPlacer{db: db}
	svc := poService(db, placer)
	_, err := svc.CreatePurchaseOrder(context.Background(), fx.principal(), erp.CreatePurchaseRequest{ExternalReference: "PO-UNLINKED", Lines: []erp.PurchaseLine{poLine(fx.unlinkedPublic, 1, "")}})
	if poCode(err) != "ERP_PO_SUPPLIER_NOT_LINKED" {
		t.Fatalf("code = %q (%v), want ERP_PO_SUPPLIER_NOT_LINKED", poCode(err), err)
	}
	// FR-031: the refusal creates no link, no customer and no account.
	var links int
	if err := db.InReadTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM org.user_organizations WHERE vendor_org_id = $1`, fx.unlinked).Scan(&links)
	}); err != nil {
		t.Fatal(err)
	}
	if links != 0 || placer.calls != 0 {
		t.Fatalf("links = %d, placer calls = %d; a refusal must create nothing", links, placer.calls)
	}
}

func TestERPPO_WalletRefused(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	fx := seedPOFixture(t, db, time.Now().UnixNano()/1000%1000000000*10+4)
	placer := &poPlacer{db: db, failErr: erp.NewError("ERP_PO_PAYMENT_NOT_SUPPORTED", 409, nil)}
	svc := poService(db, placer)
	ref := fmt.Sprintf("PO-WALLET-%d", fx.buyer)
	_, err := svc.CreatePurchaseOrder(context.Background(), fx.principal(), erp.CreatePurchaseRequest{ExternalReference: ref, Lines: []erp.PurchaseLine{poLine(fx.variantPublic, 1, "")}})
	if poCode(err) != "ERP_PO_PAYMENT_NOT_SUPPORTED" {
		t.Fatalf("code = %q (%v), want ERP_PO_PAYMENT_NOT_SUPPORTED", poCode(err), err)
	}
	if poOrderCount(t, db, fx, ref) != 0 {
		t.Fatal("a refused wallet payment must write no order")
	}
}

// TestERPPO_CrashRepair simulates a crash after the order was written and before
// the answer was stored. A retry after the stale window must return that order,
// not place a second one.
func TestERPPO_CrashRepair(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	fx := seedPOFixture(t, db, time.Now().UnixNano()/1000%1000000000*10+5)
	ref := fmt.Sprintf("PO-CRASH-%d", fx.buyer)
	req := erp.CreatePurchaseRequest{ExternalReference: ref, Lines: []erp.PurchaseLine{poLine(fx.variantPublic, 3, "")}}
	placer := &poPlacer{db: db, failAfterWrite: true, failErr: erp.NewError("ERP_INTERNAL", 500, nil)}
	if _, err := poService(db, placer).CreatePurchaseOrder(context.Background(), fx.principal(), req); poCode(err) != "ERP_INTERNAL" {
		t.Fatalf("first attempt: code %q, want the simulated crash", poCode(err))
	}
	if err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE erp.idempotency_records SET created_at = now() - interval '5 minutes', updated_at = now() - interval '5 minutes' WHERE external_reference = $1`, ref)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	retry := &poPlacer{db: db}
	got, err := poService(db, retry).CreatePurchaseOrder(context.Background(), fx.principal(), req)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !got.Replay || retry.calls != 0 {
		t.Fatalf("retry replay=%v placer calls=%d; the crashed order must be returned without a second placement", got.Replay, retry.calls)
	}
	if n := poOrderCount(t, db, fx, ref); n != 1 {
		t.Fatalf("orders for the reference = %d, want 1", n)
	}
}

func TestERPPO_SelfSupply(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	fx := seedPOFixture(t, db, time.Now().UnixNano()/1000%1000000000*10+6)
	var own string
	if err := db.InReadTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT v.public_id::text FROM catalog.product_variants v WHERE v.organization_id = $1 LIMIT 1`, fx.buyer).Scan(&own)
	}); err != nil {
		// The buyer owns no variant; make one so the self-supply check has a target.
		if err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
			var product int64
			if err := tx.QueryRow(ctx, `INSERT INTO catalog.products (organization_id, name) VALUES ($1, '{"en":"Own"}') RETURNING id`, fx.buyer).Scan(&product); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, price, status) VALUES ($1, $2, '{"en":"Own"}', 'OWN-SKU', 10.00, 'active') RETURNING public_id::text`, fx.buyer, product).Scan(&own)
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc := poService(db, &poPlacer{db: db})
	_, err := svc.CreatePurchaseOrder(context.Background(), fx.principal(), erp.CreatePurchaseRequest{ExternalReference: "PO-SELF", Lines: []erp.PurchaseLine{poLine(own, 1, "")}})
	if poCode(err) != "ERP_PO_SELF_SUPPLY" {
		t.Fatalf("code = %q (%v), want ERP_PO_SELF_SUPPLY", poCode(err), err)
	}
}

func TestTenantIsolation_ERPIdempotency(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	fx := seedPOFixture(t, db, time.Now().UnixNano()/1000%1000000000*10+7)
	ref := fmt.Sprintf("PO-ISO-%d", fx.buyer)
	if _, err := poService(db, &poPlacer{db: db}).CreatePurchaseOrder(context.Background(), fx.principal(), erp.CreatePurchaseRequest{ExternalReference: ref, Lines: []erp.PurchaseLine{poLine(fx.variantPublic, 1, "")}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// The buyer sees its own record; the supplier, another tenant, sees none.
	count := func(org int64) int {
		var n int
		if err := db.InReadTx(database.WithTenant(context.Background(), org), func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM erp.idempotency_records WHERE external_reference = $1`, ref).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count(fx.buyer) != 1 {
		t.Fatal("the buyer must see its own idempotency record")
	}
	if n := count(fx.supplier); n != 0 {
		t.Fatalf("another tenant read %d idempotency rows, want 0", n)
	}
}
