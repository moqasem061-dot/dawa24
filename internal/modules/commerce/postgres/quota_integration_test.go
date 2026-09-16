package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The per-branch quota, end to end against a real database.
//
// The unit tests in the commerce package pin the arithmetic against a stub. The
// thing that can only be proved here is that the SQL agrees with it: the sum
// over order lines, the statuses that give quota back, the release cut-off, and
// the refusal raised inside the order's own transaction.

const (
	testQuotaUserID   int64 = 88410
	testQuotaVendorID int64 = 88411
	testQuotaCustID   int64 = 88412
	testQuotaProdID   int64 = 88413
	testQuotaVarID    int64 = 88414
	testQuotaBranchA  int64 = 88415
	testQuotaBranchB  int64 = 88416
	testQuotaCatID    int64 = 88417
)

// resetQuotaFixtures tears the scenario down and builds it again, so a failed
// run leaves nothing behind that would change the next one's answers.
func resetQuotaFixtures(t *testing.T, db *database.DB, quotaLimit int) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		cleanup := []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM commerce.variant_branch_quota_releases WHERE product_variant_id = $1`, []any{testQuotaVarID}},
			{`DELETE FROM commerce.order_status_history WHERE order_id IN (SELECT id FROM commerce.orders WHERE customer_id = $1)`, []any{testQuotaUserID}},
			{`DELETE FROM commerce.order_lines WHERE organization_id = $1`, []any{testQuotaVendorID}},
			{`DELETE FROM commerce.order_shipments WHERE organization_id = $1`, []any{testQuotaVendorID}},
			{`DELETE FROM commerce.orders WHERE customer_id = $1`, []any{testQuotaUserID}},
			{`DELETE FROM catalog.product_variants WHERE id = $1`, []any{testQuotaVarID}},
			{`DELETE FROM catalog.products WHERE id = $1`, []any{testQuotaProdID}},
			{`DELETE FROM catalog.categories WHERE id = $1`, []any{testQuotaCatID}},
			{`DELETE FROM org.branches WHERE id IN ($1, $2)`, []any{testQuotaBranchA, testQuotaBranchB}},
			{`DELETE FROM org.organizations WHERE id IN ($1, $2)`, []any{testQuotaVendorID, testQuotaCustID}},
			{`DELETE FROM identity.users WHERE id = $1`, []any{testQuotaUserID}},
		}
		for _, c := range cleanup {
			if _, err := tx.Exec(txCtx, c.sql, c.args...); err != nil {
				return fmt.Errorf("cleanup %q: %w", c.sql, err)
			}
		}

		seed := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO identity.users (id, email, password_hash, name)
			  VALUES ($1, 'quota88410@example.com', 'x', '{"ar":"مستخدم","en":"Quota User"}'::jsonb)`,
				[]any{testQuotaUserID}},
			{`INSERT INTO org.organizations (id, name, type, status)
			  VALUES ($1, '{"ar":"مورد الحصص","en":"Quota Vendor"}'::jsonb, 'vendor', 'approved')`,
				[]any{testQuotaVendorID}},
			{`INSERT INTO org.organizations (id, name, type, status)
			  VALUES ($1, '{"ar":"صيدلية الحصص","en":"Quota Pharmacy"}'::jsonb, 'customer', 'approved')`,
				[]any{testQuotaCustID}},
			{`INSERT INTO org.branches (id, organization_id, name, is_main)
			  VALUES ($1, $2, '{"ar":"فرع أ","en":"Branch A"}'::jsonb, true)`,
				[]any{testQuotaBranchA, testQuotaCustID}},
			{`INSERT INTO org.branches (id, organization_id, name, is_main)
			  VALUES ($1, $2, '{"ar":"فرع ب","en":"Branch B"}'::jsonb, false)`,
				[]any{testQuotaBranchB, testQuotaCustID}},
			{`INSERT INTO catalog.categories (id, name)
			  VALUES ($1, '{"ar":"قسم الحصص","en":"Quota Cat"}'::jsonb)`,
				[]any{testQuotaCatID}},
			{`INSERT INTO catalog.products (id, organization_id, category_id, name, sku, price)
			  VALUES ($1, $2, $3, '{"ar":"دواء الحصص","en":"Quota Drug"}'::jsonb, 'QUOTA-P', 100.00)`,
				[]any{testQuotaProdID, testQuotaVendorID, testQuotaCatID}},
			{`INSERT INTO catalog.product_variants
			      (id, organization_id, product_id, name, sku, price, status, branch_id, quota_limit)
			  VALUES ($1, $2, $3, '{"ar":"عبوة","en":"Pack"}'::jsonb, 'QUOTA-V', 100.00, 'active', NULL, $4)`,
				[]any{testQuotaVarID, testQuotaVendorID, testQuotaProdID, quotaLimit}},
		}
		for _, s := range seed {
			if _, err := tx.Exec(txCtx, s.sql, s.args...); err != nil {
				return fmt.Errorf("seed %q: %w", s.sql, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reset quota fixtures: %v", err)
	}
}

// placeQuotaOrder writes one order of qty units for one branch, exactly the way
// commerce.Checkout does.
func placeQuotaOrder(t *testing.T, repo *postgres.Repository, branchID int64, qty int) (*commerce.Order, error) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	variantID := testQuotaVarID
	customerOrg := testQuotaCustID
	branch := branchID

	line := &commerce.OrderLine{
		OrganizationID:   testQuotaVendorID,
		ProductID:        ptrInt64(testQuotaProdID),
		ProductVariantID: &variantID,
		ProductName:      i18n.New("دواء الحصص", "Quota Drug"),
		UnitPrice:        money.MustParse("100.00"),
		Quantity:         qty,
		TotalPrice:       money.MustParse(fmt.Sprintf("%d.00", 100*qty)),
	}
	shipment := &commerce.OrderShipment{
		OrganizationID: testQuotaVendorID,
		Status:         commerce.StatusPending,
		Subtotal:       line.TotalPrice,
		TotalAmount:    line.TotalPrice,
		Lines:          []*commerce.OrderLine{line},
	}
	quotaOrderSeq++
	order := &commerce.Order{
		OrderNumber:    fmt.Sprintf("QUOTA-%d-%d-%d", branchID, qty, quotaOrderSeq),
		CustomerID:     testQuotaUserID,
		OrganizationID: &customerOrg,
		BranchID:       &branch,
		Status:         commerce.StatusPending,
		Subtotal:       line.TotalPrice,
		TotalAmount:    line.TotalPrice,
		FinalPrice:     line.TotalPrice,
		PaymentMethod:  "cash",
		PaymentStatus:  commerce.PaymentUnpaid,
	}
	err := repo.CreateOrder(ctx, order,
		[]*commerce.OrderShipment{shipment}, []*commerce.OrderLine{line})
	return order, err
}

func ptrInt64(v int64) *int64 { return &v }

// quotaOrderSeq keeps the fixture order numbers unique. commerce.orders has a
// unique index on order_number, and a scenario that places three orders for the
// same branch and quantity would otherwise collide on the third.
var quotaOrderSeq int

func usedBy(t *testing.T, repo *postgres.Repository, branchID int64) int {
	t.Helper()
	used, err := repo.BranchQuotaUsed(context.Background(), testQuotaVarID, branchID, 0)
	if err != nil {
		t.Fatalf("BranchQuotaUsed(branch %d): %v", branchID, err)
	}
	return used
}

func TestBranchQuotaEndToEnd(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	resetQuotaFixtures(t, db, 10)

	// 1. A fresh branch may take part of its allowance.
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 6); err != nil {
		t.Fatalf("first order of 6 against a quota of 10 must succeed: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 6 {
		t.Fatalf("used = %d, want 6", got)
	}

	// 2. A second order that would take it past the cap is refused, and refused
	//    by the write itself rather than only by the cart.
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 6); err == nil {
		t.Fatal("6 more against a remaining allowance of 4 must be refused")
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 6 {
		t.Fatalf("a refused order must not consume quota; used = %d, want 6", got)
	}

	// 3. Exactly the remainder is allowed, and finishes the branch.
	fourth, err := placeQuotaOrder(t, repo, testQuotaBranchA, 4)
	if err != nil {
		t.Fatalf("the exact remainder must be allowed: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 10 {
		t.Fatalf("used = %d, want 10", got)
	}
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 1); err == nil {
		t.Fatal("an exhausted branch must not be able to order one more")
	}

	// 4. The sibling branch of the same company has its own allowance. This is
	//    the invariant the whole feature exists for.
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchB, 10); err != nil {
		t.Fatalf("a sibling branch starts from its own zero: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchB); got != 10 {
		t.Fatalf("branch B used = %d, want 10", got)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 10 {
		t.Fatalf("branch B's order must not touch branch A; A used = %d, want 10", got)
	}

	// 5. Cancelling an order gives its units back.
	ctx := database.AsSystem(context.Background())
	if err := repo.UpdateOrderStatus(ctx, fourth.ID, commerce.StatusCancelled,
		commerce.OrderStatusHistory{OrderID: fourth.ID, ToStatus: string(commerce.StatusCancelled)}); err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 6 {
		t.Fatalf("a cancelled order must release its quota; used = %d, want 6", got)
	}
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 4); err != nil {
		t.Fatalf("the released units must be buyable again: %v", err)
	}
}

func TestBranchQuotaReleaseAndUndo(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	resetQuotaFixtures(t, db, 10)
	ctx := database.AsSystem(context.Background())

	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 10); err != nil {
		t.Fatalf("first order: %v", err)
	}
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 1); err == nil {
		t.Fatal("the branch is at its cap and must be refused")
	}

	// The supplier resets that branch. Its consumption is forgiven and it may
	// buy up to the cap again.
	if err := repo.ReleaseBranchQuota(ctx, testQuotaVendorID, testQuotaVarID, testQuotaBranchA, testQuotaUserID, "test"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 0 {
		t.Fatalf("a released branch starts from zero; used = %d", got)
	}
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 10); err != nil {
		t.Fatalf("after a reset the branch may take the full cap again: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 10 {
		t.Fatalf("post-release orders count again; used = %d, want 10", got)
	}

	// Undoing the reset restores what was forgiven, so the branch is over its
	// cap and finished.
	if err := repo.UndoBranchQuotaRelease(ctx, testQuotaVendorID, testQuotaVarID, testQuotaBranchA); err != nil {
		t.Fatalf("undo release: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 20 {
		t.Fatalf("undoing a reset restores the forgiven orders; used = %d, want 20", got)
	}

	// A supplier cannot release a branch's consumption of somebody else's item.
	err := repo.ReleaseBranchQuota(ctx, testQuotaCustID, testQuotaVarID, testQuotaBranchA, testQuotaUserID, "")
	if err == nil {
		t.Fatal("releasing another organisation's variant must be refused")
	}
}
