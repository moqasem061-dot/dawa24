package integration_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// S5 decisions D2 and D3 on the real database. The guard and the link write
// share one transaction under the supplier's advisory lock, so two saves of the
// same code cannot both pass. Codes compare case-insensitively.

// insertCodeLink writes one link row inside the caller's transaction context
// (a joined transaction when called from a guard write).
func insertCodeLink(ctx context.Context, db *database.DB, customer, supplier int64, code string) error {
	return db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org.user_organizations (user_id, customer_org_id, vendor_org_id, organization_number, status)
			VALUES ($1, $2, $3, $4, 'approved')`, customer, customer, supplier, code)
		return err
	})
}

// countCodeLinks counts link rows for one supplier and code, read as system.
func countCodeLinks(t *testing.T, db *database.DB, supplier int64, code string) int {
	t.Helper()
	var n int
	err := db.InReadTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM org.user_organizations WHERE vendor_org_id = $1 AND organization_number = $2`, supplier, code).Scan(&n)
	})
	if err != nil {
		t.Fatalf("count links: %v", err)
	}
	return n
}

func TestERPCustomerCode_ConcurrentSavesOneWins(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	base := time.Now().UnixNano()/1000 + 12000
	fx := seedCodeFixture(t, db, base)
	rival := base + 50
	seedCodeCustomer(t, db, rival)
	svc := codeService(db)
	ctx := context.Background()
	const code = "RACE-1"

	save := func(customer int64) error {
		return svc.GuardCustomerCodeWrite(ctx, fx.supplier, &customer, customer, code, 0, func(ctx context.Context) error {
			return insertCodeLink(ctx, db, customer, fx.supplier, code)
		})
	}
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, customer := range []int64{fx.other, rival} {
		wg.Add(1)
		go func(i int, customer int64) {
			defer wg.Done()
			results[i] = save(customer)
		}(i, customer)
	}
	wg.Wait()

	var passed, refused int
	for _, err := range results {
		var coded *erp.Error
		switch {
		case err == nil:
			passed++
		case errors.As(err, &coded) && coded.Code == "ERP_CODE_TAKEN":
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if passed != 1 || refused != 1 {
		t.Fatalf("concurrent saves: passed=%d refused=%d, want exactly one of each", passed, refused)
	}
	if n := countCodeLinks(t, db, fx.supplier, code); n != 1 {
		t.Fatalf("links holding %s = %d, want 1", code, n)
	}
}

func TestERPCustomerCode_WriteSharesGuardTransaction(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	base := time.Now().UnixNano()/1000 + 13000
	fx := seedCodeFixture(t, db, base)
	svc := codeService(db)
	ctx := context.Background()
	const code = "ROLL-1"
	failure := errors.New("write failed after the insert")

	err := svc.GuardCustomerCodeWrite(ctx, fx.supplier, &fx.buyer, fx.buyer, code, 0, func(ctx context.Context) error {
		if err := insertCodeLink(ctx, db, fx.buyer, fx.supplier, code); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("GuardCustomerCodeWrite = %v, want the write error", err)
	}
	if n := countCodeLinks(t, db, fx.supplier, code); n != 0 {
		t.Fatalf("links after a failed write = %d, want 0 (the insert must roll back with the guard)", n)
	}
}

func TestERPCustomerCode_CaseVariantsConflict(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano()/1000 + 14000
	fx := seedOrdersFixture(t, db, base)
	cleanupERPOrgs(t, db, ctx, fx.buyer, fx.supplierA, fx.supplierB)
	_, shipA, _ := seedSplitOrder(t, db, fx, fmt.Sprintf("CASE-%d", base))
	secondBuyer := base + 100
	seedCodeCustomer(t, db, secondBuyer)
	seedCustomerLink(t, db, fx.buyerUser, fx.buyer, fx.supplierA, "C-1", "approved")
	seedCustomerLink(t, db, secondBuyer, secondBuyer, fx.supplierA, "c-1", "pending")

	customer := salesCustomerFor(t, db, fx.supplierA, fx.integA, shipA)
	if customer.Mapped || customer.ErpCode != "" || customer.UnmappedReason != "code_conflict" {
		t.Fatalf("customer = %+v, want code_conflict for C-1 against c-1", customer)
	}
}

func TestERPCustomerCode_SuppliersUsingERPUnderBuyerTenant(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	base := time.Now().UnixNano()/1000 + 15000
	fx := seedCodeFixture(t, db, base)
	svc := codeService(db)
	// The buyer's tenant cannot see the supplier's erp rows; E8 must still answer.
	buyerCtx := database.WithTenant(context.Background(), fx.buyer)

	using, err := svc.SuppliersUsingERP(buyerCtx, []int64{fx.plain, fx.supplier})
	if err != nil {
		t.Fatal(err)
	}
	if using[fx.plain] || !using[fx.supplier] {
		t.Fatalf("SuppliersUsingERP under the buyer tenant = %v, want plain=false and supplier=true", using)
	}
}
