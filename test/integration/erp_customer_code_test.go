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
)

// S5 (US5) on the real database: the customer code a supplier already uses for
// a buyer. The order feed reads the code through the buyer's link; the guard
// and the checkout check go through the service. Every id is time based, and a
// buyer's user has the same id as its organisation.

// seedCodeCustomer creates one buyer organisation and its user with one id.
func seedCodeCustomer(t *testing.T, db *database.DB, id int64) {
	t.Helper()
	ctx := context.Background()
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Code buyer"}')`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO identity.users (id, email, password_hash) VALUES ($1, $2, 'x')`, id, fmt.Sprintf("erp-code-%d@example.test", id))
		return err
	})
	if err != nil {
		t.Fatalf("seed customer %d: %v", id, err)
	}
	cleanupERPOrgs(t, db, ctx, id)
}

// seedCustomerLink writes the link page row: the buyer's organisation number
// for one supplier. The table has no row-level policy, so AsSystem only sets up
// the fixture.
func seedCustomerLink(t *testing.T, db *database.DB, userID, customerOrg, vendorOrg int64, code, status string) {
	t.Helper()
	err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org.user_organizations (user_id, customer_org_id, vendor_org_id, organization_number, status)
			VALUES ($1, $2, $3, $4, $5)`, userID, customerOrg, vendorOrg, code, status)
		return err
	})
	if err != nil {
		t.Fatalf("seed customer link: %v", err)
	}
}

// codeFixture is one buyer, a second buyer, a supplier that uses ERP (it has an
// integration and an active key) and a supplier that does not use ERP.
type codeFixture struct {
	buyer, other, supplier, plain int64
}

func seedCodeFixture(t *testing.T, db *database.DB, base int64) codeFixture {
	t.Helper()
	ctx := context.Background()
	fx := codeFixture{buyer: base, other: base + 1, supplier: base + 2, plain: base + 3}
	seedCodeCustomer(t, db, fx.buyer)
	seedCodeCustomer(t, db, fx.other)
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"ERP supplier"}'), ($2, '{"en":"Plain supplier"}')`, fx.supplier, fx.plain); err != nil {
			return err
		}
		var branch, warehouse int64
		if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id, name) VALUES ($1, '{"en":"Code branch"}') RETURNING id`, fx.supplier).Scan(&branch); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id, name) VALUES ($1, 'Code warehouse') RETURNING id`, fx.supplier).Scan(&warehouse); err != nil {
			return err
		}
		_, _, err := seedERPKey(ctx, tx, fx.supplier, warehouse, branch, []string{erp.ScopeOrdersDecide})
		return err
	})
	if err != nil {
		t.Fatalf("code fixture: %v", err)
	}
	cleanupERPOrgs(t, db, ctx, fx.supplier, fx.plain)
	return fx
}

func codeService(db *database.DB) *erp.Service {
	return erp.NewService(erppg.New(db), erp.Ports{}, erp.Config{Provider: provider.DefaultProvider}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// salesCustomerFor reads one shipment through the supplier's order feed and
// returns its customer block.
func salesCustomerFor(t *testing.T, db *database.DB, supplier, integration, shipment int64) erp.CustomerInfo {
	t.Helper()
	rows, err := erppg.New(db).ListSalesShipmentsAsSystem(context.Background(), supplier, integration, erp.OrderFilter{Limit: 100, State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	row, ok := shipmentIDs(rows)[shipment]
	if !ok {
		t.Fatalf("shipment %d missing from the supplier feed", shipment)
	}
	return row.Customer
}

func TestERPCustomerCode_OrderWithCode(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 6000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	_, shipA, _ := seedSplitOrder(t, db, fx, fmt.Sprintf("CODE-%d", base))
	seedCustomerLink(t, db, fx.buyerUser, fx.buyer, fx.supplierA, "C-00871", "approved")

	customer := salesCustomerFor(t, db, fx.supplierA, fx.integA, shipA)
	if !customer.Mapped || customer.ErpCode != "C-00871" || customer.UnmappedReason != "" {
		t.Fatalf("customer = %+v, want erpCode C-00871, mapped", customer)
	}
}

func TestERPCustomerCode_NoLink(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 7000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	_, shipA, _ := seedSplitOrder(t, db, fx, fmt.Sprintf("NOLINK-%d", base))

	customer := salesCustomerFor(t, db, fx.supplierA, fx.integA, shipA)
	if customer.Mapped || customer.ErpCode != "" || customer.UnmappedReason != "no_link" {
		t.Fatalf("customer = %+v, want mapped=false and no_link", customer)
	}
}

func TestERPCustomerCode_EmptyCode(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 8000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	_, shipA, _ := seedSplitOrder(t, db, fx, fmt.Sprintf("EMPTY-%d", base))
	seedCustomerLink(t, db, fx.buyerUser, fx.buyer, fx.supplierA, "", "approved")

	customer := salesCustomerFor(t, db, fx.supplierA, fx.integA, shipA)
	if customer.Mapped || customer.UnmappedReason != "empty_code" {
		t.Fatalf("customer = %+v, want mapped=false and empty_code", customer)
	}
}

func TestERPCustomerCode_Conflict(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 9000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	_, shipA, _ := seedSplitOrder(t, db, fx, fmt.Sprintf("SHARED-%d", base))
	secondBuyer := base + 100
	seedCodeCustomer(t, db, secondBuyer)
	seedCustomerLink(t, db, fx.buyerUser, fx.buyer, fx.supplierA, "SHARED-7", "approved")
	seedCustomerLink(t, db, secondBuyer, secondBuyer, fx.supplierA, "SHARED-7", "pending")

	customer := salesCustomerFor(t, db, fx.supplierA, fx.integA, shipA)
	if customer.Mapped || customer.ErpCode != "" || customer.UnmappedReason != "code_conflict" {
		t.Fatalf("customer = %+v, want mapped=false and code_conflict", customer)
	}
}

func TestERPCustomerCode_SupplierWithoutERP(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	base := time.Now().UnixNano()/1000 + 10000
	fx := seedCodeFixture(t, db, base)
	seedCustomerLink(t, db, fx.buyer, fx.buyer, fx.plain, "PLN-1", "approved")
	svc := codeService(db)
	ctx := context.Background()

	using, err := svc.SuppliersUsingERP(ctx, []int64{fx.plain, fx.supplier})
	if err != nil {
		t.Fatal(err)
	}
	if using[fx.plain] || !using[fx.supplier] {
		t.Fatalf("SuppliersUsingERP = %v, want plain=false and supplier=true", using)
	}
	missing, err := svc.MissingCodes(ctx, fx.buyer, fx.buyer, []int64{fx.plain, fx.supplier})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0] != fx.supplier {
		t.Fatalf("MissingCodes = %v, want only the ERP supplier %d (no modal for the plain supplier)", missing, fx.supplier)
	}
	// A code held by another customer does not block a supplier without ERP.
	if err := svc.GuardCustomerCode(ctx, fx.plain, &fx.other, fx.other, "PLN-1", 0); err != nil {
		t.Fatalf("guard for a supplier without ERP = %v, want nil", err)
	}
}

func TestERPCustomerCode_GuardTakenAndLock(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	base := time.Now().UnixNano()/1000 + 11000
	fx := seedCodeFixture(t, db, base)
	seedCustomerLink(t, db, fx.buyer, fx.buyer, fx.supplier, "G-1", "approved")
	svc := codeService(db)
	ctx := context.Background()

	var coded *erp.Error
	err := svc.GuardCustomerCode(ctx, fx.supplier, &fx.other, fx.other, "G-1", 0)
	if !errors.As(err, &coded) || coded.Code != "ERP_CODE_TAKEN" || coded.Status != 409 {
		t.Fatalf("second customer with G-1 = %v, want ERP_CODE_TAKEN 409", err)
	}
	if err := svc.GuardCustomerCode(ctx, fx.supplier, &fx.other, fx.other, "g-1", 0); !errors.As(err, &coded) || coded.Code != "ERP_CODE_TAKEN" {
		t.Fatalf("lower-case g-1 = %v, want ERP_CODE_TAKEN (case-insensitive)", err)
	}
	if err := svc.GuardCustomerCode(ctx, fx.supplier, &fx.buyer, fx.buyer, "G-1", 0); err != nil {
		t.Fatalf("the same customer keeps G-1 = %v, want nil", err)
	}
	if err := svc.GuardCustomerCode(ctx, fx.supplier, &fx.other, fx.other, "G-2", 0); err != nil {
		t.Fatalf("free code G-2 = %v, want nil", err)
	}
}
