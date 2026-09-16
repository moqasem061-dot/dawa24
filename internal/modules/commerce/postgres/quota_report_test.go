package postgres_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The supplier's quota screen, and the two remaining paths that can move a
// branch's consumption: editing a pending order, and two checkouts racing for
// the last units.
//
// The fixtures live in quota_integration_test.go; this file is split from it
// only to keep both under the 400-line ceiling.

func TestBranchQuotaReport(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	resetQuotaFixtures(t, db, 10)
	ctx := database.AsSystem(context.Background())

	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 10); err != nil {
		t.Fatalf("branch A order: %v", err)
	}
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchB, 3); err != nil {
		t.Fatalf("branch B order: %v", err)
	}

	rows, total, err := repo.ListBranchQuotaRows(ctx, testQuotaVendorID, commerce.QuotaFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListBranchQuotaRows: %v", err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("expected two branch rows, got total=%d len=%d", total, len(rows))
	}

	byBranch := map[int64]*commerce.BranchQuotaRow{}
	for _, r := range rows {
		byBranch[r.BranchID] = r
	}
	a, b := byBranch[testQuotaBranchA], byBranch[testQuotaBranchB]
	if a == nil || b == nil {
		t.Fatalf("both branches must appear; got %v", byBranch)
	}
	if a.Used != 10 || !a.Exhausted() || a.Remaining() != 0 {
		t.Errorf("branch A: used=%d exhausted=%v remaining=%d, want 10/true/0", a.Used, a.Exhausted(), a.Remaining())
	}
	if b.Used != 3 || b.Exhausted() || b.Remaining() != 7 {
		t.Errorf("branch B: used=%d exhausted=%v remaining=%d, want 3/false/7", b.Used, b.Exhausted(), b.Remaining())
	}
	if a.QuotaLimit != 10 || a.BranchName == "" || a.CustomerName == "" {
		t.Errorf("the report must name the branch and its company: %+v", a)
	}

	// The report and the gate must agree about what counts. The exhausted
	// filter is the one a supplier acts on.
	exhausted, n, err := repo.ListBranchQuotaRows(ctx, testQuotaVendorID,
		commerce.QuotaFilter{State: commerce.QuotaStateExhausted, Limit: 50})
	if err != nil {
		t.Fatalf("filtered ListBranchQuotaRows: %v", err)
	}
	if n != 1 || len(exhausted) != 1 || exhausted[0].BranchID != testQuotaBranchA {
		t.Errorf("the exhausted filter must return only branch A; got n=%d rows=%+v", n, exhausted)
	}

	variants, vTotal, err := repo.ListQuotaVariantRows(ctx, testQuotaVendorID, commerce.QuotaFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListQuotaVariantRows: %v", err)
	}
	if vTotal != 1 || len(variants) != 1 {
		t.Fatalf("expected one restricted variant, got total=%d len=%d", vTotal, len(variants))
	}
	v := variants[0]
	if v.QuotaLimit != 10 || v.BranchCount != 2 || v.ExhaustedBranches != 1 || v.TotalUsed != 13 {
		t.Errorf("variant row = %+v, want limit 10, 2 branches, 1 exhausted, 13 used", v)
	}

	summary, err := repo.QuotaSummaryForVendor(ctx, testQuotaVendorID)
	if err != nil {
		t.Fatalf("QuotaSummaryForVendor: %v", err)
	}
	if summary.VariantsWithQuota != 1 || summary.BranchesConsuming != 2 ||
		summary.BranchesExhausted != 1 || summary.TotalUnitsUsed != 13 {
		t.Errorf("summary = %+v", summary)
	}

	// Both pickers must be answerable without error, and must see the fixture.
	vOpts, err := repo.QuotaVariantOptions(ctx, testQuotaVendorID)
	if err != nil || len(vOpts) != 1 {
		t.Errorf("QuotaVariantOptions = %v, %v; want one option", vOpts, err)
	}
	bOpts, err := repo.QuotaBranchOptions(ctx, testQuotaVendorID)
	if err != nil || len(bOpts) != 2 {
		t.Errorf("QuotaBranchOptions = %v, %v; want two options", bOpts, err)
	}
}

// Editing a pending order re-measures the whole order against the cap, and
// measures it as a replacement rather than as an addition.
func TestBranchQuotaOnOrderEdit(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	resetQuotaFixtures(t, db, 10)
	ctx := database.AsSystem(context.Background())

	order, err := placeQuotaOrder(t, repo, testQuotaBranchA, 6)
	if err != nil {
		t.Fatalf("first order: %v", err)
	}
	// CreateOrder fills the ids on the slices it was handed, not on the order
	// struct, so the line is read back rather than reached through the order.
	var lineID int64
	if err := db.Pool().QueryRow(ctx,
		`SELECT id FROM commerce.order_lines WHERE order_id = $1 LIMIT 1`, order.ID).Scan(&lineID); err != nil {
		t.Fatalf("find line: %v", err)
	}

	// 6 -> 9 is a replacement, so it is measured as 9 against the cap of 10 and
	// must be allowed. Measuring it as 6+9 would refuse it.
	if _, err := repo.UpdateCustomerPendingOrder(ctx, order,
		[]commerce.OrderLineEditItem{{ID: lineID, Quantity: 9}}, testQuotaUserID); err != nil {
		t.Fatalf("raising the line to 9 against a cap of 10 must be allowed: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 9 {
		t.Fatalf("used = %d, want 9", got)
	}

	// 9 -> 11 is over the cap and must be refused, leaving the order at 9.
	if _, err := repo.UpdateCustomerPendingOrder(ctx, order,
		[]commerce.OrderLineEditItem{{ID: lineID, Quantity: 11}}, testQuotaUserID); err == nil {
		t.Fatal("raising the line past the cap must be refused")
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 9 {
		t.Fatalf("a refused edit must roll back; used = %d, want 9", got)
	}
}

// A variant with no quota is unaffected by any of this.
func TestNoQuotaMeansNoLimit(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	resetQuotaFixtures(t, db, 10)
	ctx := database.AsSystem(context.Background())

	if _, err := db.Pool().Exec(ctx,
		`UPDATE catalog.product_variants SET quota_limit = NULL WHERE id = $1`, testQuotaVarID); err != nil {
		t.Fatalf("lift quota: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 50); err != nil {
			t.Fatalf("an unrestricted variant must accept any quantity: %v", err)
		}
	}
}

// Two checkouts racing for the last units of one branch's allowance.
//
// This is the case the gate cannot catch: both baskets pass
// commerce.CheckAvailability a microsecond apart, both read "6 remaining", and
// both arrive at the write. Exactly one may win. Without the transaction-scoped
// advisory lock in enforceOrderQuotas the two SUMs would each see zero and both
// orders would be written, putting the branch at twice its cap.
func TestBranchQuotaRefusesConcurrentOverspend(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	resetQuotaFixtures(t, db, 10)

	const racers = 4
	results := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start
			_, err := placeQuotaOrder(t, repo, testQuotaBranchA, 6)
			results <- err
		}()
	}
	close(start)

	accepted := 0
	for i := 0; i < racers; i++ {
		if err := <-results; err == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("%d of %d concurrent 6-unit orders were accepted against a cap of 10; want exactly 1",
			accepted, racers)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 6 {
		t.Fatalf("used = %d, want 6 — the branch was taken past its cap", got)
	}
}

// TestBranchQuotaReleaseThenBuyAgain verifies that when a supplier sets a cap,
// a branch orders up to the cap, becomes exhausted, gets released, and can
// order up to the cap again (only orders placed strictly after the release count).
func TestBranchQuotaReleaseThenBuyAgain(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	repo := postgres.NewRepository(db)
	resetQuotaFixtures(t, db, 3)
	ctx := database.AsSystem(context.Background())

	// 1. Cap is 3. Branch A orders 3 units -> exactly exhausts its quota.
	order1, err := placeQuotaOrder(t, repo, testQuotaBranchA, 3)
	if err != nil {
		t.Fatalf("first order of 3: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 3 {
		t.Fatalf("used = %d, want 3", got)
	}

	// 2. Further order of 1 unit must be refused with quota conflict.
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 1); err == nil {
		t.Fatal("expected order to be refused when quota is exhausted")
	}

	// 3. Confirm report shows branch A is exhausted.
	rows, total, err := repo.ListBranchQuotaRows(ctx, testQuotaVendorID, commerce.QuotaFilter{
		VariantID:     testQuotaVarID,
		CustomerOrgID: testQuotaCustID,
		BranchID:      testQuotaBranchA,
		State:         commerce.QuotaStateExhausted,
	})
	if err != nil {
		t.Fatalf("ListBranchQuotaRows: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 exhausted row, got total=%d len=%d", total, len(rows))
	}
	if rows[0].Used != 3 || !rows[0].Exhausted() || rows[0].Remaining() != 0 {
		t.Fatalf("expected used=3 exhausted=true remaining=0, got %+v", rows[0])
	}

	// 4. Supplier releases the branch.
	if err := repo.ReleaseBranchQuota(ctx, testQuotaVendorID, testQuotaVarID, testQuotaBranchA, testQuotaUserID, "approved release"); err != nil {
		t.Fatalf("release quota: %v", err)
	}

	// 5. Consumption resets to 0, releasedUnits is 3, WasReleased is true.
	if got := usedBy(t, repo, testQuotaBranchA); got != 0 {
		t.Fatalf("used after release = %d, want 0", got)
	}
	rows, _, err = repo.ListBranchQuotaRows(ctx, testQuotaVendorID, commerce.QuotaFilter{
		VariantID: testQuotaVarID,
		BranchID:  testQuotaBranchA,
	})
	if err != nil || len(rows) != 1 {
		t.Fatalf("read rows after release: %v, len=%d", err, len(rows))
	}
	if rows[0].Used != 0 || rows[0].Remaining() != 3 || !rows[0].WasReleased() || rows[0].ReleasedUnits != 3 {
		t.Fatalf("expected used=0 remaining=3 released=true releasedUnits=3, got %+v", rows[0])
	}

	// 6. Branch A orders 3 units again -> succeeds!
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 3); err != nil {
		t.Fatalf("second order of 3 after release must succeed: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 3 {
		t.Fatalf("used after post-release order = %d, want 3", got)
	}

	// 7. Ordering 1 more is refused again.
	if _, err := placeQuotaOrder(t, repo, testQuotaBranchA, 1); err == nil {
		t.Fatal("expected order to be refused after second 3 units")
	}

	// 8. Cancelling first order (which was forgiven by release anyway) doesn't change current count.
	if err := repo.UpdateOrderStatus(ctx, order1.ID, commerce.StatusCancelled,
		commerce.OrderStatusHistory{OrderID: order1.ID, ToStatus: string(commerce.StatusCancelled)}); err != nil {
		t.Fatalf("cancel first order: %v", err)
	}
	if got := usedBy(t, repo, testQuotaBranchA); got != 3 {
		t.Fatalf("cancelling pre-release order must not affect post-release tally; used = %d, want 3", got)
	}

	// 9. Customer options can be fetched and contain the customer organization.
	custOpts, err := repo.QuotaCustomerOptions(ctx, testQuotaVendorID)
	if err != nil {
		t.Fatalf("QuotaCustomerOptions: %v", err)
	}
	if len(custOpts) == 0 {
		t.Fatalf("expected at least 1 customer option, got 0")
	}
}
